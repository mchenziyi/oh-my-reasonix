package memory

import (
	"context"
	"testing"
)

func TestApplyRevisionPlanWritesOnlyValidatedNextRevision(t *testing.T) {
	ctx := context.Background()
	store := openProject(t, tempRoot(t), Options{})
	source := validRevision()
	if _, err := store.Put(ctx, source); err != nil {
		t.Fatal(err)
	}
	evidence := validEvidenceGeneration()
	if _, err := store.Put(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	target := source
	target.Revision = source.Revision + 1
	target.Title = "revised"
	target.CreatedAt = "2026-08-13T01:00:00Z"
	target.ContentSHA256 = ""
	hash, err := target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	target.ContentSHA256 = hash
	plan, err := BuildRevisionPlan(source, target, evidence.EvidenceRefs)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyRevisionPlan(ctx, store, plan, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != WriteCreated {
		t.Fatalf("expected created, got %+v", result)
	}
	replay, err := ApplyRevisionPlan(ctx, store, plan, target)
	if err != nil || replay.Status != WriteNoop {
		t.Fatalf("same plan must be idempotent: result=%+v err=%v", replay, err)
	}
}

func TestApplyRevisionPlanRejectsForgedOrIncompleteInputs(t *testing.T) {
	ctx := context.Background()
	store := openProject(t, tempRoot(t), Options{})
	source := validRevision()
	if _, err := store.Put(ctx, source); err != nil {
		t.Fatal(err)
	}
	target := source
	target.Revision = source.Revision + 1
	target.ContentSHA256 = ""
	hash, err := target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	target.ContentSHA256 = hash
	plan, err := BuildRevisionPlan(source, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRevisionPlan(ctx, store, plan, target); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("missing evidence must fail closed, got %v", err)
	}
	target.Revision = source.Revision + 2
	target.ContentSHA256 = "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := ApplyRevisionPlan(ctx, store, plan, target); ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("target identity mismatch must fail closed, got %v", err)
	}
}
