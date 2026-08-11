package memory

// MEM-02-08 failure-first tests: offline Memory Quality Benchmark. The
// benchmark is a pure function from a frozen fixture file to a byte-stable
// report; it never calls a model, never touches the network and never
// claims model-quality improvement. Malicious fixtures are rejected
// (unknown fields, hash drift, invalid enums).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeBenchmarkFixture serializes a fixture to a temp file and returns its
// path.
func writeBenchmarkFixture(t *testing.T, fx BenchmarkFixture) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	data, err := json.MarshalIndent(fx, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cleanBenchmarkFixture() BenchmarkFixture {
	rev := validRevision()
	ev := validEvidenceGeneration()
	pol := policyOf(PolicyTypeFreshness)
	usage := MemoryUsage{
		SchemaVersion: 1, UsageID: "usage_bench_1", Scope: ScopeProject,
		MemoryID: rev.MemoryID, Revision: rev.Revision, UsageStage: "affected",
		EpisodeID: "episode_bench", OccurredAt: "2026-08-11T10:00:00Z",
		Source: "local_user", CreatedAt: "2026-08-11T10:00:00Z",
	}
	usage = fillUsageHash(usage)
	outcome := Outcome{
		SchemaVersion: 1, OutcomeID: "outcome_bench_1", Scope: ScopeProject,
		UsageID: usage.UsageID, MemoryID: rev.MemoryID, Revision: rev.Revision,
		Effect: "helped", CreatedAt: "2026-08-11T11:00:00Z",
	}
	outcome = fillOutcomeHash(outcome)
	return BenchmarkFixture{
		SchemaVersion: 1,
		FixtureID:     "mem02_benchmark_clean",
		Revisions:     []MemoryRevision{rev},
		Evidences:     []MemoryEvidenceGeneration{ev},
		Judgments:     []JudgmentFact{},
		Policies:      []PolicyFact{pol},
		Usages:        []MemoryUsage{usage},
		Outcomes:      []Outcome{outcome},
	}
}

func TestBenchmarkFixtureCleanAndDeterministic(t *testing.T) {
	path := writeBenchmarkFixture(t, cleanBenchmarkFixture())
	rep1, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatalf("clean fixture must run: %v", err)
	}
	rep2, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(rep1)
	b2, _ := json.Marshal(rep2)
	if string(b1) != string(b2) {
		t.Error("benchmark report must be byte-stable across runs")
	}
	if rep1.FixtureID != "mem02_benchmark_clean" {
		t.Errorf("fixture id = %q", rep1.FixtureID)
	}
	if rep1.Metrics.TotalFacts == 0 {
		t.Error("total facts must be counted")
	}
	if rep1.Metrics.RejectedFacts != 0 {
		t.Errorf("clean fixture must have zero rejected facts, got %d", rep1.Metrics.RejectedFacts)
	}
	if rep1.Metrics.DeterministicHash == "" {
		t.Error("deterministic hash must be computed")
	}
	if rep1.Metrics.EvidenceStatus != "insufficient_evidence" {
		t.Errorf("single-episode fixture must report insufficient_evidence, got %q", rep1.Metrics.EvidenceStatus)
	}
	if !rep1.ProtocolOnly {
		t.Error("benchmark must declare protocol-only metrics")
	}
}

func TestBenchmarkFixtureRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malicious_unknown.json")
	raw := `{
		"schema_version": 1,
		"fixture_id": "mem02_malicious_unknown",
		"revisions": [],
		"unknown_fixture_field": true
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := RunBenchmarkFixture(context.Background(), path)
	if err == nil {
		t.Fatal("fixture with unknown field must be rejected")
	}
	if ErrorCode(err) != CodeUnknownField {
		t.Errorf("want unknown_field, got %v", err)
	}
}

func TestBenchmarkFixtureRejectsHashDrift(t *testing.T) {
	rev := validRevision()
	rev.ContentSHA256 = "sha256_" + "0" // invalid length
	fx := cleanBenchmarkFixture()
	fx.FixtureID = "mem02_malicious_hash"
	fx.Revisions = []MemoryRevision{rev}
	path := writeBenchmarkFixture(t, fx)

	rep, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatalf("hash-drift fixture must produce a rejection report, got %v", err)
	}
	if rep.Metrics.RejectedFacts == 0 {
		t.Error("hash-drift revision must be counted as rejected")
	}
}

func TestBenchmarkFixtureRejectsInvalidEnum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malicious_enum.json")
	raw := `{
		"schema_version": 1,
		"fixture_id": "mem02_malicious_enum",
		"revisions": [{
			"schema_version": 1,
			"memory_id": "mem_bench_enum",
			"memory_type": "not_a_type",
			"scope": "project",
			"canonical_key": "bench-enum",
			"revision": 1,
			"usage_policy": "outcome_attributed",
			"title": "x",
			"summary": "y",
			"aliases": [],
			"applies_when": [],
			"does_not_apply_when": [],
			"relations": [],
			"content_sha256": "` + testHash + `",
			"created_at": "2026-08-11T00:00:00Z"
		}],
		"evidences": [],
		"judgments": [],
		"policies": [],
		"usages": [],
		"outcomes": []
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatalf("invalid-enum fixture must produce a rejection report, got %v", err)
	}
	if rep.Metrics.RejectedFacts == 0 {
		t.Error("invalid-enum revision must be counted as rejected")
	}
}

func TestBenchmarkFixtureScopeIsolation(t *testing.T) {
	// A global-scope fact inside a fixture is rejected by a project-scope
	// benchmark run (no silent cross-scope acceptance).
	rev := validRevision()
	rev.Scope = ScopeGlobal
	fx := cleanBenchmarkFixture()
	fx.FixtureID = "mem02_malicious_scope"
	fx.Revisions = []MemoryRevision{rev}
	path := writeBenchmarkFixture(t, fx)

	rep, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatalf("cross-scope fixture must produce a rejection report, got %v", err)
	}
	if rep.Metrics.RejectedFacts == 0 {
		t.Error("cross-scope revision must be counted as rejected")
	}
}

func TestBenchmarkFixtureRebuildableAfterDeletion(t *testing.T) {
	path := writeBenchmarkFixture(t, cleanBenchmarkFixture())
	rep1, err := RunBenchmarkFixture(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// Deleting any previously produced report file (or simply re-running)
	// yields the identical report: the fixture is the only input.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := RunBenchmarkFixture(context.Background(), path); err == nil {
		t.Fatal("deleted fixture must fail cleanly")
	}
	// Recreate from the same fixture content and compare bytes.
	path2 := writeBenchmarkFixture(t, cleanBenchmarkFixture())
	rep2, err := RunBenchmarkFixture(context.Background(), path2)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(rep1)
	b2, _ := json.Marshal(rep2)
	if string(b1) != string(b2) {
		t.Error("report must be reconstructible byte-for-byte from the fixture")
	}
}
