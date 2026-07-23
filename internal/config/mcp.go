package config

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type MCPDiagnostic struct {
	Server               string   `json:"server"`
	Transport            string   `json:"transport"`
	Capabilities         []string `json:"capabilities"`
	Enabled              bool     `json:"enabled"`
	Compatibility        string   `json:"compatibility"`
	Availability         string   `json:"availability"`
	CommandAvailable     *bool    `json:"command_available,omitempty"`
	UnknownCapabilities  []string `json:"unknown_capabilities,omitempty"`
	MissingEnv           []string `json:"missing_env,omitempty"`
	Risks                []string `json:"risks,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
}

func DiagnoseMCP(cfg Config) []MCPDiagnostic {
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	diagnostics := make([]MCPDiagnostic, 0, len(names))
	for _, name := range names {
		server := cfg.MCPServers[name]
		diagnostic := MCPDiagnostic{
			Server:               name,
			Transport:            server.Transport,
			Capabilities:         append([]string(nil), server.Capabilities...),
			Enabled:              server.Enabled,
			Compatibility:        "compatible",
			Availability:         "disabled",
			RequiresConfirmation: server.Enabled,
		}
		sort.Strings(diagnostic.Capabilities)
		for _, capability := range diagnostic.Capabilities {
			if !knownMCPCapability(capability) {
				diagnostic.UnknownCapabilities = append(diagnostic.UnknownCapabilities, capability)
				diagnostic.Compatibility = "unknown"
			}
		}
		if !server.Enabled {
			diagnostics = append(diagnostics, diagnostic)
			continue
		}

		diagnostic.Availability = "ready"
		if server.Transport == "stdio" {
			diagnostic.Risks = append(diagnostic.Risks, "starts a local process")
			available := true
			if _, err := exec.LookPath(server.Command); err != nil {
				available = false
				diagnostic.Availability = "unavailable"
			}
			diagnostic.CommandAvailable = &available
		} else {
			diagnostic.Risks = append(diagnostic.Risks, "uses a remote network endpoint")
		}
		for _, name := range server.Env {
			if _, ok := os.LookupEnv(name); !ok {
				diagnostic.MissingEnv = append(diagnostic.MissingEnv, name)
				diagnostic.Availability = "unavailable"
			}
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

func (d MCPDiagnostic) Summary() string {
	parts := []string{d.Availability, d.Compatibility, "transport=" + d.Transport}
	if len(d.Capabilities) > 0 {
		parts = append(parts, "capabilities="+strings.Join(d.Capabilities, ","))
	}
	if len(d.MissingEnv) > 0 {
		parts = append(parts, "missing_env="+strings.Join(d.MissingEnv, ","))
	}
	if len(d.UnknownCapabilities) > 0 {
		parts = append(parts, "unknown_capabilities="+strings.Join(d.UnknownCapabilities, ","))
	}
	if d.CommandAvailable != nil && !*d.CommandAvailable {
		parts = append(parts, "command_not_in_path")
	}
	if len(d.Risks) > 0 {
		parts = append(parts, "risk="+strings.Join(d.Risks, ","))
	}
	if d.RequiresConfirmation {
		parts = append(parts, "user_confirmation_required")
	}
	return strings.Join(parts, "; ")
}

func (c Config) MCPPrompt() string {
	diagnostics := DiagnoseMCP(c)
	var enabled []MCPDiagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Enabled {
			enabled = append(enabled, diagnostic)
		}
	}
	if len(enabled) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Optional Project MCP\n\n")
	b.WriteString("The project declares these optional MCP servers. Use them only when the Reasonix runtime exposes their tools and the user has confirmed the server. If unavailable, continue without MCP and report the limitation; never invent sources or capabilities.\n")
	for _, diagnostic := range enabled {
		fmt.Fprintf(&b, "- `%s` (%s", diagnostic.Server, diagnostic.Transport)
		if len(diagnostic.Capabilities) > 0 {
			fmt.Fprintf(&b, "; capabilities: %s", strings.Join(diagnostic.Capabilities, ", "))
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func normalizeMCPServer(name string, server MCPServerConfig) (MCPServerConfig, error) {
	if !validMCPName(name) {
		return MCPServerConfig{}, fmt.Errorf("invalid mcp server name %q", name)
	}
	if server.Transport == "" {
		server.Transport = "stdio"
	}
	switch server.Transport {
	case "stdio":
		if strings.TrimSpace(server.Command) == "" {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: transport stdio requires command", name)
		}
		if strings.ContainsAny(server.Command, "\r\n\x00") {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: command contains control characters", name)
		}
		if server.URL != "" {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: stdio transport does not accept url", name)
		}
	case "http", "sse":
		if strings.TrimSpace(server.URL) == "" {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: transport %s requires url", name, server.Transport)
		}
		parsed, err := url.Parse(server.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: remote url must be http(s) without embedded credentials", name)
		}
		if server.Command != "" || len(server.Args) > 0 {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: remote transport does not accept command or args", name)
		}
	default:
		return MCPServerConfig{}, fmt.Errorf("invalid mcp %q: unsupported transport %q", name, server.Transport)
	}

	seenEnv := make(map[string]bool, len(server.Env))
	env := server.Env[:0]
	for _, name := range server.Env {
		if !validEnvName(name) {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp environment variable name %q", name)
		}
		if !seenEnv[name] {
			seenEnv[name] = true
			env = append(env, name)
		}
	}
	server.Env = env
	for _, capability := range server.Capabilities {
		if !validMCPName(capability) {
			return MCPServerConfig{}, fmt.Errorf("invalid mcp capability %q", capability)
		}
	}
	return server, nil
}

func validMCPName(name string) bool {
	if name == "" || len(name) > 64 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func validEnvName(name string) bool {
	if name == "" || !isEnvStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isEnvStart(name[i]) && (name[i] < '0' || name[i] > '9') {
			return false
		}
	}
	return true
}

func isEnvStart(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func knownMCPCapability(capability string) bool {
	switch capability {
	case "docs", "web", "code-search", "version-filter":
		return true
	default:
		return false
	}
}
