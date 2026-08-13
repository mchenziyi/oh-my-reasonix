package memory

import (
	"context"
	"strconv"
)

// ApplyGeneralizePlan materializes an approved GeneralizePlan as a new global
// revision. The target must explicitly retain every generalized_from source;
// this function never mutates project facts or lifecycle state.
func ApplyGeneralizePlan(ctx context.Context, projectStore, globalStore *FactStore, plan GeneralizePlan, target MemoryRevision) (WriteResult, error) {
	if projectStore == nil || globalStore == nil || !projectStore.scopeMatches(ScopeProject) || !globalStore.scopeMatches(ScopeGlobal) {
		return WriteResult{}, storeError(CodeScopeMismatch, "generalize store scope mismatch")
	}
	if plan.Operation != "generalize" || !plan.PromotionEligible || plan.Scope != ScopeGlobal || len(plan.Inputs) < 2 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "generalize plan is not eligible")
	}
	if target.Scope != ScopeGlobal || target.MemoryID != plan.ProposedGlobalMemoryID || target.Revision != 1 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "generalize target identity is invalid")
	}
	if err := target.Validate(); err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "generalize target is invalid")
	}
	targetHash, err := target.ContentHash()
	if err != nil || targetHash != target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "generalize target hash mismatch")
	}
	if err := validateGeneralizedFrom(target.Relations, plan.Inputs); err != nil {
		return WriteResult{}, err
	}
	for _, ref := range plan.Inputs {
		if ref.Scope != ScopeProject {
			return WriteResult{}, storeError(CodeScopeMismatch, "generalize source scope mismatch")
		}
		raw, err := projectStore.Get(ctx, FactKindMemoryRevision, revisionKey(ref))
		if err != nil {
			return WriteResult{}, err
		}
		source, err := DecodeStrict[MemoryRevision](raw)
		if err != nil {
			return WriteResult{}, classifyDecodeError(err)
		}
		if source.Scope != ref.Scope || source.MemoryType != ref.MemoryType || source.MemoryID != ref.MemoryID || source.Revision != ref.Revision || source.ContentSHA256 != ref.ContentSHA256 {
			return WriteResult{}, storeError(CodeHashMismatch, "generalize source hash mismatch")
		}
	}
	return globalStore.Put(ctx, target)
}

func validateGeneralizedFrom(relations []MemoryRelation, inputs []MemoryRef) error {
	seen := make(map[string]struct{}, len(relations))
	for _, rel := range relations {
		if rel.Predicate != "generalized_from" {
			continue
		}
		key := generalizeMemoryRefKey(rel.Target)
		if _, ok := seen[key]; ok {
			return storeError(CodeSchemaInvalid, "generalize target has duplicate source relation")
		}
		seen[key] = struct{}{}
	}
	if len(seen) != len(inputs) {
		return storeError(CodeSchemaInvalid, "generalize target must retain every source relation")
	}
	for _, ref := range inputs {
		if _, ok := seen[generalizeMemoryRefKey(ref)]; !ok {
			return storeError(CodeHashMismatch, "generalize target source relation mismatch")
		}
	}
	return nil
}

func generalizeMemoryRefKey(ref MemoryRef) string {
	return string(ref.Scope) + "|" + string(ref.MemoryType) + "|" + ref.MemoryID + "|" + strconv.Itoa(ref.Revision) + "|" + ref.ContentSHA256
}
