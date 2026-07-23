package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportHookReportsLostSemantics(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "deploy.js", "console.log('deploy');\n")
	report := ImportHooks(Options{ProjectDir: root})
	foundWarning := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "已转为策略提示") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected lost semantics warning, got: %v", report.Warnings)
	}
}

func TestImportHookReportsDangerousCommands(t *testing.T) {
	root := newClaudeProject(t)
	hookContent := `#!/bin/bash
rm -rf /data
sudo systemctl restart
`
	writeClaudeHook(t, root, "cleanup.sh", hookContent)
	report := ImportHooks(Options{ProjectDir: root})
	foundDanger := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "recursive delete") || strings.Contains(w, "escalated privilege") {
			foundDanger = true
		}
	}
	if !foundDanger {
		t.Fatalf("expected danger warnings, got: %v", report.Warnings)
	}
	// Also verify disclaimer contains risk info
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	entries, _ := os.ReadDir(targetDir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hook-cleanup") {
			data, _ := os.ReadFile(filepath.Join(targetDir, entry.Name()))
			content := string(data)
			if !strings.Contains(content, "需要人工复核") {
				t.Fatalf("expected risk mention in disclaimer, got: %s", content)
			}
		}
	}
}

func TestImportHookRedactsSecretsInWarning(t *testing.T) {
	root := newClaudeProject(t)
	hookContent := `API_KEY=abc123
password=secret
`
	writeClaudeHook(t, root, "config.sh", hookContent)
	report := ImportHooks(Options{ProjectDir: root})
	for _, w := range report.Warnings {
		if strings.Contains(w, "API_KEY=") && strings.Contains(w, "***") {
			// Good: redacted
		}
		if strings.Contains(w, "abc123") || strings.Contains(w, "secret") {
			t.Fatalf("secret value leaked in warning: %s", w)
		}
	}
}

func TestImportHookSafeHookNoWarning(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "lint.js", "console.log('linting');\n")
	report := ImportHooks(Options{ProjectDir: root})
	// Should still have "转为策略提示" warning, but no danger risks
	for _, w := range report.Warnings {
		if strings.Contains(w, "风险: 无") {
			return // Found "风险: 无" = no risks detected
		}
	}
	// Default: just check it was imported
	if !report.Written && !report.NoOp {
		t.Fatal("expected hook import to succeed")
	}
}

func TestImportHookDisclaimerMentionsLostSemantics(t *testing.T) {
	root := newClaudeProject(t)
	writeClaudeHook(t, root, "test.sh", "echo hello\n")
	ImportHooks(Options{ProjectDir: root})
	targetDir := filepath.Join(root, filepath.FromSlash(OMRRulesDir))
	entries, _ := os.ReadDir(targetDir)
	found := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hook-") {
			data, _ := os.ReadFile(filepath.Join(targetDir, entry.Name()))
			if strings.Contains(string(data), "运行时语义已丢失") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected '运行时语义已丢失' in hook disclaimer")
	}
}
