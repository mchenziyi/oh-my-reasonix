package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type EpisodeRef struct {
	Scope         Scope  `json:"scope"`
	EpisodeID     string `json:"episode_id"`
	ContentSHA256 string `json:"content_sha256"`
}

func (r EpisodeRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := validateID(r.EpisodeID, "episode_id"); err != nil {
		return err
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

type ContextDescriptorRef struct {
	Scope               Scope  `json:"scope"`
	ContextDescriptorID string `json:"context_descriptor_id"`
	ContentSHA256       string `json:"content_sha256"`
}

func (r ContextDescriptorRef) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := validateID(r.ContextDescriptorID, "context_descriptor_id"); err != nil {
		return err
	}
	return validateHash(r.ContentSHA256, "content_sha256")
}

func (r ContextDescriptorRef) canonMap() (map[string]any, error) {
	return map[string]any{"scope": string(r.Scope), "context_descriptor_id": r.ContextDescriptorID, "content_sha256": r.ContentSHA256}, nil
}

type ContextEnvironment struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Language  string `json:"language"`
	Framework string `json:"framework"`
	Tool      string `json:"tool"`
}

type ContextDescriptorFact struct {
	SchemaVersion           int                `json:"schema_version"`
	ContextDescriptorID     string             `json:"context_descriptor_id"`
	Scope                   Scope              `json:"scope"`
	ContextSignatureVersion int                `json:"context_signature_version"`
	ComponentRefs           []string           `json:"component_refs"`
	OperationRefs           []string           `json:"operation_refs"`
	TaskClassRefs           []string           `json:"task_class_refs"`
	Environment             ContextEnvironment `json:"environment"`
	CanonicalSHA256         string             `json:"canonical_sha256"`
	ContentSHA256           string             `json:"content_sha256"`
	CreatedAt               string             `json:"created_at"`
}

func validateIDSet(xs []string, name string) error {
	if len(xs) > maxRefs {
		return fmt.Errorf("%s exceeds limit", name)
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if err := validateID(x, name); err != nil {
			return err
		}
		if seen[x] {
			return fmt.Errorf("%s contains duplicate", name)
		}
		seen[x] = true
	}
	return nil
}

func sortedStrings(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

func (c ContextDescriptorFact) descriptorMap() (map[string]any, error) {
	if err := validateIDSet(c.ComponentRefs, "component_ref"); err != nil {
		return nil, err
	}
	if err := validateIDSet(c.OperationRefs, "operation_ref"); err != nil {
		return nil, err
	}
	if err := validateIDSet(c.TaskClassRefs, "task_class_ref"); err != nil {
		return nil, err
	}
	for _, v := range []string{c.Environment.OS, c.Environment.Arch, c.Environment.Language, c.Environment.Tool} {
		if err := validateField(v); err != nil {
			return nil, err
		}
	}
	if c.Environment.Framework != "" {
		if err := validateField(c.Environment.Framework); err != nil {
			return nil, err
		}
	}
	return map[string]any{"context_signature_version": c.ContextSignatureVersion, "component_refs": sortedStrings(c.ComponentRefs), "operation_refs": sortedStrings(c.OperationRefs), "task_class_refs": sortedStrings(c.TaskClassRefs), "environment": map[string]any{"os": c.Environment.OS, "arch": c.Environment.Arch, "language": c.Environment.Language, "framework": c.Environment.Framework, "tool": c.Environment.Tool}}, nil
}

func (c ContextDescriptorFact) canonMap() (map[string]any, error) {
	d, err := c.descriptorMap()
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema_version": c.SchemaVersion, "context_descriptor_id": c.ContextDescriptorID, "scope": string(c.Scope), "context_signature_version": d["context_signature_version"], "component_refs": d["component_refs"], "operation_refs": d["operation_refs"], "task_class_refs": d["task_class_refs"], "environment": d["environment"], "canonical_sha256": c.CanonicalSHA256, "created_at": c.CreatedAt}, nil
}
func (c ContextDescriptorFact) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return errors.New("context descriptor: invalid schema version")
	}
	if err := validateID(c.ContextDescriptorID, "context_descriptor_id"); err != nil {
		return err
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if c.ContextSignatureVersion != 1 {
		return errors.New("context descriptor: unsupported signature version")
	}
	d, err := c.descriptorMap()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(d)
	if c.CanonicalSHA256 != hashOf(b) {
		return errors.New("context descriptor: canonical hash mismatch")
	}
	if err := validateTime(c.CreatedAt, "created_at"); err != nil {
		return err
	}
	h, err := c.ContentHash()
	if err != nil {
		return err
	}
	if h != c.ContentSHA256 {
		return errors.New("context descriptor: content hash mismatch")
	}
	return nil
}
func (c ContextDescriptorFact) CanonicalBytes() ([]byte, error) {
	m, e := c.canonMap()
	if e != nil {
		return nil, e
	}
	return json.Marshal(m)
}
func (c ContextDescriptorFact) ContentHash() (string, error) {
	b, e := c.CanonicalBytes()
	if e != nil {
		return "", e
	}
	return hashOf(b), nil
}
func (c ContextDescriptorFact) EncodeCanonical() ([]byte, error) {
	m, e := c.canonMap()
	if e != nil {
		return nil, e
	}
	h, e := c.ContentHash()
	if e != nil {
		return nil, e
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}

type EpisodeFact struct {
	SchemaVersion          int                  `json:"schema_version"`
	EpisodeID              string               `json:"episode_id"`
	Scope                  Scope                `json:"scope"`
	RootTaskID             string               `json:"root_task_id"`
	ContextDescriptorRef   ContextDescriptorRef `json:"context_descriptor_ref"`
	TaskClassRefs          []string             `json:"task_class_refs"`
	ComponentRefs          []string             `json:"component_refs"`
	OperationRefs          []string             `json:"operation_refs"`
	FailureConceptRefs     []string             `json:"failure_concept_refs"`
	TaskResult             string               `json:"task_result"`
	TaskResultEvidenceRefs []EvidenceRef        `json:"task_result_evidence_refs"`
	EvidenceRefs           []EvidenceRef        `json:"evidence_refs"`
	OccurredAt             string               `json:"occurred_at"`
	ContentSHA256          string               `json:"content_sha256"`
	CreatedAt              string               `json:"created_at"`
}

func (e EpisodeFact) canonMap() (map[string]any, error) {
	er := func(rs []EvidenceRef) ([]map[string]any, error) {
		out := make([]map[string]any, 0, len(rs))
		for _, r := range rs {
			if x := r.Validate(); x != nil {
				return nil, x
			}
			m, _ := r.canonMap()
			out = append(out, m)
		}
		sort.Slice(out, func(i, j int) bool {
			a, _ := json.Marshal(out[i])
			b, _ := json.Marshal(out[j])
			return string(a) < string(b)
		})
		return out, nil
	}
	tr, x := er(e.TaskResultEvidenceRefs)
	if x != nil {
		return nil, x
	}
	ev, x := er(e.EvidenceRefs)
	if x != nil {
		return nil, x
	}
	cr, _ := e.ContextDescriptorRef.canonMap()
	return map[string]any{"schema_version": e.SchemaVersion, "episode_id": e.EpisodeID, "scope": string(e.Scope), "root_task_id": e.RootTaskID, "context_descriptor_ref": cr, "task_class_refs": sortedStrings(e.TaskClassRefs), "component_refs": sortedStrings(e.ComponentRefs), "operation_refs": sortedStrings(e.OperationRefs), "failure_concept_refs": sortedStrings(e.FailureConceptRefs), "task_result": e.TaskResult, "task_result_evidence_refs": tr, "evidence_refs": ev, "occurred_at": e.OccurredAt, "created_at": e.CreatedAt}, nil
}
func (e EpisodeFact) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return errors.New("episode: invalid schema version")
	}
	if err := validateID(e.EpisodeID, "episode_id"); err != nil {
		return err
	}
	if err := e.Scope.Validate(); err != nil {
		return err
	}
	if err := validateID(e.RootTaskID, "root_task_id"); err != nil {
		return err
	}
	if err := e.ContextDescriptorRef.Validate(); err != nil {
		return err
	}
	if e.ContextDescriptorRef.Scope != e.Scope {
		return errors.New("episode: context scope mismatch")
	}
	for n, xs := range map[string][]string{"task_class_ref": e.TaskClassRefs, "component_ref": e.ComponentRefs, "operation_ref": e.OperationRefs, "failure_concept_ref": e.FailureConceptRefs} {
		if err := validateIDSet(xs, n); err != nil {
			return err
		}
	}
	if e.TaskResult != "succeeded" && e.TaskResult != "failed" && e.TaskResult != "cancelled" && e.TaskResult != "unknown" {
		return errors.New("episode: invalid task result")
	}
	if (e.TaskResult == "failed" || e.TaskResult == "cancelled") && len(e.TaskResultEvidenceRefs) == 0 {
		return errors.New("episode: task result evidence required")
	}
	if _, err := e.canonMap(); err != nil {
		return err
	}
	if err := validateTime(e.OccurredAt, "occurred_at"); err != nil {
		return err
	}
	if err := validateTime(e.CreatedAt, "created_at"); err != nil {
		return err
	}
	h, err := e.ContentHash()
	if err != nil {
		return err
	}
	if h != e.ContentSHA256 {
		return errors.New("episode: content hash mismatch")
	}
	return nil
}
func (e EpisodeFact) CanonicalBytes() ([]byte, error) {
	m, x := e.canonMap()
	if x != nil {
		return nil, x
	}
	return json.Marshal(m)
}
func (e EpisodeFact) ContentHash() (string, error) {
	b, x := e.CanonicalBytes()
	if x != nil {
		return "", x
	}
	return hashOf(b), nil
}
func (e EpisodeFact) EncodeCanonical() ([]byte, error) {
	m, x := e.canonMap()
	if x != nil {
		return nil, x
	}
	h, x := e.ContentHash()
	if x != nil {
		return nil, x
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}
