package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[quality]\nfixtures = \"fixtures\"\nmin_qualified_rate = 0.9\nmax_cost = 1.5\n[runtime]\nmetrics_dir = \"metrics\"\nmodel = 'deepseek'\nmax_steps = 4\nconcurrency = 2\ntimeout = \"30s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "fixtures" || cfg.MinQualifiedRate != 0.9 || cfg.MaxCost != 1.5 || cfg.Model != "deepseek" || cfg.MaxSteps != 4 || cfg.Concurrency != 2 || cfg.Timeout != 30*time.Second {
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

func TestLoadCategoryRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[routing]\nfrontend = 'omr-frontend'\nexplore = \"omr-explore\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.Categories["frontend"] != "omr-frontend" {
		t.Fatalf("unexpected categories: %#v, err=%v", cfg.Categories, err)
	}
	if got := cfg.CategoryPrompt(); !strings.Contains(got, "`explore` → `omr-explore`") || !strings.Contains(got, "`frontend` → `omr-frontend`") {
		t.Fatalf("unexpected category prompt: %q", got)
	}
}

func TestLoadDisabledProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[profiles]\ndisabled = \"omr-debug, omr-research\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || len(cfg.DisabledProfiles) != 2 || !strings.Contains(cfg.DisabledProfilePrompt(), "`omr-debug`") {
		t.Fatalf("unexpected disabled profiles: %#v, err=%v", cfg.DisabledProfiles, err)
	}
}

func TestLoadRejectsInvalidAgentPromptPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-debug]\nprompt_file = \"/tmp/debug.md\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected absolute agent prompt path to be rejected")
	}
}

func TestLoadRejectsInvalidAgentProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr research]\nmodel = \"deepseek\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid agent profile to be rejected")
	}
}

func TestLoadRejectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[runtime]\nmax_steps = 4\nmax_steps = 8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected duplicate key to be rejected")
	}
}

func TestLoadRejectsNegativeConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[runtime]\nconcurrency = -1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected negative concurrency to be rejected")
	}
}

func TestLoadPreservesHashInsideQuotedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-debug]\nprompt_file = \"prompts/debug#1.md\" # comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["omr-debug"].PromptFile != "prompts/debug#1.md" {
		t.Fatalf("unexpected prompt file: %+v", cfg.Agents["omr-debug"])
	}
}
