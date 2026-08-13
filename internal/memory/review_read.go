package memory

import (
	"context"
	"time"
)

type ReviewReadRequest struct {
	Store         *FactStore
	Target        MemoryRef
	IncludeFrozen bool
	ReviewMode    bool
	Now           time.Time
}

type ReviewMemory struct {
	Revision MemoryRevision     `json:"revision"`
	State    DerivedMemoryState `json:"state"`
	Frozen   bool               `json:"frozen"`
}

// ReadMemoryForReview is a read-only, fixed-revision reader. Frozen and
// archived revisions require the explicit review-mode pair and are never
// returned through normal retrieval APIs.
func ReadMemoryForReview(ctx context.Context, req ReviewReadRequest) (ReviewMemory, error) {
	if req.Store == nil || req.Now.IsZero() || req.Target.Validate() != nil || (req.IncludeFrozen && !req.ReviewMode) {
		return ReviewMemory{}, storeError(CodeDerivedInvalidInput, "review read request is invalid")
	}
	b, err := req.Store.Get(ctx, FactKindMemoryRevision, req.Target.MemoryID+"/"+itoa(req.Target.Revision))
	if err != nil {
		return ReviewMemory{}, err
	}
	rev, err := DecodeStrict[MemoryRevision](b)
	if err != nil {
		return ReviewMemory{}, classifyDecodeError(err)
	}
	if rev.Scope != req.Target.Scope || rev.MemoryType != req.Target.MemoryType || rev.ContentSHA256 != req.Target.ContentSHA256 {
		return ReviewMemory{}, storeError(CodeHashMismatch, "review target does not match stored revision")
	}
	derived, err := DeriveState(ctx, req.Store, DerivedStateRequest{Scope: req.Target.Scope, Revision: req.Target.Revision, Now: req.Now})
	if err != nil {
		return ReviewMemory{}, err
	}
	var state *DerivedMemoryState
	for i := range derived.States {
		if derived.States[i].MemoryID == rev.MemoryID && derived.States[i].Revision == rev.Revision {
			state = &derived.States[i]
			break
		}
	}
	if state == nil {
		return ReviewMemory{}, storeError(CodeNotFound, "review target is unavailable")
	}
	if (state.Lifecycle == LifecycleFrozen || state.Lifecycle == LifecycleArchived) && !req.ReviewMode {
		return ReviewMemory{}, storeError(CodeDerivedInvalidInput, "frozen or archived memory requires review mode")
	}
	if state.Lifecycle == LifecycleFrozen && !req.IncludeFrozen {
		return ReviewMemory{}, storeError(CodeDerivedInvalidInput, "frozen memory requires explicit inclusion")
	}
	return ReviewMemory{Revision: rev, State: *state, Frozen: state.Lifecycle == LifecycleFrozen}, nil
}
