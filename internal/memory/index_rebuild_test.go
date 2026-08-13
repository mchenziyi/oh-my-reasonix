package memory

import (
	"context"
	"testing"
)

func TestRebuildIndexPreviewRequiresPinnedInputs(t *testing.T) {
	_, err := RebuildIndexPreview(context.Background(), nil, IndexRebuildRequest{Scope: ScopeProject})
	if ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("nil store must fail with scope mismatch, got %v", err)
	}
}

func TestRebuildIndexPreviewRequiresEvaluationTime(t *testing.T) {
	store := &FactStore{storeScope: StoreScopeProject}
	_, err := RebuildIndexPreview(context.Background(), store, IndexRebuildRequest{Scope: ScopeProject})
	if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero evaluation time must fail deterministically, got %v", err)
	}
}
