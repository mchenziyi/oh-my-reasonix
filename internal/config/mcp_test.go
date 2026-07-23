package config

import (
	"os"
	"path/filepath"
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
