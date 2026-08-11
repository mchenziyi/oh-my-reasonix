package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RetrievalEvaluation binds one immutable retrieval judgment to the fixed
// project/global memory world in which the retrieval happened. Judgment
// semantics remain solely in the referenced JudgmentFact.
type RetrievalEvaluation struct {
	SchemaVersion   int           `json:"schema_version"`
	EvaluationID    string        `json:"evaluation_id"`
	Scope           Scope         `json:"scope"`
	RetrievalID     string        `json:"retrieval_id"`
	MemoryContext   MemoryContext `json:"memory_context"`
	EvaluationScope string        `json:"evaluation_scope"`
	JudgmentRef     JudgmentRef   `json:"judgment_ref"`
	ContentSHA256   string        `json:"content_sha256"`
	CreatedAt       string        `json:"created_at"`
}

type RetrievalEvaluationStatus string

const (
	RetrievalEvaluationVerified    RetrievalEvaluationStatus = "verified"
	RetrievalEvaluationUnavailable RetrievalEvaluationStatus = "unavailable"
)

type RetrievalEvaluationRequest struct {
	Scope        Scope
	EvaluationID string
	ProjectStore *FactStore
	GlobalStore  *FactStore
	Now          time.Time
}

type RetrievalEvaluationResult struct {
	Status         RetrievalEvaluationStatus `json:"status"`
	EvaluationID   string                    `json:"evaluation_id"`
	RetrievalID    string                    `json:"retrieval_id"`
	JudgmentResult string                    `json:"judgment_result"`
	SourceType     string                    `json:"source_type"`
	EvaluatedAt    string                    `json:"evaluated_at"`
}

func (r RetrievalEvaluationResult) EncodeCanonical() ([]byte, error) {
	return json.Marshal(r)
}

var retrievalSourceTypes = map[string]bool{
	"fixture_oracle": true, "retrieval_critic": true, "user_review": true,
}

// ValidateRetrievalEvaluation verifies one immutable retrieval evaluation
// against its pinned project/global generation pair. It is strictly read-only.
func ValidateRetrievalEvaluation(ctx context.Context, store *FactStore, req RetrievalEvaluationRequest) (*RetrievalEvaluationResult, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "retrieval evaluation scope must be project or global")
	}
	if store == nil || !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match retrieval evaluation scope")
	}
	if err := validateID(req.EvaluationID, "evaluation_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid evaluation id")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "retrieval evaluation requires an explicit now")
	}

	raw, err := store.Get(ctx, FactKindRetrievalEvaluation, req.EvaluationID)
	if err != nil {
		return nil, err
	}
	eval, err := DecodeStrict[RetrievalEvaluation](raw)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	if eval.EvaluationID != req.EvaluationID {
		return nil, storeError(CodeHashMismatch, "retrieval evaluation identity mismatch")
	}
	if eval.Scope != req.Scope {
		return nil, storeError(CodeScopeMismatch, "retrieval evaluation scope mismatch")
	}
	if err := checkNotFuture(eval.CreatedAt, req.Now.UTC()); err != nil {
		return nil, err
	}

	worlds, unavailable, err := resolveRetrievalWorlds(ctx, eval.MemoryContext, req)
	if err != nil {
		return nil, err
	}
	if unavailable {
		return &RetrievalEvaluationResult{
			Status: RetrievalEvaluationUnavailable, EvaluationID: eval.EvaluationID,
			RetrievalID: eval.RetrievalID, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano),
		}, nil
	}

	jraw, err := store.Get(ctx, FactKindJudgment, eval.JudgmentRef.JudgmentID)
	if err != nil {
		return nil, err
	}
	j, err := DecodeStrict[JudgmentFact](jraw)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	if j.Scope != eval.JudgmentRef.Scope || j.JudgmentType != eval.JudgmentRef.JudgmentType ||
		j.JudgmentID != eval.JudgmentRef.JudgmentID || j.ContentSHA256 != eval.JudgmentRef.ContentSHA256 {
		return nil, storeError(CodeHashMismatch, "retrieval judgment ref mismatch")
	}
	if j.JudgmentType != JudgmentTypeRetrievalRelevance || j.RetrievalRelevance == nil {
		return nil, storeError(CodeSchemaInvalid, "retrieval evaluation requires a retrieval_relevance judgment")
	}
	if j.Subject.SubjectType != "retrieval" || j.Subject.RetrievalID != eval.RetrievalID {
		return nil, storeError(CodeSchemaInvalid, "retrieval judgment subject mismatch")
	}
	if !retrievalSourceTypes[j.Source.SourceType] {
		return nil, storeError(CodeSchemaInvalid, "retrieval judgment source is not allowed")
	}
	if err := checkNotFuture(j.CreatedAt, req.Now.UTC()); err != nil {
		return nil, err
	}
	for _, ref := range append(append([]MemoryRef{}, j.RetrievalRelevance.ExpectedMemoryRefs...), j.RetrievalRelevance.RetrievedMemoryRefs...) {
		if err := verifyRetrievalMemoryRef(ctx, worlds, ref); err != nil {
			if err == errWorldUnavailable {
				return &RetrievalEvaluationResult{Status: RetrievalEvaluationUnavailable, EvaluationID: eval.EvaluationID, RetrievalID: eval.RetrievalID, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano)}, nil
			}
			return nil, err
		}
	}
	for _, ref := range j.RetrievalRelevance.EvidenceRefs {
		if err := verifyRetrievalEvidenceRef(ctx, worlds, ref); err != nil {
			if err == errWorldUnavailable {
				return &RetrievalEvaluationResult{Status: RetrievalEvaluationUnavailable, EvaluationID: eval.EvaluationID, RetrievalID: eval.RetrievalID, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano)}, nil
			}
			return nil, err
		}
	}

	return &RetrievalEvaluationResult{
		Status: RetrievalEvaluationVerified, EvaluationID: eval.EvaluationID,
		RetrievalID: eval.RetrievalID, JudgmentResult: j.RetrievalRelevance.Result,
		SourceType: j.Source.SourceType, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano),
	}, nil
}

type retrievalPinnedWorld struct {
	store    *FactStore
	manifest GenerationInputManifest
}

func resolveRetrievalWorlds(ctx context.Context, mc MemoryContext, req RetrievalEvaluationRequest) (map[Scope]retrievalPinnedWorld, bool, error) {
	if err := mc.Validate(); err != nil {
		return nil, false, storeError(CodeSchemaInvalid, "invalid retrieval memory context")
	}
	worlds := make(map[Scope]retrievalPinnedWorld, 2)
	unavailable := false
	if r := mc.ProjectGenerationRef; r != nil {
		mf, missing, err := resolveRetrievalWorld(ctx, req.ProjectStore, r.Scope, r.GenerationID, r.InputManifestID, r.InputManifestSHA256, req.Now)
		if err != nil {
			return nil, false, err
		}
		if missing {
			unavailable = true
		} else {
			worlds[ScopeProject] = retrievalPinnedWorld{store: req.ProjectStore, manifest: mf}
		}
	}
	if r := mc.GlobalGenerationRef; r != nil {
		mf, missing, err := resolveRetrievalWorld(ctx, req.GlobalStore, r.Scope, r.GenerationID, r.InputManifestID, r.InputManifestSHA256, req.Now)
		if err != nil {
			return nil, false, err
		}
		if missing {
			unavailable = true
		} else {
			worlds[ScopeGlobal] = retrievalPinnedWorld{store: req.GlobalStore, manifest: mf}
		}
	}
	return worlds, unavailable, nil
}

func resolveRetrievalWorld(ctx context.Context, store *FactStore, scope Scope, genID, manifestID, manifestSHA string, now time.Time) (GenerationInputManifest, bool, error) {
	if store == nil {
		return GenerationInputManifest{}, true, nil
	}
	err := verifyGenerationRef(ctx, store, scope, genID, manifestID, manifestSHA, now)
	if err == errWorldUnavailable {
		return GenerationInputManifest{}, true, nil
	}
	if err != nil {
		return GenerationInputManifest{}, false, err
	}
	raw, err := store.Get(ctx, FactKindGenerationInputManifest, genID)
	if err != nil {
		return GenerationInputManifest{}, false, err
	}
	mf, err := DecodeStrict[GenerationInputManifest](raw)
	if err != nil {
		return GenerationInputManifest{}, false, classifyDecodeError(err)
	}
	if mf.GenerationID != manifestID || mf.InputManifestSHA256 != manifestSHA || mf.Scope != scope {
		return GenerationInputManifest{}, false, storeError(CodeHashMismatch, "retrieval generation manifest mismatch")
	}
	return mf, false, nil
}

func verifyRetrievalMemoryRef(ctx context.Context, worlds map[Scope]retrievalPinnedWorld, ref MemoryRef) error {
	w, ok := worlds[ref.Scope]
	if !ok {
		return errWorldUnavailable
	}
	wantID := fmt.Sprintf("%s@%d", ref.MemoryID, ref.Revision)
	for _, in := range w.manifest.Inputs {
		if in.FactType != "memory_revision" || in.FactID != wantID {
			continue
		}
		if in.ContentSHA256 != ref.ContentSHA256 {
			return storeError(CodeHashMismatch, "memory ref hash mismatch")
		}
		f, err := readManifestInput(ctx, w.store, in)
		if err != nil {
			return err
		}
		rev, ok := f.(MemoryRevision)
		if !ok || rev.Scope != ref.Scope || rev.MemoryType != ref.MemoryType || rev.MemoryID != ref.MemoryID || rev.Revision != ref.Revision || rev.ContentSHA256 != ref.ContentSHA256 {
			return storeError(CodeSchemaInvalid, "memory ref does not match the fixed generation")
		}
		return nil
	}
	return storeError(CodeSchemaInvalid, "memory ref is not in the fixed generation")
}

func verifyRetrievalEvidenceRef(ctx context.Context, worlds map[Scope]retrievalPinnedWorld, ref EvidenceRef) error {
	w, ok := worlds[ref.Scope]
	if !ok {
		return errWorldUnavailable
	}
	for _, in := range w.manifest.Inputs {
		if in.FactType != "memory_evidence_generation" {
			continue
		}
		f, err := readManifestInput(ctx, w.store, in)
		if err != nil {
			return err
		}
		ev, ok := f.(MemoryEvidenceGeneration)
		if !ok {
			return storeError(CodeSchemaInvalid, "invalid evidence generation input")
		}
		for _, member := range ev.EvidenceRefs {
			if evidenceRefsEqual(member, ref) {
				return nil
			}
		}
	}
	return storeError(CodeSchemaInvalid, "evidence ref is not in the fixed generation")
}

func (r RetrievalEvaluation) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("retrieval evaluation: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(r.EvaluationID, "evaluation_id"); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	if err := r.Scope.Validate(); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	if err := validateID(r.RetrievalID, "retrieval_id"); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	if err := r.MemoryContext.Validate(); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	switch r.EvaluationScope {
	case "fixture", "generation_full_scan", "expanded_index_scan", "sampled_audit":
	default:
		return errors.New("retrieval evaluation: invalid evaluation_scope")
	}
	if err := r.JudgmentRef.Validate(); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	if r.JudgmentRef.JudgmentType != JudgmentTypeRetrievalRelevance {
		return errors.New("retrieval evaluation: judgment_ref must be retrieval_relevance")
	}
	if r.JudgmentRef.Scope != r.Scope {
		return errors.New("retrieval evaluation: judgment_ref scope mismatch")
	}
	if err := validateTime(r.CreatedAt, "created_at"); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	if err := validateHash(r.ContentSHA256, "content_sha256"); err != nil {
		return fmt.Errorf("retrieval evaluation: %w", err)
	}
	h, err := r.ContentHash()
	if err != nil {
		return err
	}
	if h != r.ContentSHA256 {
		return errors.New("retrieval evaluation: content_sha256 mismatch")
	}
	return nil
}

func (r RetrievalEvaluation) canonMap() (map[string]any, error) {
	ctx, err := r.MemoryContext.canonMap()
	if err != nil {
		return nil, err
	}
	jr, err := r.JudgmentRef.canonMap()
	if err != nil {
		return nil, err
	}
	created, err := normalizeTime(r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"schema_version":   r.SchemaVersion,
		"evaluation_id":    r.EvaluationID,
		"scope":            string(r.Scope),
		"retrieval_id":     r.RetrievalID,
		"memory_context":   ctx,
		"evaluation_scope": r.EvaluationScope,
		"judgment_ref":     jr,
		"created_at":       created,
	}, nil
}

func (r RetrievalEvaluation) CanonicalBytes() ([]byte, error) {
	m, err := r.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (r RetrievalEvaluation) ContentHash() (string, error) {
	b, err := r.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (r RetrievalEvaluation) EncodeCanonical() ([]byte, error) {
	m, err := r.canonMap()
	if err != nil {
		return nil, err
	}
	h, err := r.ContentHash()
	if err != nil {
		return nil, err
	}
	m["content_sha256"] = h
	return json.MarshalIndent(m, "", "  ")
}
