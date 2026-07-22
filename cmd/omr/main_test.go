package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
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
	if err := os.WriteFile(filepath.Join(root, ".reasonix", "omr", "config.toml"), []byte("[agent.omr-research]\nmodel = \"deepseek-v4-flash\"\n"), 0o600); err != nil {
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
		ID    string `json:"id"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &profiles); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if len(profiles) != 3 || profiles[0].ID != "omr-explore" {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
	if profiles[1].Model != "deepseek-v4-flash" {
		t.Fatalf("expected configured model: %#v", profiles[1])
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
	if err := os.WriteFile(path, []byte("[agent.omr-debug]\nread_only = true\n"), 0o600); err != nil {
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
		Path  string `json:"path"`
		Valid bool   `json:"valid"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Path != path || !result.Valid {
		t.Fatalf("unexpected config result: %#v", result)
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
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %s: %v", data, err)
	}
	if result.Valid || result.Error == "" {
		t.Fatalf("unexpected invalid config result: %#v", result)
	}
}

func TestSessionRequiresResume(t *testing.T) {
	if err := runSession(nil); err == nil {
		t.Fatal("expected session resume requirement")
	}
}

func TestSessionResumeRejectsMissingBinary(t *testing.T) {
	if err := runSession([]string{"resume", "--project-dir", t.TempDir(), "--binary", "missing-reasonix"}); err == nil {
		t.Fatal("expected missing Reasonix binary error")
	}
}
