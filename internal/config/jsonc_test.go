package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadJSONC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"quality": {
			"fixtures": "fixtures",
			"min_qualified_rate": 0.9,
			"max_cost": 1.5
		},
		"runtime": {
			"metrics_dir": "metrics",
			"model": "deepseek",
			"max_steps": 4,
			"concurrency": 2,
			"timeout": "30s"
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
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

func TestLoadJSONCAgentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"agent": {
			"omr-research": {
				"model": "deepseek-v4-flash",
				"prompt_file": "prompts/research.md",
				"read_only": true
			}
		}
	}`
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

func TestLoadJSONCCategoryRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"routing": {
			"frontend": "omr-frontend",
			"explore": "omr-explore"
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Categories["frontend"] != "omr-frontend" {
		t.Fatalf("unexpected categories: %#v", cfg.Categories)
	}
	if got := cfg.CategoryPrompt(); !strings.Contains(got, "`explore` → `omr-explore`") || !strings.Contains(got, "`frontend` → `omr-frontend`") {
		t.Fatalf("unexpected category prompt: %q", got)
	}
}

func TestLoadJSONCDisabledProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"profiles": {
			"disabled": ["omr-debug", "omr-research", "omr-debug"]
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DisabledProfiles) != 2 || !strings.Contains(cfg.DisabledProfilePrompt(), "`omr-debug`") {
		t.Fatalf("unexpected disabled profiles: %#v, err=%v", cfg.DisabledProfiles, err)
	}
}

func TestLoadJSONCSingleLineComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		// This is a comment
		"quality": {
			"fixtures": "test_fixtures" // trailing comment
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "test_fixtures" {
		t.Fatalf("unexpected fixtures: %q", cfg.Fixtures)
	}
}

func TestLoadJSONCBlockComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		/* block comment */
		"quality": {
			"fixtures": "test_fixtures" /* trailing */
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "test_fixtures" {
		t.Fatalf("unexpected fixtures: %q", cfg.Fixtures)
	}
}

func TestLoadJSONCPreservesURLInQuotedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"agent": {
			"omr-research": {
				"prompt_file": "https://example.com/research.md"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Agents["omr-research"].PromptFile; got != "https://example.com/research.md" {
		t.Fatalf("URL was truncated: %q", got)
	}
}

func TestLoadJSONCExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("OMR_TEST_MODEL", "deepseek-v4-flash")
	t.Setenv("OMR_TEST_PROMPT", "prompts/research.md")
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"runtime": { "model": "$OMR_TEST_MODEL" },
		"agent": {
			"omr-research": { "prompt_file": "${OMR_TEST_PROMPT}" }
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "deepseek-v4-flash" || cfg.Agents["omr-research"].PromptFile != "prompts/research.md" {
		t.Fatalf("unexpected expanded config: %#v", cfg)
	}
}

func TestLoadJSONCRejectsMissingEnvironmentVariable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"runtime": { "model": "${OMR_MISSING_MODEL}" }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "OMR_MISSING_MODEL") {
		t.Fatalf("expected missing environment variable error, got %v", err)
	}
}

func TestLoadJSONCRejectsInvalidAgentProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"agent": {
			"omr research": { "model": "deepseek" }
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid agent profile to be rejected")
	}
}

func TestLoadJSONCRejectsInvalidAgentPromptPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"agent": {
			"omr-debug": { "prompt_file": "/tmp/debug.md" }
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected absolute agent prompt path to be rejected")
	}
}

func TestLoadJSONCRejectsNegativeConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"runtime": { "concurrency": -1 }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected negative concurrency to be rejected")
	}
}

func TestLoadJSONCRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	// Invalid JSON: trailing comma after last element
	data := `{
		"quality": {
			"fixtures": "test"
		},
		"runtime": {
			"model": "test",
		}  // <-- trailing comma after "model"
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid JSON to be rejected")
	}
	if !strings.Contains(err.Error(), "JSON syntax error") {
		t.Fatalf("expected JSON syntax error, got: %v", err)
	}
}

func TestLoadJSONCUnterminatedBlockComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		/* unterminated block comment
		"quality": {}
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected unterminated block comment error")
	}
	if !strings.Contains(err.Error(), "unterminated block comment") {
		t.Fatalf("expected unterminated block comment error, got: %v", err)
	}
}

func TestLoadJSONCRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"unknown_field": "value"
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestLoadJSONCRejectsInvalidMinQualifiedRate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"quality": { "min_qualified_rate": 1.5 }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected min_qualified_rate > 1 to be rejected")
	}
}

func TestLoadJSONCRejectsNegativeMaxCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"quality": { "max_cost": -0.5 }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected negative max_cost to be rejected")
	}
}

func TestLoadJSONCWithBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	// BOM (0xEF 0xBB 0xBF) followed by valid JSON
	bom := []byte{0xEF, 0xBB, 0xBF}
	data := append(bom, []byte(`{"quality": {"fixtures": "bom_test"}}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "bom_test" {
		t.Fatalf("unexpected fixtures with BOM: %q", cfg.Fixtures)
	}
}

func TestLoadJSONCWithJSONExt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"quality": {"fixtures": "json_ext_test"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "json_ext_test" {
		t.Fatalf("unexpected fixtures with .json: %q", cfg.Fixtures)
	}
}

func TestLoadJSONCMinQualifiedRateExplicitZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"quality": { "min_qualified_rate": 0 }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MinQualifiedRateSet {
		t.Fatal("expected MinQualifiedRateSet to be true when explicitly set to 0")
	}
	if cfg.MinQualifiedRate != 0 {
		t.Fatalf("expected MinQualifiedRate=0, got %f", cfg.MinQualifiedRate)
	}
}

func TestLoadJSONCMaxCostExplicitZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{
		"quality": { "max_cost": 0 }
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MaxCostSet {
		t.Fatal("expected MaxCostSet to be true when explicitly set to 0")
	}
	if cfg.MaxCost != 0 {
		t.Fatalf("expected MaxCost=0, got %f", cfg.MaxCost)
	}
}

func TestFindConfigPrefersJSONC(t *testing.T) {
	root := t.TempDir()
	omrDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(omrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create both .toml and .jsonc configs
	if err := os.WriteFile(filepath.Join(omrDir, "config.toml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(omrDir, "config.jsonc"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfig(root)
	if !strings.HasSuffix(got, "config.jsonc") {
		t.Fatalf("expected config.jsonc, got %s", got)
	}
}

func TestFindConfigFallsBackToTOML(t *testing.T) {
	root := t.TempDir()
	omrDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(omrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(omrDir, "config.toml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfig(root)
	if !strings.HasSuffix(got, "config.toml") {
		t.Fatalf("expected config.toml, got %s", got)
	}
}

func TestFindConfigPrefersJSONCOverJSON(t *testing.T) {
	root := t.TempDir()
	omrDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(omrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(omrDir, "config.json"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(omrDir, "config.jsonc"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfig(root)
	if !strings.HasSuffix(got, "config.jsonc") {
		t.Fatalf("expected config.jsonc, got %s", got)
	}
}

func TestFindConfigPrefersJSONOverTOML(t *testing.T) {
	root := t.TempDir()
	omrDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(omrDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(omrDir, "config.json"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(omrDir, "config.toml"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	got := FindConfig(root)
	if !strings.HasSuffix(got, "config.json") {
		t.Fatalf("expected config.json, got %s", got)
	}
}

func TestFindConfigReturnsDefaultPathWhenNonexistent(t *testing.T) {
	root := t.TempDir()
	got := FindConfig(root)
	if got == "" {
		t.Fatal("expected non-empty path even when config doesn't exist")
	}
	if !strings.HasSuffix(got, "config.toml") {
		t.Fatalf("expected default config.toml, got %s", got)
	}
}

// --- FIX-03: JSONC strict parsing tests ---

func TestLoadJSONCRejectsMultipleDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"quality": {"fixtures": "a"}}
{"quality": {"fixtures": "b"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected multiple documents to be rejected")
	}
	if !strings.Contains(err.Error(), "multiple objects") {
		t.Fatalf("expected 'multiple objects' error, got: %v", err)
	}
}

func TestLoadJSONCRejectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"quality": {"fixtures": "a"}, "quality": {"fixtures": "b"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate keys to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected 'duplicate key' error, got: %v", err)
	}
}

func TestLoadJSONCPreservesEscapeSequences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"quality": {"fixtures": "path\\to\\file"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "path\\to\\file" {
		t.Fatalf("expected escaped path preserved, got %q", cfg.Fixtures)
	}
}

func TestLoadJSONCPreservesCommentInString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"quality": {"fixtures": "value /* not a comment */"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixtures != "value /* not a comment */" {
		t.Fatalf("expected comment-like text in string preserved, got %q", cfg.Fixtures)
	}
}

func TestLoadJSONCErrorContainsLineColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := "{\n\t\"quality\": {\n\t\t\"fixtures\": \n\t}\n}"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	// Error should contain path, line and column
	errStr := err.Error()
	if !strings.Contains(errStr, ".jsonc:") || !strings.Contains(errStr, ":") {
		t.Fatalf("expected file:line:col in error, got: %v", err)
	}
}

func TestLoadJSONCRejectsDuplicateKeysNested(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	data := `{"quality": {"fixtures": "a", "fixtures": "b"}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected nested duplicate keys to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("expected 'duplicate key' error, got: %v", err)
	}
}
