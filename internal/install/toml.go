package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type tomlValue struct {
	Present bool
	Value   string
	Line    int
}

type agentConfig struct {
	Lines            []string
	HadTrailingLF    bool
	AgentSection     bool
	SystemPromptFile tomlValue
	SystemPrompt     tomlValue
}

func parseAgentConfig(text string) agentConfig {
	cfg := agentConfig{HadTrailingLF: strings.HasSuffix(text, "\n")}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	cfg.Lines = strings.Split(text, "\n")
	section := ""
	for i, line := range cfg.Lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if section == "agent" {
				cfg.AgentSection = true
			}
			continue
		}
		if section != "agent" || strings.HasPrefix(trimmed, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, raw, ok := splitTomlAssignment(line)
		if !ok {
			continue
		}
		value, ok := parseTomlString(raw)
		if !ok {
			continue
		}
		entry := tomlValue{Present: true, Value: value, Line: i}
		switch key {
		case "system_prompt_file":
			cfg.SystemPromptFile = entry
		case "system_prompt":
			cfg.SystemPrompt = entry
		}
	}
	return cfg
}

func splitTomlAssignment(line string) (key, raw string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	raw = strings.TrimSpace(line[idx+1:])
	if key == "" || raw == "" {
		return "", "", false
	}
	return key, raw, true
}

func parseTomlString(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' {
		end := closingQuote(raw, '"')
		if end < 0 {
			return "", false
		}
		// strconv.Unquote handles escaped TOML-compatible basic strings.
		value, err := strconv.Unquote(raw[:end+1])
		return value, err == nil
	}
	if len(raw) >= 2 && raw[0] == '\'' {
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", false
		}
		return raw[1 : end+1], true
	}
	if idx := strings.IndexByte(raw, '#'); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return raw, raw != ""
}

func closingQuote(raw string, quote byte) int {
	escaped := false
	for i := 1; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		if raw[i] == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if raw[i] == quote {
			return i
		}
	}
	return -1
}

func (c agentConfig) userPrompt(root string) (source, value string, present bool, err error) {
	if c.SystemPromptFile.Present {
		value, err := readPromptSource(root, c.SystemPromptFile.Value)
		if err != nil {
			return "", "", false, err
		}
		return c.SystemPromptFile.Value, value, true, nil
	}
	if c.SystemPrompt.Present {
		return "inline", c.SystemPrompt.Value, true, nil
	}
	return "", "", false, nil
}

func readPromptSource(root, source string) (string, error) {
	path := source
	if !isInlinePrompt(source) && !isBarePrompt(source) {
		if !filepath.IsAbs(source) {
			path = filepath.Join(root, source)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read user system prompt %q: %w", source, err)
		}
		return string(data), nil
	}
	return source, nil
}

func isInlinePrompt(value string) bool { return strings.HasPrefix(value, "inline:") }
func isBarePrompt(value string) bool   { return strings.HasPrefix(value, "<inline>") }

func replaceOrAppendAgentFile(text string, value string) (string, error) {
	cfg := parseAgentConfig(text)
	quoted := strconv.Quote(value)
	if cfg.SystemPromptFile.Present {
		line := cfg.Lines[cfg.SystemPromptFile.Line]
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return "", fmt.Errorf("invalid system_prompt_file assignment")
		}
		cfg.Lines[cfg.SystemPromptFile.Line] = line[:idx+1] + " " + quoted
	} else if cfg.AgentSection {
		insertAt := len(cfg.Lines)
		section := ""
		for i, line := range cfg.Lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				if section == "agent" {
					insertAt = i
					break
				}
				section = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			}
		}
		if insertAt == len(cfg.Lines) && len(cfg.Lines) > 0 && cfg.Lines[len(cfg.Lines)-1] == "" {
			// Reuse the trailing empty line so an install/uninstall round trip
			// does not accumulate blank lines.
			cfg.Lines[len(cfg.Lines)-1] = "system_prompt_file = " + quoted
		} else {
			cfg.Lines = append(cfg.Lines, "")
			copy(cfg.Lines[insertAt+1:], cfg.Lines[insertAt:])
			cfg.Lines[insertAt] = "system_prompt_file = " + quoted
		}
	} else {
		if len(cfg.Lines) > 0 && cfg.Lines[len(cfg.Lines)-1] != "" {
			cfg.Lines = append(cfg.Lines, "")
		}
		cfg.Lines = append(cfg.Lines, "[agent]", "system_prompt_file = "+quoted)
	}
	result := strings.Join(cfg.Lines, "\n")
	if cfg.HadTrailingLF && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}
