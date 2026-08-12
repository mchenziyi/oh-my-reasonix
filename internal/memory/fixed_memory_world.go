package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type fixedMemoryWorld struct {
	store  *FactStore
	inputs map[string]ManifestInput
}

func loadFixedMemoryWorld(ctx context.Context, mc MemoryContext, scope Scope, projectStore, globalStore *FactStore) (*fixedMemoryWorld, error) {
	store := projectStore
	var manifestID, manifestHash string
	if scope == ScopeProject {
		if mc.ProjectGenerationRef == nil {
			return nil, errWorldUnavailable
		}
		manifestID = mc.ProjectGenerationRef.InputManifestID
		manifestHash = mc.ProjectGenerationRef.InputManifestSHA256
	} else {
		store = globalStore
		if mc.GlobalGenerationRef == nil {
			return nil, errWorldUnavailable
		}
		manifestID = mc.GlobalGenerationRef.InputManifestID
		manifestHash = mc.GlobalGenerationRef.InputManifestSHA256
	}
	if store == nil || !store.scopeMatches(scope) {
		return nil, errWorldUnavailable
	}
	data, err := store.Get(ctx, FactKindGenerationInputManifest, manifestID)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, errWorldUnavailable
		}
		return nil, err
	}
	manifest, err := DecodeStrict[GenerationInputManifest](data)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	if manifest.Scope != scope || manifest.InputManifestSHA256 != manifestHash {
		return nil, storeError(CodeHashMismatch, "fixed world manifest mismatch")
	}
	inputs := make(map[string]ManifestInput, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		inputs[input.FactType+"|"+input.FactID] = input
	}
	return &fixedMemoryWorld{store: store, inputs: inputs}, nil
}

func (w *fixedMemoryWorld) requireRevision(ref MemoryRef) error {
	key := "memory_revision|" + fmt.Sprintf("%s@%d", ref.MemoryID, ref.Revision)
	input, ok := w.inputs[key]
	if !ok {
		return errWorldUnavailable
	}
	if input.ContentSHA256 != ref.ContentSHA256 || input.FactSchemaVersion != SchemaVersion {
		return storeError(CodeHashMismatch, "fixed world revision reference mismatch")
	}
	return nil
}

func (w *fixedMemoryWorld) collectEvidence(ctx context.Context, memoryID string, revision int, now time.Time) (map[string]bool, map[string]bool, error) {
	evidence := make(map[string]bool)
	roots := make(map[string]bool)
	prefix := fmt.Sprintf("%s@%d:evidence@", memoryID, revision)
	for _, input := range w.inputs {
		if input.FactType != "memory_evidence_generation" || !strings.HasPrefix(input.FactID, prefix) {
			continue
		}
		kind, key, err := resolveManifestInput(input.FactType, input.FactID)
		if err != nil {
			return nil, nil, storeError(CodeSchemaInvalid, "fixed world evidence identity is invalid")
		}
		data, err := w.store.Get(ctx, kind, key)
		if err != nil {
			return nil, nil, err
		}
		generation, err := DecodeStrict[MemoryEvidenceGeneration](data)
		if err != nil {
			return nil, nil, classifyDecodeError(err)
		}
		if generation.EvidenceSetSHA256 != input.ContentSHA256 || generation.MemoryID != memoryID || generation.Revision != revision {
			return nil, nil, storeError(CodeHashMismatch, "fixed world evidence reference mismatch")
		}
		if err := rejectFutureFactTime(generation.CreatedAt, now, "evidence generation lies in the future"); err != nil {
			return nil, nil, err
		}
		for _, ref := range generation.EvidenceRefs {
			evidence[evidenceKey(ref)] = true
		}
		for _, root := range generation.RootTaskRefs {
			roots[root] = true
		}
	}
	return evidence, roots, nil
}
