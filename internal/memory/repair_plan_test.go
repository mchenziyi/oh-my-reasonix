package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRepairPlanIsReadOnly(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRepairPlan(context.Background(), gs)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != "none" || plan.Scope != ScopeProject {
		t.Fatalf("unexpected empty repair plan: %+v", plan)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("repair plan must not write files")
	}
}

func TestBuildRepairAndRollbackPlansValidatePublishedGenerations(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	first := commitOne(t, gs, "repair_first", nil)
	second := commitOne(t, gs, "repair_second", &first.GenerationID)
	plan, err := BuildRepairPlan(context.Background(), gs)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != "none" || plan.CurrentGenerationID != second.GenerationID {
		t.Fatalf("valid current must need no repair: %+v", plan)
	}
	rb, err := BuildRollbackPlan(context.Background(), gs, first.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if !rb.Eligible || rb.CurrentGenerationID != second.GenerationID || rb.TargetGenerationID != first.GenerationID {
		t.Fatalf("valid historical target should be eligible: %+v", rb)
	}
	if _, err := BuildRollbackPlan(context.Background(), gs, "../outside"); err == nil {
		t.Fatal("path-like rollback target must fail closed")
	}
}

func TestBuildRollbackPlanRejectsTamperedTarget(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	first := commitOne(t, gs, "repair_tamper_first", nil)
	second := commitOne(t, gs, "repair_tamper_second", &first.GenerationID)
	if err := os.WriteFile(filepath.Join(root, "generations", first.GenerationID, "generation.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rb, err := BuildRollbackPlan(context.Background(), gs, first.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Eligible || rb.BlockedReason == "" || rb.CurrentGenerationID != second.GenerationID {
		t.Fatalf("tampered target must be blocked: %+v", rb)
	}
}
