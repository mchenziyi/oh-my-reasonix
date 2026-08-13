package memory

import "context"

// ApplySplitPlan materializes all split branches as one FactStore batch. All
// branch identities and evidence are checked before any new fact is published.
func ApplySplitPlan(ctx context.Context, store *FactStore, plan SplitPlan, targets []MemoryRevision) ([]WriteResult, error) {
	if store == nil || !store.scopeMatches(plan.Source.Scope) {
		return nil, storeError(CodeScopeMismatch, "split plan scope mismatch")
	}
	if plan.Operation != "split" || len(plan.Branches) < 2 || len(targets) != len(plan.Branches) {
		return nil, storeError(CodeSchemaInvalid, "split plan targets are incomplete")
	}
	raw, err := store.Get(ctx, FactKindMemoryRevision, revisionKey(plan.Source))
	if err != nil {
		return nil, err
	}
	source, err := DecodeStrict[MemoryRevision](raw)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	if source.Scope != plan.Source.Scope || source.MemoryType != plan.Source.MemoryType || source.MemoryID != plan.Source.MemoryID || source.Revision != plan.Source.Revision || source.ContentSHA256 != plan.Source.ContentSHA256 {
		return nil, storeError(CodeHashMismatch, "split source hash mismatch")
	}
	branches := make(map[string]SplitBranch, len(plan.Branches))
	for _, branch := range plan.Branches {
		if _, exists := branches[branch.ProposedMemoryID]; exists {
			return nil, storeError(CodeIdentityConflict, "split plan contains duplicate target")
		}
		branches[branch.ProposedMemoryID] = branch
		if err := verifyPlanEvidenceRefs(ctx, store, branch.EvidenceRefs); err != nil {
			return nil, err
		}
	}
	facts := make([]Fact, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for i, target := range targets {
		branch, ok := branches[target.MemoryID]
		if !ok || target.Scope != plan.Source.Scope || target.MemoryType != source.MemoryType || target.Revision != 1 {
			return nil, storeError(CodeSchemaInvalid, "split target identity is invalid")
		}
		if _, exists := seen[target.MemoryID]; exists {
			return nil, storeError(CodeIdentityConflict, "split targets are duplicated")
		}
		seen[target.MemoryID] = struct{}{}
		if err := target.Validate(); err != nil {
			return nil, storeError(CodeSchemaInvalid, "split target is invalid")
		}
		hash, err := target.ContentHash()
		if err != nil || hash != target.ContentSHA256 {
			return nil, storeError(CodeHashMismatch, "split target hash mismatch")
		}
		if len(branch.EvidenceRefs) == 0 {
			return nil, storeError(CodeSchemaInvalid, "split branch requires evidence")
		}
		facts[i] = target
	}
	if len(seen) != len(branches) {
		return nil, storeError(CodeSchemaInvalid, "split targets do not cover all branches")
	}
	return store.PutBatch(ctx, facts)
}
