package memory

import "context"

// PromotionCandidateSource binds a candidate source to the Project Store
// supplied by the caller. MemoryRef does not contain a filesystem identity,
// so the binding must never be guessed by OMR.
type PromotionCandidateSource struct {
	Ref               MemoryRef
	Store             *FactStore
	FamilyFingerprint string
}

type PromotionCandidateApplyRequest struct {
	Candidate GlobalPromotionCandidate
	Sources   []PromotionCandidateSource
	Target    MemoryRevision
	Global    *FactStore
}

// ApplyPromotionCandidate materializes an eligible candidate as a Global
// probation revision. It is not an approval operation and never changes
// Project facts, Lifecycle, Global CURRENT, or any derived index.
func ApplyPromotionCandidate(ctx context.Context, req PromotionCandidateApplyRequest) (WriteResult, error) {
	return applyPromotionCandidate(ctx, req, false)
}

func applyPromotionCandidateLocked(ctx context.Context, req PromotionCandidateApplyRequest) (WriteResult, error) {
	return applyPromotionCandidate(ctx, req, true)
}

func applyPromotionCandidate(ctx context.Context, req PromotionCandidateApplyRequest, locked bool) (WriteResult, error) {
	if req.Global == nil || !req.Global.scopeMatches(ScopeGlobal) {
		return WriteResult{}, storeError(CodeScopeMismatch, "promotion candidate global store mismatch")
	}
	if err := req.Candidate.Validate(); err != nil || req.Candidate.Status != promotionCandidateEligible {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate is not eligible")
	}
	if len(req.Sources) != len(req.Candidate.SourceMemoryRefs) {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate source bindings are incomplete")
	}
	wantRefs := make(map[string]MemoryRef, len(req.Candidate.SourceMemoryRefs))
	for _, ref := range req.Candidate.SourceMemoryRefs {
		wantRefs[memoryRefKey(ref)] = ref
	}
	wantFamilies := make(map[string]struct{}, len(req.Candidate.SourceProjectFamilyFingerprints))
	for _, family := range req.Candidate.SourceProjectFamilyFingerprints {
		wantFamilies[family] = struct{}{}
	}
	seenRefs := make(map[string]struct{}, len(req.Sources))
	seenFamilies := make(map[string]struct{}, len(req.Sources))
	stores := make([]*FactStore, 0, len(req.Sources))
	for _, source := range req.Sources {
		if source.Store == nil || !source.Store.scopeMatches(ScopeProject) || source.Ref.Scope != ScopeProject {
			return WriteResult{}, storeError(CodeScopeMismatch, "promotion candidate source scope mismatch")
		}
		key := memoryRefKey(source.Ref)
		if _, ok := wantRefs[key]; !ok {
			return WriteResult{}, storeError(CodeHashMismatch, "promotion candidate source binding mismatch")
		}
		if _, ok := seenRefs[key]; ok {
			return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate source binding is duplicated")
		}
		if _, ok := wantFamilies[source.FamilyFingerprint]; !ok {
			return WriteResult{}, storeError(CodeHashMismatch, "promotion candidate family binding mismatch")
		}
		if _, ok := seenFamilies[source.FamilyFingerprint]; ok {
			return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate family binding is duplicated")
		}
		seenRefs[key], seenFamilies[source.FamilyFingerprint] = struct{}{}, struct{}{}
		raw, err := source.Store.Get(ctx, FactKindMemoryRevision, revisionKey(source.Ref))
		if err != nil {
			return WriteResult{}, err
		}
		rev, err := DecodeStrict[MemoryRevision](raw)
		if err != nil || memoryRefFromRevision(rev) != source.Ref {
			return WriteResult{}, storeError(CodeHashMismatch, "promotion candidate source hash mismatch")
		}
		stores = append(stores, source.Store)
	}
	if len(seenFamilies) != len(wantFamilies) || len(seenRefs) != len(wantRefs) {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate source bindings are incomplete")
	}
	if req.Target.Scope != ScopeGlobal || req.Target.Revision != 1 || req.Target.UsagePolicy != req.Candidate.UsagePolicy {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate target is invalid")
	}
	if err := req.Target.Validate(); err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "promotion candidate target is invalid")
	}
	h, err := req.Target.ContentHash()
	if err != nil || h != req.Target.ContentSHA256 {
		return WriteResult{}, storeError(CodeHashMismatch, "promotion candidate target hash mismatch")
	}
	if err := validateGeneralizedFrom(req.Target.Relations, req.Candidate.SourceMemoryRefs); err != nil {
		return WriteResult{}, err
	}
	if len(req.Candidate.EvidenceRefs) > 0 {
		if err := verifyEvidenceRefsAcrossStores(ctx, stores, req.Candidate.EvidenceRefs); err != nil {
			return WriteResult{}, err
		}
	}
	if locked {
		return req.Global.putLocked(ctx, req.Target)
	}
	return req.Global.Put(ctx, req.Target)
}

func verifyEvidenceRefsAcrossStores(ctx context.Context, stores []*FactStore, refs []EvidenceRef) error {
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil || ref.Scope != ScopeProject {
			return storeError(CodeSchemaInvalid, "promotion candidate evidence reference is invalid")
		}
		seen[evidenceRefKey(ref)] = false
	}
	for _, store := range stores {
		if err := markEvidenceRefs(ctx, store, seen); err != nil {
			return err
		}
	}
	for _, found := range seen {
		if !found {
			return storeError(CodeNotFound, "promotion candidate evidence reference is unavailable")
		}
	}
	return nil
}

func markEvidenceRefs(ctx context.Context, store *FactStore, wanted map[string]bool) error {
	keys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return err
	}
	for _, key := range keys {
		raw, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
		if err != nil {
			return err
		}
		generation, err := DecodeStrict[MemoryEvidenceGeneration](raw)
		if err != nil {
			return classifyDecodeError(err)
		}
		for _, ref := range generation.EvidenceRefs {
			if _, ok := wanted[evidenceRefKey(ref)]; ok {
				wanted[evidenceRefKey(ref)] = true
			}
		}
	}
	return nil
}
