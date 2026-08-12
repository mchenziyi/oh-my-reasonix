package memory

import (
	"bytes"
	"context"
	"time"
)

const CompositeCompilerVersion = "mnemosyne-composite-compiler/1"

type CompositeCompileRequest struct {
	Scope          Scope
	GenerationID   string
	EvaluationTime time.Time
	OKF            OKFCompileRequest
	EpisodeRefs    []EpisodeRef
	ContextRefs    []ContextDescriptorRef
}
type CompositeCompileResult struct {
	Outputs        map[string][]byte
	CompiledSHA256 string
	Inputs         []ManifestInput
}

func CompileComposite(ctx context.Context, store *FactStore, req CompositeCompileRequest) (*CompositeCompileResult, error) {
	if store == nil || req.Scope.Validate() != nil || !store.scopeMatches(req.Scope) || req.EvaluationTime.IsZero() || validateID(req.GenerationID, "generation_id") != nil {
		return nil, storeError(CodeOKFInvalidInput, "invalid composite compile request")
	}
	if req.OKF.Scope != req.Scope {
		return nil, storeError(CodeScopeMismatch, "composite compiler scope mismatch")
	}
	req.OKF.EvaluationTime = req.EvaluationTime
	okf, err := CompileOKF(ctx, store, req.OKF)
	if err != nil {
		return nil, err
	}
	episodic, err := CompileEpisodic(ctx, EpisodicCompileRequest{Scope: req.Scope, GenerationID: req.GenerationID, CompilerVersion: EpisodicCompilerVersion, EvaluationTime: req.EvaluationTime, EpisodeRefs: req.EpisodeRefs, ContextRefs: req.ContextRefs, Store: store})
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for p, b := range okf.Outputs {
		out[p] = append([]byte(nil), b...)
	}
	for p, b := range episodic.Outputs {
		if _, ok := out[p]; ok {
			return nil, storeError(CodeOKFCompileError, "composite output path collides")
		}
		out[p] = append([]byte(nil), b...)
	}
	root, ok := out["wiki/index.md"]
	if !ok {
		return nil, storeError(CodeOKFCompileError, "composite root index is missing")
	}
	route := []byte("\n## Episodic Recall\n\n- [Episodic Index](episodes/index.md)\n")
	if bytes.Contains(root, route) {
		return nil, storeError(CodeOKFCompileError, "composite root index route collides")
	}
	out["wiki/index.md"] = append(root, route...)
	policy, err := NewPolicyStore(store).GetPolicy(ctx, req.OKF.IndexPolicyRef)
	if err != nil || policy.Config.Index == nil || len(out["wiki/index.md"]) > policy.Config.Index.MaxPageBytes {
		return nil, storeError(CodeIndexPolicyUnsatisfied, "composite root index exceeds the configured byte limit")
	}
	inputs, err := dedupeManifestInputs(append(append([]ManifestInput{}, okf.Inputs...), episodic.Inputs...))
	if err != nil {
		return nil, storeError(CodeOKFInvalidInput, "composite manifest inputs conflict")
	}
	return &CompositeCompileResult{Outputs: out, CompiledSHA256: compiledOutputHash(out), Inputs: inputs}, nil
}
