package memory

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// flockFile acquires an exclusive flock on path, simulating an external
// writer holding the store lock.
func flockFile(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	})
	return f
}

func TestLockTimeoutWhenExternallyHeld(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{LockTimeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	flockFile(t, filepath.Join(s.root, "locks", "store.lock"))

	_, err = s.Put(context.Background(), validRevision())
	if ErrorCode(err) != CodeLockTimeout {
		t.Errorf("want lock_timeout, got %v", err)
	}
}

func TestLockContextCancellation(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{LockTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	flockFile(t, filepath.Join(s.root, "locks", "store.lock"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled at entry
	if _, err := s.Put(ctx, validRevision()); err == nil {
		t.Error("cancelled context must fail the put")
	}
	if ErrorCode(mustPutErr(t, s, ctx)) != CodeLockTimeout {
		t.Error("cancelled put should map to lock_timeout")
	}
}

func mustPutErr(t *testing.T, s *FactStore, ctx context.Context) error {
	t.Helper()
	_, err := s.Put(ctx, validRevision())
	return err
}

func TestLockOwnerRecorded(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// First put acquires and records the owner, then releases.
	if _, err := s.Put(context.Background(), validRevision()); err != nil {
		t.Fatal(err)
	}
	info, err := s.LockInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Owner == nil {
		t.Fatal("lock owner should be recorded in the lock file")
	}
	if info.Owner.PID <= 0 {
		t.Errorf("owner pid missing: %+v", info.Owner)
	}

	// While externally held, Locked must be true.
	heldF, err := os.OpenFile(filepath.Join(s.root, "locks", "store.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(heldF.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	info, err = s.LockInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Locked {
		t.Error("LockInfo should report the lock as held")
	}
	syscall.Flock(int(heldF.Fd()), syscall.LOCK_UN)
	heldF.Close()
	info, err = s.LockInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Locked {
		t.Error("LockInfo should report the lock as free after release")
	}
}

func TestLockCancelWhileWaiting(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{LockTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	flockFile(t, filepath.Join(s.root, "locks", "store.lock"))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err = s.Put(ctx, validRevision())
	if ErrorCode(err) != CodeLockTimeout {
		t.Errorf("cancel while waiting: want lock_timeout, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("cancel while waiting must not block until the lock timeout")
	}
}

func TestLockTimeoutThenRetrySucceeds(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{LockTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	held := flockFile(t, filepath.Join(s.root, "locks", "store.lock"))
	if _, err := s.Put(context.Background(), validRevision()); ErrorCode(err) != CodeLockTimeout {
		t.Fatalf("want lock_timeout while held, got %v", err)
	}
	// Release the external holder, then retry: the put must succeed.
	syscall.Flock(int(held.Fd()), syscall.LOCK_UN)
	held.Close()
	if _, err := s.Put(context.Background(), validRevision()); err != nil {
		t.Fatalf("retry after release should succeed: %v", err)
	}
}

func TestLockSerializesConcurrentPuts(t *testing.T) {
	root := tempRoot(t)
	s, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Two different facts with the same identity: exactly one created,
	// the other must be a conflict or noop, never both created.
	var results []WriteResult
	var errs []error
	var mu sync.Mutex
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			r := validRevision()
			if i == 1 {
				r.Title = "Other Title"
				r = fillRevisionHash(r)
			}
			res, err := s.Put(context.Background(), r)
			mu.Lock()
			results = append(results, res)
			errs = append(errs, err)
			mu.Unlock()
		}(i)
	}
	<-done
	<-done
	created := 0
	for i, res := range results {
		if errs[i] == nil && res.Status == WriteCreated {
			created++
		}
	}
	if created != 1 {
		t.Errorf("exactly one put should create, got %d created", created)
	}
}
