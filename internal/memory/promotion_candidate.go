package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// GlobalPromotionCandidate is the durable, global-scope boundary between a
// derived promotion plan and a new Global MemoryRevision. It is not indexed
// and never changes CURRENT by itself.
type GlobalPromotionCandidate struct {
	SchemaVersion                   int                      `json:"schema_version"`
	CandidateID                     string                   `json:"candidate_id"`
	Status                          string                   `json:"status"`
	UsagePolicy                     UsagePolicy              `json:"usage_policy"`
	SourceMemoryRefs                []MemoryRef              `json:"source_memory_refs"`
	SourceProjectFamilyFingerprints []string                 `json:"source_project_family_fingerprints"`
	OutcomeRefs                     []string                 `json:"outcome_refs"`
	EvidenceRefs                    []EvidenceRef            `json:"evidence_refs"`
	ConfirmationSourceRef           *ConfirmationSourceRef   `json:"confirmation_source_ref"`
	CriticJudgmentRefs              []JudgmentRef            `json:"critic_judgment_refs"`
	ProposedAppliesWhen             []ApplicabilityCondition `json:"proposed_applies_when"`
	ProposedDoesNotApplyWhen        []ApplicabilityCondition `json:"proposed_does_not_apply_when"`
	ContentSHA256                   string                   `json:"content_sha256"`
}

const (
	promotionCandidateCollecting = "collecting"
	promotionCandidateEligible   = "eligible"
	promotionCandidateRejected   = "rejected"
)

func (c GlobalPromotionCandidate) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("promotion candidate: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(c.CandidateID, "candidate_id"); err != nil {
		return fmt.Errorf("promotion candidate: %w", err)
	}
	if c.Status != promotionCandidateCollecting && c.Status != promotionCandidateEligible && c.Status != promotionCandidateRejected {
		return errors.New("promotion candidate: invalid status")
	}
	if err := c.UsagePolicy.Validate(); err != nil {
		return fmt.Errorf("promotion candidate: %w", err)
	}
	if len(c.SourceMemoryRefs) < 2 || len(c.SourceMemoryRefs) > maxPayloadRefs {
		return errors.New("promotion candidate: at least two source memory refs are required")
	}
	seenMemory := make(map[string]struct{}, len(c.SourceMemoryRefs))
	for _, ref := range c.SourceMemoryRefs {
		if err := ref.Validate(); err != nil || ref.Scope != ScopeProject {
			return errors.New("promotion candidate: source memory refs must be valid project refs")
		}
		key := memoryRefKey(ref)
		if _, ok := seenMemory[key]; ok {
			return errors.New("promotion candidate: duplicate source memory ref")
		}
		seenMemory[key] = struct{}{}
	}
	if len(c.SourceProjectFamilyFingerprints) > maxPayloadRefs {
		return errors.New("promotion candidate: too many project family fingerprints")
	}
	seenFamilies := make(map[string]struct{}, len(c.SourceProjectFamilyFingerprints))
	for _, fp := range c.SourceProjectFamilyFingerprints {
		if err := validateHash(fp, "project_family_fingerprint"); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
		if _, ok := seenFamilies[fp]; ok {
			return errors.New("promotion candidate: duplicate project family fingerprint")
		}
		seenFamilies[fp] = struct{}{}
	}
	if c.Status == promotionCandidateEligible && len(seenFamilies) < 3 {
		return errors.New("promotion candidate: eligible status requires three project families")
	}
	for _, id := range c.OutcomeRefs {
		if err := validateID(id, "outcome_ref"); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
	}
	if len(c.OutcomeRefs) > maxPayloadRefs || len(c.EvidenceRefs) > maxPayloadRefs || len(c.CriticJudgmentRefs) > maxPayloadRefs {
		return errors.New("promotion candidate: references exceed limit")
	}
	if err := validateCandidatePolicyEvidence(c); err != nil {
		return err
	}
	for _, ref := range c.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
	}
	seenJudgments := make(map[string]struct{}, len(c.CriticJudgmentRefs))
	for _, ref := range c.CriticJudgmentRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
		if _, ok := seenJudgments[ref.JudgmentID]; ok {
			return errors.New("promotion candidate: duplicate critic judgment ref")
		}
		seenJudgments[ref.JudgmentID] = struct{}{}
	}
	if c.ConfirmationSourceRef != nil {
		if err := c.ConfirmationSourceRef.Validate(); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
	}
	if len(c.ProposedAppliesWhen) > maxConditions || len(c.ProposedDoesNotApplyWhen) > maxConditions {
		return errors.New("promotion candidate: too many applicability conditions")
	}
	seenConditions := make(map[string]struct{}, len(c.ProposedAppliesWhen)+len(c.ProposedDoesNotApplyWhen))
	for _, cond := range append(append([]ApplicabilityCondition{}, c.ProposedAppliesWhen...), c.ProposedDoesNotApplyWhen...) {
		if err := cond.Validate(); err != nil {
			return fmt.Errorf("promotion candidate: %w", err)
		}
		if _, ok := seenConditions[cond.ConditionID]; ok {
			return errors.New("promotion candidate: duplicate condition id")
		}
		seenConditions[cond.ConditionID] = struct{}{}
	}
	if err := validateHash(c.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("promotion candidate: %w", err)
	}
	h, err := c.ContentHash()
	if err != nil || h != c.ContentSHA256 {
		return errors.New("promotion candidate: content_sha256 mismatch")
	}
	return nil
}

func validateCandidatePolicyEvidence(c GlobalPromotionCandidate) error {
	switch c.UsagePolicy {
	case UsagePolicyOutcomeAttributed:
		if len(c.EvidenceRefs) != 0 || c.ConfirmationSourceRef != nil {
			return errors.New("promotion candidate: outcome policy cannot carry evidence or confirmation refs")
		}
	case UsagePolicyEvidenceValidated:
		if len(c.OutcomeRefs) != 0 || c.ConfirmationSourceRef != nil {
			return errors.New("promotion candidate: evidence policy cannot carry outcome or confirmation refs")
		}
	case UsagePolicyExplicitConfirmation:
		if len(c.OutcomeRefs) != 0 || len(c.EvidenceRefs) != 0 {
			return errors.New("promotion candidate: confirmation policy cannot carry outcome or evidence refs")
		}
	default:
		return errors.New("promotion candidate: unsupported usage policy")
	}
	return nil
}

func (c GlobalPromotionCandidate) canonMap() (map[string]any, error) {
	refs, err := canonSlice(sortMemoryRefs(c.SourceMemoryRefs))
	if err != nil {
		return nil, err
	}
	families := append([]string(nil), c.SourceProjectFamilyFingerprints...)
	sort.Strings(families)
	outcomes := append([]string(nil), c.OutcomeRefs...)
	sort.Strings(outcomes)
	evidence, err := canonSlice(c.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	judgments, err := canonSlice(c.CriticJudgmentRefs)
	if err != nil {
		return nil, err
	}
	applies, err := canonSlice(c.ProposedAppliesWhen)
	if err != nil {
		return nil, err
	}
	notApplies, err := canonSlice(c.ProposedDoesNotApplyWhen)
	if err != nil {
		return nil, err
	}
	var confirmation any
	if c.ConfirmationSourceRef != nil {
		confirmation, err = c.ConfirmationSourceRef.canonMap()
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"schema_version": c.SchemaVersion, "candidate_id": c.CandidateID, "status": c.Status,
		"usage_policy": string(c.UsagePolicy), "source_memory_refs": refs,
		"source_project_family_fingerprints": families, "outcome_refs": outcomes,
		"evidence_refs": evidence, "confirmation_source_ref": confirmation,
		"critic_judgment_refs": judgments, "proposed_applies_when": applies,
		"proposed_does_not_apply_when": notApplies,
	}, nil
}

func (c GlobalPromotionCandidate) CanonicalBytes() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}
func (c GlobalPromotionCandidate) ContentHash() (string, error) {
	b, err := c.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}
func (c GlobalPromotionCandidate) EncodeCanonical() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := c.ContentHash()
	if err != nil {
		return nil, err
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}
