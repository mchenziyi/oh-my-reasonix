package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureRootValidation(t *testing.T) {
	if _, err := secureRoot(""); err == nil {
		t.Error("empty root should be rejected")
	}
	if _, err := secureRoot("relative/path"); err == nil {
		t.Error("relative root should be rejected")
	}
	if _, err := secureRoot("../up"); err == nil {
		t.Error("traversal root should be rejected")
	}
	root := filepath.Join(tempRoot(t), "store")
	got, err := secureRoot(root)
	if err != nil {
		t.Fatalf("absolute root should be accepted: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("secureRoot must return an absolute path, got %q", got)
	}
}

func TestSecureRootVarNormalization(t *testing.T) {
	root := tempRoot(t)
	if !strings.HasPrefix(root, "/var/") {
		t.Skip("test environment does not use macOS /var layout")
	}
	got, err := secureRoot(root)
	if err != nil {
		t.Fatalf("secureRoot failed: %v", err)
	}
	if !strings.HasPrefix(got, "/private/var/") {
		t.Errorf("expected /var -> /private/var normalization, got %q", got)
	}
}

func TestValidateFactKey(t *testing.T) {
	valid := []string{
		"mem_01K7A9X2",
		"mem_01K7A9X2/2",
		"mem_01K7A9X2/2/3",
		"judgment_01K",
	}
	for _, k := range valid {
		if _, err := validateFactKey(k); err != nil {
			t.Errorf("key %q should be valid: %v", k, err)
		}
	}
	invalid := []string{
		"", "/etc/passwd", "../mem", "mem/../../etc", "a/b/c/d",
		"mem with space", "mem\x00", "mem\\evil", "./mem", "mem/.",
		strings.Repeat("x", 129),
	}
	for _, k := range invalid {
		if _, err := validateFactKey(k); err == nil {
			t.Errorf("key %q should be rejected", k)
		}
	}
}

func TestSecureJoinRejectsUnsafeLayout(t *testing.T) {
	root := tempRoot(t)
	// Build a legal base layout: facts/judgments exists as real directory.
	if err := os.MkdirAll(filepath.Join(root, "facts", "judgments"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Target file is a symlink.
	os.Symlink(tempRoot(t), filepath.Join(root, "facts", "judgments", "j1.json"))
	if _, err := secureJoin(root, []string{"facts", "judgments", "j1.json"}, true, true); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("target symlink: want symlink_rejected, got %v", err)
	}
	os.Remove(filepath.Join(root, "facts", "judgments", "j1.json"))

	// Intermediate component is a symlink.
	os.Symlink(tempRoot(t), filepath.Join(root, "facts", "evil"))
	if _, err := secureJoin(root, []string{"facts", "evil", "j2.json"}, true, true); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("intermediate symlink: want symlink_rejected, got %v", err)
	}

	// Directory replaced by regular file.
	if err := os.WriteFile(filepath.Join(root, "facts", "file-as-dir"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := secureJoin(root, []string{"facts", "file-as-dir", "x.json"}, true, true); ErrorCode(err) != CodePathUnsafe {
		t.Errorf("file-as-dir: want path_unsafe, got %v", err)
	}

	// File replaced by directory.
	if err := os.MkdirAll(filepath.Join(root, "facts", "judgments", "j2.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := secureJoin(root, []string{"facts", "judgments", "j2.json"}, true, true); ErrorCode(err) != CodePathUnsafe {
		t.Errorf("dir-as-file: want path_unsafe, got %v", err)
	}

	// Missing target in read mode.
	if _, err := secureJoin(root, []string{"facts", "judgments", "missing.json"}, false, true); ErrorCode(err) != CodeNotFound {
		t.Errorf("missing target: want not_found, got %v", err)
	}

	// Whole facts directory replaced by an external symlink (prefix bypass).
	os.RemoveAll(filepath.Join(root, "facts"))
	os.Symlink(tempRoot(t), filepath.Join(root, "facts"))
	if _, err := secureJoin(root, []string{"facts", "j3.json"}, true, true); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("root prefix bypass via facts symlink: want symlink_rejected, got %v", err)
	}
}

func TestSecureRootRejectsSymlinkParent(t *testing.T) {
	// The root does not exist yet and its direct parent is a symlink to an
	// external directory: creating the store there would silently live
	// outside the caller's intended location, so it must be rejected.
	external := tempRoot(t)
	parent := filepath.Join(tempRoot(t), "link")
	if err := os.Symlink(external, parent); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "store")
	if _, err := secureRoot(root); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("store under external symlink parent: want symlink_rejected, got %v", err)
	}
	// OpenProject must reject it too, and the external directory must stay
	// untouched.
	if _, err := OpenProject(root, Options{}); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("OpenProject under symlink parent: want symlink_rejected, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(external, "store")); !os.IsNotExist(err) {
		t.Error("external directory must not be written through the symlink")
	}
}

func TestSecureRootCreatesUnderRealParent(t *testing.T) {
	// A fresh store under a real, safe parent must be created (0700). The
	// base is normalized first so the assertion matches secureRoot's own
	// normalization (macOS /var -> /private/var).
	base, err := filepath.EvalSymlinks(tempRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "a", "b", "store")
	got, err := secureRoot(root)
	if err != nil {
		t.Fatalf("creation under real parent should succeed: %v", err)
	}
	if got != root {
		t.Errorf("normalized root = %q, want %q", got, root)
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		t.Fatalf("root must exist as a directory: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("root mode %v, want 0700", fi.Mode().Perm())
	}
	// Full OpenProject flow works.
	s, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatalf("OpenProject should succeed: %v", err)
	}
	if _, err := s.Put(context.Background(), validRevision()); err != nil {
		t.Fatalf("put into freshly created store: %v", err)
	}
}

func TestSecureRootRejectsDistantAncestorSymlink(t *testing.T) {
	// A symlink that is not the root's direct parent (there are missing
	// components between it and the root) must still be rejected: creating
	// the store under it would silently live outside the caller's intended
	// location. Only known system mappings (macOS /var, /tmp, /etc) may be
	// normalized; any other ancestor symlink fails closed.
	external := tempRoot(t)
	parent := filepath.Join(tempRoot(t), "link")
	if err := os.Symlink(external, parent); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "deep", "store")
	if _, err := secureRoot(root); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("secureRoot under distant ancestor symlink: want symlink_rejected, got %v", err)
	}
	// OpenProject must reject it too, and must not write anything through
	// the symlink: no directory inside the external target and no missing
	// component under the link.
	if _, err := OpenProject(root, Options{}); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("OpenProject under distant ancestor symlink: want symlink_rejected, got %v", err)
	}
	entries, err := os.ReadDir(external)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("external directory must not be written through the symlink, found %d entries", len(entries))
	}
	if _, err := os.Lstat(filepath.Join(parent, "deep")); !os.IsNotExist(err) {
		t.Error("missing component under the symlink must not be created")
	}
}

func TestSecureRootRejectsInsecurePermissions(t *testing.T) {
	root := tempRoot(t)
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := secureRoot(root); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("secureRoot on 0755 root: want insecure_permissions, got %v", err)
	}
	if err := os.Chmod(root, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := secureRoot(root); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("secureRoot on 0775 root: want insecure_permissions, got %v", err)
	}
}

func TestSecureJoinRejectsInsecureDir(t *testing.T) {
	// A pre-existing store whose facts directory gained group/other bits is
	// rejected at operation time, not silently chmod'ed.
	root := tempRoot(t)
	s, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.factsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), validRevision()); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("put with insecure facts dir: want insecure_permissions, got %v", err)
	}
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("get with insecure facts dir: want insecure_permissions, got %v", err)
	}
	fi, err := os.Stat(s.factsDir)
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Error("directory must not be silently chmod'ed back")
	}
}

func TestSecureRootRejectsReplacedSystemMapping(t *testing.T) {
	// A path registered as a "system mapping" must point exactly where the
	// mapping declares. If the symlink is replaced to point elsewhere, the
	// mapping must not be trusted: secureRoot fails closed instead of
	// silently following the replaced link.
	if systemSymlinkMap == nil {
		t.Skip("no system symlink mappings on this platform")
	}
	ext := tempRoot(t)
	linkPath := filepath.Join(tempRoot(t), "syslink")
	if err := os.Symlink(ext, linkPath); err != nil {
		t.Fatal(err)
	}
	// Declare the mapping as pointing at a different directory than the
	// actual symlink target, simulating a replaced system symlink.
	declared := tempRoot(t)
	systemSymlinkMap[linkPath] = declared
	defer delete(systemSymlinkMap, linkPath)

	root := filepath.Join(linkPath, "deep", "store")
	if _, err := secureRoot(root); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("replaced system mapping: want symlink_rejected, got %v", err)
	}
	if _, err := OpenProject(root, Options{}); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("OpenProject with replaced system mapping: want symlink_rejected, got %v", err)
	}
	if entries, err := os.ReadDir(ext); err != nil || len(entries) != 0 {
		t.Errorf("external target must not be written, got %d entries (%v)", len(entries), err)
	}
	if entries, err := os.ReadDir(declared); err != nil || len(entries) != 0 {
		t.Errorf("declared mapping target must not be written, got %d entries (%v)", len(entries), err)
	}
}

func TestSecureJoinCreatesMissingComponents(t *testing.T) {
	root := tempRoot(t)
	path, err := secureJoin(root, []string{"facts", "judgments", "j1.json"}, true, true)
	if err != nil {
		t.Fatalf("creating missing components should succeed: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("path must be absolute, got %q", path)
	}
	if !strings.HasPrefix(path, root) {
		t.Errorf("path must stay under root, got %q", path)
	}
}
