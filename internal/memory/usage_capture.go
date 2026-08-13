package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
)

const maxUsageSelections = 32

// MemoryUsageSelection is an untrusted, transient statement of the highest
// stage the parent agent reached for one candidate. It is never stored as a
// fact and cannot carry attribution, timestamps, identifiers or hashes of its
// own.
type MemoryUsageSelection struct {
	MemoryRef  MemoryRef `json:"memory_ref"`
	UsageStage string    `json:"usage_stage"`
}

// MemoryUsageReceipt is a transient Reasonix protocol object. OMR derives all
// trusted MemoryUsage fields from fixed facts and references after validating
// this envelope.
type MemoryUsageReceipt struct {
	SchemaVersion int                    `json:"schema_version"`
	RetrievalID   string                 `json:"retrieval_id"`
	RootTaskID    string                 `json:"root_task_id"`
	MemoryContext RetrievalContext       `json:"memory_context"`
	EpisodeRef    EpisodeRef             `json:"episode_ref"`
	Usages        []MemoryUsageSelection `json:"usages"`
}

func (r MemoryUsageReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersion || validateID(r.RetrievalID, "retrieval_id") != nil || validateID(r.RootTaskID, "root_task_id") != nil {
		return storeError(CodeUsageCaptureInvalid, "usage receipt envelope is invalid")
	}
	if r.MemoryContext.Validate() != nil || r.MemoryContext.RetrievalID != r.RetrievalID || r.EpisodeRef.Validate() != nil {
		return storeError(CodeUsageCaptureInvalid, "usage receipt references are invalid")
	}
	if len(r.Usages) > maxUsageSelections {
		return storeError(CodeUsageCaptureInvalid, "usage receipt exceeds selection limit")
	}
	for _, selection := range r.Usages {
		if selection.MemoryRef.Validate() != nil || !usageReceiptStage(selection.UsageStage) {
			return storeError(CodeUsageCaptureInvalid, "usage receipt selection is invalid")
		}
	}
	return nil
}

func usageReceiptStage(stage string) bool {
	return stage == "read" || stage == "adopted" || stage == "affected" || stage == "evaluated"
}

func (r MemoryUsageReceipt) canonMap() (map[string]any, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	usages := append([]MemoryUsageSelection(nil), r.Usages...)
	sort.Slice(usages, func(i, j int) bool {
		a, b := librarianMemoryRefKey(usages[i].MemoryRef), librarianMemoryRefKey(usages[j].MemoryRef)
		if a != b {
			return a < b
		}
		return usageStageRank(usages[i].UsageStage) < usageStageRank(usages[j].UsageStage)
	})
	return map[string]any{"schema_version": r.SchemaVersion, "retrieval_id": r.RetrievalID, "root_task_id": r.RootTaskID, "memory_context": r.MemoryContext, "episode_ref": r.EpisodeRef, "usages": usages}, nil
}
func (r MemoryUsageReceipt) CanonicalBytes() ([]byte, error) {
	m, e := r.canonMap()
	if e != nil {
		return nil, e
	}
	return json.Marshal(m)
}
func (r MemoryUsageReceipt) ContentHash() (string, error) {
	b, e := r.CanonicalBytes()
	if e != nil {
		return "", e
	}
	return hashOf(b), nil
}
func (r MemoryUsageReceipt) EncodeCanonical() ([]byte, error) {
	m, e := r.canonMap()
	if e != nil {
		return nil, e
	}
	return json.MarshalIndent(m, "", "  ")
}

type CaptureUsageRequest struct {
	Store            *FactStore
	LibrarianReceipt LibrarianReceipt
	UsageReceipt     MemoryUsageReceipt
}

type CaptureUsageResult struct {
	SchemaVersion int      `json:"schema_version"`
	Created       int      `json:"created"`
	Noop          int      `json:"noop"`
	UsageIDs      []string `json:"usage_ids"`
}

// BuildMemoryUsages validates one fixed-world, single-scope capture and
// deterministically builds immutable facts without writing anything.
func BuildMemoryUsages(ctx context.Context, req CaptureUsageRequest) ([]MemoryUsage, error) {
	if req.Store == nil || req.UsageReceipt.Validate() != nil || req.LibrarianReceipt.RetrievalID != req.UsageReceipt.RetrievalID || !reflect.DeepEqual(req.LibrarianReceipt.MemoryContext, req.UsageReceipt.MemoryContext) {
		return nil, storeError(CodeUsageCaptureInvalid, "usage capture request is invalid")
	}
	scope := req.UsageReceipt.EpisodeRef.Scope
	if !req.Store.scopeMatches(scope) {
		return nil, storeError(CodeUsageCaptureInvalid, "usage capture scope is invalid")
	}
	pinned := retrievalScopeFor(req.LibrarianReceipt.MemoryContext, scope)
	if pinned == nil {
		return nil, storeError(CodeUsageCaptureInvalid, "usage capture has no fixed scope")
	}
	tree, err := LoadLibrarianIndexTree(ctx, req.Store, *pinned)
	if err != nil {
		return nil, err
	}
	if err := ValidateLibrarianReceipt(req.LibrarianReceipt, map[Scope]*IndexTree{scope: tree}); err != nil {
		return nil, err
	}

	candidates := make(map[string]MemoryRef)
	for _, candidate := range append(append([]LibrarianCandidate{}, req.LibrarianReceipt.RecommendedPages...), req.LibrarianReceipt.OptionalPages...) {
		if candidate.MemoryRef.Scope != scope {
			return nil, storeError(CodeUsageCaptureInvalid, "mixed scope usage capture is unsupported")
		}
		candidates[librarianMemoryRefKey(candidate.MemoryRef)] = candidate.MemoryRef
	}
	if len(candidates) > maxUsageSelections {
		return nil, storeError(CodeUsageCaptureInvalid, "usage capture exceeds candidate limit")
	}
	stages := make(map[string]string, len(candidates))
	for key := range candidates {
		stages[key] = "retrieved"
	}
	for _, selection := range req.UsageReceipt.Usages {
		key := librarianMemoryRefKey(selection.MemoryRef)
		if _, ok := candidates[key]; !ok {
			return nil, storeError(CodeUsageCaptureInvalid, "usage selection is outside the fixed receipt")
		}
		if usageStageRank(selection.UsageStage) > usageStageRank(stages[key]) {
			stages[key] = selection.UsageStage
		}
	}

	episode, descriptor, err := loadUsageEpisode(ctx, req.Store, req.UsageReceipt)
	if err != nil {
		return nil, err
	}
	memoryContext := memoryContextFromRetrieval(req.LibrarianReceipt.MemoryContext)
	if err := memoryContext.Validate(); err != nil {
		return nil, storeError(CodeUsageCaptureInvalid, "fixed memory context is invalid")
	}
	evidence := &EvidenceRef{Scope: scope, EvidenceType: "episode", EvidenceID: episode.EpisodeID, ContentSHA256: episode.ContentSHA256}

	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]MemoryUsage, 0, len(keys))
	for _, key := range keys {
		ref := candidates[key]
		u := MemoryUsage{SchemaVersion: SchemaVersion, UsageID: deterministicUsageID(req.UsageReceipt.RootTaskID, req.UsageReceipt.RetrievalID, ref), Scope: ref.Scope, MemoryID: ref.MemoryID, Revision: ref.Revision, UsageStage: stages[key], EpisodeID: episode.EpisodeID, OccurredAt: episode.OccurredAt, Source: "reasonix_receipt", CreatedAt: episode.CreatedAt, RetrievalID: req.UsageReceipt.RetrievalID, RootTaskID: episode.RootTaskID, MemoryContext: &memoryContext, ContextSignatureVersion: descriptor.ContextSignatureVersion, ContextSignature: descriptor.CanonicalSHA256, ContextDescriptorRef: descriptor.ContextDescriptorID, ObservationProvenance: &ObservationProvenance{Source: "runtime_observed", EvidenceRef: evidence}}
		h, err := u.ContentHash()
		if err != nil {
			return nil, storeError(CodeUsageCaptureInvalid, "usage fact cannot be hashed")
		}
		u.ContentSHA256 = h
		if err := u.Validate(); err != nil {
			return nil, storeError(CodeUsageCaptureInvalid, "usage fact is invalid")
		}
		out = append(out, u)
	}
	return out, nil
}

func CommitMemoryUsages(ctx context.Context, req CaptureUsageRequest) (CaptureUsageResult, error) {
	usages, err := BuildMemoryUsages(ctx, req)
	if err != nil {
		return CaptureUsageResult{}, err
	}
	unlock, err := req.Store.acquireWriteLock(ctx)
	if err != nil {
		return CaptureUsageResult{}, err
	}
	defer unlock()
	// Preflight every immutable identity while holding the same lock. A
	// conflict is therefore detected before the first write.
	for _, usage := range usages {
		b, err := req.Store.Get(ctx, FactKindMemoryUsage, usage.UsageID)
		if ErrorCode(err) == CodeNotFound {
			continue
		}
		if err != nil {
			return CaptureUsageResult{}, err
		}
		existing, err := DecodeStrict[MemoryUsage](b)
		if err != nil {
			return CaptureUsageResult{}, classifyDecodeError(err)
		}
		want, _ := usage.EncodeCanonical()
		got, _ := existing.EncodeCanonical()
		if !reflect.DeepEqual(want, got) {
			return CaptureUsageResult{}, storeError(CodeIdentityConflict, "same identity with different content hash")
		}
	}
	result := CaptureUsageResult{SchemaVersion: SchemaVersion, UsageIDs: make([]string, 0, len(usages))}
	for _, usage := range usages {
		wr, err := req.Store.putLocked(ctx, usage)
		if err != nil {
			return CaptureUsageResult{}, err
		}
		if wr.Status == WriteCreated {
			result.Created++
		} else {
			result.Noop++
		}
		result.UsageIDs = append(result.UsageIDs, usage.UsageID)
	}
	return result, nil
}

func retrievalScopeFor(c RetrievalContext, scope Scope) *RetrievalScopeContext {
	if scope == ScopeProject {
		return c.Project
	}
	if scope == ScopeGlobal {
		return c.Global
	}
	return nil
}
func memoryContextFromRetrieval(c RetrievalContext) MemoryContext {
	out := MemoryContext{}
	if c.Project != nil {
		out.ProjectGenerationRef = &ProjectGenerationRef{SchemaVersion: SchemaVersion, Scope: ScopeProject, GenerationID: c.Project.GenerationID, InputManifestID: c.Project.InputManifestID, InputManifestSHA256: c.Project.InputManifestSHA256}
	}
	if c.Global != nil {
		out.GlobalGenerationRef = &GlobalGenerationRef{SchemaVersion: SchemaVersion, Scope: ScopeGlobal, GenerationID: c.Global.GenerationID, InputManifestID: c.Global.InputManifestID, InputManifestSHA256: c.Global.InputManifestSHA256}
	}
	return out
}

func loadUsageEpisode(ctx context.Context, store *FactStore, r MemoryUsageReceipt) (EpisodeFact, ContextDescriptorFact, error) {
	b, err := store.Get(ctx, FactKindEpisode, r.EpisodeRef.EpisodeID)
	if ErrorCode(err) == CodeNotFound {
		return EpisodeFact{}, ContextDescriptorFact{}, storeError(CodeUsageCapturePending, "usage episode is not available")
	}
	if err != nil {
		return EpisodeFact{}, ContextDescriptorFact{}, err
	}
	episode, err := DecodeStrict[EpisodeFact](b)
	if err != nil || episode.Scope != r.EpisodeRef.Scope || episode.ContentSHA256 != r.EpisodeRef.ContentSHA256 || episode.RootTaskID != r.RootTaskID {
		return EpisodeFact{}, ContextDescriptorFact{}, storeError(CodeUsageCaptureInvalid, "usage episode does not match the receipt")
	}
	b, err = store.Get(ctx, FactKindContextDescriptor, episode.ContextDescriptorRef.ContextDescriptorID)
	if ErrorCode(err) == CodeNotFound {
		return EpisodeFact{}, ContextDescriptorFact{}, storeError(CodeUsageCapturePending, "usage context descriptor is not available")
	}
	if err != nil {
		return EpisodeFact{}, ContextDescriptorFact{}, err
	}
	descriptor, err := DecodeStrict[ContextDescriptorFact](b)
	if err != nil || descriptor.Scope != episode.Scope || descriptor.ContentSHA256 != episode.ContextDescriptorRef.ContentSHA256 {
		return EpisodeFact{}, ContextDescriptorFact{}, storeError(CodeUsageCaptureInvalid, "usage context descriptor does not match the episode")
	}
	return episode, descriptor, nil
}

func deterministicUsageID(rootTaskID, retrievalID string, ref MemoryRef) string {
	sum := sha256.Sum256([]byte(rootTaskID + "\x00" + retrievalID + "\x00" + librarianMemoryRefKey(ref)))
	return "usage_" + hex.EncodeToString(sum[:16])
}

func usageStageRank(stage string) int {
	switch stage {
	case "retrieved":
		return 0
	case "read":
		return 1
	case "adopted":
		return 2
	case "affected":
		return 3
	case "evaluated":
		return 4
	}
	return -1
}

var _ Fact = MemoryUsageReceipt{}
