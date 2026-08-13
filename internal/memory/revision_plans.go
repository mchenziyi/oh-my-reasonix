package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// RevisionPlan is a read-only proposal for creating the next immutable
// revision. It is not a Fact and never writes to a FactStore.
type RevisionPlan struct {
	Operation    string        `json:"operation"`
	Source       MemoryRef     `json:"source"`
	Target       MemoryRef     `json:"target"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`
}

// BuildRevisionPlan validates a one-step revision without mutating the
// source or target facts.
func BuildRevisionPlan(source, target MemoryRevision, evidence []EvidenceRef) (RevisionPlan, error) {
	if err := source.Validate(); err != nil {
		return RevisionPlan{}, errors.New("revision plan: source revision is invalid")
	}
	if err := target.Validate(); err != nil {
		return RevisionPlan{}, errors.New("revision plan: target revision is invalid")
	}
	if source.MemoryID != target.MemoryID || source.Scope != target.Scope || source.MemoryType != target.MemoryType || target.Revision != source.Revision+1 {
		return RevisionPlan{}, errors.New("revision plan: target must be the next revision of the source")
	}
	refs, err := normalizeEvidenceRefs(evidence, source.Scope)
	if err != nil {
		return RevisionPlan{}, err
	}
	return RevisionPlan{Operation: "revise", Source: memoryRefFromRevision(source), Target: memoryRefFromRevision(target), EvidenceRefs: refs}, nil
}

// MergeInput is one immutable revision and its supporting evidence.
type MergeInput struct {
	Revision     MemoryRevision
	EvidenceRefs []EvidenceRef
}

// MergePlan is a deterministic, non-persistent merge proposal.
type MergePlan struct {
	Operation        string        `json:"operation"`
	Inputs           []MemoryRef   `json:"inputs"`
	Primary          MemoryRef     `json:"primary"`
	EvidenceRefs     []EvidenceRef `json:"evidence_refs"`
	ProposedMemoryID string        `json:"proposed_memory_id"`
}

// BuildMergePlan accepts revisions from one Scope and MemoryType. The primary
// source is selected by evidence count, then creation time, then stable ID.
func BuildMergePlan(inputs []MergeInput) (MergePlan, error) {
	if len(inputs) < 2 {
		return MergePlan{}, errors.New("merge plan: at least two revisions are required")
	}
	inputs = append([]MergeInput(nil), inputs...)
	refs := make([]MemoryRef, 0, len(inputs))
	allEvidence := make([]EvidenceRef, 0)
	evidenceSeen := make(map[string]struct{})
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if err := input.Revision.Validate(); err != nil {
			return MergePlan{}, errors.New("merge plan: invalid source revision")
		}
		key := revisionIdentityKey(input.Revision)
		if _, ok := seen[key]; ok {
			return MergePlan{}, errors.New("merge plan: duplicate source revision")
		}
		seen[key] = struct{}{}
		if len(refs) > 0 && (input.Revision.Scope != inputs[0].Revision.Scope || input.Revision.MemoryType != inputs[0].Revision.MemoryType) {
			return MergePlan{}, errors.New("merge plan: scope and memory type must match")
		}
		evidence, err := normalizeEvidenceRefs(input.EvidenceRefs, input.Revision.Scope)
		if err != nil {
			return MergePlan{}, err
		}
		refs = append(refs, memoryRefFromRevision(input.Revision))
		for _, ref := range evidence {
			key := planEvidenceKey(ref)
			if _, ok := evidenceSeen[key]; ok {
				continue
			}
			evidenceSeen[key] = struct{}{}
			allEvidence = append(allEvidence, ref)
		}
	}
	sort.Slice(inputs, func(i, j int) bool {
		if len(inputs[i].EvidenceRefs) != len(inputs[j].EvidenceRefs) {
			return len(inputs[i].EvidenceRefs) > len(inputs[j].EvidenceRefs)
		}
		left, _ := time.Parse(time.RFC3339Nano, inputs[i].Revision.CreatedAt)
		right, _ := time.Parse(time.RFC3339Nano, inputs[j].Revision.CreatedAt)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return inputs[i].Revision.MemoryID < inputs[j].Revision.MemoryID
	})
	primary := memoryRefFromRevision(inputs[0].Revision)
	refs = sortMemoryRefs(refs)
	evidence, err := normalizeEvidenceRefs(allEvidence, inputs[0].Revision.Scope)
	if err != nil {
		return MergePlan{}, err
	}
	seed := NewContentHash(mustPlanJSON(refs))
	return MergePlan{Operation: "merge", Inputs: refs, Primary: primary, EvidenceRefs: evidence, ProposedMemoryID: "merge_" + shortHash(seed)}, nil
}

type SplitBranch struct {
	Key              string        `json:"key"`
	EvidenceRefs     []EvidenceRef `json:"evidence_refs"`
	ProposedMemoryID string        `json:"proposed_memory_id"`
}

type SplitPlan struct {
	Operation string        `json:"operation"`
	Source    MemoryRef     `json:"source"`
	Branches  []SplitBranch `json:"branches"`
}

func BuildSplitPlan(source MemoryRevision, branches []SplitBranch) (SplitPlan, error) {
	if err := source.Validate(); err != nil {
		return SplitPlan{}, errors.New("split plan: source revision is invalid")
	}
	if len(branches) < 2 {
		return SplitPlan{}, errors.New("split plan: at least two branches are required")
	}
	seen := make(map[string]struct{}, len(branches))
	for i := range branches {
		if err := validateField(branches[i].Key); err != nil {
			return SplitPlan{}, errors.New("split plan: invalid branch key")
		}
		if _, ok := seen[branches[i].Key]; ok {
			return SplitPlan{}, errors.New("split plan: duplicate branch key")
		}
		seen[branches[i].Key] = struct{}{}
		refs, err := normalizeEvidenceRefs(branches[i].EvidenceRefs, source.Scope)
		if err != nil {
			return SplitPlan{}, err
		}
		if len(refs) == 0 {
			return SplitPlan{}, errors.New("split plan: every branch requires evidence")
		}
		branches[i].EvidenceRefs = refs
		branches[i].ProposedMemoryID = "split_" + shortHash(NewContentHash([]byte(source.MemoryID+":"+branches[i].Key)))
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].Key < branches[j].Key })
	return SplitPlan{Operation: "split", Source: memoryRefFromRevision(source), Branches: branches}, nil
}

type GeneralizeInput struct {
	Revision     MemoryRevision
	EvidenceRefs []EvidenceRef
	TrustStatus  TrustGateStatus
	Frozen       bool
	Archived     bool
}

type GeneralizePlan struct {
	Operation              string      `json:"operation"`
	Scope                  Scope       `json:"scope"`
	Inputs                 []MemoryRef `json:"inputs"`
	ProposedGlobalMemoryID string      `json:"proposed_global_memory_id"`
	PromotionEligible      bool        `json:"promotion_eligible"`
}

func BuildGeneralizePlan(inputs []GeneralizeInput) (GeneralizePlan, error) {
	if len(inputs) < 2 {
		return GeneralizePlan{}, errors.New("generalize plan: at least two project revisions are required")
	}
	refs := make([]MemoryRef, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if err := input.Revision.Validate(); err != nil || input.Revision.Scope != ScopeProject {
			return GeneralizePlan{}, errors.New("generalize plan: sources must be valid project revisions")
		}
		if input.TrustStatus != TrustGateTrusted || input.Frozen || input.Archived {
			return GeneralizePlan{}, errors.New("generalize plan: every source must be trusted and available")
		}
		if _, ok := seen[input.Revision.MemoryID]; ok {
			return GeneralizePlan{}, errors.New("generalize plan: duplicate project source")
		}
		seen[input.Revision.MemoryID] = struct{}{}
		evidence, err := normalizeEvidenceRefs(input.EvidenceRefs, ScopeProject)
		if err != nil {
			return GeneralizePlan{}, err
		}
		if len(evidence) == 0 {
			return GeneralizePlan{}, errors.New("generalize plan: every source requires evidence")
		}
		refs = append(refs, memoryRefFromRevision(input.Revision))
	}
	refs = sortMemoryRefs(refs)
	seed := NewContentHash(mustPlanJSON(refs))
	return GeneralizePlan{Operation: "generalize", Scope: ScopeGlobal, Inputs: refs, ProposedGlobalMemoryID: "global_" + shortHash(seed), PromotionEligible: true}, nil
}

func (p RevisionPlan) CanonicalBytes() ([]byte, error) {
	return json.Marshal(map[string]any{"evidence_refs": p.EvidenceRefs, "operation": p.Operation, "source": p.Source, "target": p.Target})
}
func (p MergePlan) CanonicalBytes() ([]byte, error)      { return json.Marshal(p) }
func (p SplitPlan) CanonicalBytes() ([]byte, error)      { return json.Marshal(p) }
func (p GeneralizePlan) CanonicalBytes() ([]byte, error) { return json.Marshal(p) }

func (p RevisionPlan) PlanHash() string   { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
func (p MergePlan) PlanHash() string      { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
func (p SplitPlan) PlanHash() string      { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
func (p GeneralizePlan) PlanHash() string { b, _ := p.CanonicalBytes(); return NewContentHash(b) }

func memoryRefFromRevision(r MemoryRevision) MemoryRef {
	return MemoryRef{Scope: r.Scope, MemoryType: r.MemoryType, MemoryID: r.MemoryID, Revision: r.Revision, ContentSHA256: r.ContentSHA256}
}
func revisionIdentityKey(r MemoryRevision) string {
	return fmt.Sprintf("%s/%s/%s/%d", r.Scope, r.MemoryType, r.MemoryID, r.Revision)
}
func normalizeEvidenceRefs(refs []EvidenceRef, scope Scope) ([]EvidenceRef, error) {
	seen := make(map[string]struct{}, len(refs))
	out := make([]EvidenceRef, 0, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil || ref.Scope != scope {
			return nil, errors.New("plan: evidence reference is invalid")
		}
		key := fmt.Sprintf("%s/%s/%s/%s", ref.Scope, ref.EvidenceType, ref.EvidenceID, ref.ContentSHA256)
		if _, ok := seen[key]; ok {
			return nil, errors.New("plan: duplicate evidence reference")
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return planEvidenceKey(out[i]) < planEvidenceKey(out[j]) })
	return out, nil
}
func planEvidenceKey(r EvidenceRef) string {
	return fmt.Sprintf("%s/%s/%s/%s", r.Scope, r.EvidenceType, r.EvidenceID, r.ContentSHA256)
}
func sortMemoryRefs(refs []MemoryRef) []MemoryRef {
	sort.Slice(refs, func(i, j int) bool {
		return fmt.Sprintf("%s/%s/%s/%d/%s", refs[i].Scope, refs[i].MemoryType, refs[i].MemoryID, refs[i].Revision, refs[i].ContentSHA256) < fmt.Sprintf("%s/%s/%s/%d/%s", refs[j].Scope, refs[j].MemoryType, refs[j].MemoryID, refs[j].Revision, refs[j].ContentSHA256)
	})
	return refs
}
func mustPlanJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
