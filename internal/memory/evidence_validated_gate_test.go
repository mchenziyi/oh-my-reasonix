package memory

import (
	"context"
	"testing"
)

func TestEvidenceValidatedGateSatisfied(t *testing.T) {
	evidence := []EvidenceRef{
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash},
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_002", ContentSHA256: testHash},
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_003", ContentSHA256: testHash},
	}
	_, s, tx, rev, _ := criticWorldWithEvidence(t, "gate_satisfied", evidence, []string{"root_task_001", "root_task_002"})
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_gate_critic", "passed", nil, nil)
	putConflictReview(t, s, rev, mc, "judgment_gate_conflict", "clear", "generation_full_scan", nil, nil)

	before := fileCount(t, s.root)
	got, err := EvaluateEvidenceValidatedGate(context.Background(), s, EvidenceValidatedGateRequest{
		Scope: rev.Scope, MemoryID: rev.MemoryID, Revision: rev.Revision,
		ExpectedMemoryContext: mc, ProjectStore: s, Now: criticReq(rev, mc).Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Satisfied || got.EvidenceCount != 3 || got.RootTaskCount != 2 ||
		got.CriticStatus != CriticRequirementPassed || got.ConflictStatus != ConflictRequirementClear {
		t.Fatalf("unexpected gate result: %+v", got)
	}
	if after := fileCount(t, s.root); after != before {
		t.Fatalf("gate must be read-only: before=%d after=%d", before, after)
	}
	derived, err := DeriveState(context.Background(), s, DerivedStateRequest{Scope: ScopeProject, Now: criticReq(rev, mc).Now})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range derived.States {
		if state.MemoryID == rev.MemoryID && state.Lifecycle != LifecycleProbation {
			t.Fatalf("gate must not mutate legacy lifecycle: %s", state.Lifecycle)
		}
	}
}

func TestEvidenceValidatedGateNeedsEveryCondition(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "gate_incomplete")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_gate_critic_only", "passed", nil, nil)
	got, err := EvaluateEvidenceValidatedGate(context.Background(), s, EvidenceValidatedGateRequest{
		Scope: rev.Scope, MemoryID: rev.MemoryID, Revision: rev.Revision,
		ExpectedMemoryContext: mc, ProjectStore: s, Now: criticReq(rev, mc).Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Satisfied || got.EvidenceCount != 1 || got.RootTaskCount != 0 || got.ConflictStatus != ConflictRequirementUnavailable {
		t.Fatalf("incomplete inputs must not satisfy: %+v", got)
	}
}

func TestEvidenceValidatedGateIgnoresEvidenceAddedAfterPinnedGeneration(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "gate_pinned_world")
	mc := expectedContext(t, s, tx)
	for i, id := range []string{"episode_late_2", "episode_late_3"} {
		ev := MemoryEvidenceGeneration{
			SchemaVersion: 1, MemoryID: rev.MemoryID, Revision: rev.Revision, EvidenceGeneration: i + 2,
			EvidenceRefs: []EvidenceRef{{Scope: rev.Scope, EvidenceType: "episode", EvidenceID: id, ContentSHA256: testHash}},
			RootTaskRefs: []string{"root_task_late_" + string(rune('a'+i))}, TransactionID: "tx_" + id,
			CreatedAt: "2026-08-11T00:00:00Z",
		}
		ev = fillEvidenceHash(ev)
		put(t, s, ev)
	}
	putCritic(t, s, rev, mc, "judgment_gate_pinned_critic", "passed", nil, nil)
	putConflictReview(t, s, rev, mc, "judgment_gate_pinned_conflict", "clear", "generation_full_scan", nil, nil)
	got, err := EvaluateEvidenceValidatedGate(context.Background(), s, EvidenceValidatedGateRequest{
		Scope: rev.Scope, MemoryID: rev.MemoryID, Revision: rev.Revision,
		ExpectedMemoryContext: mc, ProjectStore: s, Now: criticReq(rev, mc).Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Satisfied || got.EvidenceCount != 1 || got.RootTaskCount != 0 {
		t.Fatalf("post-generation evidence must be invisible: %+v", got)
	}
}
