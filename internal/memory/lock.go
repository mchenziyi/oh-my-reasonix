package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LockOwner records who holds (or last held) the store write lock. It is
// diagnostic evidence only; mutual exclusion comes from flock, which the
// kernel releases on process death. PID reuse is mitigated by the random
// token, which changes every Store open.
type LockOwner struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	CreatedAt string `json:"created_at"`
	Token     string `json:"token"`
}

// LockInfo is a snapshot of the store lock state for diagnostics.
type LockInfo struct {
	Locked bool       `json:"locked"`
	Owner  *LockOwner `json:"owner,omitempty"`
	Path   string     `json:"path"` // redacted relative location
}

// acquireWriteLock takes the Store-level single-writer lock: an in-process
// channel guard plus a cross-process flock on locks/store.lock. It respects
// ctx cancellation and the configured timeout and never blocks forever.
// The returned function releases the lock and must be called via defer.
func (s *FactStore) acquireWriteLock(ctx context.Context) (func(), error) {
	// In-process guard. Re-check ctx before the select so an already
	// cancelled context cannot win the channel branch by chance.
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "lock wait cancelled")
	}
	select {
	case s.local <- struct{}{}:
	case <-ctx.Done():
		return nil, storeError(CodeLockTimeout, "lock wait cancelled")
	}

	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		<-s.local
		return nil, storeError(CodePermissionDenied, "cannot open lock file")
	}
	deadline := time.Now().Add(s.lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			<-s.local
			return nil, storeError(CodePermissionDenied, "lock acquisition failed")
		}
		if time.Now().After(deadline) {
			f.Close()
			<-s.local
			return nil, storeError(CodeLockTimeout, "lock wait timed out")
		}
		select {
		case <-time.After(25 * time.Millisecond):
		case <-ctx.Done():
			f.Close()
			<-s.local
			return nil, storeError(CodeLockTimeout, "lock wait cancelled")
		}
	}
	writeLockOwner(f)

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		<-s.local
	}, nil
}

// writeLockOwner records diagnostic owner information in the lock file. It
// never fails the transaction: flock already guarantees exclusion.
func writeLockOwner(f *os.File) {
	owner := LockOwner{
		PID:       os.Getpid(),
		Host:      hostname(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Token:     randomToken(),
	}
	data, err := json.Marshal(owner)
	if err != nil {
		return
	}
	if err := f.Truncate(0); err != nil {
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		return
	}
	_, _ = f.Write(data)
	_ = f.Sync()
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func randomToken() string {
	return fmt.Sprintf("%x", time.Now().UnixNano()^int64(os.Getpid()))
}

// LockInfo reports whether the cross-process lock is currently held and the
// owner recorded in the lock file (which may be stale; only Doctor may
// decide about stale locks, never the store itself).
func (s *FactStore) LockInfo(ctx context.Context) (LockInfo, error) {
	if err := ctx.Err(); err != nil {
		return LockInfo{}, storeError(CodeLockTimeout, "lock check cancelled")
	}
	if err := os.MkdirAll(s.locksDir, 0o700); err != nil {
		return LockInfo{}, storeError(CodePermissionDenied, "cannot access lock directory")
	}
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return LockInfo{}, storeError(CodePermissionDenied, "cannot open lock file")
	}
	defer f.Close()

	info := LockInfo{Path: filepath.ToSlash(filepath.Join("locks", filepath.Base(s.lockPath)))}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	} else {
		info.Locked = true
	}
	data, err := os.ReadFile(s.lockPath)
	if err == nil {
		var owner LockOwner
		if json.Unmarshal(data, &owner) == nil && owner.PID != 0 {
			info.Owner = &owner
		}
	}
	return info, nil
}
