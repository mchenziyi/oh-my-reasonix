package evolution

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- helpers ---------------------------------------------------------------

func mustProposal(t *testing.T, store Store, id, status string, patternID string) {
	t.Helper()
	overlay := "rule for " + id
	h := sha256.Sum256([]byte(overlay))
	p := Proposal{SchemaVersion: SchemaVersion, ID: id, PatternID: patternID, Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: status, CreatedAt: Now(), UpdatedAt: Now()}
	if err := store.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
}

func mustEpisode(t *testing.T, store Store, id, at string) {
	t.Helper()
	e := Episode{SchemaVersion: SchemaVersion, ID: id, TaskClass: "build", Succeeded: true, CreatedAt: at}
	if err := store.RecordEpisode(e); err != nil {
		t.Fatal(err)
	}
}

func mustObservation(t *testing.T, store Store, id, proposalID, episodeID, phase string) {
	t.Helper()
	o := Observation{SchemaVersion: SchemaVersion, ID: id, ProposalID: proposalID, EpisodeID: episodeID, Phase: phase, Succeeded: true, CreatedAt: Now()}
	if err := store.SaveObservation(o); err != nil {
		t.Fatal(err)
	}
}

func storeFiles(t *testing.T, store Store, dir string) []string {
	t.Helper()
	p, err := store.safeJoin(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// --- Stats -----------------------------------------------------------------

func TestStoreStatsCountsCollections(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustEpisode(t, s, "ep-old", "2026-01-01T00:00:00Z")
	mustEpisode(t, s, "ep-new", "2026-02-02T00:00:00Z")
	mustObservation(t, s, "o1", "p1", "ep-old", "before")
	mustProposal(t, s, "p1", "pending", "pattern-x")

	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.SchemaVersion != SchemaVersion || stats.ScopeID != s.ScopeID {
		t.Fatalf("unexpected stats header: %+v", stats)
	}
	byName := map[string]CollectionStats{}
	for _, c := range stats.Collections {
		byName[c.Name] = c
	}
	ep := byName["episodes"]
	if ep.Files != 2 || ep.Bytes <= 0 {
		t.Fatalf("episodes stats: %+v", ep)
	}
	if ep.EarliestTime != "2026-01-01T00:00:00Z" || ep.LatestTime != "2026-02-02T00:00:00Z" {
		t.Fatalf("episodes time range: %+v", ep)
	}
	if byName["observations"].Files != 1 || byName["proposals"].Files != 1 {
		t.Fatalf("collection counts: %+v", byName)
	}
	if byName["patterns"].Files != 0 || byName["experiments"].Files != 0 {
		t.Fatalf("empty collections must be reported: %+v", byName)
	}
}

func TestStoreStatsReportsDamagedFile(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	dir, err := s.safeJoin("episodes")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range stats.Collections {
		if c.Name == "episodes" && len(c.Damaged) == 0 {
			t.Fatal("expected damaged file reported")
		}
	}
}

// --- Prune -----------------------------------------------------------------

func TestPruneDryRunWritesNothing(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p-rejected", "rejected", "pattern-x")
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	mustObservation(t, s, "o1", "p-rejected", "ep1", "after")

	result, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatal("expected dry-run flag")
	}
	before := storeFiles(t, s, "episodes")
	if len(before) != 1 || len(storeFiles(t, s, "observations")) != 1 {
		t.Fatal("dry-run must not write")
	}
	if result.EpisodesRemoved != 1 || result.ObservationsRemoved != 1 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
}

func TestPruneRemovesOnlyTerminalProposalEvidence(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	// Terminal proposal with old evidence.
	mustProposal(t, s, "p-rejected", "rejected", "pattern-rejected")
	mustObservation(t, s, "o-rejected", "p-rejected", "ep-rejected", "after")
	// Active proposal must be untouched.
	mustProposal(t, s, "p-pending", "pending", "pattern-active")
	mustObservation(t, s, "o-active", "p-pending", "ep-active", "after")
	// The episodes themselves: rejected-linked ep is old, active ep is new.
	if err := s.RecordEpisode(Episode{SchemaVersion: SchemaVersion, ID: "ep-rejected", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(Episode{SchemaVersion: SchemaVersion, ID: "ep-active", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", CreatedAt: "2026-06-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	result, err := s.Prune(PruneOptions{KeepEpisodes: 1, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.EpisodesRemoved != 1 || result.ObservationsRemoved != 1 {
		t.Fatalf("unexpected prune result: %+v", result)
	}
	if got := storeFiles(t, s, "episodes"); len(got) != 1 || got[0] != "ep-active.json" {
		t.Fatalf("active episode must be kept, got %v", got)
	}
	if got := storeFiles(t, s, "observations"); len(got) != 1 || got[0] != "o-active.json" {
		t.Fatalf("active observation must be kept, got %v", got)
	}
	if _, err := s.LoadProposal("p-pending"); err != nil {
		t.Fatal(err)
	}
}

func TestPruneIdempotent(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p-rejected", "rejected", "pattern-x")
	mustObservation(t, s, "o1", "p-rejected", "ep1", "after")
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")

	first, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if first.EpisodesRemoved != 1 || second.EpisodesRemoved != 0 {
		t.Fatalf("prune must be idempotent: first=%+v second=%+v", first, second)
	}
	if len(storeFiles(t, s, "observations")) != 0 {
		t.Fatal("observations should be empty after prune")
	}
}

func TestPruneKeepsRecentEpisodesWithinWindow(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p-rejected", "rejected", "pattern-x")
	mustEpisode(t, s, "ep-old", "2026-01-01T00:00:00Z")
	mustEpisode(t, s, "ep-mid", "2026-02-01T00:00:00Z")
	mustEpisode(t, s, "ep-new", "2026-03-01T00:00:00Z")
	mustObservation(t, s, "o-old", "p-rejected", "ep-old", "after")
	mustObservation(t, s, "o-mid", "p-rejected", "ep-mid", "after")
	mustObservation(t, s, "o-new", "p-rejected", "ep-new", "after")

	result, err := s.Prune(PruneOptions{KeepEpisodes: 2, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.EpisodesRemoved != 1 {
		t.Fatalf("expected only the oldest episode removed: %+v", result)
	}
	got := storeFiles(t, s, "episodes")
	if len(got) != 2 {
		t.Fatalf("expected 2 episodes kept, got %v", got)
	}
	for _, name := range got {
		if name == "ep-old.json" {
			t.Fatal("oldest episode must be removed")
		}
	}
}

// --- Repair ----------------------------------------------------------------

func TestRepairRemovesOrphanObservation(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p1", "approved", "pattern-x")
	// Observation references an episode that does not exist.
	mustObservation(t, s, "o-orphan", "p1", "ep-missing", "after")

	result, err := s.Repair(RepairOptions{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected orphan observation removed: %+v", result)
	}
	if len(storeFiles(t, s, "observations")) != 0 {
		t.Fatal("orphan observation must be removed")
	}
}

func TestRepairDeduplicatesDuplicateFiles(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p1", "approved", "pattern-x")
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	// Two files carrying the same observation ID.
	o := Observation{SchemaVersion: SchemaVersion, ID: "dup", ProposalID: "p1", EpisodeID: "ep1", Phase: "after", Succeeded: true, CreatedAt: Now()}
	b, err := Encode(o)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := s.safeJoin("observations")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dup.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dup.copy.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := s.Repair(RepairOptions{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected one duplicate removed: %+v", result)
	}
	if got := storeFiles(t, s, "observations"); len(got) != 1 || got[0] != "dup.json" {
		t.Fatalf("expected canonical dup.json kept, got %v", got)
	}
}

func TestRepairFixesInvalidPatternIndex(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p1", "pending", "pattern-bad")
	// Pattern references episodes that do not exist.
	p := Pattern{SchemaVersion: SchemaVersion, ID: "pattern-bad", TaskClass: "build", FailureClass: "task_failure", EpisodeIDs: []string{"ep1", "ep-missing", "ep2", "ep3"}, CreatedAt: Now()}
	if err := s.SavePattern(p); err != nil {
		t.Fatal(err)
	}
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	mustEpisode(t, s, "ep2", "2026-02-01T00:00:00Z")
	mustEpisode(t, s, "ep3", "2026-03-01T00:00:00Z")

	result, err := s.Repair(RepairOptions{DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.IndexesFixed != 1 {
		t.Fatalf("expected index fixed: %+v", result)
	}
	fixed, err := s.LoadPattern("pattern-bad")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed.EpisodeIDs) != 3 || fixed.EpisodeIDs[0] != "ep1" || fixed.EpisodeIDs[1] != "ep2" || fixed.EpisodeIDs[2] != "ep3" {
		t.Fatalf("invalid index must be dropped: %+v", fixed.EpisodeIDs)
	}
}

func TestRepairRejectsUnresolvableContent(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p1", "approved", "pattern-x")
	mustObservation(t, s, "o1", "p1", "ep1", "after")
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	dir, err := s.safeJoin("episodes")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := s.Repair(RepairOptions{DryRun: false})
	if err == nil {
		t.Fatalf("expected failure on corrupt JSON, got %+v", result)
	}
	if len(result.Unresolved) == 0 {
		t.Fatalf("expected unresolved report: %+v", result)
	}
	// Fail closed: nothing may be written.
	if len(storeFiles(t, s, "observations")) != 1 {
		t.Fatal("repair must not write when unresolvable content exists")
	}
}

func TestRepairDryRunWritesNothing(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p1", "approved", "pattern-x")
	mustObservation(t, s, "o-orphan", "p1", "ep-missing", "after")

	result, err := s.Repair(RepairOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.FilesRemoved != 1 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
	if len(storeFiles(t, s, "observations")) != 1 {
		t.Fatal("dry-run must not write")
	}
}

// --- Snapshot & restore ----------------------------------------------------

func TestPruneCreatesRestorableSnapshot(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	mustProposal(t, s, "p-rejected", "rejected", "pattern-x")
	mustObservation(t, s, "o1", "p-rejected", "ep1", "after")
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")

	result, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot == "" {
		t.Fatal("expected snapshot hash in result")
	}
	restored, err := s.RestoreSnapshot(result.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored != 2 {
		t.Fatalf("expected 2 files restored, got %d", restored)
	}
	if len(storeFiles(t, s, "episodes")) != 1 || len(storeFiles(t, s, "observations")) != 1 {
		t.Fatal("restore must bring back removed files")
	}
	// Snapshot must carry a valid hash of its own content.
	b, err := os.ReadFile(filepath.Join(s.Root, "maintenance", "snapshot-"+result.Snapshot+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.SHA256 != result.Snapshot {
		t.Fatal("snapshot hash mismatch")
	}
	if len(snap.Entries) != 2 || snap.Entries[0].ContentB64 == "" {
		t.Fatalf("snapshot entries incomplete: %+v", snap)
	}
	if _, err := base64.StdEncoding.DecodeString(snap.Entries[0].ContentB64); err != nil {
		t.Fatal(err)
	}
}

// --- Fail closed -----------------------------------------------------------

func TestPruneRejectsCrossScopeStore(t *testing.T) {
	first, _ := NewStore(t.TempDir())
	second, _ := NewStore(t.TempDir())
	second.Root = first.Root // copied store must be rejected
	mustEpisode(t, first, "ep1", "2026-01-01T00:00:00Z")
	if _, err := second.Prune(PruneOptions{KeepEpisodes: 0, DryRun: true}); err == nil {
		t.Fatal("expected cross-scope rejection")
	}
	if _, err := second.Repair(RepairOptions{DryRun: true}); err == nil {
		t.Fatal("expected cross-scope rejection")
	}
	if _, err := second.Stats(); err == nil {
		t.Fatal("expected cross-scope rejection")
	}
}

func TestPruneRejectsSymlinkInCollection(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	mustEpisode(t, s, "ep1", "2026-01-01T00:00:00Z")
	target := t.TempDir()
	link := filepath.Join(s.Root, "episodes", "evil.json")
	if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "ep1.json"), link); err != nil {
		t.Skip(err)
	}
	if _, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: true}); err == nil {
		t.Fatal("expected symlink rejection")
	}
	if _, err := s.Repair(RepairOptions{DryRun: true}); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestPruneZeroKeepKeepsNothingBeyondTerminalEvidence(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	// Unreferenced recent episode must survive (no terminal link).
	mustEpisode(t, s, "ep-unreferenced", "2026-01-01T00:00:00Z")
	result, err := s.Prune(PruneOptions{KeepEpisodes: 0, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.EpisodesRemoved != 0 {
		t.Fatalf("unreferenced episode must not be pruned: %+v", result)
	}
	if got := storeFiles(t, s, "episodes"); len(got) != 1 {
		t.Fatalf("unreferenced episode must stay: %v", got)
	}
}

func TestSnapshotRejectsUnknownSnapshot(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if _, err := s.RestoreSnapshot("missing"); err == nil {
		t.Fatal("expected unknown snapshot error")
	}
}

func TestPruneRejectsZeroEpisodesWindowNegative(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if _, err := s.Prune(PruneOptions{KeepEpisodes: -1, DryRun: true}); err == nil {
		t.Fatal("expected negative window rejection")
	}
}

// Ensure imports are used by tests referencing symbols below.
var _ = fmt.Sprintf
var _ = strings.TrimSpace
