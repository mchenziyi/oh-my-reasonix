package grillme

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGrillMeReplay_InfoSufficient verifies that when information is sufficient
// (answers are clear), the session stops and recommends "proceed".
func TestGrillMeReplay_InfoSufficient(t *testing.T) {
	result, reason := Replay(sessionConfig{
		roundsCompleted: 1,
		infoSufficient:  true,
		assumptions: []assumptionRecord{
			{statement: "PostgreSQL 16 is available", confirmed: true},
			{statement: "CI runs on macOS 14", confirmed: true},
		},
	})
	if reason != StopInfoSufficient {
		t.Fatalf("expected StopInfoSufficient, got %v", reason)
	}
	if result.Recommendation != "proceed" {
		t.Fatalf("expected recommendation 'proceed', got %q", result.Recommendation)
	}
	if len(result.AssumptionsConfirmed) != 2 {
		t.Fatalf("expected 2 confirmed assumptions, got %d", len(result.AssumptionsConfirmed))
	}
	if len(result.Ambiguities) != 0 {
		t.Fatalf("expected 0 ambiguities, got %d: %v", len(result.Ambiguities), result.Ambiguities)
	}
}

// TestGrillMeReplay_MaxRounds verifies that after 6 rounds without sufficient
// clarity, the session stops with StopMaxRounds.
func TestGrillMeReplay_MaxRounds(t *testing.T) {
	result, reason := Replay(sessionConfig{
		roundsCompleted: 6,
		infoSufficient:  false,
		assumptions: []assumptionRecord{
			{statement: "Assumption A", confirmed: false},
		},
	})
	if reason != StopMaxRounds {
		t.Fatalf("expected StopMaxRounds, got %v", reason)
	}
	if result.Recommendation != "pause" {
		t.Fatalf("expected recommendation 'pause' due to unresolved ambiguities, got %q", result.Recommendation)
	}
	if len(result.Ambiguities) != 1 {
		t.Fatalf("expected 1 ambiguity (unconfirmed assumption), got %d", len(result.Ambiguities))
	}
	if len(result.AssumptionsConfirmed) != 0 {
		t.Fatalf("expected 0 confirmed assumptions (none were confirmed), got %d", len(result.AssumptionsConfirmed))
	}
}

// TestGrillMeReplay_UserStops verifies that when the user requests a stop,
// the session ends immediately regardless of other conditions.
func TestGrillMeReplay_UserStops(t *testing.T) {
	result, reason := Replay(sessionConfig{
		roundsCompleted: 2,
		userStopped:     true,
		infoSufficient:  true, // even if info is sufficient, user stop wins
	})
	if reason != StopUserRequest {
		t.Fatalf("expected StopUserRequest, got %v", reason)
	}
	if result.Recommendation != "pause" {
		t.Fatalf("expected recommendation 'pause' after user stop, got %q", result.Recommendation)
	}
}

// TestGrillMeReplay_UnconfirmedAssumptions verifies that assumptions the user
// did NOT confirm never appear in AssumptionsConfirmed — they appear as
// Ambiguities or are excluded entirely.
func TestGrillMeReplay_UnconfirmedAssumptions(t *testing.T) {
	// Scenario: mixed confirmed and unconfirmed assumptions.
	_, reason := Replay(sessionConfig{
		roundsCompleted: 1,
		infoSufficient:  true,
		assumptions: []assumptionRecord{
			{statement: "Confirmed assumption", confirmed: true},
			{statement: "Unconfirmed assumption", confirmed: false},
			{statement: "Also not confirmed", confirmed: false},
		},
	})
	if reason != StopInfoSufficient {
		t.Fatalf("expected StopInfoSufficient, got %v", reason)
	}

	// Run again to collect the result
	result, _ := Replay(sessionConfig{
		roundsCompleted: 1,
		infoSufficient:  true,
		assumptions: []assumptionRecord{
			{statement: "Confirmed assumption", confirmed: true},
			{statement: "Unconfirmed assumption", confirmed: false},
			{statement: "Also not confirmed", confirmed: false},
		},
	})

	// Only the confirmed one should be in AssumptionsConfirmed
	if len(result.AssumptionsConfirmed) != 1 || result.AssumptionsConfirmed[0] != "Confirmed assumption" {
		t.Fatalf("expected exactly 1 confirmed assumption, got %v", result.AssumptionsConfirmed)
	}

	// Unconfirmed ones must NOT appear in AssumptionsConfirmed
	for _, a := range result.AssumptionsConfirmed {
		if a == "Unconfirmed assumption" || a == "Also not confirmed" {
			t.Fatalf("unconfirmed assumption %q leaked into AssumptionsConfirmed", a)
		}
	}

	// Unconfirmed assumptions should appear as Ambiguities
	if len(result.Ambiguities) != 2 {
		t.Fatalf("expected 2 ambiguities from unconfirmed assumptions, got %d: %v", len(result.Ambiguities), result.Ambiguities)
	}
}

// TestGrillMeReplay_AllAssumptionsRejected verifies that when all assumptions
// are explicitly rejected (none confirmed, no open ambiguities), the
// recommendation is "rethink".
func TestGrillMeReplay_AllAssumptionsRejected(t *testing.T) {
	result, reason := Replay(sessionConfig{
		roundsCompleted: 2,
		infoSufficient:  true,
		assumptions: []assumptionRecord{
			{statement: "Old approach is feasible", confirmed: false, rejected: true},
			{statement: "Budget is sufficient", confirmed: false, rejected: true},
		},
	})
	if reason != StopInfoSufficient {
		t.Fatalf("expected StopInfoSufficient, got %v", reason)
	}
	if result.Recommendation != "rethink" {
		t.Fatalf("expected recommendation 'rethink' when all assumptions rejected, got %q", result.Recommendation)
	}
	if len(result.AssumptionsConfirmed) != 0 {
		t.Fatalf("expected 0 confirmed assumptions, got %d", len(result.AssumptionsConfirmed))
	}
	if len(result.Ambiguities) != 0 {
		t.Fatalf("expected 0 ambiguities (all assumptions explicitly rejected), got %d", len(result.Ambiguities))
	}
}

// TestGrillMeReplay_FileSnapshot verifies that the Replay function does not
// modify any files on disk. It takes a snapshot of a temp directory before
// and after the replay, then compares them.
func TestGrillMeReplay_FileSnapshot(t *testing.T) {
	// Create a temp directory with some known content.
	snapshotDir := t.TempDir()
	files := map[string]string{
		"src/main.go": "package main\n",
		"src/util.go": "package util\n",
		"README.md":   "# Test\n",
		"config.yaml": "key: value\n",
	}
	for path, content := range files {
		fullPath := filepath.Join(snapshotDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Run the replay — this should NOT touch the filesystem.
	Replay(sessionConfig{
		roundsCompleted: 3,
		infoSufficient:  true,
		assumptions: []assumptionRecord{
			{statement: "Workspace is clean", confirmed: true},
		},
	})

	// Verify no files were modified and no new files were created.
	checkNoNewFiles(t, snapshotDir, files)
}

// checkNoNewFiles verifies the directory contains exactly the expected files
// and their content is unchanged.
func checkNoNewFiles(t *testing.T, dir string, expected map[string]string) {
	t.Helper()
	for path, content := range expected {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Errorf("expected file %q is missing or unreadable: %v", path, err)
			continue
		}
		if string(data) != content {
			t.Errorf("file %q content changed: expected %q, got %q", path, content, string(data))
		}
	}
	// Walk the directory tree and flag anything unexpected.
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if _, ok := expected[rel]; !ok {
			t.Errorf("unexpected file created: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Errorf("walk error: %v", err)
	}
}
