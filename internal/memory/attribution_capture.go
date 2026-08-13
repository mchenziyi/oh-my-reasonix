package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
)

const maxOutcomeCandidates = 32

// OutcomeCandidate is an untrusted model statement. It is transient and can
// never carry a fact identity, timestamp, hash or counted result.
type OutcomeCandidate struct {
	UsageID               string        `json:"usage_id"`
	TaskOutcome           string        `json:"task_outcome"`
	FailureCauseMemoryRef *MemoryRef    `json:"failure_cause_memory_ref"`
	MemoryEffect          string        `json:"memory_effect"`
	Attribution           string        `json:"attribution"`
	Critic                string        `json:"critic"`
	EvidenceRefs          []EvidenceRef `json:"evidence_refs"`
}

func (c OutcomeCandidate) Validate() error {
	if validateID(c.UsageID, "usage_id") != nil || validEffect(c.MemoryEffect, "memory_effect") != nil {
		return storeError(CodeAttributionCaptureInvalid, "attribution candidate is invalid")
	}
	if c.TaskOutcome != "succeeded" && c.TaskOutcome != "failed" && c.TaskOutcome != "cancelled" && c.TaskOutcome != "unknown" {
		return storeError(CodeAttributionCaptureInvalid, "attribution candidate task result is invalid")
	}
	if c.Attribution != "confirmed" && c.Attribution != "likely" && c.Attribution != "uncertain" {
		return storeError(CodeAttributionCaptureInvalid, "attribution confidence is invalid")
	}
	if c.Critic != "supported" && c.Critic != "unsupported" && c.Critic != "not_required" && c.Critic != "unavailable" {
		return storeError(CodeAttributionCaptureInvalid, "attribution critic result is invalid")
	}
	if c.FailureCauseMemoryRef != nil && c.FailureCauseMemoryRef.Validate() != nil {
		return storeError(CodeAttributionCaptureInvalid, "failure cause reference is invalid")
	}
	if len(c.EvidenceRefs) > maxRefs {
		return storeError(CodeAttributionCaptureInvalid, "attribution evidence limit exceeded")
	}
	seen := map[string]bool{}
	for _, ref := range c.EvidenceRefs {
		if ref.Validate() != nil || seen[evidenceRefKey(ref)] {
			return storeError(CodeAttributionCaptureInvalid, "attribution evidence is invalid")
		}
		seen[evidenceRefKey(ref)] = true
	}
	return nil
}

type AttributionReceipt struct {
	SchemaVersion int                `json:"schema_version"`
	EpisodeRef    EpisodeRef         `json:"episode_ref"`
	RootTaskID    string             `json:"root_task_id"`
	Candidates    []OutcomeCandidate `json:"candidates"`
}

func (r AttributionReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersion || r.EpisodeRef.Validate() != nil || validateID(r.RootTaskID, "root_task_id") != nil || len(r.Candidates) == 0 || len(r.Candidates) > maxOutcomeCandidates {
		return storeError(CodeAttributionCaptureInvalid, "attribution receipt envelope is invalid")
	}
	seen := map[string]bool{}
	for _, c := range r.Candidates {
		if c.Validate() != nil || seen[c.UsageID] {
			return storeError(CodeAttributionCaptureInvalid, "attribution receipt candidate is invalid")
		}
		seen[c.UsageID] = true
	}
	return nil
}

func (r AttributionReceipt) canonMap() (map[string]any, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	candidates := append([]OutcomeCandidate(nil), r.Candidates...)
	for i := range candidates {
		candidates[i].EvidenceRefs = append([]EvidenceRef(nil), candidates[i].EvidenceRefs...)
		sort.Slice(candidates[i].EvidenceRefs, func(a, b int) bool {
			return evidenceRefKey(candidates[i].EvidenceRefs[a]) < evidenceRefKey(candidates[i].EvidenceRefs[b])
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].UsageID < candidates[j].UsageID })
	return map[string]any{"schema_version": r.SchemaVersion, "episode_ref": r.EpisodeRef, "root_task_id": r.RootTaskID, "candidates": candidates}, nil
}
func (r AttributionReceipt) CanonicalBytes() ([]byte, error) {
	m, e := r.canonMap()
	if e != nil {
		return nil, e
	}
	return json.Marshal(m)
}
func (r AttributionReceipt) ContentHash() (string, error) {
	b, e := r.CanonicalBytes()
	if e != nil {
		return "", e
	}
	return hashOf(b), nil
}
func (r AttributionReceipt) EncodeCanonical() ([]byte, error) {
	m, e := r.canonMap()
	if e != nil {
		return nil, e
	}
	return json.MarshalIndent(m, "", "  ")
}

type AttributionRequest struct {
	Store           *FactStore
	Receipt         AttributionReceipt
	ExternalFailure bool
}

type CaptureOutcomeResult struct {
	SchemaVersion int      `json:"schema_version"`
	Created       int      `json:"created"`
	Noop          int      `json:"noop"`
	OutcomeIDs    []string `json:"outcome_ids"`
}

func BuildOutcomes(ctx context.Context, req AttributionRequest) ([]Outcome, error) {
	if req.Store == nil || req.Receipt.Validate() != nil || !req.Store.scopeMatches(req.Receipt.EpisodeRef.Scope) {
		return nil, storeError(CodeAttributionCaptureInvalid, "attribution request is invalid")
	}
	episode, err := loadAttributionEpisode(ctx, req.Store, req.Receipt)
	if err != nil {
		return nil, err
	}
	allowedEvidence := map[string]bool{}
	for _, ref := range episode.EvidenceRefs {
		allowedEvidence[evidenceRefKey(ref)] = true
	}
	episodeEvidence := EvidenceRef{Scope: episode.Scope, EvidenceType: "episode", EvidenceID: episode.EpisodeID, ContentSHA256: episode.ContentSHA256}
	allowedEvidence[evidenceRefKey(episodeEvidence)] = true
	out := make([]Outcome, 0, len(req.Receipt.Candidates))
	seenOutcomes := map[string]bool{}
	for _, c := range req.Receipt.Candidates {
		b, err := req.Store.Get(ctx, FactKindMemoryUsage, c.UsageID)
		if err != nil {
			return nil, err
		}
		u, err := DecodeStrict[MemoryUsage](b)
		if err != nil || !u.anchored() || u.Scope != episode.Scope || u.EpisodeID != episode.EpisodeID || u.RootTaskID != episode.RootTaskID || c.TaskOutcome != episode.TaskResult {
			return nil, storeError(CodeAttributionCaptureInvalid, "attribution usage boundary is invalid")
		}
		for _, ref := range c.EvidenceRefs {
			if ref.Scope != episode.Scope || !allowedEvidence[evidenceRefKey(ref)] {
				return nil, storeError(CodeAttributionCaptureInvalid, "attribution evidence is outside the episode")
			}
		}
		if c.FailureCauseMemoryRef != nil {
			if err := validateFailureCause(ctx, req.Store, u, *c.FailureCauseMemoryRef); err != nil {
				return nil, err
			}
		}
		effect := c.MemoryEffect
		critic := c.Critic
		if c.Attribution == "confirmed" && len(c.EvidenceRefs) == 0 {
			effect = "unknown"
		}
		if effect == "harmed" && critic != "supported" {
			effect = "unknown"
		}
		if effect != "harmed" && critic != "not_required" {
			critic = "not_required"
		}
		evaluated := u.UsageStage == "evaluated"
		countHelp := evaluated && effect == "helped" && c.Attribution == "confirmed" && !req.ExternalFailure
		countHarm := evaluated && effect == "harmed" && c.Attribution == "confirmed" && critic == "supported" && !req.ExternalFailure
		o := Outcome{SchemaVersion: SchemaVersion, OutcomeID: deterministicOutcomeID(u), Scope: u.Scope, UsageID: u.UsageID, MemoryID: u.MemoryID, Revision: u.Revision, Effect: effect, ExternalFailure: req.ExternalFailure, CreatedAt: episode.CreatedAt, EpisodeID: episode.EpisodeID, RootTaskID: episode.RootTaskID, ContextSignatureVersion: u.ContextSignatureVersion, ContextSignature: u.ContextSignature, ContextDescriptorRef: u.ContextDescriptorRef, TaskOutcome: episode.TaskResult, MemoryStage: u.UsageStage, Evaluated: boolPtr(evaluated), Attribution: c.Attribution, Critic: critic, EvidenceRefs: append([]EvidenceRef{}, c.EvidenceRefs...), CountedAsHelp: boolPtr(countHelp), CountedAsHarm: boolPtr(countHarm)}
		if seenOutcomes[o.OutcomeID] {
			return nil, storeError(CodeAttributionCaptureInvalid, "attribution receipt repeats an independent episode")
		}
		seenOutcomes[o.OutcomeID] = true
		if c.FailureCauseMemoryRef != nil {
			o.FailureCauseMemoryID = c.FailureCauseMemoryRef.MemoryID
		}
		h, err := o.ContentHash()
		if err != nil {
			return nil, storeError(CodeAttributionCaptureInvalid, "outcome fact cannot be hashed")
		}
		o.ContentSHA256 = h
		if err := o.Validate(); err != nil {
			return nil, storeError(CodeAttributionCaptureInvalid, "outcome fact is invalid")
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OutcomeID < out[j].OutcomeID })
	return out, nil
}

func CommitOutcomes(ctx context.Context, req AttributionRequest) (CaptureOutcomeResult, error) {
	outcomes, err := BuildOutcomes(ctx, req)
	if err != nil {
		return CaptureOutcomeResult{}, err
	}
	unlock, err := req.Store.acquireWriteLock(ctx)
	if err != nil {
		return CaptureOutcomeResult{}, err
	}
	defer unlock()
	for _, outcome := range outcomes {
		b, err := req.Store.Get(ctx, FactKindOutcome, outcome.OutcomeID)
		if ErrorCode(err) == CodeNotFound {
			continue
		}
		if err != nil {
			return CaptureOutcomeResult{}, err
		}
		existing, err := DecodeStrict[Outcome](b)
		if err != nil {
			return CaptureOutcomeResult{}, classifyDecodeError(err)
		}
		want, _ := outcome.EncodeCanonical()
		got, _ := existing.EncodeCanonical()
		if !reflect.DeepEqual(want, got) {
			return CaptureOutcomeResult{}, storeError(CodeIdentityConflict, "same identity with different content hash")
		}
	}
	result := CaptureOutcomeResult{SchemaVersion: SchemaVersion, OutcomeIDs: make([]string, 0, len(outcomes))}
	for _, outcome := range outcomes {
		wr, err := req.Store.putLocked(ctx, outcome)
		if err != nil {
			return CaptureOutcomeResult{}, err
		}
		if wr.Status == WriteCreated {
			result.Created++
		} else {
			result.Noop++
		}
		result.OutcomeIDs = append(result.OutcomeIDs, outcome.OutcomeID)
	}
	return result, nil
}

func loadAttributionEpisode(ctx context.Context, store *FactStore, r AttributionReceipt) (EpisodeFact, error) {
	b, err := store.Get(ctx, FactKindEpisode, r.EpisodeRef.EpisodeID)
	if err != nil {
		return EpisodeFact{}, err
	}
	e, err := DecodeStrict[EpisodeFact](b)
	if err != nil || e.Scope != r.EpisodeRef.Scope || e.ContentSHA256 != r.EpisodeRef.ContentSHA256 || e.RootTaskID != r.RootTaskID {
		return EpisodeFact{}, storeError(CodeAttributionCaptureInvalid, "attribution episode does not match receipt")
	}
	return e, nil
}

func deterministicOutcomeID(u MemoryUsage) string {
	sum := sha256.Sum256([]byte(string(u.Scope) + "\x00" + u.RootTaskID + "\x00" + u.MemoryID + "\x00" + strconv.Itoa(u.Revision) + "\x00" + u.ContextSignature))
	return "outcome_" + hex.EncodeToString(sum[:16])
}

func validateFailureCause(ctx context.Context, store *FactStore, usage MemoryUsage, ref MemoryRef) error {
	if usage.MemoryContext == nil || ref.Scope != usage.Scope || ref.MemoryType != MemoryTypeFailureConcept {
		return storeError(CodeAttributionCaptureInvalid, "failure cause is outside the attribution boundary")
	}
	var project, global *FactStore
	if usage.Scope == ScopeProject {
		project = store
	} else {
		global = store
	}
	world, err := loadFixedMemoryWorld(ctx, *usage.MemoryContext, usage.Scope, project, global)
	if err != nil {
		return err
	}
	if err := world.requireRevision(ref); err != nil {
		return err
	}
	b, err := store.Get(ctx, FactKindMemoryRevision, ref.MemoryID+"/"+strconv.Itoa(ref.Revision))
	if err != nil {
		return err
	}
	rev, err := DecodeStrict[MemoryRevision](b)
	if err != nil || rev.Scope != ref.Scope || rev.MemoryType != MemoryTypeFailureConcept || rev.ContentSHA256 != ref.ContentSHA256 {
		return storeError(CodeAttributionCaptureInvalid, "failure cause revision is invalid")
	}
	return nil
}

var _ Fact = AttributionReceipt{}

func boolPtr(v bool) *bool { return &v }
