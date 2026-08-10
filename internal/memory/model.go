// Package memory implements the OMR Mnemosyne core fact model: strict
// enumerations, typed references, applicability conditions, immutable fact
// schemas and deterministic canonicalization (MEM-01A). No Store, no CLI, no
// derived state: Lifecycle/Health/Usage are derived elsewhere and must never
// appear as fact fields.
package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Scope of a memory or fact.
type Scope string

const (
	ScopeProject  Scope = "project"
	ScopeGlobal   Scope = "global"
	ScopePortable Scope = "portable"
)

func (s Scope) Validate() error {
	switch s {
	case ScopeProject, ScopeGlobal, ScopePortable:
		return nil
	default:
		return fmt.Errorf("invalid scope %q", s)
	}
}

// MemoryType classifies the knowledge content of a memory.
//
// Note: Architecture v1's type/usage-policy matrix also includes
// `component`, which is required by Component MemoryRefs in
// ApplicabilityCondition, so it is part of the enum.
type MemoryType string

const (
	MemoryTypePattern        MemoryType = "pattern"
	MemoryTypeStrategy       MemoryType = "strategy"
	MemoryTypeDecision       MemoryType = "decision"
	MemoryTypePlaybook       MemoryType = "playbook"
	MemoryTypePreference     MemoryType = "preference"
	MemoryTypeFailureConcept MemoryType = "failure_concept"
	MemoryTypeComponent      MemoryType = "component"
)

func (m MemoryType) Validate() error {
	switch m {
	case MemoryTypePattern, MemoryTypeStrategy, MemoryTypeDecision,
		MemoryTypePlaybook, MemoryTypePreference, MemoryTypeFailureConcept,
		MemoryTypeComponent:
		return nil
	default:
		return fmt.Errorf("invalid memory_type %q", m)
	}
}

// UsagePolicy selects the evidence protocol for promotion/degradation/freeze.
type UsagePolicy string

const (
	UsagePolicyOutcomeAttributed    UsagePolicy = "outcome_attributed"
	UsagePolicyEvidenceValidated    UsagePolicy = "evidence_validated"
	UsagePolicyExplicitConfirmation UsagePolicy = "explicit_confirmation"
)

func (p UsagePolicy) Validate() error {
	switch p {
	case UsagePolicyOutcomeAttributed, UsagePolicyEvidenceValidated, UsagePolicyExplicitConfirmation:
		return nil
	default:
		return fmt.Errorf("invalid usage_policy %q", p)
	}
}

// usagePolicyAllowed implements the frozen MemoryType x UsagePolicy matrix.
func usagePolicyAllowed(mt MemoryType, up UsagePolicy) bool {
	switch mt {
	case MemoryTypeStrategy, MemoryTypePlaybook:
		return up == UsagePolicyOutcomeAttributed
	case MemoryTypeComponent, MemoryTypePattern, MemoryTypeFailureConcept:
		return up == UsagePolicyEvidenceValidated
	case MemoryTypePreference:
		return up == UsagePolicyExplicitConfirmation
	case MemoryTypeDecision:
		return up == UsagePolicyEvidenceValidated || up == UsagePolicyExplicitConfirmation
	default:
		return false
	}
}

// JudgmentType discriminates the JudgmentFact payload union.
type JudgmentType string

const (
	JudgmentTypeConfirmation          JudgmentType = "confirmation"
	JudgmentTypeAttributionOverride   JudgmentType = "attribution_override"
	JudgmentTypeRetrievalRelevance    JudgmentType = "retrieval_relevance"
	JudgmentTypeContextApplicability  JudgmentType = "context_applicability"
	JudgmentTypeContentClassification JudgmentType = "content_classification"
	JudgmentTypeEvidenceTrust         JudgmentType = "evidence_trust"
	JudgmentTypeFreshnessEvaluation   JudgmentType = "freshness_evaluation"
)

func (j JudgmentType) Validate() error {
	switch j {
	case JudgmentTypeConfirmation, JudgmentTypeAttributionOverride,
		JudgmentTypeRetrievalRelevance, JudgmentTypeContextApplicability,
		JudgmentTypeContentClassification, JudgmentTypeEvidenceTrust,
		JudgmentTypeFreshnessEvaluation:
		return nil
	default:
		return fmt.Errorf("invalid judgment_type %q", j)
	}
}

// PolicyType discriminates the PolicyFact config union.
type PolicyType string

const (
	PolicyTypeFreshness         PolicyType = "freshness"
	PolicyTypeTrust             PolicyType = "trust"
	PolicyTypeContentClassifier PolicyType = "content_classifier"
	PolicyTypeIndex             PolicyType = "index"
	PolicyTypeBenchmark         PolicyType = "benchmark"
)

func (p PolicyType) Validate() error {
	switch p {
	case PolicyTypeFreshness, PolicyTypeTrust, PolicyTypeContentClassifier,
		PolicyTypeIndex, PolicyTypeBenchmark:
		return nil
	default:
		return fmt.Errorf("invalid policy_type %q", p)
	}
}

// MemoryRelation links a memory to a target memory with a stable predicate.
type MemoryRelation struct {
	Predicate string    `json:"predicate"`
	Target    MemoryRef `json:"target"`
}

func (r MemoryRelation) Validate() error {
	if err := validateID(r.Predicate, "predicate"); err != nil {
		return fmt.Errorf("memory relation: %w", err)
	}
	return r.Target.Validate()
}

func (r MemoryRelation) canonMap() (map[string]any, error) {
	target, err := r.Target.canonMap()
	if err != nil {
		return nil, err
	}
	return map[string]any{"predicate": r.Predicate, "target": target}, nil
}

// MemoryRevision is the immutable canonical fact for one version of a
// memory's knowledge content.
type MemoryRevision struct {
	SchemaVersion         int                      `json:"schema_version"`
	MemoryID              string                   `json:"memory_id"`
	MemoryType            MemoryType               `json:"memory_type"`
	Scope                 Scope                    `json:"scope"`
	CanonicalKey          string                   `json:"canonical_key"`
	Revision              int                      `json:"revision"`
	UsagePolicy           UsagePolicy              `json:"usage_policy"`
	ConfirmationSourceRef *ConfirmationSourceRef   `json:"confirmation_source_ref"`
	Title                 string                   `json:"title"`
	Summary               string                   `json:"summary"`
	AppliesWhen           []ApplicabilityCondition `json:"applies_when"`
	DoesNotApplyWhen      []ApplicabilityCondition `json:"does_not_apply_when"`
	FailureConceptRefs    []MemoryRef              `json:"failure_concept_refs"`
	Relations             []MemoryRelation         `json:"relations"`
	Aliases               []string                 `json:"aliases"`
	ContentSHA256         string                   `json:"content_sha256"`
	CreatedAt             string                   `json:"created_at"`
}

func (r MemoryRevision) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("memory revision: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(r.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if err := r.MemoryType.Validate(); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if err := r.UsagePolicy.Validate(); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if !usagePolicyAllowed(r.MemoryType, r.UsagePolicy) {
		return fmt.Errorf("memory revision: usage_policy %q is not allowed for memory_type %q", r.UsagePolicy, r.MemoryType)
	}
	if err := validateCanonicalKey(r.CanonicalKey); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if r.Revision < 1 {
		return errors.New("memory revision: revision must be >= 1")
	}
	if r.UsagePolicy == UsagePolicyExplicitConfirmation {
		if r.ConfirmationSourceRef == nil {
			return errors.New("memory revision: explicit_confirmation requires confirmation_source_ref")
		}
		if err := r.ConfirmationSourceRef.Validate(); err != nil {
			return fmt.Errorf("memory revision: %w", err)
		}
	} else if r.ConfirmationSourceRef != nil {
		return errors.New("memory revision: confirmation_source_ref must be null unless usage_policy is explicit_confirmation")
	}
	if err := validateTitle(r.Title); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if err := validateSummary(r.Summary); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if len(r.AppliesWhen) > maxConditions {
		return fmt.Errorf("memory revision: applies_when exceeds %d conditions", maxConditions)
	}
	if len(r.DoesNotApplyWhen) > maxConditions {
		return fmt.Errorf("memory revision: does_not_apply_when exceeds %d conditions", maxConditions)
	}
	seen := make(map[string]struct{}, len(r.AppliesWhen)+len(r.DoesNotApplyWhen))
	for _, cond := range append(append([]ApplicabilityCondition{}, r.AppliesWhen...), r.DoesNotApplyWhen...) {
		if err := cond.Validate(); err != nil {
			return fmt.Errorf("memory revision: %w", err)
		}
		if _, dup := seen[cond.ConditionID]; dup {
			return fmt.Errorf("memory revision: duplicate condition_id %q", cond.ConditionID)
		}
		seen[cond.ConditionID] = struct{}{}
	}
	if len(r.FailureConceptRefs) > maxRefs {
		return fmt.Errorf("memory revision: failure_concept_refs exceeds %d refs", maxRefs)
	}
	for _, ref := range r.FailureConceptRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("memory revision: %w", err)
		}
		if ref.MemoryType != MemoryTypeFailureConcept {
			return errors.New("memory revision: failure_concept_refs must reference memory_type failure_concept")
		}
	}
	if len(r.Relations) > maxRelations {
		return fmt.Errorf("memory revision: relations exceeds %d entries", maxRelations)
	}
	for _, rel := range r.Relations {
		if err := rel.Validate(); err != nil {
			return fmt.Errorf("memory revision: %w", err)
		}
	}
	if len(r.Aliases) > maxAliases {
		return fmt.Errorf("memory revision: aliases exceeds %d entries", maxAliases)
	}
	for _, a := range r.Aliases {
		if err := validateAlias(a); err != nil {
			return fmt.Errorf("memory revision: %w", err)
		}
	}
	if err := validateTime(r.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if err := validateHash(r.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	h, err := r.ContentHash()
	if err != nil {
		return fmt.Errorf("memory revision: %w", err)
	}
	if r.ContentSHA256 != h {
		return errors.New("memory revision: content_sha256 mismatch")
	}
	return nil
}

func (r MemoryRevision) canonMap() (map[string]any, error) {
	applies, err := canonSlice(r.AppliesWhen)
	if err != nil {
		return nil, err
	}
	doesNot, err := canonSlice(r.DoesNotApplyWhen)
	if err != nil {
		return nil, err
	}
	fcRefs, err := canonSlice(r.FailureConceptRefs)
	if err != nil {
		return nil, err
	}
	relations, err := canonSlice(r.Relations)
	if err != nil {
		return nil, err
	}
	aliases, err := canonStrings(r.Aliases)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(r.CreatedAt)
	if err != nil {
		return nil, err
	}
	var confirmation any
	if r.ConfirmationSourceRef != nil {
		if confirmation, err = r.ConfirmationSourceRef.canonMap(); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"schema_version":          r.SchemaVersion,
		"memory_id":               r.MemoryID,
		"memory_type":             string(r.MemoryType),
		"scope":                   string(r.Scope),
		"canonical_key":           r.CanonicalKey,
		"revision":                r.Revision,
		"usage_policy":            string(r.UsagePolicy),
		"confirmation_source_ref": confirmation,
		"title":                   r.Title,
		"summary":                 r.Summary,
		"applies_when":            applies,
		"does_not_apply_when":     doesNot,
		"failure_concept_refs":    fcRefs,
		"relations":               relations,
		"aliases":                 aliases,
		"created_at":              created,
	}, nil
}

func (r MemoryRevision) CanonicalBytes() ([]byte, error) {
	m, err := r.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (r MemoryRevision) ContentHash() (string, error) {
	b, err := r.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (r MemoryRevision) EncodeCanonical() ([]byte, error) {
	m, err := r.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := r.ContentHash()
	if err != nil {
		return nil, err
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}

// MemoryEvidenceGeneration is the immutable canonical fact for one set of
// supporting evidence of a MemoryRevision.
type MemoryEvidenceGeneration struct {
	SchemaVersion              int           `json:"schema_version"`
	MemoryID                   string        `json:"memory_id"`
	Revision                   int           `json:"revision"`
	EvidenceGeneration         int           `json:"evidence_generation"`
	EvidenceRefs               []EvidenceRef `json:"evidence_refs"`
	EvidenceSetSHA256          string        `json:"evidence_set_sha256"`
	PreviousEvidenceGeneration *int          `json:"previous_evidence_generation"`
	TransactionID              string        `json:"transaction_id"`
	CreatedAt                  string        `json:"created_at"`
}

func (e MemoryEvidenceGeneration) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("memory evidence generation: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(e.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if e.Revision < 1 {
		return errors.New("memory evidence generation: revision must be >= 1")
	}
	if e.EvidenceGeneration < 1 {
		return errors.New("memory evidence generation: evidence_generation must be >= 1")
	}
	if e.PreviousEvidenceGeneration != nil {
		if *e.PreviousEvidenceGeneration < 1 {
			return errors.New("memory evidence generation: previous_evidence_generation must be >= 1 when set")
		}
		if *e.PreviousEvidenceGeneration >= e.EvidenceGeneration {
			return errors.New("memory evidence generation: previous_evidence_generation must be less than evidence_generation")
		}
	}
	if len(e.EvidenceRefs) > maxRefs {
		return fmt.Errorf("memory evidence generation: evidence_refs exceeds %d refs", maxRefs)
	}
	for _, ref := range e.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("memory evidence generation: %w", err)
		}
	}
	if err := validateID(e.TransactionID, "transaction_id"); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if err := validateTime(e.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if err := validateHash(e.EvidenceSetSHA256, "evidence_set_sha256"); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	h, err := e.ContentHash()
	if err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if e.EvidenceSetSHA256 != h {
		return errors.New("memory evidence generation: evidence_set_sha256 mismatch")
	}
	return nil
}

func (e MemoryEvidenceGeneration) canonMap() (map[string]any, error) {
	refs, err := canonSlice(e.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(e.CreatedAt)
	if err != nil {
		return nil, err
	}
	var previous any
	if e.PreviousEvidenceGeneration != nil {
		previous = *e.PreviousEvidenceGeneration
	}
	return map[string]any{
		"schema_version":               e.SchemaVersion,
		"memory_id":                    e.MemoryID,
		"revision":                     e.Revision,
		"evidence_generation":          e.EvidenceGeneration,
		"evidence_refs":                refs,
		"previous_evidence_generation": previous,
		"transaction_id":               e.TransactionID,
		"created_at":                   created,
	}, nil
}

func (e MemoryEvidenceGeneration) CanonicalBytes() ([]byte, error) {
	m, err := e.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (e MemoryEvidenceGeneration) ContentHash() (string, error) {
	b, err := e.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (e MemoryEvidenceGeneration) EncodeCanonical() ([]byte, error) {
	m, err := e.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := e.ContentHash()
	if err != nil {
		return nil, err
	}
	m["evidence_set_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}

// JudgmentSubject is a discriminated union identifying the subject of a
// JudgmentFact.
type JudgmentSubject struct {
	SubjectType      string       `json:"subject_type"`
	MemoryRef        *MemoryRef   `json:"memory_ref,omitempty"`
	OutcomeID        string       `json:"outcome_id,omitempty"`
	EvidenceRef      *EvidenceRef `json:"evidence_ref,omitempty"`
	TargetContextRef string       `json:"target_context_ref,omitempty"`
}

func (s JudgmentSubject) Validate() error {
	switch s.SubjectType {
	case "memory_revision":
		if s.MemoryRef == nil {
			return errors.New("judgment subject: memory_revision requires memory_ref")
		}
		if err := s.MemoryRef.Validate(); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.OutcomeID != "" || s.EvidenceRef != nil || s.TargetContextRef != "" {
			return errors.New("judgment subject: memory_revision must not carry other fields")
		}
	case "memory_outcome":
		if err := validateID(s.OutcomeID, "outcome_id"); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.MemoryRef != nil || s.EvidenceRef != nil || s.TargetContextRef != "" {
			return errors.New("judgment subject: memory_outcome must not carry other fields")
		}
	case "evidence":
		if s.EvidenceRef == nil {
			return errors.New("judgment subject: evidence requires evidence_ref")
		}
		if err := s.EvidenceRef.Validate(); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.MemoryRef != nil || s.OutcomeID != "" || s.TargetContextRef != "" {
			return errors.New("judgment subject: evidence must not carry other fields")
		}
	case "context":
		if s.MemoryRef == nil {
			return errors.New("judgment subject: context requires memory_ref")
		}
		if err := s.MemoryRef.Validate(); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if err := validateID(s.TargetContextRef, "target_context_ref"); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.OutcomeID != "" || s.EvidenceRef != nil {
			return errors.New("judgment subject: context must not carry other fields")
		}
	default:
		return fmt.Errorf("invalid subject_type %q", s.SubjectType)
	}
	return nil
}

func (s JudgmentSubject) canonMap() (map[string]any, error) {
	m := map[string]any{"subject_type": s.SubjectType}
	switch s.SubjectType {
	case "memory_revision":
		if s.MemoryRef == nil {
			return nil, errors.New("judgment subject: memory_revision requires memory_ref")
		}
		ref, err := s.MemoryRef.canonMap()
		if err != nil {
			return nil, err
		}
		m["memory_ref"] = ref
	case "memory_outcome":
		m["outcome_id"] = s.OutcomeID
	case "evidence":
		if s.EvidenceRef == nil {
			return nil, errors.New("judgment subject: evidence requires evidence_ref")
		}
		ref, err := s.EvidenceRef.canonMap()
		if err != nil {
			return nil, err
		}
		m["evidence_ref"] = ref
	case "context":
		if s.MemoryRef == nil {
			return nil, errors.New("judgment subject: context requires memory_ref")
		}
		ref, err := s.MemoryRef.canonMap()
		if err != nil {
			return nil, err
		}
		m["memory_ref"] = ref
		m["target_context_ref"] = s.TargetContextRef
	default:
		return nil, fmt.Errorf("invalid subject_type %q", s.SubjectType)
	}
	return m, nil
}

// JudgmentSource identifies who made the judgment.
type JudgmentSource struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

func (s JudgmentSource) Validate() error {
	if err := validateID(s.SourceType, "source_type"); err != nil {
		return fmt.Errorf("judgment source: %w", err)
	}
	if err := validateID(s.SourceID, "source_id"); err != nil {
		return fmt.Errorf("judgment source: %w", err)
	}
	return nil
}

func (s JudgmentSource) canonMap() (map[string]any, error) {
	return map[string]any{"source_type": s.SourceType, "source_id": s.SourceID}, nil
}

// Subtype payloads of JudgmentFact (strict discriminated union).

// ConfirmationPayload: status "confirmed" or "revoked" (revocation is the
// only schema-registered value beyond the Architecture example; a revoking
// judgment must also set SupersedesJudgmentRef).
type ConfirmationPayload struct {
	Status        string `json:"status"`
	DeclaredScope Scope  `json:"declared_scope"`
}

func (p ConfirmationPayload) Validate() error {
	switch p.Status {
	case "confirmed", "revoked":
	default:
		return fmt.Errorf("confirmation: invalid status %q", p.Status)
	}
	return p.DeclaredScope.Validate()
}

func (p ConfirmationPayload) canonMap() (map[string]any, error) {
	return map[string]any{"status": p.Status, "declared_scope": string(p.DeclaredScope)}, nil
}

// AttributionOverridePayload records a corrected effect attribution.
// previous_effect and new_effect are a frozen enum: helped | neutral |
// harmed | unknown. Any other value is rejected by strict schema validation;
// no normalization or substitution is applied.
type AttributionOverridePayload struct {
	PreviousEffect string `json:"previous_effect"`
	NewEffect      string `json:"new_effect"`
	Reason         string `json:"reason"`
}

// validEffect enforces the frozen effect vocabulary of the attribution
// protocol. No other string is accepted and no normalization is applied.
func validEffect(s, what string) error {
	switch s {
	case "helped", "neutral", "harmed", "unknown":
		return nil
	default:
		return fmt.Errorf("attribution override: %s must be one of helped|neutral|harmed|unknown, got %q", what, s)
	}
}

func (p AttributionOverridePayload) Validate() error {
	if err := validEffect(p.PreviousEffect, "previous_effect"); err != nil {
		return err
	}
	if err := validEffect(p.NewEffect, "new_effect"); err != nil {
		return err
	}
	return validateText(p.Reason, maxReasonLen, "reason", true)
}

func (p AttributionOverridePayload) canonMap() (map[string]any, error) {
	return map[string]any{"previous_effect": p.PreviousEffect, "new_effect": p.NewEffect, "reason": p.Reason}, nil
}

type RetrievalRelevancePayload struct {
	Result              string        `json:"result"`
	ExpectedMemoryRefs  []MemoryRef   `json:"expected_memory_refs"`
	RetrievedMemoryRefs []MemoryRef   `json:"retrieved_memory_refs"`
	EvidenceRefs        []EvidenceRef `json:"evidence_refs"`
}

func (p RetrievalRelevancePayload) Validate() error {
	switch p.Result {
	case "hit_relevant", "hit_irrelevant", "missed_relevant", "no_relevant_memory", "unknown", "unavailable":
	default:
		return fmt.Errorf("retrieval relevance: invalid result %q", p.Result)
	}
	for _, list := range [][]MemoryRef{p.ExpectedMemoryRefs, p.RetrievedMemoryRefs} {
		if len(list) > maxPayloadRefs {
			return fmt.Errorf("retrieval relevance: ref list exceeds %d entries", maxPayloadRefs)
		}
		for _, ref := range list {
			if err := ref.Validate(); err != nil {
				return fmt.Errorf("retrieval relevance: %w", err)
			}
		}
	}
	if len(p.EvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("retrieval relevance: evidence_refs exceeds %d entries", maxPayloadRefs)
	}
	for _, ref := range p.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("retrieval relevance: %w", err)
		}
	}
	return nil
}

func (p RetrievalRelevancePayload) canonMap() (map[string]any, error) {
	expected, err := canonSlice(p.ExpectedMemoryRefs)
	if err != nil {
		return nil, err
	}
	retrieved, err := canonSlice(p.RetrievedMemoryRefs)
	if err != nil {
		return nil, err
	}
	evidence, err := canonSlice(p.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"result":                p.Result,
		"expected_memory_refs":  expected,
		"retrieved_memory_refs": retrieved,
		"evidence_refs":         evidence,
	}, nil
}

type ContextApplicabilityPayload struct {
	Result               string        `json:"result"`
	RequiredConditionIDs []string      `json:"required_condition_ids"`
	EvidenceRefs         []EvidenceRef `json:"evidence_refs"`
}

func (p ContextApplicabilityPayload) Validate() error {
	switch p.Result {
	case "exact", "applicable", "conditionally_applicable", "not_applicable", "unknown":
	default:
		return fmt.Errorf("context applicability: invalid result %q", p.Result)
	}
	if p.Result == "conditionally_applicable" && len(p.RequiredConditionIDs) == 0 {
		return errors.New("context applicability: conditionally_applicable requires required_condition_ids")
	}
	for _, id := range p.RequiredConditionIDs {
		if err := validateID(id, "required_condition_id"); err != nil {
			return fmt.Errorf("context applicability: %w", err)
		}
	}
	if len(p.EvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("context applicability: evidence_refs exceeds %d entries", maxPayloadRefs)
	}
	for _, ref := range p.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("context applicability: %w", err)
		}
	}
	return nil
}

func (p ContextApplicabilityPayload) canonMap() (map[string]any, error) {
	ids, err := canonStrings(p.RequiredConditionIDs)
	if err != nil {
		return nil, err
	}
	evidence, err := canonSlice(p.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"result":                 p.Result,
		"required_condition_ids": ids,
		"evidence_refs":          evidence,
	}, nil
}

type ContentClassificationPayload struct {
	EvidenceRef                  EvidenceRef `json:"evidence_ref"`
	ContainsInstructionalContent bool        `json:"contains_instructional_content"`
	ContainsSensitiveContent     bool        `json:"contains_sensitive_content"`
	ClassifierPolicyRef          PolicyRef   `json:"classifier_policy_ref"`
}

func (p ContentClassificationPayload) Validate() error {
	if err := p.EvidenceRef.Validate(); err != nil {
		return fmt.Errorf("content classification: %w", err)
	}
	if err := p.ClassifierPolicyRef.Validate(); err != nil {
		return fmt.Errorf("content classification: %w", err)
	}
	if p.ClassifierPolicyRef.PolicyType != PolicyTypeContentClassifier {
		return errors.New("content classification: classifier_policy_ref must be a content_classifier policy")
	}
	return nil
}

func (p ContentClassificationPayload) canonMap() (map[string]any, error) {
	evidence, err := p.EvidenceRef.canonMap()
	if err != nil {
		return nil, err
	}
	policy, err := p.ClassifierPolicyRef.canonMap()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"evidence_ref":                   evidence,
		"contains_instructional_content": p.ContainsInstructionalContent,
		"contains_sensitive_content":     p.ContainsSensitiveContent,
		"classifier_policy_ref":          policy,
	}, nil
}

type EvidenceTrustPayload struct {
	EvidenceRef                 EvidenceRef `json:"evidence_ref"`
	ContentClassificationRef    JudgmentRef `json:"content_classification_ref"`
	TrustPolicyRef              PolicyRef   `json:"trust_policy_ref"`
	EvaluatedAt                 string      `json:"evaluated_at"`
	InstructionalContentAllowed bool        `json:"instructional_content_allowed"`
	PromotionEligible           bool        `json:"promotion_eligible"`
}

func (p EvidenceTrustPayload) Validate() error {
	if err := p.EvidenceRef.Validate(); err != nil {
		return fmt.Errorf("evidence trust: %w", err)
	}
	if err := p.ContentClassificationRef.Validate(); err != nil {
		return fmt.Errorf("evidence trust: %w", err)
	}
	if p.ContentClassificationRef.JudgmentType != JudgmentTypeContentClassification {
		return errors.New("evidence trust: content_classification_ref must be a content_classification judgment")
	}
	if err := p.TrustPolicyRef.Validate(); err != nil {
		return fmt.Errorf("evidence trust: %w", err)
	}
	if p.TrustPolicyRef.PolicyType != PolicyTypeTrust {
		return errors.New("evidence trust: trust_policy_ref must be a trust policy")
	}
	return validateTime(p.EvaluatedAt, "evaluated_at")
}

func (p EvidenceTrustPayload) canonMap() (map[string]any, error) {
	evidence, err := p.EvidenceRef.canonMap()
	if err != nil {
		return nil, err
	}
	classification, err := p.ContentClassificationRef.canonMap()
	if err != nil {
		return nil, err
	}
	policy, err := p.TrustPolicyRef.canonMap()
	if err != nil {
		return nil, err
	}
	evaluatedAt, err := normalizeTime(p.EvaluatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"evidence_ref":                  evidence,
		"content_classification_ref":    classification,
		"trust_policy_ref":              policy,
		"evaluated_at":                  evaluatedAt,
		"instructional_content_allowed": p.InstructionalContentAllowed,
		"promotion_eligible":            p.PromotionEligible,
	}, nil
}

type FreshnessEvaluationPayload struct {
	MemoryRef          MemoryRef  `json:"memory_ref"`
	Result             string     `json:"result"`
	EvaluatedAt        string     `json:"evaluated_at"`
	FreshnessPolicyRef PolicyRef  `json:"freshness_policy_ref"`
	BasisRefs          []BasisRef `json:"basis_refs"`
}

func (p FreshnessEvaluationPayload) Validate() error {
	if err := p.MemoryRef.Validate(); err != nil {
		return fmt.Errorf("freshness evaluation: %w", err)
	}
	switch p.Result {
	case "fresh", "aging", "needs_revalidation":
	default:
		return fmt.Errorf("freshness evaluation: invalid result %q", p.Result)
	}
	if err := validateTime(p.EvaluatedAt, "evaluated_at"); err != nil {
		return fmt.Errorf("freshness evaluation: %w", err)
	}
	if err := p.FreshnessPolicyRef.Validate(); err != nil {
		return fmt.Errorf("freshness evaluation: %w", err)
	}
	if p.FreshnessPolicyRef.PolicyType != PolicyTypeFreshness {
		return errors.New("freshness evaluation: freshness_policy_ref must be a freshness policy")
	}
	if len(p.BasisRefs) > maxBasisRefs {
		return fmt.Errorf("freshness evaluation: basis_refs exceeds %d entries", maxBasisRefs)
	}
	for _, b := range p.BasisRefs {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("freshness evaluation: %w", err)
		}
	}
	return nil
}

func (p FreshnessEvaluationPayload) canonMap() (map[string]any, error) {
	memory, err := p.MemoryRef.canonMap()
	if err != nil {
		return nil, err
	}
	policy, err := p.FreshnessPolicyRef.canonMap()
	if err != nil {
		return nil, err
	}
	evaluatedAt, err := normalizeTime(p.EvaluatedAt)
	if err != nil {
		return nil, err
	}
	basis, err := canonSlice(p.BasisRefs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"memory_ref":           memory,
		"result":               p.Result,
		"evaluated_at":         evaluatedAt,
		"freshness_policy_ref": policy,
		"basis_refs":           basis,
	}, nil
}

// JudgmentFact is the immutable canonical fact for Confirmation,
// Attribution Override and the other registered judgment subtypes.
type JudgmentFact struct {
	SchemaVersion         int                           `json:"schema_version"`
	JudgmentID            string                        `json:"judgment_id"`
	JudgmentType          JudgmentType                  `json:"judgment_type"`
	Scope                 Scope                         `json:"scope"`
	Subject               JudgmentSubject               `json:"subject"`
	Source                JudgmentSource                `json:"source"`
	Confirmation          *ConfirmationPayload          `json:"confirmation,omitempty"`
	AttributionOverride   *AttributionOverridePayload   `json:"attribution_override,omitempty"`
	RetrievalRelevance    *RetrievalRelevancePayload    `json:"retrieval_relevance,omitempty"`
	ContextApplicability  *ContextApplicabilityPayload  `json:"context_applicability,omitempty"`
	ContentClassification *ContentClassificationPayload `json:"content_classification,omitempty"`
	EvidenceTrust         *EvidenceTrustPayload         `json:"evidence_trust,omitempty"`
	FreshnessEvaluation   *FreshnessEvaluationPayload   `json:"freshness_evaluation,omitempty"`
	SupersedesJudgmentRef *JudgmentRef                  `json:"supersedes_judgment_ref"`
	BasisRefs             []BasisRef                    `json:"basis_refs"`
	ContentSHA256         string                        `json:"content_sha256"`
	CreatedAt             string                        `json:"created_at"`
}

func (j JudgmentFact) validatePayload() error {
	nonNil := 0
	if j.Confirmation != nil {
		nonNil++
	}
	if j.AttributionOverride != nil {
		nonNil++
	}
	if j.RetrievalRelevance != nil {
		nonNil++
	}
	if j.ContextApplicability != nil {
		nonNil++
	}
	if j.ContentClassification != nil {
		nonNil++
	}
	if j.EvidenceTrust != nil {
		nonNil++
	}
	if j.FreshnessEvaluation != nil {
		nonNil++
	}
	if nonNil != 1 {
		return fmt.Errorf("judgment: exactly one subtype payload must be set, got %d", nonNil)
	}
	var err error
	switch j.JudgmentType {
	case JudgmentTypeConfirmation:
		if j.Confirmation == nil {
			return errors.New("judgment: confirmation payload missing")
		}
		err = j.Confirmation.Validate()
	case JudgmentTypeAttributionOverride:
		if j.AttributionOverride == nil {
			return errors.New("judgment: attribution_override payload missing")
		}
		err = j.AttributionOverride.Validate()
	case JudgmentTypeRetrievalRelevance:
		if j.RetrievalRelevance == nil {
			return errors.New("judgment: retrieval_relevance payload missing")
		}
		err = j.RetrievalRelevance.Validate()
	case JudgmentTypeContextApplicability:
		if j.ContextApplicability == nil {
			return errors.New("judgment: context_applicability payload missing")
		}
		err = j.ContextApplicability.Validate()
	case JudgmentTypeContentClassification:
		if j.ContentClassification == nil {
			return errors.New("judgment: content_classification payload missing")
		}
		err = j.ContentClassification.Validate()
	case JudgmentTypeEvidenceTrust:
		if j.EvidenceTrust == nil {
			return errors.New("judgment: evidence_trust payload missing")
		}
		err = j.EvidenceTrust.Validate()
	case JudgmentTypeFreshnessEvaluation:
		if j.FreshnessEvaluation == nil {
			return errors.New("judgment: freshness_evaluation payload missing")
		}
		err = j.FreshnessEvaluation.Validate()
	default:
		return fmt.Errorf("judgment: invalid judgment_type %q", j.JudgmentType)
	}
	if err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	return nil
}

func (j JudgmentFact) Validate() error {
	if j.SchemaVersion != SchemaVersion {
		return fmt.Errorf("judgment: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(j.JudgmentID, "judgment_id"); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := j.JudgmentType.Validate(); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := j.Scope.Validate(); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := j.Subject.Validate(); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := j.Source.Validate(); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := j.validatePayload(); err != nil {
		return err
	}
	if j.SupersedesJudgmentRef != nil {
		if err := j.SupersedesJudgmentRef.Validate(); err != nil {
			return fmt.Errorf("judgment: %w", err)
		}
		if j.SupersedesJudgmentRef.JudgmentType != j.JudgmentType {
			return errors.New("judgment: supersedes_judgment_ref must reference the same judgment_type")
		}
	}
	if j.JudgmentType == JudgmentTypeConfirmation && j.Confirmation.Status == "revoked" && j.SupersedesJudgmentRef == nil {
		return errors.New("judgment: revoked confirmation requires supersedes_judgment_ref")
	}
	if len(j.BasisRefs) > maxBasisRefs {
		return fmt.Errorf("judgment: basis_refs exceeds %d entries", maxBasisRefs)
	}
	for _, b := range j.BasisRefs {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("judgment: %w", err)
		}
	}
	if err := validateTime(j.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if err := validateHash(j.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	h, err := j.ContentHash()
	if err != nil {
		return fmt.Errorf("judgment: %w", err)
	}
	if j.ContentSHA256 != h {
		return errors.New("judgment: content_sha256 mismatch")
	}
	return nil
}

func (j JudgmentFact) canonMap() (map[string]any, error) {
	if err := j.validatePayload(); err != nil {
		return nil, err
	}
	subject, err := j.Subject.canonMap()
	if err != nil {
		return nil, err
	}
	source, err := j.Source.canonMap()
	if err != nil {
		return nil, err
	}
	basis, err := canonSlice(j.BasisRefs)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(j.CreatedAt)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"schema_version": j.SchemaVersion,
		"judgment_id":    j.JudgmentID,
		"judgment_type":  string(j.JudgmentType),
		"scope":          string(j.Scope),
		"subject":        subject,
		"source":         source,
		"basis_refs":     basis,
		"created_at":     created,
	}
	if j.SupersedesJudgmentRef != nil {
		ref, err := j.SupersedesJudgmentRef.canonMap()
		if err != nil {
			return nil, err
		}
		m["supersedes_judgment_ref"] = ref
	} else {
		m["supersedes_judgment_ref"] = nil
	}
	switch j.JudgmentType {
	case JudgmentTypeConfirmation:
		p, err := j.Confirmation.canonMap()
		if err != nil {
			return nil, err
		}
		m["confirmation"] = p
	case JudgmentTypeAttributionOverride:
		p, err := j.AttributionOverride.canonMap()
		if err != nil {
			return nil, err
		}
		m["attribution_override"] = p
	case JudgmentTypeRetrievalRelevance:
		p, err := j.RetrievalRelevance.canonMap()
		if err != nil {
			return nil, err
		}
		m["retrieval_relevance"] = p
	case JudgmentTypeContextApplicability:
		p, err := j.ContextApplicability.canonMap()
		if err != nil {
			return nil, err
		}
		m["context_applicability"] = p
	case JudgmentTypeContentClassification:
		p, err := j.ContentClassification.canonMap()
		if err != nil {
			return nil, err
		}
		m["content_classification"] = p
	case JudgmentTypeEvidenceTrust:
		p, err := j.EvidenceTrust.canonMap()
		if err != nil {
			return nil, err
		}
		m["evidence_trust"] = p
	case JudgmentTypeFreshnessEvaluation:
		p, err := j.FreshnessEvaluation.canonMap()
		if err != nil {
			return nil, err
		}
		m["freshness_evaluation"] = p
	}
	return m, nil
}

func (j JudgmentFact) CanonicalBytes() ([]byte, error) {
	m, err := j.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (j JudgmentFact) ContentHash() (string, error) {
	b, err := j.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (j JudgmentFact) EncodeCanonical() ([]byte, error) {
	m, err := j.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := j.ContentHash()
	if err != nil {
		return nil, err
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}

// GovernanceEvent is the append-only audit fact for pin/unpin/manual_freeze/
// unfreeze/archive operations. It intentionally has no content hash field in
// the frozen Architecture schema; ContentHash is still available for Doctor
// integrity checks.
type GovernanceEvent struct {
	SchemaVersion int        `json:"schema_version"`
	EventID       string     `json:"event_id"`
	Scope         Scope      `json:"scope"`
	MemoryID      string     `json:"memory_id"`
	Revision      int        `json:"revision"`
	Operation     string     `json:"operation"`
	Reason        string     `json:"reason"`
	Source        string     `json:"source"`
	BasisRefs     []BasisRef `json:"basis_refs"`
	CreatedAt     string     `json:"created_at"`
}

func (g GovernanceEvent) Validate() error {
	if g.SchemaVersion != SchemaVersion {
		return fmt.Errorf("governance event: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(g.EventID, "event_id"); err != nil {
		return fmt.Errorf("governance event: %w", err)
	}
	if err := g.Scope.Validate(); err != nil {
		return fmt.Errorf("governance event: %w", err)
	}
	if err := validateID(g.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("governance event: %w", err)
	}
	if g.Revision < 1 {
		return errors.New("governance event: revision must be >= 1")
	}
	switch g.Operation {
	case "pin", "unpin", "manual_freeze", "unfreeze", "archive":
	default:
		return fmt.Errorf("governance event: invalid operation %q", g.Operation)
	}
	if err := validateText(g.Reason, maxReasonLen, "reason", true); err != nil {
		return fmt.Errorf("governance event: %w", err)
	}
	if err := validateID(g.Source, "source"); err != nil {
		return fmt.Errorf("governance event: %w", err)
	}
	if len(g.BasisRefs) > maxBasisRefs {
		return fmt.Errorf("governance event: basis_refs exceeds %d entries", maxBasisRefs)
	}
	for _, b := range g.BasisRefs {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("governance event: %w", err)
		}
	}
	if g.Operation == "unfreeze" {
		if len(g.BasisRefs) == 0 {
			return errors.New("governance event: unfreeze requires basis_refs referencing judgment/evidence/memory facts")
		}
		for _, b := range g.BasisRefs {
			if b.PolicyRef != nil {
				return errors.New("governance event: unfreeze basis_refs must not contain policy refs")
			}
		}
	}
	return validateTime(g.CreatedAt, "created_at")
}

func (g GovernanceEvent) canonMap() (map[string]any, error) {
	basis, err := canonSlice(g.BasisRefs)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema_version": g.SchemaVersion,
		"event_id":       g.EventID,
		"scope":          string(g.Scope),
		"memory_id":      g.MemoryID,
		"revision":       g.Revision,
		"operation":      g.Operation,
		"reason":         g.Reason,
		"source":         g.Source,
		"basis_refs":     basis,
		"created_at":     created,
	}, nil
}

func (g GovernanceEvent) CanonicalBytes() ([]byte, error) {
	m, err := g.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (g GovernanceEvent) ContentHash() (string, error) {
	b, err := g.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (g GovernanceEvent) EncodeCanonical() ([]byte, error) {
	m, err := g.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// GenerationInputManifest permanently records the exact fact inputs of one
// Generation. Inputs are deduplicated by fact_type + fact_id and sorted
// deterministically; input_manifest_sha256 covers the whole canonical
// document except the hash field itself (including output_sha256).
type GenerationInputManifest struct {
	SchemaVersion           int             `json:"schema_version"`
	GenerationID            string          `json:"generation_id"`
	Scope                   Scope           `json:"scope"`
	BaseGeneration          *string         `json:"base_generation"`
	CompilerVersion         string          `json:"compiler_version"`
	CanonicalizationVersion int             `json:"canonicalization_version"`
	Inputs                  []ManifestInput `json:"inputs"`
	InputManifestSHA256     string          `json:"input_manifest_sha256"`
	OutputSHA256            string          `json:"output_sha256"`
	TransactionID           string          `json:"transaction_id"`
	CreatedAt               string          `json:"created_at"`
}

func (m GenerationInputManifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("generation input manifest: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(m.GenerationID, "generation_id"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if err := m.Scope.Validate(); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if m.BaseGeneration != nil {
		if err := validateID(*m.BaseGeneration, "base_generation"); err != nil {
			return fmt.Errorf("generation input manifest: %w", err)
		}
	}
	if err := validateVersionID(m.CompilerVersion, "compiler_version"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if m.CanonicalizationVersion < 1 {
		return errors.New("generation input manifest: canonicalization_version must be >= 1")
	}
	if _, err := dedupeManifestInputs(m.Inputs); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	for _, in := range m.Inputs {
		if err := in.Validate(); err != nil {
			return fmt.Errorf("generation input manifest: %w", err)
		}
	}
	if err := validateHash(m.OutputSHA256, "output_sha256"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if err := validateID(m.TransactionID, "transaction_id"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if err := validateTime(m.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if err := validateHash(m.InputManifestSHA256, "input_manifest_sha256"); err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	h, err := m.ContentHash()
	if err != nil {
		return fmt.Errorf("generation input manifest: %w", err)
	}
	if m.InputManifestSHA256 != h {
		return errors.New("generation input manifest: input_manifest_sha256 mismatch")
	}
	return nil
}

func (m GenerationInputManifest) canonMap() (map[string]any, error) {
	inputs, err := dedupeManifestInputs(m.Inputs)
	if err != nil {
		return nil, err
	}
	entryMaps := make([]any, 0, len(inputs))
	for _, in := range inputs {
		em, err := in.canonMap()
		if err != nil {
			return nil, err
		}
		entryMaps = append(entryMaps, em)
	}
	created, err := normalizeTime(m.CreatedAt)
	if err != nil {
		return nil, err
	}
	var base any
	if m.BaseGeneration != nil {
		base = *m.BaseGeneration
	}
	return map[string]any{
		"schema_version":           m.SchemaVersion,
		"generation_id":            m.GenerationID,
		"scope":                    string(m.Scope),
		"base_generation":          base,
		"compiler_version":         m.CompilerVersion,
		"canonicalization_version": m.CanonicalizationVersion,
		"inputs":                   entryMaps,
		"output_sha256":            m.OutputSHA256,
		"transaction_id":           m.TransactionID,
		"created_at":               created,
	}, nil
}

func (m GenerationInputManifest) CanonicalBytes() ([]byte, error) {
	cm, err := m.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(cm)
}

func (m GenerationInputManifest) ContentHash() (string, error) {
	b, err := m.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (m GenerationInputManifest) EncodeCanonical() ([]byte, error) {
	cm, err := m.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := m.ContentHash()
	if err != nil {
		return nil, err
	}
	cm["input_manifest_sha256"] = h
	return json.MarshalIndent(cm, "", "  ")
}

// ManifestInput references one canonical fact used by the Generation.
type ManifestInput struct {
	FactType          string `json:"fact_type"`
	FactID            string `json:"fact_id"`
	FactSchemaVersion int    `json:"fact_schema_version"`
	ContentSHA256     string `json:"content_sha256"`
}

func (in ManifestInput) Validate() error {
	if err := validateID(in.FactType, "fact_type"); err != nil {
		return fmt.Errorf("manifest input: %w", err)
	}
	if err := validateID(in.FactID, "fact_id"); err != nil {
		return fmt.Errorf("manifest input: %w", err)
	}
	if in.FactSchemaVersion < 1 {
		return errors.New("manifest input: fact_schema_version must be >= 1")
	}
	return validateHash(in.ContentSHA256, "content_sha256")
}

func (in ManifestInput) canonMap() (map[string]any, error) {
	return map[string]any{
		"fact_type":           in.FactType,
		"fact_id":             in.FactID,
		"fact_schema_version": in.FactSchemaVersion,
		"content_sha256":      in.ContentSHA256,
	}, nil
}

// dedupeManifestInputs sorts inputs by fact_type + fact_id and rejects
// conflicting entries that share the key but differ in content.
func dedupeManifestInputs(items []ManifestInput) ([]ManifestInput, error) {
	if items == nil {
		return []ManifestInput{}, nil
	}
	type entry struct {
		key  string
		item ManifestInput
	}
	entries := make([]entry, 0, len(items))
	for _, it := range items {
		entries = append(entries, entry{key: it.FactType + "\x00" + it.FactID, item: it})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	out := make([]ManifestInput, 0, len(entries))
	for i, e := range entries {
		if i > 0 && entries[i-1].key == e.key {
			prev := entries[i-1].item
			if prev.ContentSHA256 != e.item.ContentSHA256 || prev.FactSchemaVersion != e.item.FactSchemaVersion {
				return nil, fmt.Errorf("conflicting manifest inputs for %s/%s", e.item.FactType, e.item.FactID)
			}
			continue
		}
		out = append(out, e.item)
	}
	return out, nil
}
