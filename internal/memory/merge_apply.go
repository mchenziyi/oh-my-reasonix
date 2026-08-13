package memory

import "context"

// ApplyMergePlan materializes one approved MergePlan as a new immutable
// revision. The caller supplies the complete target fact; this function only
// verifies the plan and delegates the single write to FactStore.
func ApplyMergePlan(ctx context.Context, store *FactStore, plan MergePlan, target MemoryRevision) (WriteResult, error) {
	if store == nil || !store.scopeMatches(plan.Primary.Scope) {
		return WriteResult{}, storeError(CodeScopeMismatch, "merge plan scope mismatch")
	}
	if plan.Operation != "merge" || len(plan.Inputs) < 2 || plan.ProposedMemoryID == "" {
		return WriteResult{}, storeError(CodeSchemaInvalid, "merge plan is invalid")
	}
	if target.Scope != plan.Primary.Scope || target.MemoryType != plan.Primary.MemoryType ||
		target.MemoryID != plan.ProposedMemoryID || target.Revision != 1 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "merge target identity is invalid")
	}
	if err := target.Validate(); err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "merge target is invalid")
	}
	hash, err := target.ContentHash()
	if err != nil || hash != target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "merge target hash mismatch")
	}
	for _, ref := range plan.Inputs {
		if ref.Scope != plan.Primary.Scope || ref.MemoryType != plan.Primary.MemoryType {
			return WriteResult{}, storeError(CodeScopeMismatch, "merge input scope mismatch")
		}
		raw, err := store.Get(ctx, FactKindMemoryRevision, revisionKey(ref))
		if err != nil {
			return WriteResult{}, err
		}
		source, err := DecodeStrict[MemoryRevision](raw)
		if err != nil {
			return WriteResult{}, classifyDecodeError(err)
		}
		if source.Scope != ref.Scope || source.MemoryType != ref.MemoryType || source.MemoryID != ref.MemoryID || source.Revision != ref.Revision || source.ContentSHA256 != ref.ContentSHA256 {
			return WriteResult{}, storeError(CodeHashMismatch, "merge input hash mismatch")
		}
	}
	if err := verifyPlanEvidenceRefs(ctx, store, plan.EvidenceRefs); err != nil {
		return WriteResult{}, err
	}
	return store.Put(ctx, target)
}
