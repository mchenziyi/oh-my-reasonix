package memory

import (
	"context"
	"testing"
)

func TestPutBatchPublishesAllOrNoneAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openProject(t, tempRoot(t), Options{})
	a := validRevision()
	b := a
	b.MemoryID = "mem_batch_01"
	b.CanonicalKey = "batch-second"
	b.ContentSHA256 = ""
	hash, err := b.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	b.ContentSHA256 = hash
	results, err := s.PutBatch(ctx, []Fact{a, b})
	if err != nil || len(results) != 2 || results[0].Status != WriteCreated || results[1].Status != WriteCreated {
		t.Fatalf("batch publish failed: results=%+v err=%v", results, err)
	}
	replay, err := s.PutBatch(ctx, []Fact{b, a})
	if err != nil || replay[0].Status != WriteNoop || replay[1].Status != WriteNoop {
		t.Fatalf("batch replay must be noop: results=%+v err=%v", replay, err)
	}
}

func TestPutBatchRejectsConflictBeforeAnyWrite(t *testing.T) {
	ctx := context.Background()
	s := openProject(t, tempRoot(t), Options{})
	a := validRevision()
	conflict := a
	conflict.Title = "conflicting batch identity"
	conflict.ContentSHA256 = ""
	hash, err := conflict.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	conflict.ContentSHA256 = hash
	if _, err := s.PutBatch(ctx, []Fact{a, conflict}); ErrorCode(err) != CodeIdentityConflict {
		t.Fatalf("conflicting identities must fail before write, got %v", err)
	}
	if _, err := s.Get(ctx, FactKindMemoryRevision, revisionKey(memoryRefFromRevision(a))); ErrorCode(err) != CodeNotFound {
		t.Fatalf("conflicting batch wrote a partial fact: %v", err)
	}
}
