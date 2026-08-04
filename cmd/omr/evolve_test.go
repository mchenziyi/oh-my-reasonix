package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/evolution"
)

// captureRunOutput runs fn while capturing everything written to os.Stdout.
func captureRunOutput(fn func() error) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	original := os.Stdout
	os.Stdout = writer
	runErr := fn()
	_ = writer.Close()
	os.Stdout = original
	if runErr != nil {
		return "", runErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func evolveTestStore(t *testing.T) (string, evolution.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := evolution.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	overlay := "rule"
	h := sha256.Sum256([]byte(overlay))
	// One terminal proposal with linked evidence.
	if err := s.SaveProposal(evolution.Proposal{SchemaVersion: 1, ID: "p-rejected", PatternID: "pattern-x", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "rejected", CreatedAt: evolution.Now(), UpdatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(evolution.Episode{SchemaVersion: 1, ID: "ep1", TaskClass: "build", Succeeded: true, CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveObservation(evolution.Observation{SchemaVersion: 1, ID: "o1", ProposalID: "p-rejected", EpisodeID: "ep1", Phase: "after", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	return dir, s
}

func TestEvolveDoctorJSONReportsStats(t *testing.T) {
	dir, _ := evolveTestStore(t)
	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"doctor", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var stats struct {
		SchemaVersion int `json:"schema_version"`
		Collections   []struct {
			Name    string   `json:"name"`
			Files   int      `json:"files"`
			Damaged []string `json:"damaged"`
		} `json:"collections"`
	}
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if stats.SchemaVersion != 1 {
		t.Fatalf("schema_version: %+v", stats)
	}
	found := map[string]int{}
	for _, c := range stats.Collections {
		found[c.Name] = c.Files
	}
	if found["episodes"] != 1 || found["observations"] != 1 || found["proposals"] != 1 {
		t.Fatalf("unexpected collection counts: %v", found)
	}
}

func TestEvolveDoctorHumanOutput(t *testing.T) {
	dir, _ := evolveTestStore(t)
	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"doctor", "--project-dir", dir})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Evolution store: PASS") {
		t.Fatalf("unexpected doctor output: %s", out)
	}
}

func TestEvolvePruneDryRunAndExecute(t *testing.T) {
	dir, s := evolveTestStore(t)

	// Dry-run: zero writes.
	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"prune", "--dry-run", "--keep-episodes", "0", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		DryRun              bool   `json:"dry_run"`
		EpisodesRemoved     int    `json:"episodes_removed"`
		ObservationsRemoved int    `json:"observations_removed"`
		Snapshot            string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if !preview.DryRun || preview.EpisodesRemoved != 1 || preview.ObservationsRemoved != 1 || preview.Snapshot != "" {
		t.Fatalf("unexpected dry-run: %+v", preview)
	}
	episodes, _ := s.ListEpisodes()
	if len(episodes) != 1 {
		t.Fatal("dry-run must not write")
	}

	// Execute: evidence removed and snapshot recorded.
	out2, err := captureRunOutput(func() error {
		return runEvolve([]string{"prune", "--keep-episodes", "0", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var executed struct {
		DryRun              bool   `json:"dry_run"`
		EpisodesRemoved     int    `json:"episodes_removed"`
		ObservationsRemoved int    `json:"observations_removed"`
		Snapshot            string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(out2), &executed); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out2)
	}
	if executed.DryRun || executed.EpisodesRemoved != 1 || executed.Snapshot == "" {
		t.Fatalf("unexpected execution: %+v", executed)
	}
	episodes, _ = s.ListEpisodes()
	if len(episodes) != 0 {
		t.Fatal("episode must be pruned")
	}
}

func TestEvolvePruneKeepsActiveProposalEvidence(t *testing.T) {
	dir, s := evolveTestStore(t)
	overlay := "active"
	h := sha256.Sum256([]byte(overlay))
	if err := s.SaveProposal(evolution.Proposal{SchemaVersion: 1, ID: "p-active", PatternID: "pattern-a", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "approved", CreatedAt: evolution.Now(), UpdatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveObservation(evolution.Observation{SchemaVersion: 1, ID: "o-active", ProposalID: "p-active", EpisodeID: "ep-active", Phase: "after", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(evolution.Episode{SchemaVersion: 1, ID: "ep-active", TaskClass: "build", Succeeded: true, CreatedAt: "2026-06-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	_, err := captureRunOutput(func() error {
		return runEvolve([]string{"prune", "--keep-episodes", "0", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	episodes, _ := s.ListEpisodes()
	if len(episodes) != 1 || episodes[0].ID != "ep-active" {
		t.Fatalf("active episode must survive: %+v", episodes)
	}
	obs, _ := s.ListObservations()
	if len(obs) != 1 || obs[0].ID != "o-active" {
		t.Fatalf("active observation must survive: %+v", obs)
	}
}

func TestEvolveRepairDryRunAndExecute(t *testing.T) {
	dir, s := evolveTestStore(t)
	// Orphan observation referencing a missing episode.
	if err := s.SaveObservation(evolution.Observation{SchemaVersion: 1, ID: "o-orphan", ProposalID: "p-rejected", EpisodeID: "ep-missing", Phase: "after", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"repair", "--dry-run", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		DryRun       bool   `json:"dry_run"`
		FilesRemoved int    `json:"files_removed"`
		Snapshot     string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if !preview.DryRun || preview.FilesRemoved != 1 || preview.Snapshot != "" {
		t.Fatalf("unexpected dry-run: %+v", preview)
	}

	out2, err := captureRunOutput(func() error {
		return runEvolve([]string{"repair", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var executed struct {
		DryRun       bool   `json:"dry_run"`
		FilesRemoved int    `json:"files_removed"`
		Snapshot     string `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(out2), &executed); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out2)
	}
	if executed.DryRun || executed.FilesRemoved != 1 || executed.Snapshot == "" {
		t.Fatalf("unexpected execution: %+v", executed)
	}
	obs, _ := s.ListObservations()
	if len(obs) != 1 || obs[0].ID != "o1" {
		t.Fatalf("orphan must be removed, keep o1: %+v", obs)
	}
}

func TestEvolveRepairFailsClosedOnCorruptJSON(t *testing.T) {
	dir, _ := evolveTestStore(t)
	epDir := filepath.Join(dir, ".reasonix", "omr", "evolution", "episodes")
	if err := os.MkdirAll(epDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epDir, "broken.json"), []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := captureRunOutput(func() error {
		return runEvolve([]string{"repair", "--project-dir", dir, "--json"})
	})
	if err == nil {
		t.Fatal("expected corrupt JSON to fail closed")
	}
	if _, err := os.Stat(filepath.Join(epDir, "broken.json")); err != nil {
		t.Fatal("corrupt file must not be touched")
	}
}

func TestEvolveRestoreSnapshotCommandPath(t *testing.T) {
	_, s := evolveTestStore(t)
	result, err := s.Prune(evolution.PruneOptions{KeepEpisodes: 0, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if episodes, _ := s.ListEpisodes(); len(episodes) != 0 {
		t.Fatal("expected prune")
	}
	restored, err := s.RestoreSnapshot(result.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 2 {
		t.Fatalf("expected 2 restored, got %d", restored)
	}
	if episodes, _ := s.ListEpisodes(); len(episodes) != 1 {
		t.Fatal("expected restore")
	}
}
