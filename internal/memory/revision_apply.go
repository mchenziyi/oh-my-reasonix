package memory

import (
	"context"
	"strconv"
)

// ApplyRevisionPlan is the explicit write boundary for a one-step revision.
// It revalidates every input instead of trusting a previously produced plan;
// the FactStore remains the only persistence path and supplies no-overwrite
// and idempotency semantics.
func ApplyRevisionPlan(ctx context.Context, store *FactStore, plan RevisionPlan, target MemoryRevision) (WriteResult, error) {
	if store == nil || !store.scopeMatches(plan.Source.Scope) || plan.Source.Scope != plan.Target.Scope {
		return WriteResult{}, storeError(CodeScopeMismatch, "revision plan scope mismatch")
	}
	if plan.Operation != "revise" || plan.Source.MemoryType != plan.Target.MemoryType ||
		plan.Source.MemoryID != plan.Target.MemoryID || plan.Target.Revision != plan.Source.Revision+1 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "revision plan identity is invalid")
	}
	if target.Scope != plan.Target.Scope || target.MemoryType != plan.Target.MemoryType ||
		target.MemoryID != plan.Target.MemoryID || target.Revision != plan.Target.Revision ||
		target.ContentSHA256 != plan.Target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "revision plan target mismatch")
	}
	if err := target.Validate(); err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "revision target is invalid")
	}
	hash, err := target.ContentHash()
	if err != nil || hash != target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "revision target hash mismatch")
	}
	raw, err := store.Get(ctx, FactKindMemoryRevision, revisionKey(plan.Source))
	if err != nil {
		return WriteResult{}, err
	}
	source, err := DecodeStrict[MemoryRevision](raw)
	if err != nil {
		return WriteResult{}, classifyDecodeError(err)
	}
	if source.Scope != plan.Source.Scope || source.MemoryType != plan.Source.MemoryType ||
		source.MemoryID != plan.Source.MemoryID || source.Revision != plan.Source.Revision ||
		source.ContentSHA256 != plan.Source.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "revision plan source mismatch")
	}
	if len(plan.EvidenceRefs) == 0 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "revision plan requires evidence")
	}
	if err := verifyPlanEvidenceRefs(ctx, store, plan.EvidenceRefs); err != nil {
		return WriteResult{}, err
	}
	return store.Put(ctx, target)
}

func verifyPlanEvidenceRefs(ctx context.Context, store *FactStore, refs []EvidenceRef) error {
	keys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return err
	}
	found := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil || !store.scopeMatches(ref.Scope) {
			return storeError(CodeSchemaInvalid, "revision evidence reference is invalid")
		}
		found[evidenceRefKey(ref)] = false
	}
	for _, key := range keys {
		raw, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
		if err != nil {
			return err
		}
		ev, err := DecodeStrict[MemoryEvidenceGeneration](raw)
		if err != nil {
			return classifyDecodeError(err)
		}
		for _, ref := range ev.EvidenceRefs {
			if _, ok := found[evidenceRefKey(ref)]; ok {
				found[evidenceRefKey(ref)] = true
			}
		}
	}
	for _, ok := range found {
		if !ok {
			return storeError(CodeNotFound, "revision evidence reference is unavailable")
		}
	}
	return nil
}

func revisionKey(ref MemoryRef) string {
	return ref.MemoryID + "/" + strconv.Itoa(ref.Revision)
}
