package memory

import (
	"context"
	"testing"
)

func TestGovernanceEventsAreRevisionScoped(t *testing.T) {
	store := openProject(t, tempRoot(t), Options{})
	rev1 := mkStrategy("mem_governance_scope", "governance-scope", 1)
	rev2 := rev1
	rev2.Revision = 2
	rev2.Title = "Strategy revision two"
	rev2 = fillRevisionHash(rev2)
	put(t, store, rev1)
	put(t, store, rev2)
	put(t, store, GovernanceEvent{SchemaVersion: 1, EventID: "governance_pin_old", Scope: ScopeProject, MemoryID: rev1.MemoryID, Revision: 1, Operation: "pin", Reason: "review old revision", Source: "local_user", BasisRefs: []BasisRef{}, CreatedAt: "2026-08-12T00:00:00Z"})
	put(t, store, GovernanceEvent{SchemaVersion: 1, EventID: "governance_archive_old", Scope: ScopeProject, MemoryID: rev1.MemoryID, Revision: 1, Operation: "archive", Reason: "archive old revision", Source: "local_user", BasisRefs: []BasisRef{}, CreatedAt: "2026-08-12T00:00:01Z"})

	result, err := DeriveState(context.Background(), store, DerivedStateRequest{Scope: ScopeProject, Now: deriveNow})
	if err != nil {
		t.Fatal(err)
	}
	latest := stateByID(t, result, rev2.MemoryID)
	if latest.Revision != 2 || latest.Pinned || latest.Archived || latest.Lifecycle == LifecycleArchived {
		t.Fatalf("old-revision governance leaked into revision 2: %+v", latest)
	}
}
