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
	DisabledProfiles    []string
}

type AgentConfig struct {
	Model      string
	PromptFile string
	ReadOnly   *bool
}

// DisabledRoutingConflicts returns category routes that target a disabled Profile.
func (c Config) DisabledRoutingConflicts() []string {
	if len(c.Categories) == 0 || len(c.DisabledProfiles) == 0 {
		return nil
	}
	disabled := make(map[string]bool, len(c.DisabledProfiles))
	for _, profile := range c.DisabledProfiles {
		disabled[profile] = true
	}
	categories := make([]string, 0)
	for category, profile := range c.Categories {
		if disabled[profile] {
			categories = append(categories, category)
		}
	}
	sort.Strings(categories)
	return categories
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
			if section != "quality" && section != "runtime" && section != "routing" && section != "profiles" && !strings.HasPrefix(section, "agent.") {
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
	if len(cfg.DisabledProfiles) > 1 {
		seenProfiles := make(map[string]bool, len(cfg.DisabledProfiles))
		uniqueProfiles := cfg.DisabledProfiles[:0]
		for _, profile := range cfg.DisabledProfiles {
			if !seenProfiles[profile] {
				seenProfiles[profile] = true
				uniqueProfiles = append(uniqueProfiles, profile)
			}
		}
		cfg.DisabledProfiles = uniqueProfiles
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
			value, err := expandEnv(stringValue(raw))
			if err != nil {
				return err
			}
			cfg.Fixtures = value
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
			value, err := expandEnv(stringValue(raw))
			if err != nil {
				return err
			}
			cfg.MetricsDir = value
		case "model":
			value, err := expandEnv(stringValue(raw))
			if err != nil {
				return err
			}
			cfg.Model = value
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
			value, err := expandEnv(stringValue(raw))
			if err != nil {
				return err
			}
			agent.Model = value
		case "prompt_file":
			value, err := expandEnv(stringValue(raw))
			if err != nil {
				return err
			}
			agent.PromptFile = value
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
	if section == "profiles" {
		if key != "disabled" {
			return fmt.Errorf("unsupported profiles key %q", key)
		}
		for _, profile := range strings.Split(stringValue(raw), ",") {
			profile = strings.TrimSpace(profile)
			if profile == "" || strings.ContainsAny(profile, " \t/\\") {
				return fmt.Errorf("invalid disabled Profile %q", profile)
			}
			cfg.DisabledProfiles = append(cfg.DisabledProfiles, profile)
		}
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

func (c Config) DisabledProfilePrompt() string {
	if len(c.DisabledProfiles) == 0 {
		return ""
	}
	profiles := append([]string(nil), c.DisabledProfiles...)
	sort.Strings(profiles)
	var b strings.Builder
	b.WriteString("\n\n## Disabled OMR Profiles\n\nDo not route tasks to these Profiles:\n")
	for _, profile := range profiles {
		fmt.Fprintf(&b, "- `%s`\n", profile)
	}
	return b.String()
}

func stringValue(raw string) string {
	if len(raw) >= 2 && ((raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'')) {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func expandEnv(value string) (string, error) {
	missing := ""
	expanded := os.Expand(value, func(key string) string {
		resolved, ok := os.LookupEnv(key)
		if !ok && missing == "" {
			missing = key
		}
		return resolved
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %q is not set", missing)
	}
	return expanded, nil
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
		if r == '/' && quote == 0 && i+1 < len(line) && line[i+1] == '/' {
			return line[:i]
		}
	}
	return line
}
