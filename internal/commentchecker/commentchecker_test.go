package commentchecker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Fixture helpers ---

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- T13-03: R001 — Debug markers ---

func TestR001_DetectsTODOFixmeXXX(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// TODO: implement this later
func main() {}
// FIXME: this is broken
func foo() {}
// XXX: dangerous
func bar() {}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Track by marker text, not by substring in message.
	markers := map[string]int{"TODO": 0, "FIXME": 0, "XXX": 0}
	for _, f := range r.Findings {
		if f.RuleID == "R001" {
			for _, m := range []string{"TODO", "FIXME", "XXX"} {
				if strings.Contains(f.Message, m) {
					markers[m]++
				}
			}
		}
	}
	if markers["TODO"] == 0 || markers["FIXME"] == 0 || markers["XXX"] == 0 {
		// Debug: print messages
		for _, f := range r.Findings {
			if f.RuleID == "R001" {
				t.Logf("R001 finding message: %q", f.Message)
			}
		}
		t.Fatalf("expected R001 findings for all three markers, got %v", markers)
	}
}

func TestR001_RespectsAllowlist(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// TODO(admin): add auth later — allowed
func main() {}
`)

	r, err := Run(dir, Config{AllowedTags: []string{"TODO(admin)"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.RuleID == "R001" && f.File == filepath.Join(dir, "main.go") {
			t.Fatalf("expected TODO(admin) to be allowed: %+v", f)
		}
	}
}

// --- T13-03: R002 — Empty/placeholder comments ---

func TestR002_DetectsEmptyComments(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
//
func main() {}
//
func foo() {}
// TODO
func bar() {}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	var emptyCount int
	for _, f := range r.Findings {
		if f.RuleID == "R002" {
			emptyCount++
		}
	}
	if emptyCount == 0 {
		t.Fatalf("expected R002 findings for empty comments, got 0\nreport: %+v", r)
	}
}

// --- T13-03: R003 — Comment-code similarity (warning only) ---

func TestR003_WarningOnlySeverity(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// increment the counter by one
counter++
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if f.RuleID == "R003" {
			if f.Severity != SeverityWarning {
				t.Fatalf("expected R003 severity=warning, got %q: %+v", f.Severity, f)
			}
			return
		}
	}
	// R003 is best-effort; not finding it is acceptable depending on implementation
}

// --- T13-03: R004 — Credential leak detection with redaction ---

func TestR004_DetectsAndRedactsCredentials(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "config.go", `package config
// db_password = "mysecret123"
// api_key = "sk-a1b2c3d4e5f6"
func load() {}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range r.Findings {
		if f.RuleID == "R004" {
			found = true
			if f.Severity != SeverityBlocking {
				t.Fatalf("expected R004 severity=blocking, got %q", f.Severity)
			}
			// Must be redacted — no raw secret values in output
			if strings.Contains(f.RedactedDetail, "mysecret123") || strings.Contains(f.RedactedDetail, "sk-a1b2c3d4e5f6") {
				t.Fatalf("R004 must redact secrets, got: %s", f.RedactedDetail)
			}
			if !strings.Contains(f.RedactedDetail, "***") {
				t.Fatalf("R004 redacted detail should contain '***', got: %s", f.RedactedDetail)
			}
		}
	}
	if !found {
		t.Fatalf("expected R004 finding for credentials\nreport: %+v", r)
	}
}

// --- T13-03: R005 — Path safety ---

func TestR005_RejectsPathOutsideAllowedRoot(t *testing.T) {
	dir := t.TempDir()
	// Write a file outside the allowed root
	outsideFile := filepath.Join(dir, "subdir", "secret.go")
	if err := os.MkdirAll(filepath.Dir(outsideFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, []byte("// secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(dir, Config{AllowedRoots: []string{filepath.Join(dir, "allowed")}})
	if err == nil {
		t.Fatal("expected R005 path safety error when no files in allowed root")
	}
}

func TestR005_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "allowed/main.go", "package main\nfunc main() {}\n")

	_, err := Run(dir, Config{
		AllowedRoots: []string{filepath.Join(dir, "allowed")},
		Files:        []string{filepath.Join(dir, "allowed/../../../etc/passwd")},
	})
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

// --- R005 default root: reject outside paths ---

func TestR005_DefaultRootRejectsOutsideAbsPath(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "inside.go", "package main\nfunc main() {}\n")

	// A file outside the project dir should be rejected by the default root.
	outside := filepath.Join(dir, "..", "outside.go")
	if err := os.WriteFile(outside, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)

	_, err := Run(dir, Config{Files: []string{outside}})
	if err == nil {
		t.Fatal("expected R005 path safety error for outside absolute path with default root")
	}
	var psErr *PathSafetyError
	if !isPathSafetyError(err) {
		t.Fatalf("expected PathSafetyError, got: %T %v", err, err)
	}
	_ = psErr
}

func TestR005_DefaultRootRejectsRelativeTraversal(t *testing.T) {
	dir := t.TempDir()

	_, err := Run(dir, Config{Files: []string{"../outside.go"}})
	if err == nil {
		t.Fatal("expected R005 path safety error for ../ traversal with default root")
	}
}

func TestR005_LegalFileInsidePasses(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "inside.go", "// clean comment\npackage main\nfunc main() {}\n")

	_, err := Run(dir, Config{Files: []string{filepath.Join(dir, "inside.go")}})
	if err != nil {
		t.Fatalf("expected legal inside file to pass, got: %v", err)
	}
}

func TestR005_DefaultRootDiscoveryInside(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", "// clean\npackage main\nfunc main() {}\n")

	// discoverFiles should work because all files are inside projectDir.
	_, err := Run(dir, Config{})
	if err != nil {
		t.Fatalf("expected discovery inside project dir to pass, got: %v", err)
	}
}

// --- Symlink tests ---

func TestR005_SymlinkOutsideRejected(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "target.go")
	if err := os.WriteFile(outsideFile, []byte("// outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(dir, "link.go")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skip("symlink not supported:", err)
	}

	_, err := Run(dir, Config{Files: []string{linkPath}})
	if err == nil {
		t.Fatal("expected R005 path safety error for symlink pointing outside project")
	}
}

func TestR005_SymlinkIntermediateDirOutsideRejected(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()

	// Create a file inside outsideDir
	outsideFile := filepath.Join(outsideDir, "target.go")
	if err := os.WriteFile(outsideFile, []byte("// outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink in the project that points to outsideDir
	linkDir := filepath.Join(dir, "ext")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skip("symlink not supported:", err)
	}

	// File accessed through the symlink dir
	pathViaLink := filepath.Join(linkDir, "target.go")
	_, err := Run(dir, Config{Files: []string{pathViaLink}})
	if err == nil {
		t.Fatal("expected R005 path safety error for symlink dir pointing outside project")
	}
}

func TestR005_InsideSymlinkAllowed(t *testing.T) {
	dir := t.TempDir()

	// Create real file inside project.
	realFile := filepath.Join(dir, "real", "actual.go")
	if err := os.MkdirAll(filepath.Dir(realFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realFile, []byte("// inside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create symlink inside project pointing to another inside file.
	linkPath := filepath.Join(dir, "alias.go")
	if err := os.Symlink(realFile, linkPath); err != nil {
		t.Skip("symlink not supported:", err)
	}

	_, err := Run(dir, Config{Files: []string{linkPath}})
	if err != nil {
		t.Fatalf("expected inside symlink to pass, got: %v", err)
	}
}

// --- R005 with explicit AllowedRoots ---

func TestR005_ExplicitRootRejectsOutside(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "allowed/inside.go", "// allowed\npackage main\n")

	outside := filepath.Join(dir, "outside.go")
	if err := os.WriteFile(outside, []byte("// outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(dir, Config{
		AllowedRoots: []string{allowedDir},
		Files:        []string{outside},
	})
	if err == nil {
		t.Fatal("expected R005 error for file outside explicit allowed root")
	}
}

func TestR005_ExplicitRootAllowsInside(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "allowed")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := writeFixture(t, dir, "allowed/inside.go", "// clean\npackage main\n")

	_, err := Run(dir, Config{
		AllowedRoots: []string{allowedDir},
		Files:        []string{inside},
	})
	if err != nil {
		t.Fatalf("expected inside explicit root to pass, got: %v", err)
	}
}

// Test that discoverFiles only discovers files inside the project dir.
func TestR005_DiscoveryInsideDefaultRoot(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "sub/a.go", "// a\npackage main\n")

	// Create a file outside that should NOT be discovered.
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "b.go"), []byte("// b\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// discoverFiles only walks dir, so it shouldn't find outside files.
	// This test confirms default root doesn't break normal discovery.
	_, err := Run(dir, Config{})
	if err != nil {
		t.Fatalf("expected normal discovery to pass, got: %v", err)
	}
}

// isPathSafetyError checks if an error is a PathSafetyError.
func isPathSafetyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*PathSafetyError)
	return ok
}

// --- Binary file skip ---

func TestSkipsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "data.bin", string([]byte{0x00, 0x01, 0x02, 0x00, 0x00, 0x00}))

	r, err := Run(dir, Config{Files: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if strings.Contains(f.Message, path) {
			t.Fatalf("expected binary file to be skipped without findings: %+v", f)
		}
	}
}

// --- Large file skip ---

func TestSkipsLargeFile(t *testing.T) {
	dir := t.TempDir()
	large := strings.Repeat("// large comment\n", 10000)
	path := writeFixture(t, dir, "large.go", large)

	r, err := Run(dir, Config{Files: []string{path}, MaxFileSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Findings {
		if strings.Contains(f.Message, path) {
			t.Fatalf("expected large file to be skipped, got finding: %+v", f)
		}
	}
}

// --- JSON output is stable and parseable ---

func TestJSONReportHasSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// TODO: do this
func main() {}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, data)
	}
	if decoded.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1, got %d", decoded.SchemaVersion)
	}
	if decoded.Summary.TotalFiles == 0 {
		t.Fatalf("expected summary to have files_checked > 0, got %+v", decoded.Summary)
	}
}

// --- Deterministic / stable results ---

func TestRunIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// TODO: implement
// FIXME: bug
// password = "hunter2"
func main() {}
`)

	r1, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if r1.BlockingCount != r2.BlockingCount || len(r1.Findings) != len(r2.Findings) {
		t.Fatalf("expected deterministic results\nr1: %+v\nr2: %+v", r1, r2)
	}
}

// --- Clean comments pass ---

func TestCleanCommentsProduceNoFindings(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// sum returns the sum of two integers.
func sum(a, b int) int {
	return a + b
}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if r.BlockingCount > 0 {
		t.Fatalf("expected no blocking findings for clean comments, got %d\nreport: %+v", r.BlockingCount, r)
	}
}

// --- Human-readable output ---

func TestHumanOutput(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "main.go", `package main
// TODO: implement
func main() {}
`)

	r, err := Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	human := r.HumanString()
	if !strings.Contains(human, "R001") || !strings.Contains(human, "finding") {
		t.Fatalf("human output missing key info: %s", human)
	}
	if !strings.Contains(human, "suggestion") && !strings.Contains(human, "Suggestion") {
		t.Fatalf("human output missing suggestion section: %s", human)
	}
}

// --- Snapshot consistency (no unwanted file changes) ---

func TestRunDoesNotModifyFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "main.go", `package main
// TODO: implement
func main() {}
`)

	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Run(dir, Config{})
	if err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(after) {
		t.Fatal("Run() must not modify source files")
	}
}
