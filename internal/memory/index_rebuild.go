package memory

import (
	"context"
	"time"
)

// IndexRebuildRequest pins every input used by the read-only index preview.
// An empty DerivationInputs is rejected so the operation can never silently
// fall back to scanning the whole store.
type IndexRebuildRequest struct {
	Scope            Scope
	EvaluationTime   time.Time
	IndexPolicyRef   PolicyRef
	DerivationInputs []ManifestInput
	Revisions        []MemoryRevisionRef
}

// IndexRebuildResult is a derived preview. It is never persisted by this API.
type IndexRebuildResult struct {
	Tree       *IndexTree
	RootIndex  RootIndexDoc
	LocalIndex LocalIndexDoc
}

// RebuildIndexPreview derives a deterministic index from explicitly pinned
// facts and an immutable index policy. It does not read CURRENT or write any
// fact, generation, manifest, or index file.
func RebuildIndexPreview(ctx context.Context, store *FactStore, req IndexRebuildRequest) (*IndexRebuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "index rebuild cancelled")
	}
	if err := req.Scope.Validate(); err != nil || store == nil || !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "index rebuild scope does not match store")
	}
	if req.EvaluationTime.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "index rebuild requires an explicit evaluation time")
	}
	if err := req.IndexPolicyRef.Validate(); err != nil || req.IndexPolicyRef.PolicyType != PolicyTypeIndex {
		return nil, storeError(CodeDerivedInvalidInput, "invalid index policy reference")
	}
	if len(req.DerivationInputs) == 0 || len(req.Revisions) == 0 {
		return nil, storeError(CodeOKFInvalidInput, "index rebuild requires explicit inputs")
	}
	policy, err := NewPolicyStore(store).GetPolicy(ctx, req.IndexPolicyRef)
	if err != nil {
		return nil, err
	}
	if policy.Config.Index == nil {
		return nil, storeError(CodeSchemaInvalid, "index policy config is missing")
	}
	states, _, err := deriveSelectedStates(ctx, store, req.Scope, req.Revisions, req.DerivationInputs, req.EvaluationTime)
	if err != nil {
		return nil, err
	}
	tree, err := compileIndexTree(req.Scope, states, *policy.Config.Index, &req.IndexPolicyRef, nil)
	if err != nil {
		return nil, err
	}
	root, local, _, err := buildIndexes(req.Scope, states, *policy.Config.Index)
	if err != nil {
		return nil, err
	}
	return &IndexRebuildResult{Tree: tree, RootIndex: root, LocalIndex: local}, nil
}
