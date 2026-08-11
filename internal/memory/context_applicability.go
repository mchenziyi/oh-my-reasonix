package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ContextApplicabilityStatus is a verifier status, not a persisted Judgment
// result. Unavailable is used for byte-compatible legacy judgments that do
// not carry the MEM-02E basis context anchor.
type ContextApplicabilityStatus string

const (
	ContextApplicabilityVerified    ContextApplicabilityStatus = "verified"
	ContextApplicabilityUnavailable ContextApplicabilityStatus = "unavailable"
)

type ContextApplicabilityRequest struct {
	Scope      Scope
	JudgmentID string
	Store      *FactStore
	Now        time.Time
}

type ContextApplicabilityResult struct {
	Status           ContextApplicabilityStatus `json:"status"`
	JudgmentID       string                     `json:"judgment_id"`
	MemoryRef        MemoryRef                  `json:"memory_ref"`
	TargetContextRef string                     `json:"target_context_ref"`
	Result           string                     `json:"result"`
	EvaluatedAt      string                     `json:"evaluated_at"`
}

func (r ContextApplicabilityResult) EncodeCanonical() ([]byte, error) {
	return json.Marshal(r)
}

// ValidateContextApplicability verifies one immutable Context Applicability
// judgment and its exact revision/evidence references. It never reads CURRENT
// and never writes to the store.
func ValidateContextApplicability(ctx context.Context, req ContextApplicabilityRequest) (*ContextApplicabilityResult, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "context applicability scope must be project or global")
	}
	if req.Store == nil {
		return nil, storeError(CodeDerivedInvalidInput, "context applicability requires a store")
	}
	if !req.Store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match context applicability scope")
	}
	if err := validateID(req.JudgmentID, "judgment_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid judgment id")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "context applicability requires an explicit now")
	}

	j, err := loadContextJudgment(ctx, req.Store, req.JudgmentID)
	if err != nil {
		return nil, err
	}
	if j.JudgmentID != req.JudgmentID {
		return nil, storeError(CodeHashMismatch, "context applicability judgment identity mismatch")
	}
	if j.Scope != req.Scope {
		return nil, storeError(CodeScopeMismatch, "context applicability judgment scope mismatch")
	}
	if err := checkNotFuture(j.CreatedAt, req.Now.UTC()); err != nil {
		return nil, err
	}
	if j.JudgmentType != JudgmentTypeContextApplicability || j.ContextApplicability == nil ||
		j.Subject.SubjectType != "context" || j.Subject.MemoryRef == nil {
		return nil, storeError(CodeSchemaInvalid, "judgment is not context_applicability")
	}

	base := &ContextApplicabilityResult{
		Status: ContextApplicabilityUnavailable, JudgmentID: j.JudgmentID,
		MemoryRef: *j.Subject.MemoryRef, TargetContextRef: j.Subject.TargetContextRef,
		Result: j.ContextApplicability.Result, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano),
	}
	if j.BasisContextRefs == nil {
		return base, nil
	}

	rev, err := loadContextRevision(ctx, req.Store, *j.Subject.MemoryRef, req.Now.UTC())
	if err != nil {
		return nil, err
	}
	if err := verifyContextConditions(rev, j.ContextApplicability.RequiredConditionIDs); err != nil {
		return nil, err
	}
	if err := verifyContextEvidence(ctx, req.Store, rev, j.ContextApplicability.EvidenceRefs, req.Now.UTC()); err != nil {
		return nil, err
	}
	if err := verifyContextSupersedeChain(ctx, req.Store, j, req.Now.UTC()); err != nil {
		return nil, err
	}
	base.Status = ContextApplicabilityVerified
	return base, nil
}

func loadContextJudgment(ctx context.Context, store *FactStore, id string) (JudgmentFact, error) {
	raw, err := store.Get(ctx, FactKindJudgment, id)
	if err != nil {
		return JudgmentFact{}, err
	}
	j, err := DecodeStrict[JudgmentFact](raw)
	if err != nil {
		return JudgmentFact{}, classifyDecodeError(err)
	}
	return j, nil
}

func loadContextRevision(ctx context.Context, store *FactStore, ref MemoryRef, now time.Time) (MemoryRevision, error) {
	raw, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", ref.MemoryID, ref.Revision))
	if err != nil {
		return MemoryRevision{}, err
	}
	rev, err := DecodeStrict[MemoryRevision](raw)
	if err != nil {
		return MemoryRevision{}, classifyDecodeError(err)
	}
	if rev.Scope != ref.Scope || rev.MemoryType != ref.MemoryType || rev.MemoryID != ref.MemoryID ||
		rev.Revision != ref.Revision || rev.ContentSHA256 != ref.ContentSHA256 {
		return MemoryRevision{}, storeError(CodeHashMismatch, "context applicability memory ref mismatch")
	}
	if err := checkNotFuture(rev.CreatedAt, now); err != nil {
		return MemoryRevision{}, err
	}
	return rev, nil
}

func verifyContextConditions(rev MemoryRevision, required []string) error {
	known := make(map[string]bool, len(rev.AppliesWhen)+len(rev.DoesNotApplyWhen))
	for _, c := range rev.AppliesWhen {
		known[c.ConditionID] = true
	}
	for _, c := range rev.DoesNotApplyWhen {
		known[c.ConditionID] = true
	}
	for _, id := range required {
		if !known[id] {
			return storeError(CodeSchemaInvalid, "required applicability condition is missing")
		}
	}
	return nil
}

func verifyContextEvidence(ctx context.Context, store *FactStore, rev MemoryRevision, refs []EvidenceRef, now time.Time) error {
	if len(refs) == 0 {
		return nil
	}
	want := make(map[string]bool, len(refs))
	futureOnly := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref.Scope != rev.Scope {
			return storeError(CodeScopeMismatch, "context applicability evidence scope mismatch")
		}
		want[evidenceKey(ref)] = false
	}
	keys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("%s/%d/", rev.MemoryID, rev.Revision)
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		raw, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
		if err != nil {
			return err
		}
		ev, err := DecodeStrict[MemoryEvidenceGeneration](raw)
		if err != nil {
			return classifyDecodeError(err)
		}
		if ev.MemoryID != rev.MemoryID || ev.Revision != rev.Revision {
			return storeError(CodeHashMismatch, "context applicability evidence identity mismatch")
		}
		matched := false
		for _, member := range ev.EvidenceRefs {
			if _, ok := want[evidenceKey(member)]; ok {
				matched = true
			}
		}
		if !matched {
			continue
		}
		if err := checkNotFuture(ev.CreatedAt, now); err != nil {
			if ErrorCode(err) != CodeEvaluationFutureReference {
				return err
			}
			for _, member := range ev.EvidenceRefs {
				if _, ok := want[evidenceKey(member)]; ok {
					futureOnly[evidenceKey(member)] = true
				}
			}
			continue
		}
		for _, member := range ev.EvidenceRefs {
			if _, ok := want[evidenceKey(member)]; ok {
				want[evidenceKey(member)] = true
			}
		}
	}
	for key, found := range want {
		if !found {
			if futureOnly[key] {
				return storeError(CodeEvaluationFutureReference, "context applicability references future evidence")
			}
			return storeError(CodeSchemaInvalid, "context applicability evidence is missing")
		}
	}
	return nil
}

func verifyContextSupersedeChain(ctx context.Context, store *FactStore, newest JudgmentFact, now time.Time) error {
	seen := map[string]bool{newest.JudgmentID: true}
	cur := newest
	for cur.SupersedesJudgmentRef != nil {
		ref := *cur.SupersedesJudgmentRef
		if seen[ref.JudgmentID] {
			return storeError(CodeSchemaInvalid, "context applicability supersede cycle")
		}
		older, err := loadContextJudgment(ctx, store, ref.JudgmentID)
		if err != nil {
			return err
		}
		if err := validateJudgmentRefTarget(ref, older); err != nil {
			return err
		}
		if !sameContextJudgmentIdentity(cur, older) {
			return storeError(CodeSchemaInvalid, "context applicability supersede identity mismatch")
		}
		if err := checkNotFuture(older.CreatedAt, now); err != nil {
			return err
		}
		seen[older.JudgmentID] = true
		cur = older
	}
	return nil
}

func sameContextJudgmentIdentity(a, b JudgmentFact) bool {
	if a.JudgmentType != JudgmentTypeContextApplicability || b.JudgmentType != JudgmentTypeContextApplicability ||
		a.Scope != b.Scope || a.Subject.SubjectType != "context" || b.Subject.SubjectType != "context" ||
		a.Subject.MemoryRef == nil || b.Subject.MemoryRef == nil ||
		a.Subject.TargetContextRef != b.Subject.TargetContextRef {
		return false
	}
	if *a.Subject.MemoryRef != *b.Subject.MemoryRef {
		return false
	}
	return true
}
