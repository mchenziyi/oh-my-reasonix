package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[quality]\nfixtures = \"fixtures\"\nmin_qualified_rate = 0.9\n[runtime]\nmetrics_dir = \"metrics\"\nmodel = 'deepseek'\nmax_steps = 4\ntimeout = \"30s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "fixtures" || cfg.MinQualifiedRate != 0.9 || cfg.Model != "deepseek" || cfg.MaxSteps != 4 || cfg.Timeout != 30*time.Second {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadAgentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := "[agent.omr-research]\nmodel = 'deepseek-v4-flash'\nprompt_file = \"prompts/research.md\"\nread_only = true\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := cfg.Agents["omr-research"]
	if !ok || agent.Model != "deepseek-v4-flash" || agent.PromptFile != "prompts/research.md" || agent.ReadOnly == nil || !*agent.ReadOnly {
		t.Fatalf("unexpected agent config: %+v", cfg.Agents)
	}
}
