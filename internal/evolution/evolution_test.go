package evolution

import (
	"crypto/sha256"
	"encoding/hex"
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
