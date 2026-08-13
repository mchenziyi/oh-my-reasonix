package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// PromotionInput is a verified, project-scoped source candidate. It contains
// no project name or path; ProjectFamilyFingerprint is an irreversible
// caller-provided family identity.
type PromotionInput struct {
	Revision                 MemoryRevision
	EvidenceRefs             []EvidenceRef
	TrustStatus              TrustGateStatus
	Frozen                   bool
	Archived                 bool
	ProjectFamilyFingerprint string
	RootTaskIDs              []string
}

type PromotionRequest struct {
	Inputs    []PromotionInput
	Policy    PolicyFact
	PolicyRef PolicyRef
}

// PromotionPlan is derived data. PromotionEligible means only that the
// deterministic gate passed; it does not write or activate a Global memory.
type PromotionPlan struct {
	Operation                string        `json:"operation"`
	Scope                    Scope         `json:"scope"`
	SourceRefs               []MemoryRef   `json:"source_refs"`
	EvidenceRefs             []EvidenceRef `json:"evidence_refs"`
	PolicyRef                PolicyRef     `json:"policy_ref"`
	SourceProjectCount       int           `json:"source_project_count"`
	ProjectFamilyFingerprint string        `json:"project_family_fingerprint"`
	ProposedGlobalMemoryID   string        `json:"proposed_global_memory_id"`
	PromotionEligible        bool          `json:"promotion_eligible"`
	BlockedReasons           []string      `json:"blocked_reasons"`
}

func BuildPromotionPlan(req PromotionRequest) (PromotionPlan, error) {
	if len(req.Inputs) < 2 {
		return PromotionPlan{}, errors.New("promotion plan: at least two project sources are required")
	}
	if err := req.Policy.Validate(); err != nil || req.Policy.PolicyType != PolicyTypeTrust {
		return PromotionPlan{}, errors.New("promotion plan: invalid trust policy")
	}
	if err := req.PolicyRef.Validate(); err != nil || req.PolicyRef.PolicyType != PolicyTypeTrust || req.PolicyRef.PolicyID != req.Policy.PolicyID {
		return PromotionPlan{}, errors.New("promotion plan: policy reference is invalid")
	}
	h, err := req.Policy.ContentHash()
	if err != nil || h != req.PolicyRef.ContentSHA256 {
		return PromotionPlan{}, errors.New("promotion plan: policy reference hash mismatch")
	}
	if !req.Policy.Config.Trust.PromotionRequiresPolicyEvidence {
		return PromotionPlan{}, errors.New("promotion plan: policy does not permit promotion")
	}

	inputs := append([]PromotionInput(nil), req.Inputs...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Revision.MemoryID < inputs[j].Revision.MemoryID })
	refs := make([]MemoryRef, 0, len(inputs))
	evidence := make([]EvidenceRef, 0)
	seenFamily := make(map[string]struct{}, len(inputs))
	seenRoot := make(map[string]struct{})
	seenMemory := make(map[string]struct{}, len(inputs))
	seenEvidence := make(map[string]struct{})
	familyParts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if err := input.Revision.Validate(); err != nil || input.Revision.Scope != ScopeProject {
			return PromotionPlan{}, errors.New("promotion plan: source must be a valid project revision")
		}
		if input.TrustStatus != TrustGateTrusted || input.Frozen || input.Archived {
			return PromotionPlan{}, errors.New("promotion plan: every source must be trusted and available")
		}
		if err := validateHash(input.ProjectFamilyFingerprint, "project_family_fingerprint"); err != nil {
			return PromotionPlan{}, errors.New("promotion plan: invalid project family fingerprint")
		}
		if _, ok := seenFamily[input.ProjectFamilyFingerprint]; ok {
			return PromotionPlan{}, errors.New("promotion plan: sources are not independent")
		}
		seenFamily[input.ProjectFamilyFingerprint] = struct{}{}
		if _, ok := seenMemory[input.Revision.MemoryID]; ok {
			return PromotionPlan{}, errors.New("promotion plan: duplicate memory source")
		}
		seenMemory[input.Revision.MemoryID] = struct{}{}
		if len(input.RootTaskIDs) == 0 {
			return PromotionPlan{}, errors.New("promotion plan: every source requires a root task")
		}
		for _, root := range input.RootTaskIDs {
			if err := validateID(root, "root_task_id"); err != nil {
				return PromotionPlan{}, errors.New("promotion plan: invalid root task")
			}
			if _, ok := seenRoot[root]; ok {
				return PromotionPlan{}, errors.New("promotion plan: sources share a root task")
			}
			seenRoot[root] = struct{}{}
		}
		refs = append(refs, memoryRefFromRevision(input.Revision))
		familyParts = append(familyParts, input.ProjectFamilyFingerprint)
		refsForInput, err := normalizeEvidenceRefs(input.EvidenceRefs, ScopeProject)
		if err != nil || len(refsForInput) == 0 {
			return PromotionPlan{}, errors.New("promotion plan: every source requires evidence")
		}
		for _, ref := range refsForInput {
			key := planEvidenceKey(ref)
			if _, ok := seenEvidence[key]; ok {
				continue
			}
			seenEvidence[key] = struct{}{}
			evidence = append(evidence, ref)
		}
	}
	refs = sortMemoryRefs(refs)
	sort.Strings(familyParts)
	familyHash := NewContentHash([]byte(fmt.Sprintf("%v", familyParts)))
	seed := mustPlanJSON(struct {
		Refs   []MemoryRef
		Policy PolicyRef
	}{refs, req.PolicyRef})
	return PromotionPlan{Operation: "global_promotion", Scope: ScopeGlobal, SourceRefs: refs, EvidenceRefs: evidence, PolicyRef: req.PolicyRef, SourceProjectCount: len(seenFamily), ProjectFamilyFingerprint: familyHash, ProposedGlobalMemoryID: "global_" + shortHash(NewContentHash(seed)), PromotionEligible: true, BlockedReasons: []string{}}, nil
}

func (p PromotionPlan) CanonicalBytes() ([]byte, error) { return json.Marshal(p) }
func (p PromotionPlan) PlanHash() string                { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
