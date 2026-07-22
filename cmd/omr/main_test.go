package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/qualitybench"
)

func TestQualityBenchmarkConfigPathsAreProjectRelative(t *testing.T) {
	projectDir := t.TempDir()
	otherDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatal(err)
		}
	})

	configDir := filepath.Join(projectDir, ".reasonix", "omr")
	fixtureDir := filepath.Join(projectDir, "qfixtures", "smoke")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[quality]\nfixtures = \"qfixtures\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "fixture.yaml")
	fixture := `{"id":"smoke","task":"task","replay":{"hidden_tests_passed":true,"regression_passed":true,"required_effects_met":true}}`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	err = runQualityBenchmark([]string{"--project-dir", projectDir, "--replay", "--min-qualified-rate", "1"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProfileListReadsInstalledProfiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := install.Assets{
		Root:         "test-assets",
		BasePrompt:   []byte("base\n"),
		Orchestrator: []byte("orchestrator\n"),
		Explore:      []byte("explore\n"),
		Research:     []byte("research\n"),
		Debug:        []byte("debug\n"),
		ReviewBrief:  []byte("review\n"),
	}
	if _, err := install.Init(install.Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	if err := runProfile([]string{"list", "--project-dir", root}); err != nil {
		t.Fatal(err)
	}
}

func TestProfileListJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, err := loadAssetsFromInvocation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Init(install.Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".reasonix", "omr", "config.toml"), []byte("[agent.omr-research]\nmodel = \"deepseek-v4-flash\"\n[routing]\nresearch = \"omr-research\"\n[profiles]\ndisabled = \"omr-debug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runProfile([]string{"list", "--project-dir", root, "--json"})
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var profiles []struct {
		ID         string   `json:"id"`
		Model      string   `json:"model"`
		Categories []string `json:"categories"`
		Disabled   bool     `json:"disabled"`
	}
	if err := json.Unmarshal(data, &profiles); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if len(profiles) != 5 || profiles[0].ID != "omr-explore" {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
	if profiles[1].Model != "deepseek-v4-flash" {
		t.Fatalf("expected configured model: %#v", profiles[1])
	}
	if len(profiles[1].Categories) != 1 || profiles[1].Categories[0] != "research" {
		t.Fatalf("expected category mapping: %#v", profiles[1])
	}
	if !profiles[2].Disabled {
		t.Fatalf("expected disabled profile marker: %#v", profiles[2])
	}
}

func TestProfileListHumanShowsRoutingState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, err := loadAssetsFromInvocation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Init(install.Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".reasonix", "omr", "config.toml"), []byte("[routing]\nfrontend = \"omr-frontend\"\n[profiles]\ndisabled = \"omr-debug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runProfile([]string{"list", "--project-dir", root})
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "omr-frontend") || !strings.Contains(output, "categories=frontend") || !strings.Contains(output, "omr-debug") || !strings.Contains(output, "disabled") {
		t.Fatalf("profile list missing routing state: %q", output)
	}
}

func TestDoctorJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, err := loadAssetsFromInvocation()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Init(install.Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runDoctor([]string{"--project-dir", root, "--json"})
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Checks []struct {
			Name string `json:"name"`
		} `json:"checks"`
		Warnings []string `json:"warnings"`
		Errors   []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if len(result.Checks) == 0 {
		t.Fatalf("expected doctor checks in JSON: %s", data)
	}
	if result.Warnings == nil || result.Errors == nil {
		t.Fatalf("expected JSON arrays for warnings/errors: %s", data)
	}
}

func TestConfigValidateJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-debug]\nread_only = true\n[quality]\nmax_cost = 1.5\n[runtime]\nconcurrency = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runConfig([]string{"validate", "--config", path, "--json"})
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Path        string  `json:"path"`
		Valid       bool    `json:"valid"`
		Concurrency int     `json:"concurrency"`
		MaxCost     float64 `json:"max_cost"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Path != path || !result.Valid || result.Concurrency != 2 || result.MaxCost != 1.5 {
		t.Fatalf("unexpected config result: %#v", result)
	}
}

func TestConfigSchema(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runConfig([]string{"schema"})
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Schema     string `json:"$schema"`
		Type       string `json:"type"`
		Properties map[string]struct {
			AdditionalProperties any `json:"additionalProperties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil || schema.Schema == "" || schema.Type != "object" {
		t.Fatalf("invalid config schema: %s, err=%v", data, err)
	}
	if _, ok := schema.Properties["agent"]; !ok {
		t.Fatalf("schema missing agent properties: %s", data)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	quality := raw["properties"].(map[string]any)["quality"].(map[string]any)
	if quality["additionalProperties"] != false {
		t.Fatalf("quality schema should reject unknown keys: %#v", quality)
	}
	if raw["additionalProperties"] != false {
		t.Fatalf("root schema should reject unknown sections: %#v", raw)
	}
}

func TestConfigValidateJSONReportsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("[unsupported]\nvalue = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runConfig([]string{"validate", "--config", path, "--json"})
	_ = writer.Close()
	os.Stdout = original
	if runErr == nil {
		t.Fatal("expected invalid config error")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Valid  bool     `json:"valid"`
		Error  string   `json:"error"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Valid || result.Error == "" || len(result.Errors) != 1 || result.Errors[0] != result.Error {
		t.Fatalf("unexpected invalid config result: %#v", result)
	}
}

func TestConfigValidateRejectsDisabledRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[routing]\nz = \"omr-debug\"\na = \"omr-explore\"\n[profiles]\ndisabled = \"omr-explore, omr-debug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runConfig([]string{"validate", "--config", path}); err == nil || !strings.Contains(err.Error(), "category \"a\"") || !strings.Contains(err.Error(), "category \"z\"") {
		t.Fatal("expected disabled routing validation error")
	}
}

func TestConfigValidateJSONReportsAllDisabledRoutingErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[routing]\nz = \"omr-debug\"\na = \"omr-explore\"\n[profiles]\ndisabled = \"omr-explore, omr-debug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := runConfig([]string{"validate", "--config", path, "--json"})
	_ = writer.Close()
	os.Stdout = original
	if runErr == nil {
		t.Fatal("expected disabled routing validation error")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s", data)
	}
	if result.Valid || len(result.Errors) != 2 {
		t.Fatalf("unexpected errors: %#v", result)
	}
}

func TestConfigValidateRejectsMissingPromptFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-research]\nprompt_file = \"prompts/missing.md\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runConfig([]string{"validate", "--project-dir", root, "--config", path}); err == nil || !strings.Contains(err.Error(), "omr-research") {
		t.Fatalf("expected missing Prompt file error, got %v", err)
	}
}

func TestSessionRequiresResume(t *testing.T) {
	if err := runSession(nil); err == nil {
		t.Fatal("expected session subcommand requirement")
	}
}

func TestQualityGatesApplyCostBudget(t *testing.T) {
	report := qualitybench.Report{FixtureCount: 1, EvaluatedCount: 1, QualifiedCount: 1, QualifiedRate: 1, Metrics: qualitybench.Metrics{Cost: 1.2}}
	if err := checkQualityGates(report, 1, 1); err == nil {
		t.Fatal("expected cost budget failure")
	}
}

func TestSessionExportRequiresSession(t *testing.T) {
	if err := runSession([]string{"export", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("expected session export branch requirement")
	}
}

func TestSessionExportAcceptsFlagsBeforeSession(t *testing.T) {
	if err := runSession([]string{"export", "--project-dir", t.TempDir(), "--binary", "missing-reasonix", "branch-1"}); err == nil {
		t.Fatal("expected missing Reasonix binary error after parsing flags")
	}
}

func TestSessionResumeRejectsMissingBinary(t *testing.T) {
	if err := runSession([]string{"resume", "--project-dir", t.TempDir(), "--binary", "missing-reasonix"}); err == nil {
		t.Fatal("expected missing Reasonix binary error")
	}
}
