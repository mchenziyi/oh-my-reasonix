package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256MatchesFileHash(t *testing.T) {
	data := []byte("oh-my-reasonix")
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != SHA256(data) {
		t.Fatalf("hash mismatch: got %q want %q", got, SHA256(data))
	}
	if _, err := SHA256File(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing file hash error")
	}
}

func TestAtomicWriteCreatesParentAndPreservesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "output.txt")
	if err := AtomicWrite(path, []byte("first"), 0o640); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first" {
		t.Fatalf("unexpected atomic write: data=%q err=%v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("unexpected mode: %o", info.Mode().Perm())
	}
	if err := AtomicWrite(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "second" {
		t.Fatalf("unexpected replacement: data=%q err=%v", data, err)
	}
}

func TestCopyFileCopiesContentAndMode(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.txt")
	dst := filepath.Join(root, "out", "copy.txt")
	if err := os.WriteFile(src, []byte("copy me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("unexpected copied mode: %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "copy me" {
		t.Fatalf("unexpected copied data: %q err=%v", data, err)
	}
	if err := CopyFile(filepath.Join(root, "missing"), dst, 0o644); err == nil {
		t.Fatal("expected missing source copy error")
	}
}
