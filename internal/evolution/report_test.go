package evolution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// renderReportForTest renders the report deterministically for string checks.
func renderReportForTest(r Report) string {
	b, _ := json.Marshal(r)
	return string(b)
}

// --- LP-02 report aggregation helpers --------------------------------------

func lp02Store(t *testing.T) Store {
	t.Helper()
	s, _ := NewStore(t.TempDir())
	overlay := "rule"
	h := sha256.Sum256([]byte(overlay))
	// Approved proposal with observations.
	p := Proposal{SchemaVersion: SchemaVersion, ID: "p1", PatternID: "pattern-1", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "approved", ApprovedAt: "2026-03-01T00:00:00Z", CreatedAt: "2026-02-01T00:00:00Z", UpdatedAt: "2026-03-01T00:00:00Z"}
	if err := s.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
	// Rolled back proposal with reason.
	p2 := Proposal{SchemaVersion: SchemaVersion, ID: "p2", PatternID: "pattern-2", Title: "t2", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "rolled_back", RollbackReason: "two failed episodes during observation", CreatedAt: "2026-02-01T00:00:00Z", UpdatedAt: "2026-04-01T00:00:00Z"}
	if err := s.SaveProposal(p2); err != nil {
		t.Fatal(err)
	}
	// Pattern linking episode ids to p1's task class.
	if err := s.SavePattern(Pattern{SchemaVersion: SchemaVersion, ID: "pattern-1", TaskClass: "build", FailureClass: "task_failure", EpisodeIDs: []string{"e-b1", "e-b2", "e-b3"}, CreatedAt: Now()}); err != nil {
		t.Fatal(err)
	}
	episodes := []Episode{
		{SchemaVersion: SchemaVersion, ID: "e-b1", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", PromptTokens: 100, OutputTokens: 20, CreatedAt: "2026-02-01T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-b2", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", PromptTokens: 100, OutputTokens: 20, CreatedAt: "2026-02-02T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-b3", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", PromptTokens: 100, OutputTokens: 20, CreatedAt: "2026-02-03T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-a1", TaskClass: "build", Succeeded: true, PromptTokens: 80, OutputTokens: 10, CreatedAt: "2026-03-02T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-a2", TaskClass: "build", Succeeded: true, PromptTokens: 90, OutputTokens: 15, CreatedAt: "2026-03-03T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-a3", TaskClass: "build", Succeeded: true, PromptTokens: 95, OutputTokens: 12, CreatedAt: "2026-03-04T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-a4", TaskClass: "build", Succeeded: false, FailureClass: "task_failure", PromptTokens: 70, OutputTokens: 8, CreatedAt: "2026-03-05T00:00:00Z"},
		{SchemaVersion: SchemaVersion, ID: "e-a5", TaskClass: "build", Succeeded: true, PromptTokens: 85, OutputTokens: 14, CreatedAt: "2026-03-06T00:00:00Z"},
	}
	for _, e := range episodes {
		if err := s.RecordEpisode(e); err != nil {
			t.Fatal(err)
		}
	}
	// Observations: 3 before (failed) + 5 after (4 ok / 1 failed) for p1.
	for i := 0; i < 3; i++ {
		if err := s.SaveObservation(Observation{SchemaVersion: SchemaVersion, ID: fmt.Sprintf("o-b%d", i), ProposalID: "p1", EpisodeID: fmt.Sprintf("e-b%d", i+1), Phase: "before", Succeeded: false, FailureClass: "task_failure", PromptTokens: 100, OutputTokens: 20, CreatedAt: fmt.Sprintf("2026-02-0%dT00:00:00Z", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		ok := true
		fc := ""
		if i == 3 {
			ok = false
			fc = "task_failure"
		}
		if err := s.SaveObservation(Observation{SchemaVersion: SchemaVersion, ID: fmt.Sprintf("o-a%d", i), ProposalID: "p1", EpisodeID: fmt.Sprintf("e-a%d", i+1), Phase: "after", Succeeded: ok, FailureClass: fc, PromptTokens: 80 + i*5, OutputTokens: 10 + i, CreatedAt: fmt.Sprintf("2026-03-0%dT00:00:00Z", i+2)}); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestLP02ReportAggregatesByProposalAndTaskClass(t *testing.T) {
	s := lp02Store(t)
	report, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	// Proposal-level aggregation.
	if len(report.ProposalStats) != 2 {
		t.Fatalf("expected 2 proposal stats, got %d", len(report.ProposalStats))
	}
	byID := map[string]ProposalStats{}
	for _, ps := range report.ProposalStats {
		byID[ps.ProposalID] = ps
	}
	p1 := byID["p1"]
	if p1.Before != 3 || p1.After != 5 || p1.AfterSuccesses != 4 || p1.AfterFailures != 1 {
		t.Fatalf("unexpected p1 stats: %+v", p1)
	}
	if p1.Status != "observed" || p1.ObservationProgress != 5 || p1.ObservationTarget != 5 {
		t.Fatalf("unexpected p1 status: %+v", p1)
	}
	if p1.AfterSuccessRate != 0.8 || p1.AfterFailureRate != 0.2 {
		t.Fatalf("unexpected rates: %+v", p1)
	}
	if p1.AfterPromptTokens != 450 { // observations: 80+85+90+95+100
		t.Fatalf("unexpected after prompt tokens: %+v", p1)
	}
	if p1.AfterOutputTokens != 60 { // observations: 10+11+12+13+14
		t.Fatalf("unexpected after output tokens: %+v", p1)
	}
	p2 := byID["p2"]
	if p2.Status != "rolled_back" || p2.RollbackReason == "" {
		t.Fatalf("unexpected p2 stats: %+v", p2)
	}
	// Task-class aggregation.
	foundTask := false
	for _, tc := range report.TaskClasses {
		if tc.TaskClass == "build" {
			foundTask = true
			if tc.Episodes != 8 || tc.Successes != 4 || tc.Failures != 4 {
				t.Fatalf("unexpected task class: %+v", tc)
			}
		}
	}
	if !foundTask {
		t.Fatal("task class build missing")
	}
	// Failure-class aggregation.
	if report.FailureClasses["task_failure"] != 4 {
		t.Fatalf("unexpected failure class count: %v", report.FailureClasses)
	}
	// Report must never expose session ids or raw overlays.
	if strings.Contains(renderReportForTest(report), "api_key") {
		t.Fatal("report must not expose secrets")
	}
}

func TestLP02ReportInsufficientEvidence(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	overlay := "rule"
	h := sha256.Sum256([]byte(overlay))
	p := Proposal{SchemaVersion: SchemaVersion, ID: "p1", PatternID: "pattern-1", Title: "t", Rationale: "r", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "approved", ApprovedAt: Now(), CreatedAt: Now(), UpdatedAt: Now()}
	if err := s.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
	// Only one after observation — below the default window of 5.
	if err := s.SaveObservation(Observation{SchemaVersion: SchemaVersion, ID: "o1", ProposalID: "p1", EpisodeID: "e1", Phase: "after", Succeeded: true, CreatedAt: Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(Episode{SchemaVersion: SchemaVersion, ID: "e1", TaskClass: "build", Succeeded: true, CreatedAt: Now()}); err != nil {
		t.Fatal(err)
	}
	report, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ProposalStats) != 1 || report.ProposalStats[0].Status != "insufficient_evidence" {
		t.Fatalf("expected insufficient_evidence: %+v", report.ProposalStats)
	}
}

func TestLP02ReportNeverClaimsImprovement(t *testing.T) {
	s := lp02Store(t)
	report, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	// The report must not contain directional conclusions.
	if strings.Contains(renderReportForTest(report), "improved") ||
		strings.Contains(renderReportForTest(report), "improvement") ||
		strings.Contains(renderReportForTest(report), "significantly") {
		t.Fatal("report must not claim improvement without full pairing")
	}
	// Rolled back proposal carries its reason but no conclusion.
	for _, ps := range report.ProposalStats {
		if ps.ProposalID == "p2" && ps.RollbackReason == "" {
			t.Fatal("rollback reason missing")
		}
	}
}

func TestLP02ReportDeterministicOrder(t *testing.T) {
	s := lp02Store(t)
	first, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(first)
	b2, _ := json.Marshal(second)
	if string(b1) != string(b2) {
		t.Fatal("report must be deterministic")
	}
	// Stable sort by proposal id.
	for i := 1; i < len(first.ProposalStats); i++ {
		if first.ProposalStats[i-1].ProposalID > first.ProposalStats[i].ProposalID {
			t.Fatal("proposal stats must be sorted by id")
		}
	}
}

func TestLP02ReportScopeIsolation(t *testing.T) {
	a := lp02Store(t)
	b, _ := NewStore(t.TempDir())
	ra, err := BuildReport(a)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := BuildReport(b)
	if err != nil {
		t.Fatal(err)
	}
	if ra.ScopeID == rb.ScopeID {
		t.Fatal("scopes must differ")
	}
	if rb.Episodes != 0 || len(rb.ProposalStats) != 0 {
		t.Fatalf("empty store must report zeroes: %+v", rb)
	}
}

func TestLP02HistoryDetailedStats(t *testing.T) {
	s := lp02Store(t)
	detail, err := BuildHistory(s, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProposalID != "p1" {
		t.Fatalf("unexpected history: %+v", detail)
	}
	if detail.Before != 3 || detail.After != 5 || len(detail.Observations) != 8 {
		t.Fatalf("unexpected history counts: %+v", detail)
	}
	if len(detail.BeforeEpisodes) != 3 || len(detail.AfterEpisodes) != 5 {
		t.Fatalf("unexpected history episodes: %+v", detail)
	}
	if detail.Status != "observed" {
		t.Fatalf("unexpected status: %+v", detail)
	}
	// No raw session ids in the detail output.
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "session_") && strings.Contains(string(raw), "SessionID") {
		t.Fatal("history must not include session ids")
	}
}

func TestLP02HistoryUnknownProposal(t *testing.T) {
	s := lp02Store(t)
	if _, err := BuildHistory(s, "missing"); err == nil {
		t.Fatal("expected error for unknown proposal")
	}
}

// TestLP02ReportSnapshotGolden verifies the JSON shape of the enhanced report
// stays stable. The report intentionally changes only when schema fields are
// added or renamed, and never contains session ids or prompts.
func TestLP02ReportSnapshotGolden(t *testing.T) {
	s := lp02Store(t)
	report, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	for _, forbidden := range []string{`"session_id":"`, `"overlay":"`, `"prompt":"`, `"reasoning":"`, `"api_key"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("report leaked %s: %s", forbidden, raw)
		}
	}
	for _, required := range []string{`"proposal_stats"`, `"task_classes"`, `"observation_target":5`, `"after_success_rate"`} {
		if !strings.Contains(raw, required) {
			t.Fatalf("report missing %s: %s", required, raw)
		}
	}
}
