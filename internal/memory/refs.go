package memory

import (
	"errors"
	"fmt"
)

// MemoryRef references a memory version. Machine relations must never use
// paths, titles or Markdown file names.
type MemoryRef struct {
	Scope         Scope      `json:"scope"`
	MemoryType    MemoryType `json:"memory_type"`
	MemoryID      string     `json:"memory_id"`
	Revision      int        `json:"revision"`
	ContentSHA256 string     `json:"content_sha256"`
}

func (r MemoryRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("memory ref: %w", err)
	}
	if err := r.MemoryType.Validate(); err != nil {
		return fmt.Errorf("memory ref: %w", err)
	}
	if err := validateID(r.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("memory ref: %w", err)
	}
	if r.Revision < 1 {
		return errors.New("memory ref: revision must be >= 1")
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r MemoryRef) canonMap() (map[string]any, error) {
	return map[string]any{
		"scope":          string(r.Scope),
		"memory_type":    string(r.MemoryType),
		"memory_id":      r.MemoryID,
		"revision":       r.Revision,
		"content_sha256": r.ContentSHA256,
	}, nil
}

// EvidenceRef references an evidence fact. evidence_type is a
// schema-bound identifier; its vocabulary is fixed by the fact registries in
// later MEM phases, not by Architecture v1.
type EvidenceRef struct {
	Scope         Scope  `json:"scope"`
	EvidenceType  string `json:"evidence_type"`
	EvidenceID    string `json:"evidence_id"`
	ContentSHA256 string `json:"content_sha256"`
}

func (r EvidenceRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("evidence ref: %w", err)
	}
	if err := validateID(r.EvidenceType, "evidence_type"); err != nil {
		return fmt.Errorf("evidence ref: %w", err)
	}
	if err := validateID(r.EvidenceID, "evidence_id"); err != nil {
		return fmt.Errorf("evidence ref: %w", err)
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r EvidenceRef) canonMap() (map[string]any, error) {
	return map[string]any{
		"scope":          string(r.Scope),
		"evidence_type":  r.EvidenceType,
		"evidence_id":    r.EvidenceID,
		"content_sha256": r.ContentSHA256,
	}, nil
}

// JudgmentRef references an immutable JudgmentFact.
type JudgmentRef struct {
	Scope         Scope        `json:"scope"`
	JudgmentType  JudgmentType `json:"judgment_type"`
	JudgmentID    string       `json:"judgment_id"`
	ContentSHA256 string       `json:"content_sha256"`
}

func (r JudgmentRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("judgment ref: %w", err)
	}
	if err := r.JudgmentType.Validate(); err != nil {
		return fmt.Errorf("judgment ref: %w", err)
	}
	if err := validateID(r.JudgmentID, "judgment_id"); err != nil {
		return fmt.Errorf("judgment ref: %w", err)
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r JudgmentRef) canonMap() (map[string]any, error) {
	return map[string]any{
		"scope":          string(r.Scope),
		"judgment_type":  string(r.JudgmentType),
		"judgment_id":    r.JudgmentID,
		"content_sha256": r.ContentSHA256,
	}, nil
}

// ConfirmationSourceRef is a constrained JudgmentRef whose judgment_type
// must be confirmation.
type ConfirmationSourceRef struct {
	Scope         Scope        `json:"scope"`
	JudgmentType  JudgmentType `json:"judgment_type"`
	JudgmentID    string       `json:"judgment_id"`
	ContentSHA256 string       `json:"content_sha256"`
}

func (r ConfirmationSourceRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("confirmation source ref: %w", err)
	}
	if r.JudgmentType != JudgmentTypeConfirmation {
		return fmt.Errorf("confirmation source ref: judgment_type must be confirmation, got %q", r.JudgmentType)
	}
	if err := validateID(r.JudgmentID, "judgment_id"); err != nil {
		return fmt.Errorf("confirmation source ref: %w", err)
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r ConfirmationSourceRef) canonMap() (map[string]any, error) {
	return map[string]any{
		"scope":          string(r.Scope),
		"judgment_type":  string(r.JudgmentType),
		"judgment_id":    r.JudgmentID,
		"content_sha256": r.ContentSHA256,
	}, nil
}

// BasisRef is a discriminated union of exactly one reference, used by
// governance events, judgments and revalidation records.
type BasisRef struct {
	MemoryRef   *MemoryRef   `json:"memory_ref,omitempty"`
	EvidenceRef *EvidenceRef `json:"evidence_ref,omitempty"`
	JudgmentRef *JudgmentRef `json:"judgment_ref,omitempty"`
	PolicyRef   *PolicyRef   `json:"policy_ref,omitempty"`
}

func (b BasisRef) Validate() error {
	n := 0
	for _, set := range []bool{b.MemoryRef != nil, b.EvidenceRef != nil, b.JudgmentRef != nil, b.PolicyRef != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("basis ref: exactly one ref must be set, got %d", n)
	}
	switch {
	case b.MemoryRef != nil:
		return b.MemoryRef.Validate()
	case b.EvidenceRef != nil:
		return b.EvidenceRef.Validate()
	case b.JudgmentRef != nil:
		return b.JudgmentRef.Validate()
	default:
		return b.PolicyRef.Validate()
	}
}

func (b BasisRef) canonMap() (map[string]any, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	switch {
	case b.MemoryRef != nil:
		ref, err := b.MemoryRef.canonMap()
		if err != nil {
			return nil, err
		}
		return map[string]any{"memory_ref": ref}, nil
	case b.EvidenceRef != nil:
		ref, err := b.EvidenceRef.canonMap()
		if err != nil {
			return nil, err
		}
		return map[string]any{"evidence_ref": ref}, nil
	case b.JudgmentRef != nil:
		ref, err := b.JudgmentRef.canonMap()
		if err != nil {
			return nil, err
		}
		return map[string]any{"judgment_ref": ref}, nil
	case b.PolicyRef != nil:
		ref, err := b.PolicyRef.canonMap()
		if err != nil {
			return nil, err
		}
		return map[string]any{"policy_ref": ref}, nil
	default:
		return nil, errors.New("memory: empty basis ref")
	}
}
