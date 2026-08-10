package memory

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Per-type config shells. MEM-01A only freezes the discriminated union and
// strict decoding: unknown fields are rejected. The actual config fields are
// frozen by MEM-01C; until then every config must be the empty object.
type PolicyConfigFreshness struct{}
type PolicyConfigTrust struct{}
type PolicyConfigContentClassifier struct{}
type PolicyConfigIndex struct{}
type PolicyConfigBenchmark struct{}

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
	return nil
}

func (c PolicyConfig) canonMap(pt PolicyType) (map[string]any, error) {
	if err := c.Validate(pt); err != nil {
		return nil, err
	}
	switch pt {
	case PolicyTypeFreshness:
		return map[string]any{"freshness": map[string]any{}}, nil
	case PolicyTypeTrust:
		return map[string]any{"trust": map[string]any{}}, nil
	case PolicyTypeContentClassifier:
		return map[string]any{"content_classifier": map[string]any{}}, nil
	case PolicyTypeIndex:
		return map[string]any{"index": map[string]any{}}, nil
	case PolicyTypeBenchmark:
		return map[string]any{"benchmark": map[string]any{}}, nil
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
