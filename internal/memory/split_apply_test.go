package memory

import (
	"context"
	"testing"
)

func TestApplySplitPlanPublishesBranchesAsBatch(t *testing.T) {
	ctx := context.Background()
	s := openProject(t, tempRoot(t), Options{})
	source := validRevision()
	if _, err := s.Put(ctx, source); err != nil {
		t.Fatal(err)
	}
	ev := validEvidenceGeneration()
	if _, err := s.Put(ctx, ev); err != nil {
		t.Fatal(err)
	}
	branches := []SplitBranch{{Key: "sqlite", EvidenceRefs: ev.EvidenceRefs}, {Key: "postgres", EvidenceRefs: ev.EvidenceRefs}}
	plan, err := BuildSplitPlan(source, branches)
	if err != nil {
		t.Fatal(err)
	}
	targets := make([]MemoryRevision, 0, len(plan.Branches))
	for _, branch := range plan.Branches {
		target := source
		target.MemoryID = branch.ProposedMemoryID
		target.CanonicalKey = "split-" + branch.Key
		target.Revision = 1
		target.ContentSHA256 = ""
		hash, err := target.ContentHash()
		if err != nil {
			t.Fatal(err)
		}
		target.ContentSHA256 = hash
		targets = append(targets, target)
	}
	results, err := ApplySplitPlan(ctx, s, plan, targets)
	if err != nil || len(results) != 2 || results[0].Status != WriteCreated || results[1].Status != WriteCreated {
		t.Fatalf("split apply failed: results=%+v err=%v", results, err)
	}
	if _, err := ApplySplitPlan(ctx, s, plan, targets); err != nil {
		t.Fatalf("split replay must be idempotent: %v", err)
	}
}

func TestApplySplitPlanRejectsIncompleteTargetsWithoutWrite(t *testing.T) {
	ctx := context.Background()
	s := openProject(t, tempRoot(t), Options{})
	source := validRevision()
	if _, err := s.Put(ctx, source); err != nil {
		t.Fatal(err)
	}
	ev := validEvidenceGeneration()
	if _, err := s.Put(ctx, ev); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildSplitPlan(source, []SplitBranch{{Key: "one", EvidenceRefs: ev.EvidenceRefs}, {Key: "two", EvidenceRefs: ev.EvidenceRefs}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySplitPlan(ctx, s, plan, nil); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("incomplete targets must fail closed, got %v", err)
	}
}
