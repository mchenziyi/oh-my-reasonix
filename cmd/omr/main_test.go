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
	promptPath := filepath.Join(root, "prompts", "research.md")
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("research"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".reasonix", "omr", "config.toml"), []byte("[agent.omr-research]\nmodel = \"deepseek-v4-flash\"\nprompt_file = \"prompts/research.md\"\n[agent.omr-project]\nmodel = \"test-model\"\n[routing]\nresearch = \"omr-research\"\n[profiles]\ndisabled = \"omr-debug\"\n"), 0o600); err != nil {
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
		ID               string   `json:"id"`
		Model            string   `json:"model"`
		PromptFileExists *bool    `json:"prompt_file_exists"`
		Categories       []string `json:"categories"`
		Disabled         bool     `json:"disabled"`
		Description      string   `json:"description"`
		ReadOnlyBool     bool     `json:"read_only_bool"`
		AllowedTools     []string `json:"allowed_tools"`
		InputTypes       []string `json:"input_types"`
		OutputSections   []string `json:"output_sections"`
		Source           string   `json:"source"`
		Status           string   `json:"status"`
		PromptShortHash  string   `json:"prompt_short_hash"`
	}
	if err := json.Unmarshal(data, &profiles); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if len(profiles) != 10 || profiles[0].ID != "omr-explore" {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
	// Find project-only profile
	foundProject := false
	for _, p := range profiles {
		if p.Source == "project" {
			foundProject = true
			if p.Status != "missing" {
				t.Fatalf("expected project profile to have status 'missing', got %q", p.Status)
			}
		}
	}
	if !foundProject {
		t.Fatal("expected project-only profile in JSON output")
	}
	if profiles[0].ReadOnlyBool != true {
		t.Fatal("expected omr-explore to be read-only")
	}
	if profiles[1].Model != "deepseek-v4-flash" {
		t.Fatalf("expected configured model: %#v", profiles[1])
	}
	if profiles[1].PromptFileExists == nil || !*profiles[1].PromptFileExists {
		t.Fatalf("expected existing Prompt file marker: %#v", profiles[1])
	}
	if len(profiles[1].Categories) != 1 || profiles[1].Categories[0] != "research" {
		t.Fatalf("expected category mapping: %#v", profiles[1])
	}
	if !profiles[2].Disabled {
		t.Fatalf("expected disabled profile marker: %#v", profiles[2])
	}
	if profiles[0].Source != "builtin" {
		t.Fatalf("expected source=builtin, got: %v", profiles[0].Source)
	}
	if profiles[0].Status != "enabled" {
		t.Fatalf("expected status=enabled for omr-explore, got: %v", profiles[0].Status)
	}
	if profiles[2].Status != "disabled" {
		t.Fatalf("expected status=disabled for omr-debug, got: %v", profiles[2].Status)
	}
	if len(profiles[0].PromptShortHash) != 8 {
		t.Fatalf("expected 8-char short hash, got: %q", profiles[0].PromptShortHash)
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
	if !strings.Contains(output, "omr-frontend") || !strings.Contains(output, "frontend") || !strings.Contains(output, "omr-debug") || !strings.Contains(output, "disabled") {
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
		Configured  bool    `json:"configured"`
		Concurrency int     `json:"concurrency"`
		MaxCost     float64 `json:"max_cost"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Path != path || !result.Valid || !result.Configured || result.Concurrency != 2 || result.MaxCost != 1.5 {
		t.Fatalf("unexpected config result: %#v", result)
	}
}

func TestConfigValidateJSONIncludesMCPDiagnostics(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	data := "[mcp.docs]\ncommand = \"definitely-missing-omr-mcp\"\nenabled = true\nenv = [\"OMR_MISSING_DOCS_KEY\"]\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
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
	dataOut, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		MCP []struct {
			Server        string `json:"server"`
			Availability  string `json:"availability"`
			Compatibility string `json:"compatibility"`
		} `json:"mcp"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(dataOut, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", dataOut, err)
	}
	if len(result.MCP) != 1 || result.MCP[0].Server != "docs" ||
		result.MCP[0].Availability != "unavailable" ||
		result.MCP[0].Compatibility != "compatible" ||
		len(result.Warnings) != 1 {
		t.Fatalf("unexpected MCP diagnostics: %#v", result)
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
	mcp := raw["properties"].(map[string]any)["mcp"].(map[string]any)
	server := mcp["additionalProperties"].(map[string]any)
	if server["additionalProperties"] != false {
		t.Fatalf("MCP schema should reject unknown keys: %#v", server)
	}
	transport := server["properties"].(map[string]any)["transport"].(map[string]any)
	transports := transport["enum"].([]any)
	if len(transports) != 3 || transports[2] != "sse" {
		t.Fatalf("MCP schema missing supported transports: %#v", transport)
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
		Valid      bool     `json:"valid"`
		Configured bool     `json:"configured"`
		Error      string   `json:"error"`
		Errors     []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Valid || !result.Configured || result.Error == "" || len(result.Errors) != 1 || result.Errors[0] != result.Error {
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
		Valid      bool     `json:"valid"`
		Configured bool     `json:"configured"`
		Errors     []string `json:"errors"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s", data)
	}
	if result.Valid || !result.Configured || len(result.Errors) != 2 {
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

func TestConfigValidateMissingConfigSucceeds(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	err := runConfig([]string{"validate", "--config", path})
	if err != nil {
		t.Fatalf("expected success for missing config, got: %v", err)
	}
}

func TestConfigValidateMissingConfigJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
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
		t.Fatalf("expected success for missing config JSON, got: %v", runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Path       string `json:"path"`
		Valid      bool   `json:"valid"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Path != path || !result.Valid || result.Configured {
		t.Fatalf("unexpected missing config result: %#v", result)
	}
}

func TestConfigValidateEmptyConfigSucceeds(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
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
		t.Fatalf("expected success for empty config, got: %v", runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Path       string `json:"path"`
		Valid      bool   `json:"valid"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if !result.Valid || !result.Configured {
		t.Fatalf("unexpected empty config result: %#v", result)
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

func TestHookDoctorJSON(t *testing.T) {
	dir := t.TempDir()
	mockBin := makeMockReasonixBinary(t, dir)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		writer.Close()
	}()
	runErr := runHook([]string{"doctor", "--binary", mockBin, "--project-dir", dir, "--json"})
	writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		List struct {
			Hooks         []any `json:"hooks"`
			SchemaVersion int   `json:"schema_version"`
		} `json:"list"`
		Status struct {
			SchemaVersion  int  `json:"schema_version"`
			TrustedProject bool `json:"trusted_project"`
			Sources        []struct {
				Scope     string `json:"scope"`
				HookCount int    `json:"hook_count"`
			} `json:"sources"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.List.SchemaVersion != 1 {
		t.Fatalf("expected list schema_version=1, got: %s", data)
	}
	if result.Status.SchemaVersion != 1 {
		t.Fatalf("expected status schema_version=1, got: %s", data)
	}
	if !result.Status.TrustedProject || len(result.Status.Sources) != 1 || result.Status.Sources[0].HookCount != 1 {
		t.Fatalf("expected trusted project hook source, got: %s", data)
	}
}

func TestHookDoctorHumanOutput(t *testing.T) {
	dir := t.TempDir()
	mockBin := makeMockReasonixBinary(t, dir)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		writer.Close()
	}()
	runErr := runHook([]string{"doctor", "--binary", mockBin, "--project-dir", dir})
	writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "EVENT") || !strings.Contains(output, "STATUS") {
		t.Fatalf("expected human table header EVENT/STATUS, got: %s", output)
	}
	if !strings.Contains(output, "trusted_project=true") {
		t.Fatalf("expected trusted project status, got: %s", output)
	}
	if !strings.Contains(output, "PreToolUse") || !strings.Contains(output, "Bash") {
		t.Fatalf("expected hook event and matcher in table, got: %s", output)
	}
}

func TestHookDoctorJSONParsesWithHomeDir(t *testing.T) {
	dir := t.TempDir()
	mockBin := makeMockReasonixBinary(t, dir)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		writer.Close()
	}()
	runErr := runHook([]string{"doctor", "--binary", mockBin, "--project-dir", dir, "--home-dir", "/tmp/test-home", "--json"})
	writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"list"`) || !strings.Contains(string(data), `"status"`) {
		t.Fatalf("expected JSON with list/status keys, got: %s", data)
	}
}

// --- Comment Checker CLI tests ---

func TestCommentCheckHumanOutput(t *testing.T) {
	dir := t.TempDir()
	// Use a credential leak (R004, blocking) for a clear blocking finding.
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte("# password = \"hunter2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	runErr := runCommentCheck([]string{"--project-dir", dir})
	writer.Close()
	os.Stdout = original
	if runErr == nil {
		t.Fatal("expected blocking finding error")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "R004") || !strings.Contains(output, "password") {
		t.Fatalf("expected human output with R004/password, got: %s", output)
	}
	if !strings.Contains(output, "blocking") {
		t.Fatalf("expected blocking status, got: %s", output)
	}
}

func TestCommentCheckJSONOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte("# password = \"hunter2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	runErr := runCommentCheck([]string{"--project-dir", dir, "--json"})
	writer.Close()
	os.Stdout = original
	if runErr == nil {
		t.Fatal("expected blocking finding error")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion int `json:"schema_version"`
		Findings      []struct {
			RuleID   string `json:"rule_id"`
			Severity string `json:"severity"`
		} `json:"findings"`
		BlockingCount int `json:"blocking_count"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON report: %s: %v", data, err)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1, got %d", report.SchemaVersion)
	}
	if report.BlockingCount <= 0 {
		t.Fatalf("expected blocking_count > 0, got %d", report.BlockingCount)
	}
}

func TestCommentCheckWithPathFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// TODO: implement\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanFile := filepath.Join(dir, "clean.go")
	if err := os.WriteFile(cleanFile, []byte("package main\n// clean comment\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCommentCheck([]string{"--project-dir", dir, "--path", cleanFile})
	if err != nil {
		t.Fatalf("expected no error for clean file with --path, got: %v", err)
	}
}

func TestCommentCheckAllowTags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// TODO(admin): add auth later\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCommentCheck([]string{"--project-dir", dir, "--allow-tags", "TODO(admin)"})
	if err != nil {
		t.Fatalf("expected no blocking error with allowed tag, got: %v", err)
	}
}

func TestCommentCheckMaxFileSize(t *testing.T) {
	dir := t.TempDir()
	largeContent := strings.Repeat("// large comment\n", 1000)
	if err := os.WriteFile(filepath.Join(dir, "large.go"), []byte(largeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCommentCheck([]string{"--project-dir", dir, "--max-file-size", "1"})
	if err != nil {
		t.Fatalf("expected no error when all files skipped, got: %v", err)
	}
}

func TestCommentCheckBlockingExitCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte("# password = \"hunter2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCommentCheck([]string{"--project-dir", dir})
	if err == nil {
		t.Fatal("expected blocking error for credential leak")
	}
	if !strings.Contains(err.Error(), "blocking") {
		t.Fatalf("expected blocking in error message, got: %v", err)
	}
}

func TestCommentCheckCleanServicePass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// add returns the sum of a and b.\nfunc add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCommentCheck([]string{"--project-dir", dir})
	if err != nil {
		t.Fatalf("expected no error for clean comments, got: %v", err)
	}
}

func TestCommentCheckNoHelpOnSuccess(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	runErr := runCommentCheck([]string{"--project-dir", dir})
	writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatalf("expected pass, got: %v", runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "PASS") {
		t.Fatalf("expected success output, got: %s", output)
	}
}

// --- CLI path safety tests ---

func TestCommentCheckDefaultRootRejectsOutside(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(dir, "..", "outside.go")
	if err := os.WriteFile(outsideFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outsideFile)

	err := runCommentCheck([]string{"--project-dir", dir, "--path", outsideFile})
	if err == nil {
		t.Fatal("expected path safety error for outside file with default root")
	}
	if !strings.Contains(err.Error(), "path not allowed") && !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected path safety error message, got: %v", err)
	}
}

func TestCommentCheckPathResolvesRelativeToProjectDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// clean\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When --path is relative, it should be resolved against --project-dir,
	// not the shell cwd.  Run from an empty temp dir to prove the point.
	cwd := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	err = runCommentCheck([]string{"--project-dir", dir, "--path", "main.go"})
	if err != nil {
		t.Fatalf("expected relative --path resolved against --project-dir, got: %v", err)
	}
}

func TestCommentCheckPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	err := runCommentCheck([]string{"--project-dir", dir, "--path", "../outside.go"})
	if err == nil {
		t.Fatal("expected path safety error for ../ traversal")
	}
}

func TestCommentCheckAllowedRootsExplicit(t *testing.T) {
	dir := t.TempDir()
	allowedDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(allowedDir, "main.go"), []byte("package main\n// clean\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(dir, "outside.go")
	if err := os.WriteFile(outsideFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// File outside allowed-roots should be rejected even though it's inside --project-dir.
	err := runCommentCheck([]string{"--project-dir", dir, "--allowed-roots", allowedDir, "--path", outsideFile})
	if err == nil {
		t.Fatal("expected path safety error for file outside explicit allowed-roots")
	}
}

func TestCommentCheckJSONErrorForBlockedPath(t *testing.T) {
	dir := t.TempDir()
	runErr := runCommentCheck([]string{"--project-dir", dir, "--path", "../outside.go", "--json"})
	if runErr == nil {
		t.Fatal("expected path safety error")
	}
	if !strings.Contains(runErr.Error(), "path not allowed") && !strings.Contains(runErr.Error(), "outside") {
		t.Fatalf("expected descriptive error, got: %v", runErr)
	}
}

func TestCompatibleReasonixVersion(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{output: "reasonix v1.17.20", want: true},
		{output: "reasonix v1.18.0", want: true},
		{output: "reasonix v2.0.0", want: true},
		{output: "reasonix v1.17.19", want: false},
		{output: "reasonix v1.16.99", want: false},
		{output: "not-a-version", want: false},
	}
	for _, tt := range tests {
		if got := compatibleReasonixVersion(tt.output); got != tt.want {
			t.Errorf("compatibleReasonixVersion(%q) = %v, want %v", tt.output, got, tt.want)
		}
	}
}

func TestHookDoctorProjectDir(t *testing.T) {
	dir := t.TempDir()
	mockBin := makeMockReasonixBinary(t, dir)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = original
		writer.Close()
	}()
	runErr := runHook([]string{"doctor", "--binary", mockBin, "--project-dir", dir, "--json"})
	writer.Close()
	os.Stdout = original
	if runErr != nil {
		t.Fatal(runErr)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"list"`) || !strings.Contains(string(data), `"status"`) {
		t.Fatalf("expected JSON with list/status keys, got: %s", data)
	}
}
