package memory

import (
	"context"
	"testing"
	"time"
)

func TestBuildGovernanceEventDeterministic(t *testing.T) {
	ref := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_governance_build", Revision: 1, ContentSHA256: testHash}
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	req := GovernanceRequest{Target: ref, Operation: "pin", Reason: "reviewed", Source: "local_user", Now: now}
	a, err := BuildGovernanceEvent(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildGovernanceEvent(req)
	if err != nil {
		t.Fatal(err)
	}
	if a.EventID == "" || a.EventID != b.EventID || a.CreatedAt != b.CreatedAt {
		t.Fatalf("event is not deterministic: %+v %+v", a, b)
	}
}

func TestCommitGovernanceEventRequiresExactRevision(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev := mkStrategy("mem_governance_commit", "governance-commit", 1)
	put(t, s, rev)
	ref := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	res, err := CommitGovernanceEvent(context.Background(), GovernanceRequest{Store: s, Target: ref, Operation: "pin", Reason: "reviewed", Source: "local_user", Now: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)})
	if err != nil || res.Status != WriteCreated {
		t.Fatalf("commit failed: %+v %v", res, err)
	}
	bad := ref
	bad.Revision = 2
	if _, err := CommitGovernanceEvent(context.Background(), GovernanceRequest{Store: s, Target: bad, Operation: "pin", Reason: "reviewed", Source: "local_user", Now: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)}); ErrorCode(err) == "" {
		t.Fatal("missing revision must fail closed")
	}
}

func TestCommitGovernanceEventUnfreezeRemainsFailClosed(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev := mkStrategy("mem_governance_unfreeze", "governance-unfreeze", 1)
	put(t, s, rev)
	ref := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	_, err := CommitGovernanceEvent(context.Background(), GovernanceRequest{Store: s, Target: ref, Operation: "unfreeze", Reason: "reviewed", Source: "local_user", BasisRefs: []BasisRef{{MemoryRef: &ref}}, Now: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)})
	if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("unfreeze must remain gated, got %v", err)
	}
}

func TestCommitGovernanceEventUnfreezeManualFreezeAllowed(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev := mkStrategy("mem_governance_manual_unfreeze", "governance-manual-unfreeze", 1)
	put(t, s, rev)
	ref := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	put(t, s, governanceEvent("governance_manual_freeze", rev, "manual_freeze", "2026-08-16T00:00:00Z"))
	res, err := CommitGovernanceEvent(context.Background(), GovernanceRequest{Store: s, Target: ref, Operation: "unfreeze", Reason: "reviewed", Source: "local_user", BasisRefs: []BasisRef{{MemoryRef: &ref}}, Now: time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)})
	if err != nil || res.Status != WriteCreated {
		t.Fatalf("manual freeze should be releasable: %+v %v", res, err)
	}
}
