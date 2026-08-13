package memory

import "context"

// ApplyPromotionPlan explicitly materializes a Global Revision after the
// caller supplies the complete target fact. Source facts remain in their
// Project store; this function never changes Lifecycle, CURRENT, or source
// data and delegates the target write to the Global FactStore.
func ApplyPromotionPlan(ctx context.Context, sourceStore, globalStore *FactStore, plan PromotionPlan, policy PolicyFact, target MemoryRevision) (WriteResult, error) {
	if sourceStore == nil || globalStore == nil || !sourceStore.scopeMatches(ScopeProject) || !globalStore.scopeMatches(ScopeGlobal) {
		return WriteResult{}, storeError(CodeScopeMismatch, "promotion store scope mismatch")
	}
	if plan.Operation != "global_promotion" || !plan.PromotionEligible || plan.Scope != ScopeGlobal || len(plan.SourceRefs) < 2 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion plan is not eligible")
	}
	if err := policy.Validate(); err != nil || policy.PolicyType != PolicyTypeTrust || !policy.Config.Trust.PromotionRequiresPolicyEvidence {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion policy is invalid")
	}
	policyHash, err := policy.ContentHash()
	if err != nil || policyHash != plan.PolicyRef.ContentSHA256 || policy.PolicyID != plan.PolicyRef.PolicyID || policy.PolicyType != plan.PolicyRef.PolicyType {
		return WriteResult{}, storeError(CodeHashMismatch, "promotion policy hash mismatch")
	}
	if target.Scope != ScopeGlobal || target.MemoryID != plan.ProposedGlobalMemoryID || target.Revision != 1 {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion target identity is invalid")
	}
	if err := target.Validate(); err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion target is invalid")
	}
	targetHash, err := target.ContentHash()
	if err != nil || targetHash != target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "promotion target hash mismatch")
	}
	for _, ref := range plan.SourceRefs {
		if ref.Scope != ScopeProject {
			return WriteResult{}, storeError(CodeScopeMismatch, "promotion source scope mismatch")
		}
		raw, err := sourceStore.Get(ctx, FactKindMemoryRevision, revisionKey(ref))
		if err != nil {
			return WriteResult{}, err
		}
		rev, err := DecodeStrict[MemoryRevision](raw)
		if err != nil {
			return WriteResult{}, classifyDecodeError(err)
		}
		if rev.Scope != ref.Scope || rev.MemoryType != ref.MemoryType || rev.MemoryID != ref.MemoryID || rev.Revision != ref.Revision || rev.ContentSHA256 != ref.ContentSHA256 {
			return WriteResult{}, storeError(CodeHashMismatch, "promotion source hash mismatch")
		}
	}
	if err := verifyPlanEvidenceRefs(ctx, sourceStore, plan.EvidenceRefs); err != nil {
		return WriteResult{}, err
	}
	return globalStore.Put(ctx, target)
}
