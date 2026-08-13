package memory

import (
	"context"
	"testing"
	"time"
)

func TestApplyRollbackPlanAuditedAndIdempotent(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	first := commitOne(t, gs, "rollback_first", nil)
	second := commitOne(t, gs, "rollback_second", &first.GenerationID)
	plan, err := BuildRollbackPlan(context.Background(), gs, first.GenerationID)
	if err != nil || !plan.Eligible {
		t.Fatalf("rollback plan: %+v %v", plan, err)
	}
	now := time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC)
	req := RollbackRequest{Plan: plan, Operator: "operator-1", Reason: "restore verified generation", Now: now, IdempotencyKey: "rollback-key"}
	res, err := ApplyRollbackPlan(context.Background(), gs, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "committed" || res.SourceGenerationID != second.GenerationID || res.TargetGenerationID != first.GenerationID {
		t.Fatalf("unexpected rollback result: %+v", res)
	}
	impl := gs.(*generationStore)
	cur, err := impl.readCurrent(context.Background())
	if err != nil || cur.GenerationID != first.GenerationID || cur.TransactionID != res.RollbackID || cur.CreatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("CURRENT not rolled back: %+v %v", cur, err)
	}
	secondBefore, _, _ := readPublishedGeneration(impl, second.GenerationID)
	secondAfter, _, _ := readPublishedGeneration(impl, second.GenerationID)
	if secondBefore != secondAfter {
		t.Fatal("rollback must not change historical generations")
	}
	replay, err := ApplyRollbackPlan(context.Background(), gs, req)
	if err != nil || replay.RollbackID != res.RollbackID || replay.Status != "committed" {
		t.Fatalf("rollback replay not idempotent: %+v %v", replay, err)
	}
}

func TestApplyRollbackPlanCASAndIntegrity(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	first := commitOne(t, gs, "rollback_cas_first", nil)
	second := commitOne(t, gs, "rollback_cas_second", &first.GenerationID)
	plan, err := BuildRollbackPlan(context.Background(), gs, first.GenerationID)
	if err != nil || !plan.Eligible {
		t.Fatal(err)
	}
	_ = commitOne(t, gs, "rollback_cas_third", &second.GenerationID)
	// The preview is stale after a newer commit.
	if _, err := ApplyRollbackPlan(context.Background(), gs, RollbackRequest{Plan: plan, Operator: "op", Reason: "stale", Now: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), IdempotencyKey: "rollback-stale"}); ErrorCode(err) != CodeGenerationCurrentCAS {
		t.Fatalf("stale plan must fail CAS, got %v", err)
	}
}
