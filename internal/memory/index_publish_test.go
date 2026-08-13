package memory

import (
	"context"
	"strings"
	"testing"
)

func TestPublishIndexGenerationCommitsFixedOKF(t *testing.T) {
	root := tempRoot(t)
	store := openProject(t, root, Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, store, rev, ev)
	result, err := PublishIndexGeneration(context.Background(), store, IndexPublishRequest{OKF: okfRequest(rev, ev), IdempotencyKey: "index_publish_first"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Commit.Status != CommitCommitted || result.Commit.GenerationID == "" || result.CompiledSHA256 == "" {
		t.Fatalf("unexpected publish result: %+v", result)
	}
	current, err := readCurrentForTest(root)
	if err != nil || current.GenerationID != result.Commit.GenerationID {
		t.Fatalf("CURRENT did not point at published generation: %+v %v", current, err)
	}
}

func TestPublishIndexGenerationBindsCompleteRequest(t *testing.T) {
	root := tempRoot(t)
	store := openProject(t, root, Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, store, rev, ev)
	req := okfRequest(rev, ev)
	if _, err := PublishIndexGeneration(context.Background(), store, IndexPublishRequest{OKF: req, IdempotencyKey: "index_publish_binding"}); err != nil {
		t.Fatal(err)
	}
	req.Revisions[0].ContentSHA256 = "sha256_" + strings.Repeat("a", 64)
	if _, err := PublishIndexGeneration(context.Background(), store, IndexPublishRequest{OKF: req, IdempotencyKey: "index_publish_binding"}); ErrorCode(err) != CodeGenerationIdempotency {
		t.Fatalf("changed input with same key must fail closed, got %v", err)
	}
}

func TestPublishIndexGenerationDoesNotReplaceComposite(t *testing.T) {
	store, rev, ev, c, e := compositeFixture(t)
	gs := NewGenerationStore(store)
	tx, err := gs.Begin(context.Background(), BeginGenerationRequest{Scope: ScopeProject, CompilerVersion: CompositeCompilerVersion, CanonicalizationVersion: 1, SchemaVersion: 1, IdempotencyKey: "index_publish_composite_seed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range []Fact{rev, ev, policyOf(PolicyTypeIndex), c, e} {
		if err := gs.PrepareFact(context.Background(), tx, fact); err != nil {
			t.Fatal(err)
		}
	}
	compiled, err := CompileComposite(context.Background(), store, compositeRequest(rev, ev, c, e, tx.GenerationID))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, compiled.Inputs)); err != nil {
		t.Fatal(err)
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, compiled.Outputs); err != nil {
		t.Fatal(err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishIndexGeneration(context.Background(), store, IndexPublishRequest{OKF: okfRequest(rev, ev), IdempotencyKey: "index_publish_over_composite"}); ErrorCode(err) != CodeGenerationCompilerUnavailable {
		t.Fatalf("memory-only publish must reject composite CURRENT, got %v", err)
	}
}
