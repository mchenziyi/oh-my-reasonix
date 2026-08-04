package profilebench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Fixture load / discover -----------------------------------------------

func TestDiscoverProfileFixtures(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "explore", `{"schema_version":1,"id":"explore-evidence","profile":"omr-explore","task":"t","acceptance":["a1","a2"],"required_evidence":["e1","e2","e3"],"forbidden_paths":["outside"],"replay":{"acceptance_met":[true,true],"evidence_covered":[true,true,true],"out_of_scope_changes":0,"human_corrections":0,"metrics":{"prompt_tokens":10,"completion_tokens":5,"cost":0.01,"duration_ms":100}}}`)
	writeFixture(t, root, "debug", `{"schema_version":1,"id":"debug-root-cause","profile":"omr-debug","task":"t","acceptance":["a1"],"required_evidence":["e1"],"replay":{"acceptance_met":[true],"evidence_covered":[true]}}`)

	fixtures, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 {
		t.Fatalf("expected 2 fixtures, got %d", len(fixtures))
	}
	if fixtures[0].ID != "debug-root-cause" || fixtures[1].ID != "explore-evidence" {
		t.Fatalf("fixtures must be sorted: %+v", fixtures)
	}
}

func TestLoadFixtureRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.yaml")
	data := `{"schema_version":1,"id":"x","profile":"omr-explore","task":"t","unknown_field":1}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixture(path); err == nil {
		t.Fatal("expected unknown-field rejection")
	}
}

func TestLoadFixtureRequiresProfileAndAcceptance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.yaml")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"id":"x","task":"t"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixture(path); err == nil {
		t.Fatal("expected missing profile/acceptance rejection")
	}
}

// --- Evaluation -------------------------------------------------------------

func TestEvaluateComputesProcessMetrics(t *testing.T) {
	fixture := ProfileFixture{
		SchemaVersion:    1,
		ID:               "explore-evidence",
		Profile:          "omr-explore",
		Task:             "t",
		Acceptance:       []string{"a1", "a2"},
		RequiredEvidence: []string{"e1", "e2"},
		ForbiddenPaths:   []string{"outside"},
		Replay: &ProfileReplay{
			AcceptanceMet:     []bool{true, false},
			EvidenceCovered:   []bool{true, false},
			OutOfScopeChanges: 2,
			HumanCorrections:  1,
			Metrics:           ProfileMetrics{PromptTokens: 100, CompletionTokens: 50, Cost: 0.3, DurationMs: 500},
		},
	}
	ev := Evaluate(fixture, *fixture.Replay)
	if ev.AcceptancePassRate != 0.5 || ev.EvidenceCompleteRate != 0.5 {
		t.Fatalf("unexpected rates: %+v", ev)
	}
	if ev.OutOfScopeChanges != 2 || ev.HumanCorrections != 1 {
		t.Fatalf("unexpected counts: %+v", ev)
	}
	if ev.Metrics.PromptTokens != 100 || ev.Metrics.DurationMs != 500 {
		t.Fatalf("unexpected metrics: %+v", ev)
	}
	if ev.Qualified {
		t.Fatal("half-met acceptance must not qualify")
	}
}

func TestEvaluateQualifiesOnFullCoverage(t *testing.T) {
	fixture := ProfileFixture{
		SchemaVersion: 1,
		ID:            "planner-executable",
		Profile:       "omr-planner",
		Task:          "t",
		Acceptance:    []string{"a1"},
		Replay: &ProfileReplay{
			AcceptanceMet:   []bool{true},
			EvidenceCovered: []bool{true},
		},
	}
	ev := Evaluate(fixture, *fixture.Replay)
	if !ev.Qualified || ev.AcceptancePassRate != 1 || ev.EvidenceCompleteRate != 1 {
		t.Fatalf("expected qualification: %+v", ev)
	}
}

func TestEvaluateBlockedByForbiddenPaths(t *testing.T) {
	fixture := ProfileFixture{
		SchemaVersion:  1,
		ID:             "comment-checker-block",
		Profile:        "omr-comment-check",
		Task:           "t",
		Acceptance:     []string{"a1"},
		ForbiddenPaths: []string{"outside"},
		Replay:         &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}, ChangedPaths: []string{"outside/leak.go"}},
	}
	ev := Evaluate(fixture, *fixture.Replay)
	if ev.Qualified {
		t.Fatal("forbidden path change must not qualify")
	}
}

// --- Report -----------------------------------------------------------------

func TestBuildReportMatrix(t *testing.T) {
	fixtures := []ProfileFixture{
		{SchemaVersion: 1, ID: "a", Profile: "omr-explore", Task: "t", Acceptance: []string{"a1"}, RequiredEvidence: []string{"e1"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}}},
		{SchemaVersion: 1, ID: "b", Profile: "omr-debug", Task: "t", Acceptance: []string{"a1", "a2"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true, false}, EvidenceCovered: []bool{true}}},
	}
	results := map[string]ProfileReplay{
		"a": *fixtures[0].Replay,
		"b": *fixtures[1].Replay,
	}
	report := EvaluateAll(fixtures, results, "run-1", true)
	if report.SchemaVersion != 1 || report.FixtureCount != 2 || report.EvaluatedCount != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.QualifiedCount != 1 || report.QualifiedRate != 0.5 {
		t.Fatalf("unexpected qualification: %+v", report)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results: %+v", report)
	}
	// Metric totals.
	if report.Metrics.PromptTokens != 0 || report.Metrics.Cost != 0 {
		t.Fatalf("unexpected totals: %+v", report.Metrics)
	}
	// The report must carry the explicit non-claim.
	if !strings.Contains(report.Claim, "not") || !strings.Contains(report.Claim, "quality") {
		t.Fatalf("report must disclaim quality proof: %q", report.Claim)
	}
}

func TestBuildReportSingleProfileFilter(t *testing.T) {
	fixtures := []ProfileFixture{
		{SchemaVersion: 1, ID: "a", Profile: "omr-explore", Task: "t", Acceptance: []string{"a1"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}}},
		{SchemaVersion: 1, ID: "b", Profile: "omr-debug", Task: "t", Acceptance: []string{"a1"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}}},
	}
	results := map[string]ProfileReplay{
		"a": *fixtures[0].Replay,
		"b": *fixtures[1].Replay,
	}
	report := EvaluateAll(fixtures, results, "run-1", false, WithProfile("omr-explore"))
	if report.FixtureCount != 1 || report.EvaluatedCount != 1 || report.Results[0].Profile != "omr-explore" {
		t.Fatalf("unexpected filtered report: %+v", report)
	}
}

func TestEvaluateAllKeepsFailureEvidence(t *testing.T) {
	fixtures := []ProfileFixture{
		{SchemaVersion: 1, ID: "a", Profile: "omr-explore", Task: "t", Acceptance: []string{"a1", "a2"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true, false}, EvidenceCovered: []bool{true, false}}},
	}
	results := map[string]ProfileReplay{"a": *fixtures[0].Replay}
	report := EvaluateAll(fixtures, results, "run-1", true)
	if len(report.Results) != 1 || len(report.Results[0].Failures) == 0 {
		t.Fatalf("failure evidence must be retained: %+v", report)
	}
	if report.Results[0].AcceptancePassRate != 0.5 || report.Results[0].EvidenceCompleteRate != 0.5 {
		t.Fatalf("rates must be preserved: %+v", report.Results[0])
	}
}

func TestReportJSONRoundTrip(t *testing.T) {
	fixtures := []ProfileFixture{
		{SchemaVersion: 1, ID: "a", Profile: "omr-explore", Task: "t", Acceptance: []string{"a1"}, Replay: &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}}},
	}
	report := EvaluateAll(fixtures, map[string]ProfileReplay{"a": *fixtures[0].Replay}, "run-1", true)
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var back ProfileReport
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.SchemaVersion != 1 || back.QualifiedRate != 1 {
		t.Fatalf("round trip failed: %+v", back)
	}
}

// --- Paired -----------------------------------------------------------------

func TestReplayPairedNativeOMR(t *testing.T) {
	fixture := ProfileFixture{
		SchemaVersion: 1,
		ID:            "paired-x",
		Profile:       "omr-explore",
		Task:          "t",
		Acceptance:    []string{"a1"},
		NativeReplay:  &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}},
		OMRReplay:     &ProfileReplay{AcceptanceMet: []bool{false}, EvidenceCovered: []bool{false}},
	}
	native, omr, err := ReplayPaired(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !native.Qualified || omr.Qualified {
		t.Fatalf("unexpected paired outcome: native=%+v omr=%+v", native, omr)
	}
}

func TestReplayPairedRequiresBothSides(t *testing.T) {
	fixture := ProfileFixture{
		SchemaVersion: 1,
		ID:            "one-sided",
		Profile:       "omr-explore",
		Task:          "t",
		Acceptance:    []string{"a1"},
		NativeReplay:  &ProfileReplay{AcceptanceMet: []bool{true}, EvidenceCovered: []bool{true}},
	}
	if _, _, err := ReplayPaired(fixture); err == nil {
		t.Fatal("expected error when omr side missing")
	}
}

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
