package memory

import (
	"context"
	"testing"
	"time"
)

func TestReadMemoryForReviewIsolation(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev := mkStrategy("mem_review_read", "review-read", 1)
	put(t, s, rev)
	put(t, s, governanceEvent("governance_review_freeze", rev, "manual_freeze", "2026-08-16T00:00:00Z"))
	ref := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	now := time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)
	if _, err := ReadMemoryForReview(context.Background(), ReviewReadRequest{Store: s, Target: ref, Now: now}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("normal frozen read must fail: %v", err)
	}
	got, err := ReadMemoryForReview(context.Background(), ReviewReadRequest{Store: s, Target: ref, IncludeFrozen: true, ReviewMode: true, Now: now})
	if err != nil || !got.Frozen || got.State.Lifecycle != LifecycleFrozen {
		t.Fatalf("review read failed: %+v %v", got, err)
	}
	if _, err := ReadMemoryForReview(context.Background(), ReviewReadRequest{Store: s, Target: ref, IncludeFrozen: true, Now: now}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatal("include-frozen without review mode must fail")
	}
}
