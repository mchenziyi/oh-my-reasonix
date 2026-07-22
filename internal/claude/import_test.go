package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newClaudeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Create .git to make ProjectRoot work
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeClaudeRule(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(RulesDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportRulesDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	report := ImportRules(Options{ProjectDir: root, DryRun: true})
	if report.Errors != nil {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run should show pending changes")
	}
	if len(report.Changes) != 1 || report.Changes[0].Action != "IMPORT" {
		t.Fatalf("expected 1 IMPORT change, got: %v", report.Changes)
	}

	// Verify no files were written
	targetPath := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")
	if _, err := os.Stat(targetPath); err == nil {
		t.Fatal("dry-run should not write files")
	}
}

func TestImportRulesWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	report := ImportRules(Options{ProjectDir: root})
	if report.Errors != nil {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if !report.Written {
		t.Fatal("expected files to be written")
	}

	targetPath := filepath.Join(root, filepath.FromSlash(OMRRulesDir), "style.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target file not written: %v", err)
	}
	if string(data) != "# Style Guide\nUse tabs.\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestImportRulesIdempotent(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style Guide\nUse tabs.\n")

	// First import
	report1 := ImportRules(Options{ProjectDir: root})
	if !report1.Written {
		t.Fatal("expected first import to write")
	}

	// Second import - should be no-op
	report2 := ImportRules(Options{ProjectDir: root})
	if !report2.NoOp {
		t.Fatalf("expected second import to be no-op, got changes: %v", report2.Changes)
	}
}

func TestImportRulesConflictDetected(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Claude Style\n")

	// Create existing but different file in target
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "style.md"), []byte("# Different Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := ImportRules(Options{ProjectDir: root})
	if len(report.Conflicts) == 0 {
		t.Fatal("expected conflict")
	}
	if report.Written {
		t.Fatal("should not write on conflict")
	}
	if !strings.Contains(report.Conflicts[0], "already exists") {
		t.Fatalf("expected 'already exists' conflict, got: %v", report.Conflicts)
	}
}

func TestImportRulesForceOverwritesConflict(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Claude Style\n")

	// Existing different content
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "style.md"), []byte("# Different Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := ImportRules(Options{ProjectDir: root, Force: true})
	if len(report.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", report.Errors)
	}
	if !report.Written {
		t.Fatal("expected force to write")
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Claude Style\n" {
		t.Fatalf("expected overwritten content, got: %q", data)
	}
}

func TestImportRulesMultipleRules(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "style.md", "# Style\n")
	writeClaudeRule(t, root, "security.md", "# Security\n")

	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected write")
	}
	if len(report.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(report.Changes), report.Changes)
	}
}

func TestImportRulesNoClaudeDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportRules(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op when no .claude/rules exists")
	}
}

func TestImportRulesRollbackOnWriteFailure(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeRule(t, root, "a.md", "# A\n")
	writeClaudeRule(t, root, "b.md", "# B\n")

	// First write succeeds
	report := ImportRules(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected first import to succeed")
	}

	// Change rules
	writeClaudeRule(t, root, "a.md", "# A v2\n")
	writeClaudeRule(t, root, "b.md", "# B v2\n")

	// Make target files read-only so os.WriteFile fails
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	os.Chmod(filepath.Join(targetDir, "a.md"), 0o444)
	os.Chmod(filepath.Join(targetDir, "b.md"), 0o444)

	report2 := ImportRules(Options{ProjectDir: root, Force: true})
	if len(report2.Errors) == 0 {
		t.Fatal("expected write errors")
	}

	// Restore permissions for assertions
	os.Chmod(filepath.Join(targetDir, "a.md"), 0o644)
	os.Chmod(filepath.Join(targetDir, "b.md"), 0o644)

	// Verify rollback: files should still be original content
	dataA, _ := os.ReadFile(filepath.Join(targetDir, "a.md"))
	if string(dataA) != "# A\n" {
		t.Fatalf("expected rollback of a.md to %q, got %q", "# A\n", string(dataA))
	}
	dataB, _ := os.ReadFile(filepath.Join(targetDir, "b.md"))
	if string(dataB) != "# B\n" {
		t.Fatalf("expected rollback of b.md to %q, got %q", "# B\n", string(dataB))
	}
}

func TestDiscoverRulesIgnoresNonMdFiles(t *testing.T) {
	root := newClaudeProject(t)
	rulesDir := filepath.Join(root, filepath.FromSlash(RulesDir))
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("text"), 0o644)
	os.WriteFile(filepath.Join(rulesDir, "style.md"), []byte("# Style"), 0o644)

	rules, err := DiscoverRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].SourceRel != "style.md" {
		t.Fatalf("expected 1 rule, got %d: %v", len(rules), rules)
	}
}
