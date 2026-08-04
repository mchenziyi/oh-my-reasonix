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

func TestEvolveReportJSONIncludesProposalStats(t *testing.T) {
	dir, s := evolveTestStore(t)
	// Add an approved proposal with after observations.
	overlay := "active"
	h := sha256.Sum256([]byte(overlay))
	if err := s.SaveProposal(evolution.Proposal{SchemaVersion: 1, ID: "p-active", PatternID: "pattern-a", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "approved", ApprovedAt: evolution.Now(), CreatedAt: evolution.Now(), UpdatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveObservation(evolution.Observation{SchemaVersion: 1, ID: "o-active", ProposalID: "p-active", EpisodeID: "ep-active", Phase: "after", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(evolution.Episode{SchemaVersion: 1, ID: "ep-active", TaskClass: "build", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"report", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SchemaVersion int `json:"schema_version"`
		ProposalStats []struct {
			ProposalID string `json:"proposal_id"`
			Status     string `json:"status"`
		} `json:"proposal_stats"`
		TaskClasses []struct {
			TaskClass string `json:"task_class"`
			Episodes  int    `json:"episodes"`
		} `json:"task_classes"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("schema_version: %+v", report)
	}
	foundActive := false
	for _, ps := range report.ProposalStats {
		if ps.ProposalID == "p-active" {
			foundActive = true
			if ps.Status != "insufficient_evidence" {
				t.Fatalf("expected insufficient_evidence, got %q", ps.Status)
			}
		}
	}
	if !foundActive {
		t.Fatalf("p-active missing from proposal stats: %+v", report.ProposalStats)
	}
	foundTask := false
	for _, tc := range report.TaskClasses {
		if tc.TaskClass == "build" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatal("task_classes missing build")
	}
}

func TestEvolveHistoryJSONDetailedStats(t *testing.T) {
	dir, s := evolveTestStore(t)
	if err := s.SaveObservation(evolution.Observation{SchemaVersion: 1, ID: "o-orphan", ProposalID: "p-rejected", EpisodeID: "ep-missing", Phase: "after", Succeeded: true, CreatedAt: evolution.Now()}); err != nil {
		t.Fatal(err)
	}
	out, err := captureRunOutput(func() error {
		return runEvolve([]string{"history", "p-rejected", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var detail struct {
		ProposalID string `json:"proposal_id"`
		Status     string `json:"status"`
		Before     int    `json:"before"`
		After      int    `json:"after"`
	}
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if detail.ProposalID != "p-rejected" || detail.Before != 0 || detail.After != 2 {
		t.Fatalf("unexpected history: %+v", detail)
	}
	if detail.Status != "insufficient_evidence" {
		t.Fatalf("expected insufficient_evidence, got %q", detail.Status)
	}
}

func TestEvolveHistoryUnknownProposalFails(t *testing.T) {
	dir, _ := evolveTestStore(t)
	_, err := captureRunOutput(func() error {
		return runEvolve([]string{"history", "nope", "--project-dir", dir, "--json"})
	})
	if err == nil {
		t.Fatal("expected unknown proposal error")
	}
}

func TestEvolveExportSignAndImportRequireSignature(t *testing.T) {
	dir, s := evolveTestStore(t)
	// Create a signed key pair as PEM files.
	privPEM, pubPEM := makeTestKeyPairPEM(t, dir)
	pkgPath := filepath.Join(t.TempDir(), "signed.json")

	// Unsigned import without --require-signature succeeds with warning.
	if err := runEvolve([]string{"export", "--output", pkgPath, "--project-dir", dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListProposals(); err != nil {
		t.Fatal(err)
	}

	// Signed export.
	if err := runEvolve([]string{"export", "--sign", "--key", privPEM, "--output", pkgPath, "--project-dir", dir}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		SignatureAlgorithm string `json:"signature_algorithm"`
		Signature          string `json:"signature"`
		OMRVersion         string `json:"omr_version"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.SignatureAlgorithm != "ed25519" || pkg.Signature == "" || pkg.OMRVersion == "" {
		t.Fatalf("signed export missing fields: %+v", pkg)
	}

	// Import into a fresh store with --require-signature --trusted-key.
	targetDir := t.TempDir()
	_, err = captureRunOutput(func() error {
		return runEvolve([]string{"import", "--input", pkgPath, "--require-signature", "--trusted-key", pubPEM, "--project-dir", targetDir})
	})
	if err != nil {
		t.Fatal(err)
	}
	targetStore, _ := evolution.NewStore(targetDir)
	props, _ := targetStore.ListProposals()
	if len(props) != 1 || props[0].Status != "pending" {
		t.Fatalf("import must keep pending: %+v", props)
	}
}

func TestEvolveImportRequireSignatureRejectsUnsigned(t *testing.T) {
	dir, _ := evolveTestStore(t)
	pkgPath := filepath.Join(t.TempDir(), "unsigned.json")
	if err := runEvolve([]string{"export", "--output", pkgPath, "--project-dir", dir}); err != nil {
		t.Fatal(err)
	}
	targetDir := t.TempDir()
	_, err := captureRunOutput(func() error {
		return runEvolve([]string{"import", "--input", pkgPath, "--require-signature", "--project-dir", targetDir})
	})
	if err == nil {
		t.Fatal("expected rejection of unsigned import with --require-signature")
	}
	targetStore, _ := evolution.NewStore(targetDir)
	if props, _ := targetStore.ListProposals(); len(props) != 0 {
		t.Fatal("failed import must not write")
	}
}

func TestEvolveExportSignMissingKeyFails(t *testing.T) {
	dir, _ := evolveTestStore(t)
	err := runEvolve([]string{"export", "--sign", "--output", filepath.Join(t.TempDir(), "x.json"), "--project-dir", dir})
	if err == nil {
		t.Fatal("expected error when --sign lacks --key")
	}
}
