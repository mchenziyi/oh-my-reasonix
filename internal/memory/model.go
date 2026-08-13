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
	JudgmentTypeCriticReview          JudgmentType = "critic_review"
	JudgmentTypeConflictReview        JudgmentType = "conflict_review"
)

func (j JudgmentType) Validate() error {
	switch j {
	case JudgmentTypeConfirmation, JudgmentTypeAttributionOverride,
		JudgmentTypeRetrievalRelevance, JudgmentTypeContextApplicability,
		JudgmentTypeContentClassification, JudgmentTypeEvidenceTrust,
		JudgmentTypeFreshnessEvaluation, JudgmentTypeCriticReview,
		JudgmentTypeConflictReview:
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
	RootTaskRefs               []string      `json:"root_task_refs"`
	EvidenceSetSHA256          string        `json:"evidence_set_sha256"`
	PreviousEvidenceGeneration *int          `json:"previous_evidence_generation"`
	TransactionID              string        `json:"transaction_id"`
	CreatedAt                  string        `json:"created_at"`
	// MEM-02C: Evidence Provenance (Architecture v1 6.2.3). All six fields
	// must be present together (Enriched) or all absent (Legacy); partial
	// presence is rejected. nil vs [] distinguishes an absent provenance set
	// from an explicit empty root set, and *bool distinguishes an explicit
	// false from a missing boolean.
	EvidenceOrigin               string        `json:"evidence_origin,omitempty"`
	AcquisitionMethod            string        `json:"acquisition_method,omitempty"`
	VerificationStatus           string        `json:"verification_status,omitempty"`
	ProvenanceRefs               []EvidenceRef `json:"provenance_refs,omitempty"`
	ContainsInstructionalContent *bool         `json:"contains_instructional_content,omitempty"`
	ContainsSensitiveContent     *bool         `json:"contains_sensitive_content,omitempty"`
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
	// root_task_refs names the independent Root Tasks / formal sources this
	// evidence generation covers (the evidence_validated source-independence
	// signal). Entries are controlled identifiers and must be unique; the
	// derived layer counts distinct entries, never a repeated one.
	seenTask := map[string]bool{}
	for _, t := range e.RootTaskRefs {
		if err := validateID(t, "root_task_ref"); err != nil {
			return fmt.Errorf("memory evidence generation: %w", err)
		}
		if seenTask[t] {
			return fmt.Errorf("memory evidence generation: root_task_refs must not repeat %q", t)
		}
		seenTask[t] = true
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
	if err := e.validateProvenance(); err != nil {
		return err
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

// provenancePresent reports whether any of the six MEM-02C fields is set.
func (e MemoryEvidenceGeneration) provenancePresent() bool {
	return e.EvidenceOrigin != "" || e.AcquisitionMethod != "" || e.VerificationStatus != "" ||
		e.ProvenanceRefs != nil || e.ContainsInstructionalContent != nil || e.ContainsSensitiveContent != nil
}

// provenanceComplete reports whether all six MEM-02C fields are present
// (the Enriched form).
func (e MemoryEvidenceGeneration) provenanceComplete() bool {
	return e.EvidenceOrigin != "" && e.AcquisitionMethod != "" && e.VerificationStatus != "" &&
		e.ProvenanceRefs != nil && e.ContainsInstructionalContent != nil && e.ContainsSensitiveContent != nil
}

// validateProvenance enforces the MEM-02C two-form contract: Legacy (all six
// absent, byte-compatible with MEM-01B) or Enriched (all six present with
// frozen Architecture v1 enums, closed provenance refs). Partial presence
// fails closed; nothing is defaulted.
func (e MemoryEvidenceGeneration) validateProvenance() error {
	if !e.provenancePresent() {
		return nil // Legacy
	}
	if !e.provenanceComplete() {
		return errors.New("memory evidence generation: provenance fields must be all present or all absent")
	}
	if err := validEvidenceOrigin(e.EvidenceOrigin); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if err := validAcquisitionMethod(e.AcquisitionMethod); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if err := validVerificationStatus(e.VerificationStatus); err != nil {
		return fmt.Errorf("memory evidence generation: %w", err)
	}
	if len(e.ProvenanceRefs) > maxRefs {
		return fmt.Errorf("memory evidence generation: provenance_refs exceeds %d refs", maxRefs)
	}
	members := make(map[string]bool, len(e.EvidenceRefs))
	for _, r := range e.EvidenceRefs {
		members[evidenceRefKey(r)] = true
	}
	seen := make(map[string]bool, len(e.ProvenanceRefs))
	for _, r := range e.ProvenanceRefs {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("memory evidence generation: %w", err)
		}
		k := evidenceRefKey(r)
		if seen[k] {
			return errors.New("memory evidence generation: provenance_refs must not repeat")
		}
		seen[k] = true
		if !members[k] {
			return errors.New("memory evidence generation: provenance ref must be a member of evidence_refs")
		}
	}
	if e.AcquisitionMethod != "direct" && len(e.ProvenanceRefs) == 0 {
		return errors.New("memory evidence generation: non-direct acquisition requires at least one provenance ref")
	}
	return nil
}

// validEvidenceOrigin enforces the frozen Architecture v1 6.2.3 enum.
func validEvidenceOrigin(s string) error {
	switch s {
	case "runtime", "user", "official", "project", "external":
		return nil
	default:
		return errors.New("evidence_origin must be one of runtime|user|official|project|external")
	}
}

// validAcquisitionMethod enforces the frozen Architecture v1 6.2.3 enum.
func validAcquisitionMethod(s string) error {
	switch s {
	case "direct", "tool_observed", "model_extracted", "imported":
		return nil
	default:
		return errors.New("acquisition_method must be one of direct|tool_observed|model_extracted|imported")
	}
}

// validVerificationStatus enforces the frozen Architecture v1 6.2.3 enum.
func validVerificationStatus(s string) error {
	switch s {
	case "verified", "confirmed", "inferred", "unverified":
		return nil
	default:
		return errors.New("verification_status must be one of verified|confirmed|inferred|unverified")
	}
}

// evidenceRefKey is the deterministic identity of an EvidenceRef.
func evidenceRefKey(r EvidenceRef) string {
	return string(r.Scope) + "|" + r.EvidenceType + "|" + r.EvidenceID + "|" + r.ContentSHA256
}

func (e MemoryEvidenceGeneration) canonMap() (map[string]any, error) {
	refs, err := canonSlice(e.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	taskRefs := make([]string, len(e.RootTaskRefs))
	copy(taskRefs, e.RootTaskRefs)
	sort.Strings(taskRefs) // set semantics: order must not change the hash
	taskRefsAny := make([]any, 0, len(taskRefs))
	for _, t := range taskRefs {
		taskRefsAny = append(taskRefsAny, t)
	}
	created, err := normalizeTime(e.CreatedAt)
	if err != nil {
		return nil, err
	}
	var previous any
	if e.PreviousEvidenceGeneration != nil {
		previous = *e.PreviousEvidenceGeneration
	}
	m := map[string]any{
		"schema_version":               e.SchemaVersion,
		"memory_id":                    e.MemoryID,
		"revision":                     e.Revision,
		"evidence_generation":          e.EvidenceGeneration,
		"evidence_refs":                refs,
		"root_task_refs":               taskRefsAny,
		"previous_evidence_generation": previous,
		"transaction_id":               e.TransactionID,
		"created_at":                   created,
	}
	// Enriched form: all six provenance fields enter the canonical bytes and
	// the content hash. Legacy keeps the pre-MEM-02C key set unchanged.
	// The booleans are guarded so a partial (invalid) form returns an error
	// instead of panicking when canonMap is called before Validate.
	if e.EvidenceOrigin != "" || e.AcquisitionMethod != "" || e.VerificationStatus != "" ||
		e.ProvenanceRefs != nil || e.ContainsInstructionalContent != nil || e.ContainsSensitiveContent != nil {
		if e.EvidenceOrigin == "" || e.AcquisitionMethod == "" || e.VerificationStatus == "" ||
			e.ProvenanceRefs == nil || e.ContainsInstructionalContent == nil || e.ContainsSensitiveContent == nil {
			return nil, errors.New("memory evidence generation: provenance fields must be all present or all absent")
		}
		provRefs, err := canonSlice(e.ProvenanceRefs)
		if err != nil {
			return nil, err
		}
		m["evidence_origin"] = e.EvidenceOrigin
		m["acquisition_method"] = e.AcquisitionMethod
		m["verification_status"] = e.VerificationStatus
		m["provenance_refs"] = provRefs
		m["contains_instructional_content"] = *e.ContainsInstructionalContent
		m["contains_sensitive_content"] = *e.ContainsSensitiveContent
	}
	return m, nil
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
	RetrievalID      string       `json:"retrieval_id,omitempty"`
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
		if s.OutcomeID != "" || s.EvidenceRef != nil || s.TargetContextRef != "" || s.RetrievalID != "" {
			return errors.New("judgment subject: memory_revision must not carry other fields")
		}
	case "memory_outcome":
		if err := validateID(s.OutcomeID, "outcome_id"); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.MemoryRef != nil || s.EvidenceRef != nil || s.TargetContextRef != "" || s.RetrievalID != "" {
			return errors.New("judgment subject: memory_outcome must not carry other fields")
		}
	case "evidence":
		if s.EvidenceRef == nil {
			return errors.New("judgment subject: evidence requires evidence_ref")
		}
		if err := s.EvidenceRef.Validate(); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.MemoryRef != nil || s.OutcomeID != "" || s.TargetContextRef != "" || s.RetrievalID != "" {
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
		if s.OutcomeID != "" || s.EvidenceRef != nil || s.RetrievalID != "" {
			return errors.New("judgment subject: context must not carry other fields")
		}
	case "retrieval":
		if err := validateID(s.RetrievalID, "retrieval_id"); err != nil {
			return fmt.Errorf("judgment subject: %w", err)
		}
		if s.MemoryRef != nil || s.OutcomeID != "" || s.EvidenceRef != nil || s.TargetContextRef != "" {
			return errors.New("judgment subject: retrieval must not carry other fields")
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
	case "retrieval":
		m["retrieval_id"] = s.RetrievalID
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
		seen := make(map[string]bool, len(list))
		for _, ref := range list {
			if err := ref.Validate(); err != nil {
				return fmt.Errorf("retrieval relevance: %w", err)
			}
			key := string(ref.Scope) + "|" + string(ref.MemoryType) + "|" + ref.MemoryID + "|" + fmt.Sprint(ref.Revision) + "|" + ref.ContentSHA256
			if seen[key] {
				return errors.New("retrieval relevance: duplicate memory ref")
			}
			seen[key] = true
		}
	}
	if len(p.EvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("retrieval relevance: evidence_refs exceeds %d entries", maxPayloadRefs)
	}
	seenEvidence := make(map[string]bool, len(p.EvidenceRefs))
	for _, ref := range p.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("retrieval relevance: %w", err)
		}
		key := evidenceKey(ref)
		if seenEvidence[key] {
			return errors.New("retrieval relevance: duplicate evidence ref")
		}
		seenEvidence[key] = true
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
	if p.Result != "conditionally_applicable" && len(p.RequiredConditionIDs) != 0 {
		return errors.New("context applicability: required_condition_ids are only allowed for conditionally_applicable")
	}
	if len(p.RequiredConditionIDs) > maxPayloadRefs {
		return fmt.Errorf("context applicability: required_condition_ids exceeds %d entries", maxPayloadRefs)
	}
	seenConditions := make(map[string]bool, len(p.RequiredConditionIDs))
	for _, id := range p.RequiredConditionIDs {
		if err := validateID(id, "required_condition_id"); err != nil {
			return fmt.Errorf("context applicability: %w", err)
		}
		if seenConditions[id] {
			return errors.New("context applicability: duplicate required condition id")
		}
		seenConditions[id] = true
	}
	if len(p.EvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("context applicability: evidence_refs exceeds %d entries", maxPayloadRefs)
	}
	seenEvidence := make(map[string]bool, len(p.EvidenceRefs))
	for _, ref := range p.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("context applicability: %w", err)
		}
		key := evidenceKey(ref)
		if seenEvidence[key] {
			return errors.New("context applicability: duplicate evidence ref")
		}
		seenEvidence[key] = true
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

// CriticReviewPayload records a critic review of one revision within a
// pinned Generation Pair (MEM-02B). No free text, prompts, commands or
// credentials are stored anywhere; the critic source lives in the envelope.
type CriticReviewPayload struct {
	Result               string        `json:"result"`
	EvaluationScope      string        `json:"evaluation_scope"`
	MemoryContext        MemoryContext `json:"memory_context"`
	RequiredEvidenceRefs []EvidenceRef `json:"required_evidence_refs"`
}

// ConflictReviewPayload records whether a fixed Generation Pair contains a
// conflicting revision. It stores references only; review reasoning remains
// outside the fact store.
type ConflictReviewPayload struct {
	Result                string        `json:"result"`
	EvaluationScope       string        `json:"evaluation_scope"`
	MemoryContext         MemoryContext `json:"memory_context"`
	CounterpartMemoryRefs []MemoryRef   `json:"counterpart_memory_refs"`
	EvidenceRefs          []EvidenceRef `json:"evidence_refs"`
}

var conflictResults = map[string]bool{"clear": true, "conflict": true, "unavailable": true}

func (p ConflictReviewPayload) Validate() error {
	if !conflictResults[p.Result] {
		return errors.New("conflict review: result must be clear|conflict|unavailable")
	}
	if !criticEvaluationScopes[p.EvaluationScope] {
		return errors.New("conflict review: invalid evaluation_scope")
	}
	if err := p.MemoryContext.Validate(); err != nil {
		return fmt.Errorf("conflict review: %w", err)
	}
	if len(p.CounterpartMemoryRefs) > maxPayloadRefs || len(p.EvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("conflict review: references exceed %d entries", maxPayloadRefs)
	}
	if p.Result == "conflict" && len(p.CounterpartMemoryRefs) == 0 {
		return errors.New("conflict review: conflict requires counterpart_memory_refs")
	}
	if p.Result != "conflict" && len(p.CounterpartMemoryRefs) != 0 {
		return errors.New("conflict review: only conflict may carry counterpart_memory_refs")
	}
	seenMemory := make(map[string]bool, len(p.CounterpartMemoryRefs))
	for i := range p.CounterpartMemoryRefs {
		if err := p.CounterpartMemoryRefs[i].Validate(); err != nil {
			return fmt.Errorf("conflict review: %w", err)
		}
		key := memoryRefKey(p.CounterpartMemoryRefs[i])
		if seenMemory[key] {
			return errors.New("conflict review: duplicate counterpart_memory_ref")
		}
		seenMemory[key] = true
	}
	seenEvidence := make(map[string]bool, len(p.EvidenceRefs))
	for i := range p.EvidenceRefs {
		if err := p.EvidenceRefs[i].Validate(); err != nil {
			return fmt.Errorf("conflict review: %w", err)
		}
		key := evidenceKey(p.EvidenceRefs[i])
		if seenEvidence[key] {
			return errors.New("conflict review: duplicate evidence_ref")
		}
		seenEvidence[key] = true
	}
	return nil
}

func (p ConflictReviewPayload) canonMap() (map[string]any, error) {
	ctx, err := p.MemoryContext.canonMap()
	if err != nil {
		return nil, err
	}
	counterparts, err := canonSlice(p.CounterpartMemoryRefs)
	if err != nil {
		return nil, err
	}
	evidence, err := canonSlice(p.EvidenceRefs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"result": p.Result, "evaluation_scope": p.EvaluationScope,
		"memory_context": ctx, "counterpart_memory_refs": counterparts,
		"evidence_refs": evidence,
	}, nil
}

// criticResults / criticEvaluationScopes are the frozen critic vocabularies.
var (
	criticResults          = map[string]bool{"passed": true, "failed": true, "unavailable": true}
	criticEvaluationScopes = map[string]bool{
		"fixture": true, "generation_full_scan": true,
		"expanded_index_scan": true, "sampled_audit": true,
	}
)

func (p CriticReviewPayload) Validate() error {
	if !criticResults[p.Result] {
		return errors.New("critic review: result must be passed|failed|unavailable")
	}
	if !criticEvaluationScopes[p.EvaluationScope] {
		return errors.New("critic review: evaluation_scope must be fixture|generation_full_scan|expanded_index_scan|sampled_audit")
	}
	if err := p.MemoryContext.Validate(); err != nil {
		return fmt.Errorf("critic review: %w", err)
	}
	if len(p.RequiredEvidenceRefs) > maxPayloadRefs {
		return fmt.Errorf("critic review: required_evidence_refs exceeds %d entries", maxPayloadRefs)
	}
	for i := range p.RequiredEvidenceRefs {
		if err := p.RequiredEvidenceRefs[i].Validate(); err != nil {
			return fmt.Errorf("critic review: %w", err)
		}
	}
	if p.Result == "passed" && len(p.RequiredEvidenceRefs) == 0 {
		return errors.New("critic review: passed requires required_evidence_refs")
	}
	return nil
}

func (p CriticReviewPayload) canonMap() (map[string]any, error) {
	ctxMap, err := p.MemoryContext.canonMap()
	if err != nil {
		return nil, err
	}
	req, err := canonSlice(p.RequiredEvidenceRefs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"result":                 p.Result,
		"evaluation_scope":       p.EvaluationScope,
		"memory_context":         ctxMap,
		"required_evidence_refs": req,
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
	BasisContextRefs      []string                      `json:"basis_context_refs,omitempty"`
	Confirmation          *ConfirmationPayload          `json:"confirmation,omitempty"`
	AttributionOverride   *AttributionOverridePayload   `json:"attribution_override,omitempty"`
	RetrievalRelevance    *RetrievalRelevancePayload    `json:"retrieval_relevance,omitempty"`
	ContextApplicability  *ContextApplicabilityPayload  `json:"context_applicability,omitempty"`
	ContentClassification *ContentClassificationPayload `json:"content_classification,omitempty"`
	EvidenceTrust         *EvidenceTrustPayload         `json:"evidence_trust,omitempty"`
	FreshnessEvaluation   *FreshnessEvaluationPayload   `json:"freshness_evaluation,omitempty"`
	CriticReview          *CriticReviewPayload          `json:"critic_review,omitempty"`
	ConflictReview        *ConflictReviewPayload        `json:"conflict_review,omitempty"`
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
	if j.CriticReview != nil {
		nonNil++
	}
	if j.ConflictReview != nil {
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
	case JudgmentTypeCriticReview:
		if j.CriticReview == nil {
			return errors.New("judgment: critic_review payload missing")
		}
		err = j.CriticReview.Validate()
	case JudgmentTypeConflictReview:
		if j.ConflictReview == nil {
			return errors.New("judgment: conflict_review payload missing")
		}
		err = j.ConflictReview.Validate()
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
	if err := j.validateCriticConstraints(); err != nil {
		return err
	}
	if err := j.validateConflictConstraints(); err != nil {
		return err
	}
	if err := j.validateContextApplicabilityConstraints(); err != nil {
		return err
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

// validateContextApplicabilityConstraints implements the MEM-02E enriched
// shape while leaving nil as the byte-compatible legacy representation.
func (j JudgmentFact) validateContextApplicabilityConstraints() error {
	if j.JudgmentType != JudgmentTypeContextApplicability {
		if j.BasisContextRefs != nil {
			return errors.New("judgment: basis_context_refs are only allowed for context_applicability")
		}
		return nil
	}
	if j.Subject.SubjectType != "context" || j.Subject.MemoryRef == nil {
		return errors.New("judgment: context_applicability subject must be context")
	}
	if j.Subject.MemoryRef.Scope != j.Scope {
		return errors.New("judgment: context_applicability subject scope must equal judgment scope")
	}
	if j.BasisContextRefs == nil {
		return nil // legacy canonical form
	}
	if len(j.BasisContextRefs) == 0 {
		return errors.New("judgment: enriched context_applicability requires basis_context_refs")
	}
	if len(j.BasisContextRefs) > maxBasisRefs {
		return fmt.Errorf("judgment: basis_context_refs exceeds %d entries", maxBasisRefs)
	}
	seen := make(map[string]bool, len(j.BasisContextRefs))
	for _, id := range j.BasisContextRefs {
		if err := validateID(id, "basis_context_ref"); err != nil {
			return errors.New("judgment: invalid basis_context_ref")
		}
		if seen[id] {
			return errors.New("judgment: duplicate basis_context_ref")
		}
		seen[id] = true
	}
	return nil
}

// criticSourceTypes is the frozen critic source vocabulary (MEM-02B). It is
// enforced only for critic_review judgments so other subtypes keep their
// existing source freedom.
var criticSourceTypes = map[string]bool{
	"fixture_oracle": true, "offline_rule": true, "user_review": true,
}

// validateCriticConstraints enforces the MEM-02B critic_review specifics.
func (j JudgmentFact) validateCriticConstraints() error {
	if j.JudgmentType != JudgmentTypeCriticReview {
		return nil
	}
	if j.Subject.SubjectType != "memory_revision" {
		return errors.New("judgment: critic_review subject must be a memory_revision")
	}
	if j.Subject.MemoryRef != nil && j.Subject.MemoryRef.Scope != j.Scope {
		return errors.New("judgment: critic_review subject scope must equal judgment scope")
	}
	if !criticSourceTypes[j.Source.SourceType] {
		return errors.New("judgment: critic_review source_type must be fixture_oracle|offline_rule|user_review")
	}
	if j.CriticReview.Result == "passed" {
		if len(j.BasisRefs) == 0 {
			return errors.New("judgment: passed critic_review requires at least one basis_ref")
		}
		for _, req := range j.CriticReview.RequiredEvidenceRefs {
			if !basisContainsEvidence(j.BasisRefs, req) {
				return errors.New("judgment: passed critic_review required evidence must be fully present in basis_refs")
			}
		}
	}
	return nil
}

var conflictSourceTypes = map[string]bool{
	"fixture_oracle": true, "offline_rule": true, "user_review": true, "conflict_critic": true,
}

func (j JudgmentFact) validateConflictConstraints() error {
	if j.JudgmentType != JudgmentTypeConflictReview {
		return nil
	}
	if j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil {
		return errors.New("judgment: conflict_review subject must be a memory_revision")
	}
	if j.Subject.MemoryRef.Scope != j.Scope {
		return errors.New("judgment: conflict_review subject scope must equal judgment scope")
	}
	if !conflictSourceTypes[j.Source.SourceType] {
		return errors.New("judgment: invalid conflict_review source_type")
	}
	for _, counterpart := range j.ConflictReview.CounterpartMemoryRefs {
		if memoryRefsEqual(counterpart, *j.Subject.MemoryRef) {
			return errors.New("judgment: conflict_review counterpart must not reference its subject")
		}
	}
	if j.ConflictReview.Result == "clear" && j.SupersedesJudgmentRef != nil {
		hasNonPolicyBasis := false
		for _, basis := range j.BasisRefs {
			if basis.PolicyRef == nil {
				hasNonPolicyBasis = true
				break
			}
		}
		if !hasNonPolicyBasis {
			return errors.New("judgment: clear conflict_review supersede requires non-policy basis_ref")
		}
	}
	return nil
}

func memoryRefsEqual(a, b MemoryRef) bool {
	return a.Scope == b.Scope && a.MemoryType == b.MemoryType && a.MemoryID == b.MemoryID &&
		a.Revision == b.Revision && a.ContentSHA256 == b.ContentSHA256
}

func memoryRefKey(r MemoryRef) string {
	return string(r.Scope) + "|" + string(r.MemoryType) + "|" + r.MemoryID + "|" + fmt.Sprint(r.Revision) + "|" + r.ContentSHA256
}

// evidenceRefsEqual compares two EvidenceRefs field by field.
func evidenceRefsEqual(a, b EvidenceRef) bool {
	return a.Scope == b.Scope && a.EvidenceType == b.EvidenceType &&
		a.EvidenceID == b.EvidenceID && a.ContentSHA256 == b.ContentSHA256
}

// basisContainsEvidence reports whether any basis entry carries exactly the
// wanted EvidenceRef.
func basisContainsEvidence(basis []BasisRef, want EvidenceRef) bool {
	for i := range basis {
		if basis[i].EvidenceRef != nil && evidenceRefsEqual(*basis[i].EvidenceRef, want) {
			return true
		}
	}
	return false
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
	if j.BasisContextRefs != nil {
		basisContexts, err := canonStrings(j.BasisContextRefs)
		if err != nil {
			return nil, err
		}
		m["basis_context_refs"] = basisContexts
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
	case JudgmentTypeCriticReview:
		p, err := j.CriticReview.canonMap()
		if err != nil {
			return nil, err
		}
		m["critic_review"] = p
	case JudgmentTypeConflictReview:
		p, err := j.ConflictReview.canonMap()
		if err != nil {
			return nil, err
		}
		m["conflict_review"] = p
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

// ValidateWithoutID validates all request-controlled fields before OMR
// derives the immutable event identity.
func (g GovernanceEvent) ValidateWithoutID() error {
	g.EventID = "governance_pending"
	return g.Validate()
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
