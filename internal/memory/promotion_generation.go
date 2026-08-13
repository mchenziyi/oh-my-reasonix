package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// PromotionGenerationRequest describes publication of a Global OKF
// generation after a candidate has been materialized. All stores and
// compiler inputs are explicit; no project CURRENT is consulted.
type PromotionGenerationRequest struct {
	Candidate      GlobalPromotionCandidate
	Sources        []PromotionCandidateSource
	Target         MemoryRevision
	Global         *FactStore
	Compile        OKFCompileRequest
	EvaluationTime time.Time
	IdempotencyKey string
	BaseGeneration *string
}

// PublishPromotionGeneration materializes/validates the candidate, compiles
// the explicitly bound Global world, and commits one Generation transaction.
func PublishPromotionGeneration(ctx context.Context, req PromotionGenerationRequest) (CommitResult, error) {
	if req.Global == nil || !req.Global.scopeMatches(ScopeGlobal) {
		return CommitResult{}, storeError(CodeScopeMismatch, "promotion generation global store mismatch")
	}
	if req.EvaluationTime.IsZero() {
		return CommitResult{}, storeError(CodeDerivedInvalidInput, "promotion generation requires an explicit evaluation time")
	}
	binding, err := promotionGenerationRequestBinding(req)
	if err != nil {
		return CommitResult{}, storeError(CodeDerivedInvalidInput, "promotion generation request is invalid")
	}
	gs := NewGenerationStore(req.Global)
	tx, err := gs.Begin(ctx, BeginGenerationRequest{
		Scope: ScopeGlobal, BaseGeneration: req.BaseGeneration,
		CompilerVersion: OKFCompilerVersion, CanonicalizationVersion: OKFCanonicalizationVersion,
		SchemaVersion: SchemaVersion, IdempotencyKey: req.IdempotencyKey,
		RequestBindingSHA256: binding,
	})
	if err != nil {
		return CommitResult{}, err
	}
	if tx.AlreadyCommitted() {
		return gs.Commit(ctx, tx)
	}
	abort := func(cause error) (CommitResult, error) {
		_ = gs.Abort(ctx, tx, "promotion generation failed")
		return CommitResult{}, cause
	}
	if _, err := applyPromotionCandidateLocked(ctx, PromotionCandidateApplyRequest{Candidate: req.Candidate, Sources: req.Sources, Target: req.Target, Global: req.Global}); err != nil {
		return abort(err)
	}
	raw, err := req.Global.Get(ctx, FactKindPromotionCandidate, req.Candidate.CandidateID)
	if err != nil {
		return abort(err)
	}
	candidate, err := DecodeStrict[GlobalPromotionCandidate](raw)
	if err != nil || candidate.ContentSHA256 != req.Candidate.ContentSHA256 {
		return abort(storeError(CodeHashMismatch, "promotion candidate hash mismatch"))
	}
	compileReq := req.Compile
	compileReq.Scope = ScopeGlobal
	compileReq.EvaluationTime = req.EvaluationTime
	compileReq.BaseGeneration = req.BaseGeneration
	compiled, err := CompileOKF(ctx, req.Global, compileReq)
	if err != nil {
		return abort(err)
	}
	inputs := append([]ManifestInput{}, compiled.Inputs...)
	candidateType, candidateID, err := factIdentity(candidate)
	if err != nil {
		return abort(storeError(CodeGenerationManifestMismatch, "promotion candidate identity is invalid"))
	}
	candidateHash, err := candidate.ContentHash()
	if err != nil {
		return abort(storeError(CodeGenerationManifestMismatch, "promotion candidate hash is invalid"))
	}
	inputs = append(inputs, ManifestInput{FactType: candidateType, FactID: candidateID, FactSchemaVersion: factSchemaVersion(candidate), ContentSHA256: candidateHash})
	inputs, err = dedupeManifestInputs(inputs)
	if err != nil {
		return abort(storeError(CodeGenerationManifestMismatch, "promotion generation inputs conflict"))
	}
	for _, in := range inputs {
		kind, key, rerr := resolveManifestInput(in.FactType, in.FactID)
		if rerr != nil {
			return abort(storeError(CodeGenerationManifestMismatch, "promotion generation input identity is invalid"))
		}
		data, gerr := req.Global.Get(ctx, kind, key)
		if gerr != nil {
			return abort(gerr)
		}
		fact, derr := decodeKind(kind, data)
		if derr != nil {
			return abort(storeError(CodeGenerationManifestMismatch, "promotion generation input is unreadable"))
		}
		if err := gs.PrepareFact(ctx, tx, fact); err != nil {
			return abort(err)
		}
	}
	manifest := manifestForPromotionGeneration(tx, inputs, req.EvaluationTime)
	if err := gs.PrepareManifest(ctx, tx, manifest); err != nil {
		return abort(err)
	}
	if err := gs.WriteCompiledOutput(ctx, tx, compiled.Outputs); err != nil {
		return abort(err)
	}
	if err := gs.ValidateStaging(ctx, tx); err != nil {
		return abort(err)
	}
	result, err := gs.Commit(ctx, tx)
	if err != nil {
		return result, err
	}
	return result, nil
}

func promotionGenerationRequestBinding(req PromotionGenerationRequest) (string, error) {
	sources := make([]struct {
		Ref               MemoryRef
		FamilyFingerprint string
	}, len(req.Sources))
	for i, source := range req.Sources {
		sources[i].Ref = source.Ref
		sources[i].FamilyFingerprint = source.FamilyFingerprint
	}
	b, err := json.Marshal(struct {
		Candidate      GlobalPromotionCandidate
		Sources        any
		Target         MemoryRevision
		Compile        OKFCompileRequest
		EvaluationTime time.Time
		BaseGeneration *string
	}{req.Candidate, sources, req.Target, req.Compile, req.EvaluationTime.UTC(), req.BaseGeneration})
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func manifestForPromotionGeneration(tx *GenerationTx, inputs []ManifestInput, now time.Time) GenerationInputManifest {
	gen := generationDoc{SchemaVersion: SchemaVersion, GenerationID: tx.GenerationID, Scope: ScopeGlobal, CompilerVersion: tx.CompilerVersion, CanonicalizationVersion: tx.CanonicalizationVersion, TransactionID: tx.TransactionID}
	if tx.BaseGeneration != nil {
		gen.BaseGeneration = *tx.BaseGeneration
	}
	out, err := gen.outputHash()
	if err != nil {
		panic(fmt.Sprintf("generation output hash: %v", err))
	}
	m := GenerationInputManifest{SchemaVersion: SchemaVersion, GenerationID: tx.GenerationID, Scope: ScopeGlobal, BaseGeneration: tx.BaseGeneration, CompilerVersion: tx.CompilerVersion, CanonicalizationVersion: tx.CanonicalizationVersion, Inputs: inputs, OutputSHA256: out, TransactionID: tx.TransactionID, CreatedAt: now.UTC().Format(time.RFC3339Nano)}
	h, err := m.ContentHash()
	if err != nil {
		panic(fmt.Sprintf("manifest hash: %v", err))
	}
	m.InputManifestSHA256 = h
	return m
}
