package memory

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MemoryUsage records one concrete use of a memory revision by a task. It is
// an immutable, scope-bound audit fact: usage_count and last_used_at are
// derived from these facts and never stored on the Revision. Repeated writes
// of the same usage_id are NOOP, so a usage event can never be double
// counted.
//
// usage_stage pins the usage phase from the frozen lifecycle pipeline
// (retrieved → read → adopted → affected → evaluated): only affected and
// evaluated usages are attribution evidence for outcome_attributed memories
// (architecture 6.1); earlier phases still count as usage but never as
// help/harm. episode_id is the optional controlled identifier of the
// independent Episode/Root Task that produced this usage — it is a real
// independence signal, never a stand-in for usage_id. The Episode fact set
// itself is a later phase, so this stage only validates the identifier
// shape, not episode existence (documented protocol gap).
type MemoryUsage struct {
	SchemaVersion int    `json:"schema_version"`
	UsageID       string `json:"usage_id"`
	Scope         Scope  `json:"scope"`
	MemoryID      string `json:"memory_id"`
	Revision      int    `json:"revision"`
	UsageStage    string `json:"usage_stage"`
	EpisodeID     string `json:"episode_id"`
	OccurredAt    string `json:"occurred_at"`
	Source        string `json:"source"`
	ContentSHA256 string `json:"content_sha256"`
	CreatedAt     string `json:"created_at"`

	// MEM-02A evaluation anchors. Two forms are legal: Legacy (all anchor
	// fields absent, canonical bytes/hash identical to pre-MEM-02A) and
	// Anchored (all anchor fields present and valid). Partial anchors are
	// corrupt input and fail closed; nothing is auto-filled.
	RetrievalID             string                 `json:"retrieval_id,omitempty"`
	RootTaskID              string                 `json:"root_task_id,omitempty"`
	MemoryContext           *MemoryContext         `json:"memory_context,omitempty"`
	ContextSignatureVersion int                    `json:"context_signature_version,omitempty"`
	ContextSignature        string                 `json:"context_signature,omitempty"`
	ContextDescriptorRef    string                 `json:"context_descriptor_ref,omitempty"`
	ObservationProvenance   *ObservationProvenance `json:"observation_provenance,omitempty"`
}

// anchored reports whether the usage carries any MEM-02A anchor field.
func (u MemoryUsage) anchored() bool {
	return u.RetrievalID != "" || u.RootTaskID != "" || u.MemoryContext != nil ||
		u.ContextSignatureVersion != 0 || u.ContextSignature != "" ||
		u.ContextDescriptorRef != "" || u.ObservationProvenance != nil
}

// usageStages is the frozen five-phase pipeline; the schema rejects anything
// outside it and never normalizes.
var usageStages = map[string]bool{
	"retrieved": true,
	"read":      true,
	"adopted":   true,
	"affected":  true,
	"evaluated": true,
}

// usageStageAttributed reports whether a stage produces attribution evidence
// for outcome_attributed memories (affected + evaluated + confirmed
// attribution).
func usageStageAttributed(stage string) bool {
	return stage == "affected" || stage == "evaluated"
}

func (u MemoryUsage) Validate() error {
	if u.SchemaVersion != SchemaVersion {
		return fmt.Errorf("memory usage: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(u.UsageID, "usage_id"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := u.Scope.Validate(); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := validateID(u.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if u.Revision < 1 {
		return errors.New("memory usage: revision must be >= 1")
	}
	if !usageStages[u.UsageStage] {
		return errors.New("memory usage: usage_stage must be one of retrieved|read|adopted|affected|evaluated")
	}
	if u.EpisodeID != "" {
		if err := validateID(u.EpisodeID, "episode_id"); err != nil {
			return fmt.Errorf("memory usage: %w", err)
		}
	}
	if err := validateTime(u.OccurredAt, "occurred_at"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := validateID(u.Source, "source"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := validateHash(u.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := validateTime(u.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	return u.validateAnchors()
}

// validateAnchors enforces the all-or-none MEM-02A anchor contract: Legacy
// (everything absent) or Anchored (everything present and valid); any partial
// anchor is corrupt input and fails closed without auto-filling defaults.
func (u MemoryUsage) validateAnchors() error {
	if !u.anchored() {
		return nil // legacy form: canonical output stays pre-MEM-02A
	}
	if u.RetrievalID == "" {
		return errors.New("memory usage: anchored usage requires retrieval_id")
	}
	if err := validateID(u.RetrievalID, "retrieval_id"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if u.RootTaskID == "" {
		return errors.New("memory usage: anchored usage requires root_task_id")
	}
	if err := validateID(u.RootTaskID, "root_task_id"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if u.MemoryContext == nil {
		return errors.New("memory usage: anchored usage requires memory_context")
	}
	if err := u.MemoryContext.Validate(); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if u.ContextSignatureVersion != 1 {
		return errors.New("memory usage: context_signature_version must be 1")
	}
	if err := validateHash(u.ContextSignature, "context_signature"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if err := validateID(u.ContextDescriptorRef, "context_descriptor_ref"); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	if u.ObservationProvenance == nil {
		return errors.New("memory usage: anchored usage requires observation_provenance")
	}
	if err := u.ObservationProvenance.Validate(); err != nil {
		return fmt.Errorf("memory usage: %w", err)
	}
	return nil
}

func (u MemoryUsage) canonMap() (map[string]any, error) {
	occurred, err := normalizeTime(u.OccurredAt)
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(u.CreatedAt)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"schema_version": u.SchemaVersion,
		"usage_id":       u.UsageID,
		"scope":          string(u.Scope),
		"memory_id":      u.MemoryID,
		"revision":       u.Revision,
		"usage_stage":    u.UsageStage,
		"episode_id":     u.EpisodeID,
		"occurred_at":    occurred,
		"source":         u.Source,
		"content_sha256": u.ContentSHA256,
		"created_at":     created,
	}
	if u.anchored() {
		if u.MemoryContext == nil || u.ObservationProvenance == nil {
			return nil, errors.New("memory usage: partial anchors cannot be canonicalized")
		}
		ctxMap, err := u.MemoryContext.canonMap()
		if err != nil {
			return nil, err
		}
		provMap, err := u.ObservationProvenance.canonMap()
		if err != nil {
			return nil, err
		}
		m["retrieval_id"] = u.RetrievalID
		m["root_task_id"] = u.RootTaskID
		m["memory_context"] = ctxMap
		m["context_signature_version"] = u.ContextSignatureVersion
		m["context_signature"] = u.ContextSignature
		m["context_descriptor_ref"] = u.ContextDescriptorRef
		m["observation_provenance"] = provMap
	}
	return m, nil
}

func (u MemoryUsage) CanonicalBytes() ([]byte, error) {
	m, err := u.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (u MemoryUsage) ContentHash() (string, error) {
	b, err := u.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (u MemoryUsage) EncodeCanonical() ([]byte, error) {
	m, err := u.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// Outcome records the observed effect of one usage event on the task. The
// effect is the machine-readable attribution input for outcome_attributed
// memories; AttributionOverride Judgments may correct it, and the derived
// layer applies the latest override instead of trusting the raw outcome
// alone. external_failure marks failures caused by a third-party service: the
// derived layer never attributes them to the memory (help/harm counters stay
// untouched), matching "third-party failures are not auto-attributed".
type Outcome struct {
	SchemaVersion   int    `json:"schema_version"`
	OutcomeID       string `json:"outcome_id"`
	Scope           Scope  `json:"scope"`
	UsageID         string `json:"usage_id"`
	MemoryID        string `json:"memory_id"`
	Revision        int    `json:"revision"`
	Effect          string `json:"effect"`
	ExternalFailure bool   `json:"external_failure"`
	ContentSHA256   string `json:"content_sha256"`
	CreatedAt       string `json:"created_at"`
}

func (o Outcome) Validate() error {
	if o.SchemaVersion != SchemaVersion {
		return fmt.Errorf("outcome: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(o.OutcomeID, "outcome_id"); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	if err := o.Scope.Validate(); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	if err := validateID(o.UsageID, "usage_id"); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	if err := validateID(o.MemoryID, "memory_id"); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	if o.Revision < 1 {
		return errors.New("outcome: revision must be >= 1")
	}
	if err := validEffect(o.Effect, "effect"); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	if err := validateHash(o.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("outcome: %w", err)
	}
	return validateTime(o.CreatedAt, "created_at")
}

func (o Outcome) canonMap() (map[string]any, error) {
	created, err := normalizeTime(o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema_version":   o.SchemaVersion,
		"outcome_id":       o.OutcomeID,
		"scope":            string(o.Scope),
		"usage_id":         o.UsageID,
		"memory_id":        o.MemoryID,
		"revision":         o.Revision,
		"effect":           o.Effect,
		"external_failure": o.ExternalFailure,
		"content_sha256":   o.ContentSHA256,
		"created_at":       created,
	}, nil
}

func (o Outcome) CanonicalBytes() ([]byte, error) {
	m, err := o.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (o Outcome) ContentHash() (string, error) {
	b, err := o.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (o Outcome) EncodeCanonical() ([]byte, error) {
	m, err := o.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}
