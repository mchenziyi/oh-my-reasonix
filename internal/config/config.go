package config

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Fixtures            string
	MetricsDir          string
	Model               string
	MaxSteps            int
	Concurrency         int
	Timeout             time.Duration
	MinQualifiedRate    float64
	MinQualifiedRateSet bool
	MaxCost             float64
	MaxCostSet          bool
	TimeoutSet          bool
	Agents              map[string]AgentConfig
	Categories          map[string]string
}

type AgentConfig struct {
	Model      string
	PromptFile string
	ReadOnly   *bool
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	section := ""
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section != "quality" && section != "runtime" && section != "routing" && !strings.HasPrefix(section, "agent.") {
				return Config{}, fmt.Errorf("%s:%d: unsupported section %q", path, lineNo, section)
			}
			if strings.HasPrefix(section, "agent.") && strings.TrimSpace(strings.TrimPrefix(section, "agent.")) == "" {
				return Config{}, fmt.Errorf("%s:%d: agent profile is required", path, lineNo)
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return Config{}, fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		seenKey := section + "." + key
		if seen[seenKey] {
			return Config{}, fmt.Errorf("%s:%d: duplicate key %q", path, lineNo, key)
		}
		seen[seenKey] = true
		if err := assign(&cfg, section, key, value); err != nil {
			return Config{}, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	if cfg.MaxSteps < 0 || cfg.Concurrency < 0 || cfg.MinQualifiedRate < 0 || cfg.MinQualifiedRate > 1 || cfg.MaxCost < 0 || cfg.Timeout < 0 {
		return Config{}, fmt.Errorf("invalid OMR benchmark configuration")
	}
	for profile, agent := range cfg.Agents {
		if strings.TrimSpace(profile) != profile || strings.ContainsAny(profile, " \t/\\") {
			return Config{}, fmt.Errorf("invalid agent profile %q", profile)
		}
		if strings.ContainsAny(agent.Model, "\r\n\t") {
			return Config{}, fmt.Errorf("invalid model for agent %q", profile)
		}
		if strings.HasPrefix(agent.PromptFile, "/") || strings.Contains(agent.PromptFile, "\\") {
			return Config{}, fmt.Errorf("prompt_file for agent %q must be a project-relative path", profile)
		}
	}
	return cfg, nil
}

func assign(cfg *Config, section, key, raw string) error {
	if section == "quality" {
		switch key {
		case "fixtures":
			cfg.Fixtures = stringValue(raw)
		case "min_qualified_rate":
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return fmt.Errorf("invalid min_qualified_rate")
			}
			cfg.MinQualifiedRate = value
			cfg.MinQualifiedRateSet = true
		case "max_cost":
			value, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return fmt.Errorf("invalid max_cost")
			}
			cfg.MaxCost = value
			cfg.MaxCostSet = true
		default:
			return fmt.Errorf("unsupported quality key %q", key)
		}
		return nil
	}
	if section == "runtime" {
		switch key {
		case "metrics_dir":
			cfg.MetricsDir = stringValue(raw)
		case "model":
			cfg.Model = stringValue(raw)
		case "max_steps":
			value, err := strconv.Atoi(raw)
			if err != nil {
				return fmt.Errorf("invalid max_steps")
			}
			cfg.MaxSteps = value
		case "concurrency":
			value, err := strconv.Atoi(raw)
			if err != nil {
				return fmt.Errorf("invalid concurrency")
			}
			cfg.Concurrency = value
		case "timeout":
			value, err := time.ParseDuration(stringValue(raw))
			if err != nil {
				return fmt.Errorf("invalid timeout")
			}
			cfg.Timeout = value
			cfg.TimeoutSet = true
		default:
			return fmt.Errorf("unsupported runtime key %q", key)
		}
		return nil
	}
	if strings.HasPrefix(section, "agent.") {
		profile := strings.TrimSpace(strings.TrimPrefix(section, "agent."))
		if cfg.Agents == nil {
			cfg.Agents = make(map[string]AgentConfig)
		}
		agent := cfg.Agents[profile]
		switch key {
		case "model":
			agent.Model = stringValue(raw)
		case "prompt_file":
			agent.PromptFile = stringValue(raw)
		case "read_only":
			value, err := strconv.ParseBool(raw)
			if err != nil {
				return fmt.Errorf("invalid read_only")
			}
			agent.ReadOnly = &value
		default:
			return fmt.Errorf("unsupported agent key %q", key)
		}
		cfg.Agents[profile] = agent
		return nil
	}
	if section == "routing" {
		if key == "" || strings.ContainsAny(key, " \t/\\") {
			return fmt.Errorf("invalid category %q", key)
		}
		profile := stringValue(raw)
		if profile == "" || strings.ContainsAny(profile, "\r\n\t /\\") {
			return fmt.Errorf("invalid category profile for %q", key)
		}
		if cfg.Categories == nil {
			cfg.Categories = make(map[string]string)
		}
		cfg.Categories[key] = profile
		return nil
	}
	return fmt.Errorf("key %q must be under [quality] or [runtime]", key)
}

// CategoryPrompt renders deterministic project routing instructions.
func (c Config) CategoryPrompt() string {
	if len(c.Categories) == 0 {
		return ""
	}
	keys := make([]string, 0, len(c.Categories))
	for key := range c.Categories {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("\n\n## Project Category Routing\n\n")
	b.WriteString("When a task matches one of these categories, prefer the configured Profile:\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "- `%s` → `%s`\n", key, c.Categories[key])
	}
	return b.String()
}

func stringValue(raw string) string {
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func stripComment(line string) string {
	var quote rune
	for i, r := range line {
		if (r == '\'' || r == '"') && (i == 0 || line[i-1] != '\\') {
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		}
		if r == '#' && quote == 0 {
			return line[:i]
		}
	}
	return line
}
