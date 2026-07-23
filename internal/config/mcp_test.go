package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMCPServerStdio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.docs]\ntransport = \"stdio\"\ncommand = \"mcp-docs\"\nargs = [\"--port\", \"8080\"]\ncapabilities = [\"docs\", \"web\"]\nenabled = true\nenv = [\"API_KEY\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	srv, ok := cfg.MCPServers["docs"]
	if !ok {
		t.Fatal("expected docs server")
	}
	if srv.Transport != "stdio" {
		t.Fatalf("expected transport stdio, got %q", srv.Transport)
	}
	if srv.Command != "mcp-docs" {
		t.Fatalf("expected command mcp-docs, got %q", srv.Command)
	}
	if !srv.Enabled {
		t.Fatal("expected enabled=true")
	}
	if len(srv.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(srv.Args))
	}
}

func TestLoadMCPServerHTTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.web]\ntransport = \"http\"\nurl = \"https://example.com/mcp\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.MCPServers["web"]; !ok {
		t.Fatal("expected web server")
	}
}

func TestLoadMCPDefaultDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.docs]\ntransport = \"stdio\"\ncommand = \"mcp-docs\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MCPServers["docs"].Enabled {
		t.Fatal("expected default disabled")
	}
}

func TestLoadRejectsUnknownMCPTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.bad]\ntransport = \"unknown\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for unknown transport")
	}
}

func TestLoadRejectsMCPMissingCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.bad]\ntransport = \"stdio\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestLoadRejectsMCPMissingURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.bad]\ntransport = \"http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestLoadMCPEnvNamesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.docs]\ntransport = \"stdio\"\ncommand = \"mcp-docs\"\nenv = [\"API_KEY\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.MCPServers["docs"].Env) != 1 || cfg.MCPServers["docs"].Env[0] != "API_KEY" {
		t.Fatalf("expected env names, got: %v", cfg.MCPServers["docs"].Env)
	}
}

func TestLoadRejectsMCPEnvValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mcp.docs]\ncommand = \"mcp-docs\"\nenv = [\"API_KEY=secret\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "environment variable name") {
		t.Fatalf("expected env name error, got %v", err)
	}
}

func TestLoadJSONCMCPAndSSE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"mcp":{"docs":{"transport":"sse","url":"https://example.com/mcp","enabled":true,"capabilities":["docs"]}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPServers["docs"].Transport != "sse" || !cfg.MCPServers["docs"].Enabled {
		t.Fatalf("unexpected MCP config: %#v", cfg.MCPServers)
	}
}

func TestLoadJSONCMCPDefaultsToStdio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	if err := os.WriteFile(path, []byte(`{"mcp":{"docs":{"command":"mcp-docs"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MCPServers["docs"].Transport != "stdio" {
		t.Fatalf("expected stdio default, got %#v", cfg.MCPServers["docs"])
	}
}

func TestLoadRejectsRemoteCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "[mcp.docs]\ntransport = \"http\"\nurl = \"https://token@example.com/mcp\"\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "embedded credentials") {
		t.Fatalf("expected credential rejection, got %v", err)
	}
}

func TestDiagnoseMCPReportsMissingCommandAndEnvWithoutValues(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := Config{MCPServers: map[string]MCPServerConfig{
		"docs": {
			Transport:    "stdio",
			Command:      "missing-docs-command",
			Enabled:      true,
			Capabilities: []string{"web", "docs"},
			Env:          []string{"MISSING_DOCS_KEY"},
		},
	}}
	diagnostics := DiagnoseMCP(cfg)
	if len(diagnostics) != 1 || diagnostics[0].Availability != "unavailable" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if diagnostics[0].CommandAvailable == nil || *diagnostics[0].CommandAvailable {
		t.Fatalf("expected missing command: %#v", diagnostics[0])
	}
	if len(diagnostics[0].MissingEnv) != 1 || strings.Contains(diagnostics[0].Summary(), "secret") {
		t.Fatalf("unexpected redaction result: %#v", diagnostics[0])
	}
}

func TestMCPPromptOnlyIncludesEnabledSafeMetadata(t *testing.T) {
	cfg := Config{MCPServers: map[string]MCPServerConfig{
		"docs": {Transport: "http", URL: "https://example.com/private", Enabled: true, Capabilities: []string{"docs"}},
		"off":  {Transport: "stdio", Command: "/private/tool", Enabled: false},
	}}
	prompt := cfg.MCPPrompt()
	if !strings.Contains(prompt, "`docs`") || !strings.Contains(prompt, "capabilities: docs") {
		t.Fatalf("missing enabled MCP metadata: %q", prompt)
	}
	if strings.Contains(prompt, "example.com") || strings.Contains(prompt, "/private/tool") || strings.Contains(prompt, "`off`") {
		t.Fatalf("prompt leaked endpoint, command, or disabled server: %q", prompt)
	}
}

func TestDiagnoseMCPMarksUnknownCapabilities(t *testing.T) {
	cfg := Config{MCPServers: map[string]MCPServerConfig{
		"docs": {Transport: "http", URL: "https://example.com/mcp", Enabled: true, Capabilities: []string{"docs", "semantic-magic"}},
	}}
	diagnostics := DiagnoseMCP(cfg)
	if len(diagnostics) != 1 || diagnostics[0].Compatibility != "unknown" ||
		len(diagnostics[0].UnknownCapabilities) != 1 ||
		diagnostics[0].UnknownCapabilities[0] != "semantic-magic" {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}
