package main

import (
	"os"
	"path/filepath"
	"testing"
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
