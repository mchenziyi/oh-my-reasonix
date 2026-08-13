package memory

import (
	"context"
	"encoding/json"
	"time"
)

// IndexPublishRequest is the explicit, complete input to a Memory-only OKF
// Generation publish. The compiler remains pure; this wrapper only stages its
// exact outputs through the existing GenerationStore transaction.
type IndexPublishRequest struct {
	OKF            OKFCompileRequest
	IdempotencyKey string
}

type IndexPublishResult struct {
	Commit         CommitResult `json:"commit"`
	InputCount     int          `json:"input_count"`
	CompiledSHA256 string       `json:"compiled_sha256"`
}

// PublishIndexGeneration compiles a fixed Memory OKF world and publishes it
// through the single Generation/CURRENT transaction. It never publishes an
// Index Fact and refuses to replace a currently effective Composite world.
func PublishIndexGeneration(ctx context.Context, store *FactStore, req IndexPublishRequest) (IndexPublishResult, error) {
	if store == nil || req.OKF.Scope.Validate() != nil || !store.scopeMatches(req.OKF.Scope) {
		return IndexPublishResult{}, storeError(CodeScopeMismatch, "index publish scope does not match store")
	}
	if err := validateID(req.IdempotencyKey, "idempotency_key"); err != nil {
		return IndexPublishResult{}, storeError(CodeDerivedInvalidInput, "index publish idempotency key is invalid")
	}
	binding, err := indexPublishRequestHash(req)
	if err != nil {
		return IndexPublishResult{}, storeError(CodeDerivedInvalidInput, "index publish request is invalid")
	}
	gs := NewGenerationStore(store)
	concrete := gs.(*generationStore)
	current, err := concrete.readCurrent(ctx)
	if err != nil {
		return IndexPublishResult{}, err
	}
	if current != nil {
		gen, _, readErr := readPublishedGeneration(concrete, current.GenerationID)
		if readErr != nil {
			return IndexPublishResult{}, readErr
		}
		if gen.CompilerVersion == CompositeCompilerVersion {
			return IndexPublishResult{}, storeError(CodeGenerationCompilerUnavailable, "memory-only index publish cannot replace a composite generation")
		}
	}
	tx, err := gs.Begin(ctx, BeginGenerationRequest{
		Scope:                   req.OKF.Scope,
		BaseGeneration:          req.OKF.BaseGeneration,
		CompilerVersion:         OKFCompilerVersion,
		CanonicalizationVersion: OKFCanonicalizationVersion,
		SchemaVersion:           SchemaVersion,
		IdempotencyKey:          req.IdempotencyKey,
		RequestBindingSHA256:    binding,
	})
	if err != nil {
		return IndexPublishResult{}, err
	}
	abort := func(e error) (IndexPublishResult, error) {
		_ = gs.Abort(ctx, tx, "index publish failed")
		return IndexPublishResult{}, e
	}
	// Claim before compiling so a changed replay gets the stable idempotency
	// error even when its payload is otherwise invalid.
	compiled, err := CompileOKF(ctx, store, req.OKF)
	if err != nil {
		return abort(err)
	}
	for _, input := range compiled.Inputs {
		fact, readErr := readManifestInput(ctx, store, input)
		if readErr != nil {
			return abort(readErr)
		}
		if err := gs.PrepareFact(ctx, tx, fact); err != nil {
			return abort(err)
		}
	}
	gen := generationDoc{
		SchemaVersion:           SchemaVersion,
		GenerationID:            tx.GenerationID,
		Scope:                   tx.Scope,
		CompilerVersion:         tx.CompilerVersion,
		CanonicalizationVersion: tx.CanonicalizationVersion,
		TransactionID:           tx.TransactionID,
		CompiledOutputSHA256:    compiled.CompiledSHA256,
	}
	if tx.BaseGeneration != nil {
		gen.BaseGeneration = *tx.BaseGeneration
	}
	outHash, err := gen.outputHash()
	if err != nil {
		return abort(storeError(CodeSchemaInvalid, "index publish output hash cannot be computed"))
	}
	manifest := GenerationInputManifest{
		SchemaVersion:           SchemaVersion,
		GenerationID:            tx.GenerationID,
		Scope:                   tx.Scope,
		BaseGeneration:          tx.BaseGeneration,
		CompilerVersion:         tx.CompilerVersion,
		CanonicalizationVersion: tx.CanonicalizationVersion,
		Inputs:                  compiled.Inputs,
		OutputSHA256:            outHash,
		TransactionID:           tx.TransactionID,
		CreatedAt:               nowRFC3339(),
	}
	manifest.InputManifestSHA256, err = manifest.ContentHash()
	if err != nil {
		return abort(storeError(CodeSchemaInvalid, "index publish manifest hash cannot be computed"))
	}
	if err := gs.PrepareManifest(ctx, tx, manifest); err != nil {
		return abort(err)
	}
	if err := gs.WriteCompiledOutput(ctx, tx, compiled.Outputs); err != nil {
		return abort(err)
	}
	if err := gs.ValidateStaging(ctx, tx); err != nil {
		return abort(err)
	}
	commit, err := gs.Commit(ctx, tx)
	if err != nil {
		return IndexPublishResult{}, err
	}
	return IndexPublishResult{Commit: commit, InputCount: len(compiled.Inputs), CompiledSHA256: compiled.CompiledSHA256}, nil
}

func indexPublishRequestHash(req IndexPublishRequest) (string, error) {
	b, err := json.Marshal(struct {
		Scope            Scope
		BaseGeneration   *string
		EvaluationTime   time.Time
		IndexPolicyRef   PolicyRef
		DerivationInputs []ManifestInput
		Revisions        []MemoryRevisionRef
		Evidence         []MemoryEvidenceRef
	}{req.OKF.Scope, req.OKF.BaseGeneration, req.OKF.EvaluationTime.UTC(), req.OKF.IndexPolicyRef, req.OKF.DerivationInputs, req.OKF.Revisions, req.OKF.Evidence})
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}
