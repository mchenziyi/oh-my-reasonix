package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// PolicyConfigSchemaVersion is the frozen config schema version required in
// every PolicyFact config. Config fields are fixed by MEM-01C; unknown
// fields, wrong union keys and out-of-contract values are rejected.
const PolicyConfigSchemaVersion = 1

// Fixed index topology dimensions; split_order may only use these and must
// not repeat any.
var indexSplitDimensions = []string{"component", "operation", "memory_type", "stable_id_prefix"}

// Anti-abuse bounds for index policy numbers (not Schema semantics).
const (
	maxIndexEntriesPerPage = 1024
	maxIndexPageBytes      = 1 << 20 // 1 MiB
	maxIndexShardDepth     = 16
)

// PolicyRef references an immutable PolicyFact. It always carries
// policy_id + policy_type + content_sha256; bare policy ids and "current
// policy" shortcuts are not auditable references.
type PolicyRef struct {
	PolicyID      string     `json:"policy_id"`
	PolicyType    PolicyType `json:"policy_type"`
	ContentSHA256 string     `json:"content_sha256"`
}

func (r PolicyRef) Validate() error {
	if err := validateID(r.PolicyID, "policy_id"); err != nil {
		return fmt.Errorf("policy ref: %w", err)
	}
	if err := r.PolicyType.Validate(); err != nil {
		return fmt.Errorf("policy ref: %w", err)
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r PolicyRef) canonMap() (map[string]any, error) {
	return map[string]any{
		"policy_id":      r.PolicyID,
		"policy_type":    string(r.PolicyType),
		"content_sha256": r.ContentSHA256,
	}, nil
}

// PolicyConfigFreshness parametrizes future freshness evaluation. No
// evaluation runs in this phase; the window order is enforced and no
// auto-modification actions (delete/freeze/edit) may be configured.
type PolicyConfigFreshness struct {
	EvaluationWindowDays      int      `json:"evaluation_window_days"`
	AgingAfterDays            int      `json:"aging_after_days"`
	StaleAfterDays            int      `json:"stale_after_days"`
	RevalidationEvidenceTypes []string `json:"revalidation_evidence_types"`
	Version                   int      `json:"version"`
}

func (c PolicyConfigFreshness) validate() error {
	if c.Version != PolicyConfigSchemaVersion {
		return fmt.Errorf("freshness config: version must be %d", PolicyConfigSchemaVersion)
	}
	if c.EvaluationWindowDays < 1 {
		return errors.New("freshness config: evaluation_window_days must be a positive integer")
	}
	if c.AgingAfterDays <= c.EvaluationWindowDays {
		return errors.New("freshness config: aging_after_days must be strictly greater than evaluation_window_days")
	}
	if c.StaleAfterDays <= c.AgingAfterDays {
		return errors.New("freshness config: stale_after_days must be strictly greater than aging_after_days")
	}
	if len(c.RevalidationEvidenceTypes) == 0 {
		return errors.New("freshness config: revalidation_evidence_types must not be empty")
	}
	for _, et := range c.RevalidationEvidenceTypes {
		if err := validateField(et); err != nil {
			return fmt.Errorf("freshness config: evidence type: %w", err)
		}
	}
	return nil
}

func (c PolicyConfigFreshness) canonMap() (map[string]any, error) {
	evs, err := canonStrings(c.RevalidationEvidenceTypes)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"evaluation_window_days":      c.EvaluationWindowDays,
		"aging_after_days":            c.AgingAfterDays,
		"stale_after_days":            c.StaleAfterDays,
		"revalidation_evidence_types": evs,
		"version":                     c.Version,
	}, nil
}

// PolicyConfigTrust is a security root: it stores boundaries and judgment
// inputs only and can never be changed by Mnemosyne self-evolution. The
// provenance / verification hard constraints cannot be switched off and
// external unverified content cannot be allowed as instruction material.
type PolicyConfigTrust struct {
	AllowedAcquisitionMethods            []string `json:"allowed_acquisition_methods"`
	RequireProvenance                    bool     `json:"require_provenance"`
	RequireVerificationStatus            bool     `json:"require_verification_status"`
	ExternalUnverifiedInstructionAllowed bool     `json:"external_unverified_instruction_allowed"`
	PromotionRequiresPolicyEvidence      bool     `json:"promotion_requires_policy_evidence"`
	Version                              int      `json:"version"`
}

func (c PolicyConfigTrust) validate() error {
	if c.Version != PolicyConfigSchemaVersion {
		return fmt.Errorf("trust config: version must be %d", PolicyConfigSchemaVersion)
	}
	if len(c.AllowedAcquisitionMethods) == 0 {
		return errors.New("trust config: allowed_acquisition_methods must not be empty")
	}
	for _, m := range c.AllowedAcquisitionMethods {
		if err := validateField(m); err != nil {
			return fmt.Errorf("trust config: acquisition method: %w", err)
		}
	}
	if !c.RequireProvenance {
		return errors.New("trust config: require_provenance cannot be disabled")
	}
	if !c.RequireVerificationStatus {
		return errors.New("trust config: require_verification_status cannot be disabled")
	}
	if c.ExternalUnverifiedInstructionAllowed {
		return errors.New("trust config: external unverified content cannot be allowed as instruction")
	}
	return nil
}

func (c PolicyConfigTrust) canonMap() (map[string]any, error) {
	methods, err := canonStrings(c.AllowedAcquisitionMethods)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"allowed_acquisition_methods":             methods,
		"require_provenance":                      c.RequireProvenance,
		"require_verification_status":             c.RequireVerificationStatus,
		"external_unverified_instruction_allowed": c.ExternalUnverifiedInstructionAllowed,
		"promotion_requires_policy_evidence":      c.PromotionRequiresPolicyEvidence,
		"version":                                 c.Version,
	}, nil
}

// PolicyConfigContentClassifier declares structured classifier references
// and safe defaults; the classifier itself is not run in this phase. Secret
// and unsafe classes always block promotion.
type PolicyConfigContentClassifier struct {
	ClassifierID                string   `json:"classifier_id"`
	AllowedClasses              []string `json:"allowed_classes"`
	DefaultClass                string   `json:"default_class"`
	SecretClassesBlockPromotion bool     `json:"secret_classes_block_promotion"`
	Version                     int      `json:"version"`
}

func (c PolicyConfigContentClassifier) validate() error {
	if c.Version != PolicyConfigSchemaVersion {
		return fmt.Errorf("content_classifier config: version must be %d", PolicyConfigSchemaVersion)
	}
	if err := validateField(c.ClassifierID); err != nil {
		return fmt.Errorf("content_classifier config: classifier_id: %w", err)
	}
	if len(c.AllowedClasses) == 0 {
		return errors.New("content_classifier config: allowed_classes must not be empty")
	}
	for _, cl := range c.AllowedClasses {
		if err := validateField(cl); err != nil {
			return fmt.Errorf("content_classifier config: class: %w", err)
		}
	}
	if !containsString(c.AllowedClasses, c.DefaultClass) {
		return fmt.Errorf("content_classifier config: default_class %q must be one of allowed_classes", c.DefaultClass)
	}
	if !c.SecretClassesBlockPromotion {
		return errors.New("content_classifier config: secret/unsafe classes must block promotion")
	}
	return nil
}

func (c PolicyConfigContentClassifier) canonMap() (map[string]any, error) {
	classes, err := canonStrings(c.AllowedClasses)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"classifier_id":                  c.ClassifierID,
		"allowed_classes":                classes,
		"default_class":                  c.DefaultClass,
		"secret_classes_block_promotion": c.SecretClassesBlockPromotion,
		"version":                        c.Version,
	}, nil
}

// PolicyConfigIndex declares the topology bounds of a future derived index.
// No index files are generated in this phase. split_order keeps its order
// (it is a topology, not a set) and only uses the fixed dimensions.
type PolicyConfigIndex struct {
	MaxEntriesPerPage int      `json:"max_entries_per_page"`
	MaxPageBytes      int      `json:"max_page_bytes"`
	MaxShardDepth     int      `json:"max_shard_depth"`
	SplitOrder        []string `json:"split_order"`
	OverflowBucket    string   `json:"overflow_bucket"`
	Version           int      `json:"version"`
}

func (c PolicyConfigIndex) validate() error {
	if c.Version != PolicyConfigSchemaVersion {
		return fmt.Errorf("index config: version must be %d", PolicyConfigSchemaVersion)
	}
	if c.MaxEntriesPerPage < 1 || c.MaxEntriesPerPage > maxIndexEntriesPerPage {
		return fmt.Errorf("index config: max_entries_per_page must be within [1, %d]", maxIndexEntriesPerPage)
	}
	if c.MaxPageBytes < 1 || c.MaxPageBytes > maxIndexPageBytes {
		return fmt.Errorf("index config: max_page_bytes must be within [1, %d]", maxIndexPageBytes)
	}
	if c.MaxShardDepth < 1 || c.MaxShardDepth > maxIndexShardDepth {
		return fmt.Errorf("index config: max_shard_depth must be within [1, %d]", maxIndexShardDepth)
	}
	if len(c.SplitOrder) == 0 {
		return errors.New("index config: split_order must not be empty")
	}
	seen := map[string]bool{}
	for _, d := range c.SplitOrder {
		if !containsString(indexSplitDimensions, d) {
			return fmt.Errorf("index config: split dimension %q is not a fixed topology dimension", d)
		}
		if seen[d] {
			return fmt.Errorf("index config: split dimension %q must not repeat", d)
		}
		seen[d] = true
	}
	if err := validateField(c.OverflowBucket); err != nil {
		return fmt.Errorf("index config: overflow_bucket: %w", err)
	}
	return nil
}

func (c PolicyConfigIndex) canonMap() (map[string]any, error) {
	order := make([]any, 0, len(c.SplitOrder))
	for _, d := range c.SplitOrder {
		order = append(order, d)
	}
	return map[string]any{
		"max_entries_per_page": c.MaxEntriesPerPage,
		"max_page_bytes":       c.MaxPageBytes,
		"max_shard_depth":      c.MaxShardDepth,
		"split_order":          order,
		"overflow_bucket":      c.OverflowBucket,
		"version":              c.Version,
	}, nil
}

// PolicyConfigBenchmark pre-registers quality gates for a later benchmark
// phase; no benchmark runs here and no capability claims are made.
type PolicyConfigBenchmark struct {
	FixtureSetID             string             `json:"fixture_set_id"`
	MinimumCases             int                `json:"minimum_cases"`
	RequiredMetrics          []string           `json:"required_metrics"`
	PassThresholds           map[string]float64 `json:"pass_thresholds"`
	PairedComparisonRequired bool               `json:"paired_comparison_required"`
	Version                  int                `json:"version"`
}

func (c PolicyConfigBenchmark) validate() error {
	if c.Version != PolicyConfigSchemaVersion {
		return fmt.Errorf("benchmark config: version must be %d", PolicyConfigSchemaVersion)
	}
	if err := validateField(c.FixtureSetID); err != nil {
		return fmt.Errorf("benchmark config: fixture_set_id: %w", err)
	}
	if c.MinimumCases < 1 {
		return errors.New("benchmark config: minimum_cases must be a positive integer")
	}
	if len(c.RequiredMetrics) == 0 {
		return errors.New("benchmark config: required_metrics must not be empty")
	}
	for _, m := range c.RequiredMetrics {
		if err := validateField(m); err != nil {
			return fmt.Errorf("benchmark config: metric: %w", err)
		}
	}
	if len(c.PassThresholds) == 0 {
		return errors.New("benchmark config: pass_thresholds must not be empty")
	}
	for name, v := range c.PassThresholds {
		if err := validateField(name); err != nil {
			return fmt.Errorf("benchmark config: threshold metric: %w", err)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("benchmark config: threshold %q must be a finite value in [0, 1]", name)
		}
	}
	return nil
}

func (c PolicyConfigBenchmark) canonMap() (map[string]any, error) {
	metrics, err := canonStrings(c.RequiredMetrics)
	if err != nil {
		return nil, err
	}
	// Normalize -0.0 to +0.0 so mathematically identical thresholds hash
	// identically (Go's JSON encoder renders them differently).
	thresholds := make(map[string]float64, len(c.PassThresholds))
	for k, v := range c.PassThresholds {
		if v == 0 {
			v = 0
		}
		thresholds[k] = v
	}
	return map[string]any{
		"fixture_set_id":             c.FixtureSetID,
		"minimum_cases":              c.MinimumCases,
		"required_metrics":           metrics,
		"pass_thresholds":            thresholds,
		"paired_comparison_required": c.PairedComparisonRequired,
		"version":                    c.Version,
	}, nil
}

func containsString(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

// PolicyConfig is a discriminated union keyed by the policy_type discriminant.
type PolicyConfig struct {
	Freshness         *PolicyConfigFreshness         `json:"freshness,omitempty"`
	Trust             *PolicyConfigTrust             `json:"trust,omitempty"`
	ContentClassifier *PolicyConfigContentClassifier `json:"content_classifier,omitempty"`
	Index             *PolicyConfigIndex             `json:"index,omitempty"`
	Benchmark         *PolicyConfigBenchmark         `json:"benchmark,omitempty"`
}

func (c PolicyConfig) Validate(pt PolicyType) error {
	n := 0
	for _, set := range []bool{
		c.Freshness != nil, c.Trust != nil, c.ContentClassifier != nil,
		c.Index != nil, c.Benchmark != nil,
	} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("policy config: exactly one config key must be set, got %d", n)
	}
	var key PolicyType
	switch {
	case c.Freshness != nil:
		key = PolicyTypeFreshness
	case c.Trust != nil:
		key = PolicyTypeTrust
	case c.ContentClassifier != nil:
		key = PolicyTypeContentClassifier
	case c.Index != nil:
		key = PolicyTypeIndex
	default:
		key = PolicyTypeBenchmark
	}
	if key != pt {
		return fmt.Errorf("policy config: key %q does not match policy_type %q", key, pt)
	}
	switch pt {
	case PolicyTypeFreshness:
		return c.Freshness.validate()
	case PolicyTypeTrust:
		return c.Trust.validate()
	case PolicyTypeContentClassifier:
		return c.ContentClassifier.validate()
	case PolicyTypeIndex:
		return c.Index.validate()
	case PolicyTypeBenchmark:
		return c.Benchmark.validate()
	}
	return fmt.Errorf("policy config: invalid policy_type %q", pt)
}

func (c PolicyConfig) canonMap(pt PolicyType) (map[string]any, error) {
	if err := c.Validate(pt); err != nil {
		return nil, err
	}
	switch pt {
	case PolicyTypeFreshness:
		m, err := c.Freshness.canonMap()
		return map[string]any{"freshness": m}, err
	case PolicyTypeTrust:
		m, err := c.Trust.canonMap()
		return map[string]any{"trust": m}, err
	case PolicyTypeContentClassifier:
		m, err := c.ContentClassifier.canonMap()
		return map[string]any{"content_classifier": m}, err
	case PolicyTypeIndex:
		m, err := c.Index.canonMap()
		return map[string]any{"index": m}, err
	case PolicyTypeBenchmark:
		m, err := c.Benchmark.canonMap()
		return map[string]any{"benchmark": m}, err
	default:
		return nil, fmt.Errorf("policy config: invalid policy_type %q", pt)
	}
}

// PolicyFact is the immutable canonical fact for one version of a policy.
type PolicyFact struct {
	SchemaVersion int          `json:"schema_version"`
	PolicyID      string       `json:"policy_id"`
	PolicyType    PolicyType   `json:"policy_type"`
	PolicyVersion int          `json:"policy_version"`
	Config        PolicyConfig `json:"config"`
	ContentSHA256 string       `json:"content_sha256"`
	CreatedAt     string       `json:"created_at"`
}

func (p PolicyFact) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("policy fact: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(p.PolicyID, "policy_id"); err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	if err := p.PolicyType.Validate(); err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	if p.PolicyVersion < 1 {
		return errors.New("policy fact: policy_version must be >= 1")
	}
	if err := p.Config.Validate(p.PolicyType); err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	if err := validateTime(p.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	if err := validateHash(p.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	h, err := p.ContentHash()
	if err != nil {
		return fmt.Errorf("policy fact: %w", err)
	}
	if p.ContentSHA256 != h {
		return errors.New("policy fact: content_sha256 mismatch")
	}
	return nil
}

func (p PolicyFact) canonMap() (map[string]any, error) {
	config, err := p.Config.canonMap(p.PolicyType)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema_version": p.SchemaVersion,
		"policy_id":      p.PolicyID,
		"policy_type":    string(p.PolicyType),
		"policy_version": p.PolicyVersion,
		"config":         config,
		"created_at":     created,
	}, nil
}

func (p PolicyFact) CanonicalBytes() ([]byte, error) {
	m, err := p.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (p PolicyFact) ContentHash() (string, error) {
	b, err := p.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (p PolicyFact) EncodeCanonical() ([]byte, error) {
	m, err := p.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := p.ContentHash()
	if err != nil {
		return nil, err
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}
