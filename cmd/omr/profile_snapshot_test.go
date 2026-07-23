package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
)

func TestProfileListHumanAndJSONConsistent(t *testing.T) {
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

	// Capture human output
	humanReader, humanWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = humanWriter
	humanErr := runProfile([]string{"list", "--project-dir", root})
	_ = humanWriter.Close()
	os.Stdout = original
	if humanErr != nil {
		t.Fatal(humanErr)
	}
	humanData, _ := io.ReadAll(humanReader)
	humanOut := string(humanData)

	// Capture JSON output
	jsonReader, jsonWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = jsonWriter
	jsonErr := runProfile([]string{"list", "--project-dir", root, "--json"})
	_ = jsonWriter.Close()
	os.Stdout = original
	if jsonErr != nil {
		t.Fatal(jsonErr)
	}
	jsonData, _ := io.ReadAll(jsonReader)

	var profiles []struct {
		ID       string `json:"id"`
		Disabled bool   `json:"disabled"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(jsonData, &profiles); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check consistency: disabled in JSON → "disabled" in human
	for _, p := range profiles {
		if p.Disabled && !strings.Contains(humanOut, p.ID) {
			t.Fatalf("profile %q in JSON but missing from human output", p.ID)
		}
		if p.Status == "disabled" && !strings.Contains(humanOut, "disabled") {
			t.Fatalf("disabled profile %q not shown as disabled in human output", p.ID)
		}
	}
}

func TestProfileListJSONSnapshot(t *testing.T) {
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

	var profiles []map[string]interface{}
	if err := json.Unmarshal(data, &profiles); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(profiles) != 7 {
		t.Fatalf("expected 7 profiles, got %d", len(profiles))
	}
	for _, p := range profiles {
		if _, ok := p["id"]; !ok {
			t.Fatal("each profile must have 'id' field")
		}
		if _, ok := p["source"]; !ok {
			t.Fatal("each profile must have 'source' field")
		}
		if _, ok := p["status"]; !ok {
			t.Fatal("each profile must have 'status' field")
		}
		if _, ok := p["prompt_short_hash"]; !ok {
			t.Fatal("each profile must have 'prompt_short_hash' field")
		}
	}

	disabled := 0
	enabled := 0
	for _, p := range profiles {
		status, _ := p["status"].(string)
		if status == "disabled" {
			disabled++
		} else if status == "enabled" {
			enabled++
		}
	}
	if disabled != 1 {
		t.Fatalf("expected 1 disabled profile, got %d", disabled)
	}
	if enabled != 6 {
		t.Fatalf("expected 4 enabled profiles, got %d", enabled)
	}
}
