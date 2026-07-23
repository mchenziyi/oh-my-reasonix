package claude

import (
	"strings"
	"testing"
)

func TestImportMCPCompatReport(t *testing.T) {
	root := newClaudeProject(t)
	mcpJSON := `{"mcpServers":{"server1":{"command":"node","args":["server.js"],"env":{"KEY":"value"}}}}`
	writeClaudeMCP(t, root, mcpJSON)
	report := ImportMCP(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected MCP import to write")
	}
	// Should have warnings about compatibility
	foundCompat := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "MCP: server1") {
			foundCompat = true
		}
	}
	if !foundCompat {
		t.Fatalf("expected MCP compat warning, got: %v", report.Warnings)
	}
}

func TestImportMCPRedactsEnvVars(t *testing.T) {
	root := newClaudeProject(t)
	mcpJSON := `{"mcpServers":{"srv":{"command":"echo","env":{"SECRET":"topsecret","API_KEY":"abc123"}}}}`
	writeClaudeMCP(t, root, mcpJSON)
	report := ImportMCP(Options{ProjectDir: root})
	foundRedacted := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "***") {
			foundRedacted = true
		}
		if strings.Contains(w, "topsecret") || strings.Contains(w, "abc123") {
			t.Fatalf("env value leaked in warning: %s", w)
		}
	}
	if !foundRedacted {
		t.Fatalf("expected redacted env values in warnings, got: %v", report.Warnings)
	}
}

func TestImportMCPDryRunNoWrite(t *testing.T) {
	root := newClaudeProject(t)
	mcpJSON := `{"mcpServers":{"s":{"command":"test"}}}`
	writeClaudeMCP(t, root, mcpJSON)
	report := ImportMCP(Options{ProjectDir: root, DryRun: true})
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run should show planned changes")
	}
}

func TestImportMCPEmptyServers(t *testing.T) {
	root := newClaudeProject(t)
	mcpJSON := `{"mcpServers":{}}`
	writeClaudeMCP(t, root, mcpJSON)
	report := ImportMCP(Options{ProjectDir: root})
	if !report.Written {
		t.Fatal("expected MCP import to write even with empty servers")
	}
}
