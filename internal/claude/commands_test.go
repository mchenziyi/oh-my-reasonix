package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeClaudeCommand(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(CommandsDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportCommandsDryRun(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeCommand(t, root, "deploy.sh", "#!/bin/sh\necho deploy\n")
	report := ImportCommands(Options{ProjectDir: root, DryRun: true})
	if report.NoOp {
		t.Fatal("expected planned changes in dry-run")
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if len(report.Changes) != 1 || report.Changes[0].Action != "IMPORT" {
		t.Fatalf("expected 1 IMPORT change, got: %v", report.Changes)
	}
}

func TestImportCommandsWritesFiles(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeCommand(t, root, "test.sh", "#!/bin/sh\necho hello\n")
	report := ImportCommands(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected files to be written")
	}
	target := filepath.Join(root, filepath.FromSlash(OMRSkillsDir), "cmd-test", "SKILL.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected target file: %v", err)
	}
	if !strings.Contains(string(data), "cmd-test") {
		t.Fatalf("expected cmd-test in generated content, got: %s", string(data))
	}
}

func TestImportCommandsNoDir(t *testing.T) {
	root := newClaudeProject(t)
	report := ImportCommands(Options{ProjectDir: root})
	if !report.NoOp {
		t.Fatal("expected no-op when commands dir missing")
	}
}

func TestImportCommandsDuplicateName(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeCommand(t, root, "build.sh", "echo build\n")
	writeClaudeCommand(t, root, "build.txt", "echo build again\n")
	report := ImportCommands(Options{ProjectDir: root})
	if len(report.Conflicts) == 0 {
		t.Fatal("expected conflict for duplicate command name")
	}
}

func TestImportCommandsSkipsBinary(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeCommand(t, root, "script.sh", "#!/bin/sh\necho ok\n")
	writeClaudeCommand(t, root, "binary.bin", string([]byte{0, 1, 2, 3}))
	report := ImportCommands(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected script to be imported")
	}
	// Only the script should be imported (.bin extension is skipped)
	if len(report.Changes) != 1 {
		t.Fatalf("expected 1 change (binary skipped), got %d: %v", len(report.Changes), report.Changes)
	}
}
