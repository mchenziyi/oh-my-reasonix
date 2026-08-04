package evolution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordRunCreatesProposalAfterThreeFailures(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	for i := 0; i < 3; i++ {
		r := reasonix.Result{ExitCode: 1}
		st := reasonix.EventStream{SessionID: string(rune('a' + i))}
		if err := RecordRun(s, "build", r, st); err != nil {
			t.Fatal(err)
		}
	}
	ps, err := s.ListProposals()
	if err != nil || len(ps) != 1 {
		t.Fatalf("proposals=%d err=%v", len(ps), err)
	}
}

type fakeProposer struct{ calls int }

func (f *fakeProposer) Propose(pattern Pattern) (Proposal, error) {
	f.calls++
	overlay := "retry with evidence"
	h := sha256.Sum256([]byte(overlay))
	return Proposal{SchemaVersion: 1, ID: NewID("proposal", pattern.ID), PatternID: pattern.ID, Title: "fake", Rationale: "fake", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "pending", CreatedAt: Now(), UpdatedAt: Now()}, nil
}
func TestRecordRunFakeProposerOneShot(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	f := &fakeProposer{}
	for i := 0; i < 4; i++ {
		if err := RecordRunWithProposer(s, "build", reasonix.Result{ExitCode: 1}, reasonix.EventStream{SessionID: string(rune('a' + i))}, f); err != nil {
			t.Fatal(err)
		}
	}
	if f.calls != 1 {
		t.Fatalf("calls=%d", f.calls)
	}
	ps, _ := s.ListProposals()
	if len(ps) != 1 {
		t.Fatalf("proposals=%d", len(ps))
	}
}

func TestStoreEpisodeIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := Episode{SchemaVersion: 1, ID: "ep1", TaskClass: "build", FailureClass: "test_failure", CreatedAt: Now()}
	if err = s.RecordEpisode(e); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordEpisode(e); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListEpisodes()
	if err != nil || len(got) != 1 {
		t.Fatalf("got %d %v", len(got), err)
	}
}
func TestTriggerThreeOnly(t *testing.T) {
	es := []Episode{{SchemaVersion: 1, ID: "a", TaskClass: "build", FailureClass: "compile", CreatedAt: Now()}, {SchemaVersion: 1, ID: "b", TaskClass: "build", FailureClass: "compile", CreatedAt: Now()}, {SchemaVersion: 1, ID: "c", TaskClass: "build", FailureClass: "compile", CreatedAt: Now()}}
	p := DetectPattern(es, 3)
	if p == nil || len(p.EpisodeIDs) != 3 {
		t.Fatal("expected pattern")
	}
	if DetectPattern(es[:2], 3) != nil {
		t.Fatal("unexpected pattern")
	}
}
func TestStoreRejectsSymlinkRoot(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(dir, ".reasonix")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := Episode{SchemaVersion: 1, ID: "x", TaskClass: "x", FailureClass: "x", CreatedAt: Now()}
	if err := s.RecordEpisode(e); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
func TestProposalRejectsSecret(t *testing.T) {
	p := Proposal{SchemaVersion: 1, ID: "p", PatternID: "x", Title: "x", Overlay: "api_key=secret", Status: "pending"}
	if p.Validate() == nil {
		t.Fatal("expected rejection")
	}
}
func TestOverlayRollbackToEmpty(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if err := s.WriteOverlay("rule"); err != nil {
		t.Fatal(err)
	}
	if err := s.SnapshotOverlay("p"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteOverlay("new"); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreOverlay("p"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.ReadOverlay(); v != "rule" {
		t.Fatal(v)
	}
}

func TestObserveApprovedRollsBackAfterTwoFailures(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if err := s.SnapshotOverlay("p1"); err != nil {
		t.Fatal(err)
	}
	p := Proposal{SchemaVersion: 1, ID: "p1", PatternID: "pattern", Title: "rule", Overlay: "rule", Status: "approved", ApprovedAt: "2026-08-04T01:00:00Z", UpdatedAt: "2026-08-04T01:00:00Z"}
	if err := s.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
	for i, at := range []string{"2026-08-04T01:01:00Z", "2026-08-04T01:02:00Z"} {
		e := Episode{SchemaVersion: 1, ID: fmt.Sprintf("e%d", i), TaskClass: "build", FailureClass: "task_failure", CreatedAt: at}
		if err := s.RecordEpisode(e); err != nil {
			t.Fatal(err)
		}
	}
	called := 0
	rolled, err := ObserveApproved(s, func(id string) error {
		called++
		return s.RestoreOverlay(id)
	})
	if err != nil || len(rolled) != 1 || called != 1 {
		t.Fatalf("rolled=%v called=%d err=%v", rolled, called, err)
	}
	got, err := s.LoadProposal("p1")
	if err != nil || got.Status != "rolled_back" {
		t.Fatalf("proposal=%+v err=%v", got, err)
	}
}

func TestObserveApprovedWaitsForSecondFailure(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	p := Proposal{SchemaVersion: 1, ID: "p1", PatternID: "pattern", Title: "rule", Overlay: "rule", Status: "approved", ApprovedAt: "2026-08-04T01:00:00Z", UpdatedAt: "2026-08-04T01:00:00Z"}
	if err := s.SaveProposal(p); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(Episode{SchemaVersion: 1, ID: "e1", TaskClass: "build", FailureClass: "task_failure", CreatedAt: "2026-08-04T01:01:00Z"}); err != nil {
		t.Fatal(err)
	}
	rolled, err := ObserveApproved(s, func(string) error { return fmt.Errorf("unexpected rollback") })
	if err != nil || len(rolled) != 0 {
		t.Fatalf("rolled=%v err=%v", rolled, err)
	}
}

func TestBuildReportAggregatesWithoutSensitiveContent(t *testing.T) {
	s, _ := NewStore(t.TempDir())
	if err := s.RecordEpisode(Episode{SchemaVersion: 1, ID: "ok", TaskClass: "build", Succeeded: true, CreatedAt: Now(), PromptTokens: 10, OutputTokens: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEpisode(Episode{SchemaVersion: 1, ID: "bad", TaskClass: "build", FailureClass: "task_failure", CreatedAt: Now(), PromptTokens: 20, OutputTokens: 4}); err != nil {
		t.Fatal(err)
	}
	r, err := BuildReport(s)
	if err != nil {
		t.Fatal(err)
	}
	if r.Episodes != 2 || r.Successes != 1 || r.Failures != 1 || r.PromptTokens != 30 || r.OutputTokens != 7 || r.FailureClasses["task_failure"] != 1 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestStoreRejectsCopiedProjectScope(t *testing.T) {
	first, _ := NewStore(t.TempDir())
	if err := first.RecordEpisode(Episode{SchemaVersion: 1, ID: "ep", TaskClass: "build", Succeeded: true, CreatedAt: Now()}); err != nil {
		t.Fatal(err)
	}
	second, _ := NewStore(t.TempDir())
	second.Root = first.Root
	if _, err := second.ListEpisodes(); err == nil {
		t.Fatal("expected scope mismatch")
	}
}
