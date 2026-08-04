package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const profileFixtureContent = `{"schema_version":1,"id":"planner-executable","profile":"omr-planner","task":"t","acceptance":["a1","a2"],"replay":{"acceptance_met":[true,true],"evidence_covered":[true,true]}}`

func writeProfileFixture(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(path, []byte(profileFixtureContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBenchmarkProfileReplayJSON(t *testing.T) {
	root := t.TempDir()
	writeProfileFixture(t, root, "planner-executable")

	out, err := captureRunOutput(func() error {
		return runProfileBenchmark([]string{"--profile", "omr-planner", "--replay", "--fixtures", root, "--run-id", "run-1"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion  int     `json:"schema_version"`
		RunID          string  `json:"run_id"`
		Profile        string  `json:"profile"`
		FixtureCount   int     `json:"fixture_count"`
		EvaluatedCount int     `json:"evaluated_count"`
		QualifiedRate  float64 `json:"qualified_rate"`
		Claim          string  `json:"claim"`
		Results        []struct {
			AcceptancePassRate   float64 `json:"acceptance_pass_rate"`
			EvidenceCompleteRate float64 `json:"evidence_complete_rate"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if report.SchemaVersion != 1 || report.Profile != "omr-planner" || report.FixtureCount != 1 || report.EvaluatedCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.QualifiedRate != 1 || len(report.Results) != 1 {
		t.Fatalf("unexpected results: %+v", report)
	}
	if report.Results[0].AcceptancePassRate != 1 {
		t.Fatalf("unexpected acceptance rate: %+v", report.Results[0])
	}
	if !strings.Contains(report.Claim, "not a model quality proof") {
		t.Fatalf("claim missing: %q", report.Claim)
	}
}

func TestBenchmarkProfileMatrix(t *testing.T) {
	root := t.TempDir()
	writeProfileFixture(t, root, "planner-executable")

	out, err := captureRunOutput(func() error {
		return runProfileBenchmark([]string{"--matrix", "--replay", "--fixtures", root, "--run-id", "run-m"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Matrix  bool   `json:"matrix"`
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if !report.Matrix || report.Profile != "" {
		t.Fatalf("unexpected matrix report: %+v", report)
	}
}

func TestBenchmarkProfileRequiresReplay(t *testing.T) {
	root := t.TempDir()
	writeProfileFixture(t, root, "planner-executable")
	_, err := captureRunOutput(func() error {
		return runProfileBenchmark([]string{"--fixtures", root})
	})
	if err == nil {
		t.Fatal("expected --replay requirement")
	}
}

func TestBenchmarkProfileMatrixProfileConflict(t *testing.T) {
	root := t.TempDir()
	writeProfileFixture(t, root, "planner-executable")
	_, err := captureRunOutput(func() error {
		return runProfileBenchmark([]string{"--matrix", "--profile", "omr-planner", "--replay", "--fixtures", root})
	})
	if err == nil {
		t.Fatal("expected matrix+profile conflict")
	}
}

func TestBenchmarkProfileUsesRealFixtures(t *testing.T) {
	// The repository ships profile-quality fixtures; discover must find six.
	root := filepath.Join("..", "..", "benchmarks", "profile-quality")
	out, err := captureRunOutput(func() error {
		return runProfileBenchmark([]string{"--matrix", "--replay", "--fixtures", root, "--run-id", "run-real"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		FixtureCount int `json:"fixture_count"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if report.FixtureCount != 6 {
		t.Fatalf("expected 6 profile-quality fixtures, got %d", report.FixtureCount)
	}
}
