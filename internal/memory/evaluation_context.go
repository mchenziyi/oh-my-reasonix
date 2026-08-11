package memory

// MEM-02-01: 评估上下文与 Generation Pin。
//
// EvaluationContext 冻结一次评估使用的历史世界（Project/Global Generation
// Pair）：它记录 generation_id、永久 input manifest 身份与 hash、compiler
// 版本与 context signature。评估必须锚定固定 Generation，禁止拿未来 CURRENT
// 评价过去检索；Generation 已清理时只能通过永久 Input Manifest 精确重建，
// 无法重建返回 unavailable（不猜测）。
//
// 本文件只新增只读类型与只读构建/重建函数：不修改任何已冻结 Schema，不写
// 事实，不改 CURRENT，不创建第二事实源。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ProjectGenerationRef pins the project Generation world of an evaluation.
type ProjectGenerationRef struct {
	SchemaVersion       int    `json:"schema_version"`
	Scope               Scope  `json:"scope"`
	GenerationID        string `json:"generation_id"`
	InputManifestID     string `json:"input_manifest_id"`
	InputManifestSHA256 string `json:"input_manifest_sha256"`
}

func (r ProjectGenerationRef) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("project generation ref: schema_version must be %d", SchemaVersion)
	}
	if r.Scope != ScopeProject {
		return errors.New("project generation ref: scope must be project")
	}
	if err := validateID(r.GenerationID, "generation_id"); err != nil {
		return fmt.Errorf("project generation ref: %w", err)
	}
	if err := validateID(r.InputManifestID, "input_manifest_id"); err != nil {
		return fmt.Errorf("project generation ref: %w", err)
	}
	return validateHash(r.InputManifestSHA256, "input_manifest_sha256")
}

func (r ProjectGenerationRef) canonMap() map[string]any {
	return map[string]any{
		"schema_version":        r.SchemaVersion,
		"scope":                 string(r.Scope),
		"generation_id":         r.GenerationID,
		"input_manifest_id":     r.InputManifestID,
		"input_manifest_sha256": r.InputManifestSHA256,
	}
}

// GlobalGenerationRef pins the global Generation world of an evaluation.
type GlobalGenerationRef struct {
	SchemaVersion       int    `json:"schema_version"`
	Scope               Scope  `json:"scope"`
	GenerationID        string `json:"generation_id"`
	InputManifestID     string `json:"input_manifest_id"`
	InputManifestSHA256 string `json:"input_manifest_sha256"`
}

func (r GlobalGenerationRef) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("global generation ref: schema_version must be %d", SchemaVersion)
	}
	if r.Scope != ScopeGlobal {
		return errors.New("global generation ref: scope must be global")
	}
	if err := validateID(r.GenerationID, "generation_id"); err != nil {
		return fmt.Errorf("global generation ref: %w", err)
	}
	if err := validateID(r.InputManifestID, "input_manifest_id"); err != nil {
		return fmt.Errorf("global generation ref: %w", err)
	}
	return validateHash(r.InputManifestSHA256, "input_manifest_sha256")
}

func (r GlobalGenerationRef) canonMap() map[string]any {
	return map[string]any{
		"schema_version":        r.SchemaVersion,
		"scope":                 string(r.Scope),
		"generation_id":         r.GenerationID,
		"input_manifest_id":     r.InputManifestID,
		"input_manifest_sha256": r.InputManifestSHA256,
	}
}

// EvaluationContext freezes the world an evaluation is anchored to. All
// fields are immutable; ContextSignature is the program-computed content
// hash of the document (excluding itself) and must match on every read.
type EvaluationContext struct {
	SchemaVersion           int                   `json:"schema_version"`
	ContextID               string                `json:"context_id"`
	Scope                   Scope                 `json:"scope"`
	ProjectGenerationRef    *ProjectGenerationRef `json:"project_generation_ref,omitempty"`
	GlobalGenerationRef     *GlobalGenerationRef  `json:"global_generation_ref,omitempty"`
	CompilerVersion         string                `json:"compiler_version"`
	CanonicalizationVersion int                   `json:"canonicalization_version"`
	ContextSignatureVersion int                   `json:"context_signature_version"`
	ContextSignature        string                `json:"context_signature"`
	CreatedAt               string                `json:"created_at"`
}

func (c EvaluationContext) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("evaluation context: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(c.ContextID, "context_id"); err != nil {
		return fmt.Errorf("evaluation context: %w", err)
	}
	if c.Scope != ScopeProject && c.Scope != ScopeGlobal {
		return errors.New("evaluation context: scope must be project or global")
	}
	if c.ProjectGenerationRef != nil {
		if c.Scope != ScopeProject {
			return errors.New("evaluation context: project generation ref is only valid in a project scope")
		}
		if err := c.ProjectGenerationRef.Validate(); err != nil {
			return fmt.Errorf("evaluation context: %w", err)
		}
	}
	if c.GlobalGenerationRef != nil {
		if c.Scope != ScopeGlobal {
			return errors.New("evaluation context: global generation ref is only valid in a global scope")
		}
		if err := c.GlobalGenerationRef.Validate(); err != nil {
			return fmt.Errorf("evaluation context: %w", err)
		}
	}
	if c.ProjectGenerationRef == nil && c.GlobalGenerationRef == nil {
		return errors.New("evaluation context: at least one generation ref is required")
	}
	if err := validateVersionID(c.CompilerVersion, "compiler_version"); err != nil {
		return fmt.Errorf("evaluation context: %w", err)
	}
	if c.CanonicalizationVersion < 1 {
		return errors.New("evaluation context: canonicalization_version must be >= 1")
	}
	if c.ContextSignatureVersion < 1 {
		return errors.New("evaluation context: context_signature_version must be >= 1")
	}
	if err := validateTime(c.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("evaluation context: %w", err)
	}
	if err := validateHash(c.ContextSignature, "context_signature"); err != nil {
		return fmt.Errorf("evaluation context: %w", err)
	}
	h, err := c.ContentHash()
	if err != nil {
		return fmt.Errorf("evaluation context: %w", err)
	}
	if c.ContextSignature != h {
		return errors.New("evaluation context: context_signature mismatch")
	}
	return nil
}

func (c EvaluationContext) canonMap() (map[string]any, error) {
	m := map[string]any{
		"schema_version":            c.SchemaVersion,
		"context_id":                c.ContextID,
		"scope":                     string(c.Scope),
		"compiler_version":          c.CompilerVersion,
		"canonicalization_version":  c.CanonicalizationVersion,
		"context_signature_version": c.ContextSignatureVersion,
		"created_at":                c.CreatedAt,
	}
	if c.ProjectGenerationRef != nil {
		m["project_generation_ref"] = c.ProjectGenerationRef.canonMap()
	}
	if c.GlobalGenerationRef != nil {
		m["global_generation_ref"] = c.GlobalGenerationRef.canonMap()
	}
	return m, nil
}

func (c EvaluationContext) CanonicalBytes() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (c EvaluationContext) ContentHash() (string, error) {
	b, err := c.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (c EvaluationContext) EncodeCanonical() ([]byte, error) {
	m, err := c.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// EvaluationContextRequest selects the world to freeze.
type EvaluationContextRequest struct {
	Scope        Scope
	ContextID    string
	GenerationID string // empty = the CURRENT generation
	Now          time.Time
}

// BuildEvaluationContext pins the CURRENT (or an explicit) generation of the
// store's scope and fully verifies generation.json + permanent manifest.
// Future references (generation recorded after Now), hash drift, missing
// scope and cross-scope use all fail closed with stable redacted codes.
func BuildEvaluationContext(ctx context.Context, store *FactStore, req EvaluationContextRequest) (*EvaluationContext, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "evaluation scope must be project or global")
	}
	if err := validateID(req.ContextID, "context_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid context id")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match evaluation scope")
	}
	now := req.Now
	if now.IsZero() {
		// The evaluation instant is explicit protocol input: a zero value
		// is a caller bug, never a licence to read the wall clock. Without
		// this, identical calls would produce different contexts over time.
		return nil, storeError(CodeDerivedInvalidInput, "evaluation requires an explicit now timestamp")
	}

	genID := req.GenerationID
	if genID == "" {
		cur, err := readCurrentPointer(store)
		if err != nil {
			return nil, err
		}
		genID = cur.GenerationID
	}
	if err := validateID(genID, "generation_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid generation id")
	}

	gen, mf, err := resolveGenerationWorld(ctx, store, req.Scope, genID, now)
	if err != nil {
		return nil, err
	}

	ec := EvaluationContext{
		SchemaVersion:           SchemaVersion,
		ContextID:               req.ContextID,
		Scope:                   req.Scope,
		CompilerVersion:         gen.CompilerVersion,
		CanonicalizationVersion: gen.CanonicalizationVersion,
		ContextSignatureVersion: 1,
		CreatedAt:               now.UTC().Format(time.RFC3339Nano),
	}
	if req.Scope == ScopeProject {
		ec.ProjectGenerationRef = &ProjectGenerationRef{
			SchemaVersion:       SchemaVersion,
			Scope:               ScopeProject,
			GenerationID:        gen.GenerationID,
			InputManifestID:     mf.GenerationID,
			InputManifestSHA256: mf.InputManifestSHA256,
		}
	} else {
		ec.GlobalGenerationRef = &GlobalGenerationRef{
			SchemaVersion:       SchemaVersion,
			Scope:               ScopeGlobal,
			GenerationID:        gen.GenerationID,
			InputManifestID:     mf.GenerationID,
			InputManifestSHA256: mf.InputManifestSHA256,
		}
	}
	sig, err := ec.ContentHash()
	if err != nil {
		return nil, storeError(CodeSchemaInvalid, "cannot compute context signature")
	}
	ec.ContextSignature = sig
	return &ec, nil
}

func readCurrentPointer(store *FactStore) (*currentPointer, error) {
	path, err := secureJoin(store.root, []string{"CURRENT"}, false, true)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storeError(CodeNotFound, "no current generation")
		}
		return nil, storeError(CodeCorruptFile, "cannot read CURRENT")
	}
	var cur currentPointer
	if err := json.Unmarshal(data, &cur); err != nil {
		return nil, storeError(CodeCorruptFile, "invalid CURRENT document")
	}
	if err := validateID(cur.GenerationID, "generation_id"); err != nil {
		return nil, storeError(CodeCorruptFile, "invalid CURRENT generation id")
	}
	return &cur, nil
}

// resolveGenerationWorld reads and fully verifies one published generation
// and its permanent manifest. Every check is fail closed.
func resolveGenerationWorld(ctx context.Context, store *FactStore, scope Scope, genID string, now time.Time) (generationDoc, GenerationInputManifest, error) {
	var zeroDoc generationDoc
	var zeroMf GenerationInputManifest

	genPath, err := secureJoin(store.root, []string{"generations", genID, "generation.json"}, false, true)
	if err != nil {
		return zeroDoc, zeroMf, err
	}
	genData, err := os.ReadFile(genPath)
	if err != nil {
		if os.IsNotExist(err) {
			return zeroDoc, zeroMf, storeError(CodeNotFound, "generation not found")
		}
		return zeroDoc, zeroMf, storeError(CodeCorruptFile, "cannot read generation document")
	}
	var gen generationDoc
	if err := json.Unmarshal(genData, &gen); err != nil {
		return zeroDoc, zeroMf, storeError(CodeCorruptFile, "invalid generation document")
	}
	if gen.SchemaVersion != SchemaVersion {
		return zeroDoc, zeroMf, storeError(CodeSchemaInvalid, "unsupported generation schema version")
	}
	if gen.GenerationID != genID {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "generation identity mismatch")
	}
	if !store.scopeMatches(gen.Scope) {
		return zeroDoc, zeroMf, storeError(CodeScopeMismatch, "generation scope mismatch")
	}
	if !generationCompilerAvailable(gen.CompilerVersion, gen.CanonicalizationVersion) {
		return zeroDoc, zeroMf, storeError(CodeGenerationCompilerUnavailable, "generation compiler unavailable")
	}
	h, err := gen.outputHash()
	if err != nil {
		return zeroDoc, zeroMf, storeError(CodeSchemaInvalid, "cannot compute generation output hash")
	}
	if gen.OutputGenerationSHA256 != h {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "generation output hash mismatch")
	}
	// Recompute the compiled output hash over the published views.
	gsi := NewGenerationStore(store)
	gs, ok := gsi.(*generationStore)
	if !ok {
		return zeroDoc, zeroMf, storeError(CodeSchemaInvalid, "internal generation store unavailable")
	}
	genDir, err := gs.publishedGenDir(ctx, genID)
	if err != nil {
		return zeroDoc, zeroMf, err
	}
	compiled, err := gs.compiledOutputHash(ctx, genDir)
	if err != nil {
		return zeroDoc, zeroMf, storeError(CodeCorruptFile, "cannot verify compiled output")
	}
	if gen.CompiledOutputSHA256 != compiled {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "compiled output hash mismatch")
	}

	// Permanent manifest must exist, match and not lie in the future.
	mfData, err := store.Get(ctx, FactKindGenerationInputManifest, genID)
	if err != nil {
		return zeroDoc, zeroMf, err
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfData)
	if err != nil {
		return zeroDoc, zeroMf, classifyDecodeError(err)
	}
	if mf.GenerationID != genID {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "manifest identity mismatch")
	}
	if !store.scopeMatches(mf.Scope) {
		return zeroDoc, zeroMf, storeError(CodeScopeMismatch, "manifest scope mismatch")
	}
	if mf.CompilerVersion != gen.CompilerVersion || mf.CanonicalizationVersion != gen.CanonicalizationVersion {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "manifest compiler mismatch")
	}
	if mf.OutputSHA256 != gen.OutputGenerationSHA256 {
		return zeroDoc, zeroMf, storeError(CodeHashMismatch, "manifest output hash mismatch")
	}
	created, err := time.Parse(time.RFC3339Nano, mf.CreatedAt)
	if err != nil {
		return zeroDoc, zeroMf, storeError(CodeSchemaInvalid, "invalid manifest timestamp")
	}
	if created.After(now) {
		return zeroDoc, zeroMf, storeError(CodeEvaluationFutureReference, "generation lies in the future")
	}
	return gen, mf, nil
}

// EvaluationRebuildStatus reports whether a pinned generation world is
// resolvable: available means fully verifiable in place or exactly rebuilt
// from the permanent manifest; unavailable means the world cannot be
// reconstructed and no guess is made.
type EvaluationRebuildStatus string

const (
	EvaluationRebuildAvailable   EvaluationRebuildStatus = "available"
	EvaluationRebuildUnavailable EvaluationRebuildStatus = "unavailable"
)

// EvaluationRebuild is the read-only result of resolving a pinned context.
type EvaluationRebuild struct {
	Status           EvaluationRebuildStatus
	ContextSignature string
	CompiledSHA256   string
	Outputs          map[string][]byte
}

// RebuildEvaluationContext verifies a pinned evaluation world. When the
// published generation is still in place it is fully re-verified. When it
// has been cleaned up, the world is rebuilt from the permanent input
// manifest and its output hash must match the manifest anchor exactly;
// otherwise the result is unavailable, never a guess. Signature drift and
// tampering still fail closed with errors.
func RebuildEvaluationContext(ctx context.Context, store *FactStore, ec *EvaluationContext) (*EvaluationRebuild, error) {
	if ec == nil {
		return nil, storeError(CodeDerivedInvalidInput, "nil evaluation context")
	}
	if err := ec.Validate(); err != nil {
		return nil, classifyValidateError(err)
	}
	if !store.scopeMatches(ec.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match evaluation context")
	}

	var genID, manifestSHA256 string
	if ec.ProjectGenerationRef != nil {
		genID = ec.ProjectGenerationRef.GenerationID
		manifestSHA256 = ec.ProjectGenerationRef.InputManifestSHA256
	} else {
		genID = ec.GlobalGenerationRef.GenerationID
		manifestSHA256 = ec.GlobalGenerationRef.InputManifestSHA256
	}

	// Probe whether the published generation is still in place: secureJoin
	// with creating=true resolves the path even when the directory or file
	// is gone, then the stat below decides which branch to take.
	probe, err := secureJoin(store.root, []string{"generations", genID, "generation.json"}, true, true)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(probe); err == nil {
		// Re-verify with the context's own pinned instant (parsed from
		// CreatedAt), never the wall clock: the rebuild must be
		// deterministic and anchored to the same world the context froze.
		anchor, err := time.Parse(time.RFC3339Nano, ec.CreatedAt)
		if err != nil {
			return nil, storeError(CodeSchemaInvalid, "invalid evaluation context timestamp")
		}
		gen, _, err := resolveGenerationWorld(ctx, store, ec.Scope, genID, anchor)
		if err != nil {
			return nil, err
		}
		return &EvaluationRebuild{
			Status:           EvaluationRebuildAvailable,
			ContextSignature: ec.ContextSignature,
			CompiledSHA256:   gen.CompiledOutputSHA256,
		}, nil
	}

	return rebuildFromManifest(ctx, store, genID, manifestSHA256, ec.ContextSignature)
}

// rebuildFromManifest reconstructs a cleaned-up generation from its permanent
// input manifest. Missing inputs or a failing compile yield unavailable, not
// an error; tampering with the manifest or its inputs fails closed.
func rebuildFromManifest(ctx context.Context, store *FactStore, genID, manifestSHA256, signature string) (*EvaluationRebuild, error) {
	mfData, err := store.Get(ctx, FactKindGenerationInputManifest, genID)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return &EvaluationRebuild{Status: EvaluationRebuildUnavailable}, nil
		}
		return nil, err
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfData)
	if err != nil {
		if ErrorCode(err) == CodeUnknownField || ErrorCode(err) == CodeInvalidJSON {
			// The manifest itself is corrupt: the world cannot be
			// reconstructed, but no sensitive detail leaks.
			return &EvaluationRebuild{Status: EvaluationRebuildUnavailable}, nil
		}
		return nil, classifyDecodeError(err)
	}
	if mf.GenerationID != genID || mf.InputManifestSHA256 != manifestSHA256 {
		return nil, storeError(CodeHashMismatch, "manifest identity or hash mismatch")
	}
	if !store.scopeMatches(mf.Scope) {
		return nil, storeError(CodeScopeMismatch, "manifest scope mismatch")
	}

	req := OKFCompileRequest{Scope: mf.Scope}
	for _, in := range mf.Inputs {
		f, err := readManifestInput(ctx, store, in)
		if err != nil {
			if ErrorCode(err) == CodeNotFound {
				return &EvaluationRebuild{Status: EvaluationRebuildUnavailable}, nil
			}
			return nil, err
		}
		switch v := f.(type) {
		case MemoryRevision:
			req.Revisions = append(req.Revisions, MemoryRevisionRef{
				MemoryID: v.MemoryID, Revision: v.Revision, ContentSHA256: v.ContentSHA256,
			})
		case MemoryEvidenceGeneration:
			req.Evidence = append(req.Evidence, MemoryEvidenceRef{
				MemoryID: v.MemoryID, Revision: v.Revision,
				EvidenceGeneration: v.EvidenceGeneration, EvidenceSetSHA256: v.EvidenceSetSHA256,
			})
		}
	}
	res, err := CompileOKF(ctx, store, req)
	if err != nil {
		if ErrorCode(err) == CodeOKFInvalidInput || ErrorCode(err) == CodeOKFCompileError {
			return &EvaluationRebuild{Status: EvaluationRebuildUnavailable}, nil
		}
		return nil, err
	}

	gen := generationDoc{
		SchemaVersion:           SchemaVersion,
		GenerationID:            mf.GenerationID,
		Scope:                   mf.Scope,
		CompilerVersion:         mf.CompilerVersion,
		CanonicalizationVersion: mf.CanonicalizationVersion,
		TransactionID:           mf.TransactionID,
		CompiledOutputSHA256:    res.CompiledSHA256,
	}
	if mf.BaseGeneration != nil {
		gen.BaseGeneration = *mf.BaseGeneration
	}
	outHash, err := gen.outputHash()
	if err != nil {
		return &EvaluationRebuild{Status: EvaluationRebuildUnavailable}, nil
	}
	if outHash != mf.OutputSHA256 {
		return nil, storeError(CodeHashMismatch, "manifest rebuild output hash mismatch")
	}
	return &EvaluationRebuild{
		Status:           EvaluationRebuildAvailable,
		ContextSignature: signature,
		CompiledSHA256:   res.CompiledSHA256,
		Outputs:          res.Outputs,
	}, nil
}

// readManifestInput loads one manifest input through the full verification
// chain: strict decode + validate + hash must match the manifest record.
func readManifestInput(ctx context.Context, store *FactStore, in ManifestInput) (Fact, error) {
	kind, key, err := resolveManifestInput(in.FactType, in.FactID)
	if err != nil {
		return nil, storeError(CodeSchemaInvalid, "invalid manifest input identity")
	}
	data, err := store.Get(ctx, kind, key)
	if err != nil {
		return nil, err
	}
	f, err := decodeKind(kind, data)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	h, err := f.ContentHash()
	if err != nil {
		return nil, storeError(CodeSchemaInvalid, "cannot compute manifest input hash")
	}
	if h != in.ContentSHA256 {
		return nil, storeError(CodeHashMismatch, "manifest input hash mismatch")
	}
	return f, nil
}
