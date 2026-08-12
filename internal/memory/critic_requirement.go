package memory

// MEM-02B: Critic Requirement 评估（只读验证器）。
//
// critic_review 已注册为 JudgmentType 联合的第 8 个分支。本验证器在固定
// Generation Pair（ExpectedMemoryContext）上判断证据有效性条件，只读、不
// 写事实、不读 CURRENT、不读取墙上时间（Now 显式传入）。Critic passed
// 只令 Satisfied=true；Conflict Fact 未冻结，DeriveState 中
// evidence_validated 仍保持 probation。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CriticRequirementStatus is the derived status of the evidence_validated
// critic condition.
type CriticRequirementStatus string

const (
	CriticRequirementUnavailable CriticRequirementStatus = "unavailable"
	CriticRequirementPassed      CriticRequirementStatus = "passed"
	CriticRequirementFailed      CriticRequirementStatus = "failed"
)

// CriticRequirementRequest selects the revision whose critic condition is
// evaluated. The world is pinned explicitly: ExpectedMemoryContext fixes the
// Generation Pair, ProjectStore/GlobalStore back the non-null sides, and Now
// is the explicit evaluation instant (zero value is rejected, never replaced
// by the wall clock).
type CriticRequirementRequest struct {
	Scope                 Scope
	MemoryID              string
	Revision              int
	ExpectedMemoryContext MemoryContext
	ProjectStore          *FactStore
	GlobalStore           *FactStore
	Now                   time.Time
}

// CriticRequirementResult is derived, read-only data: it is never written
// back to any fact.
type CriticRequirementResult struct {
	Status      CriticRequirementStatus
	Satisfied   bool
	UsagePolicy UsagePolicy
}

// errWorldUnavailable marks a fixed world that cannot be verified or exactly
// rebuilt: the evaluator returns unavailable instead of guessing.
var errWorldUnavailable = errors.New("critic world unavailable")

func unavailableResult(rev MemoryRevision) *CriticRequirementResult {
	return &CriticRequirementResult{Status: CriticRequirementUnavailable, Satisfied: false, UsagePolicy: rev.UsagePolicy}
}

// EvaluateCriticRequirement checks the evidence_validated critic condition
// for one revision within the pinned expected world.
func EvaluateCriticRequirement(ctx context.Context, store *FactStore, req CriticRequirementRequest) (*CriticRequirementResult, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "critic evaluation scope must be project or global")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match critic evaluation scope")
	}
	if err := validateID(req.MemoryID, "memory_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid memory id")
	}
	if req.Revision < 1 {
		return nil, storeError(CodeDerivedInvalidInput, "invalid revision")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "critic evaluation requires an explicit now")
	}
	if err := req.ExpectedMemoryContext.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid expected memory context")
	}

	revData, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", req.MemoryID, req.Revision))
	if err != nil {
		return nil, err
	}
	rev, err := DecodeStrict[MemoryRevision](revData)
	if err != nil {
		return nil, classifyDecodeError(err)
	}

	// Fixed world verification (structure, scope, generation, manifest,
	// compiled output, time; exact rebuild only, never CURRENT).
	if err := verifyExpectedMemoryContext(ctx, req); err != nil {
		if err == errWorldUnavailable {
			return unavailableResult(rev), nil
		}
		return nil, err
	}
	fixedWorld, err := loadFixedMemoryWorld(ctx, req.ExpectedMemoryContext, req.Scope, req.ProjectStore, req.GlobalStore)
	if err != nil {
		if err == errWorldUnavailable {
			return unavailableResult(rev), nil
		}
		return nil, err
	}
	targetRef := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	if err := fixedWorld.requireRevision(targetRef); err != nil {
		if err == errWorldUnavailable {
			return unavailableResult(rev), nil
		}
		return nil, err
	}

	// Exact evidence union of the target revision's evidence generations.
	evidenceSet, _, err := fixedWorld.collectEvidence(ctx, req.MemoryID, req.Revision, req.Now)
	if err != nil {
		return nil, err
	}

	// Full strict scan; corrupt/unknown-field facts fail closed.
	judgments, err := loadAllJudgments(ctx, store)
	if err != nil {
		return nil, err
	}

	cands := make([]JudgmentFact, 0)
	for _, j := range judgments {
		if j.JudgmentType != JudgmentTypeCriticReview {
			continue
		}
		if !criticMatches(j, req, rev) {
			continue
		}
		cands = append(cands, j)
	}
	if len(cands) == 0 {
		return unavailableResult(rev), nil
	}

	byID := make(map[string]JudgmentFact, len(judgments))
	for _, j := range judgments {
		byID[j.JudgmentID] = j
	}
	terminals, err := criticTerminalStates(cands, byID)
	if err != nil {
		if err == errWorldUnavailable {
			return unavailableResult(rev), nil
		}
		return nil, err
	}

	stateSet := make(map[string]bool)
	for _, t := range terminals {
		stateSet[t.Result] = true
	}
	if len(stateSet) > 1 {
		// Parallel terminals disagree: unavailable, never a guessed pass.
		return unavailableResult(rev), nil
	}
	var state string
	for s := range stateSet {
		state = s
	}

	if state == "passed" {
		// Every effective passed terminal must have its required evidence
		// inside the revision's exact evidence union.
		for _, t := range terminals {
			if t.Result != "passed" {
				continue
			}
			node := byID[t.JudgmentID]
			for _, want := range node.CriticReview.RequiredEvidenceRefs {
				if !evidenceSet[evidenceKey(want)] {
					return unavailableResult(rev), nil
				}
			}
		}
		return &CriticRequirementResult{Status: CriticRequirementPassed, Satisfied: true, UsagePolicy: rev.UsagePolicy}, nil
	}
	if state == "failed" {
		return &CriticRequirementResult{Status: CriticRequirementFailed, Satisfied: false, UsagePolicy: rev.UsagePolicy}, nil
	}
	return unavailableResult(rev), nil
}

// ---- fixed world ----

// verifyExpectedMemoryContext validates every non-null side of the pinned
// Generation Pair. Missing stores or irreconstructible worlds yield
// errWorldUnavailable; structural/scope/hash/time corruption fails closed.
func verifyExpectedMemoryContext(ctx context.Context, req CriticRequirementRequest) error {
	if req.ExpectedMemoryContext.ProjectGenerationRef != nil {
		r := req.ExpectedMemoryContext.ProjectGenerationRef
		if err := verifyGenerationRef(ctx, req.ProjectStore, r.Scope, r.GenerationID, r.InputManifestID, r.InputManifestSHA256, req.Now); err != nil {
			return err
		}
	}
	if req.ExpectedMemoryContext.GlobalGenerationRef != nil {
		r := req.ExpectedMemoryContext.GlobalGenerationRef
		if err := verifyGenerationRef(ctx, req.GlobalStore, r.Scope, r.GenerationID, r.InputManifestID, r.InputManifestSHA256, req.Now); err != nil {
			return err
		}
	}
	return nil
}

// verifyGenerationRef validates one pinned generation reference: in-place
// verification when the generation still exists, exact manifest rebuild when
// it has been cleaned up, and unavailable when neither is possible.
func verifyGenerationRef(ctx context.Context, store *FactStore, scope Scope, genID, manifestID, manifestSHA string, now time.Time) error {
	if store == nil {
		return errWorldUnavailable
	}
	if !store.scopeMatches(scope) {
		return storeError(CodeScopeMismatch, "generation store scope mismatch")
	}
	_, mf, err := resolveGenerationWorld(ctx, store, scope, genID, now)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return verifyRebuildFromManifest(ctx, store, scope, genID, manifestID, manifestSHA, now)
		}
		return err
	}
	if mf.GenerationID != manifestID || mf.InputManifestSHA256 != manifestSHA {
		return storeError(CodeHashMismatch, "generation manifest ref mismatch")
	}
	return nil
}

// verifyRebuildFromManifest checks the permanent manifest (identity, hash,
// scope, created_at <= now) and rebuilds the world exactly.
func verifyRebuildFromManifest(ctx context.Context, store *FactStore, scope Scope, genID, manifestID, manifestSHA string, now time.Time) error {
	mfData, err := store.Get(ctx, FactKindGenerationInputManifest, genID)
	if err != nil {
		if ErrorCode(err) == CodeNotFound || ErrorCode(err) == CodeUnknownField || ErrorCode(err) == CodeInvalidJSON {
			return errWorldUnavailable
		}
		return err
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfData)
	if err != nil {
		if ErrorCode(err) == CodeUnknownField || ErrorCode(err) == CodeInvalidJSON {
			return errWorldUnavailable
		}
		return classifyDecodeError(err)
	}
	if mf.GenerationID != genID || mf.GenerationID != manifestID || mf.InputManifestSHA256 != manifestSHA {
		return storeError(CodeHashMismatch, "manifest identity or hash mismatch")
	}
	if !store.scopeMatches(mf.Scope) || mf.Scope != scope {
		return storeError(CodeScopeMismatch, "manifest scope mismatch")
	}
	created, err := time.Parse(time.RFC3339Nano, mf.CreatedAt)
	if err != nil {
		return storeError(CodeSchemaInvalid, "invalid manifest timestamp")
	}
	if created.After(now) {
		return storeError(CodeEvaluationFutureReference, "generation lies in the future")
	}
	reb, err := rebuildFromManifest(ctx, store, genID, manifestSHA, "")
	if err != nil {
		return err
	}
	if reb.Status == EvaluationRebuildUnavailable {
		return errWorldUnavailable
	}
	return nil
}

// ---- evidence union ----

func evidenceKey(r EvidenceRef) string {
	return string(r.Scope) + "|" + r.EvidenceType + "|" + r.EvidenceID + "|" + r.ContentSHA256
}

// ---- judgment scan ----

// loadAllJudgments decodes every judgment strictly; any corrupt,
// unknown-field or unregistered-type fact fails closed. Duplicate judgment
// ids are corrupt (the store forbids them by identity conflict) and also
// fail closed so supersede lookups never resolve to a silently overwritten
// node.
func loadAllJudgments(ctx context.Context, store *FactStore) ([]JudgmentFact, error) {
	keys, err := store.List(ctx, FactKindJudgment)
	if err != nil {
		return nil, err
	}
	out := make([]JudgmentFact, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindJudgment, key)
		if err != nil {
			return nil, err
		}
		j, err := DecodeStrict[JudgmentFact](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		if seen[j.JudgmentID] {
			return nil, storeError(CodeSchemaInvalid, "duplicate judgment id")
		}
		seen[j.JudgmentID] = true
		out = append(out, j)
	}
	return out, nil
}

// criticMatches filters a critic judgment to the requested revision and
// expected world: scope, subject memory identity/hash and MemoryContext must
// match exactly.
func criticMatches(j JudgmentFact, req CriticRequirementRequest, rev MemoryRevision) bool {
	if j.Scope != req.Scope {
		return false
	}
	if j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil {
		return false
	}
	r := j.Subject.MemoryRef
	if r.Scope != rev.Scope || r.MemoryType != rev.MemoryType || r.MemoryID != req.MemoryID ||
		r.Revision != req.Revision || r.ContentSHA256 != rev.ContentSHA256 {
		return false
	}
	return memoryContextsEqual(j.CriticReview.MemoryContext, req.ExpectedMemoryContext)
}

// ---- supersede chain ----

type criticTerminal struct {
	JudgmentID string
	Result     string
}

// criticTerminalStates resolves the effective (not-superseded) critic
// terminals. supersedes_judgment_ref points from a newer node to the older
// node it replaces, so the effective node is one that no judgment supersedes.
// Chain consistency (same subject/context) is verified for every effective
// node; if every candidate is superseded the finite chain can only be a
// cycle, which fails closed.
func criticTerminalStates(cands []JudgmentFact, byID map[string]JudgmentFact) ([]criticTerminal, error) {
	superseded := make(map[string]bool)
	supersederOf := make(map[string]string)
	// Only critic supersede edges count, and only after the ref fully
	// matches its target AND the two chain nodes evaluate the same subject
	// (five fields) and memory context. A mismatched or orphan edge is
	// corrupt data and fails closed before any effective/superseded
	// decision trusts the id — even when the bad branch is filtered out of
	// the candidate set by its subject/context.
	for _, j := range byID {
		if j.JudgmentType != JudgmentTypeCriticReview || j.SupersedesJudgmentRef == nil {
			continue
		}
		ref := *j.SupersedesJudgmentRef
		target, ok := byID[ref.JudgmentID]
		if !ok {
			return nil, storeError(CodeSchemaInvalid, "critic supersede target missing")
		}
		if err := validateJudgmentRefTarget(ref, target); err != nil {
			return nil, err
		}
		if !criticNodeMatches(j, target) {
			return nil, storeError(CodeSchemaInvalid, "critic supersede chain nodes must share subject and memory context")
		}
		superseded[target.JudgmentID] = true
		supersederOf[target.JudgmentID] = j.JudgmentID
	}
	effective := make([]JudgmentFact, 0)
	for _, c := range cands {
		if !superseded[c.JudgmentID] {
			effective = append(effective, c)
		}
	}
	if len(effective) == 0 {
		// Every candidate is superseded: the chain head lies outside the
		// candidate set. The frozen chain constraint (plan 2.3) still
		// applies to every adjacent pair — a differing subject/context
		// superseder is corrupt and fails closed, never silently
		// downgraded to unavailable.
		for _, c := range cands {
			if err := verifySupersederChain(c, byID, supersederOf); err != nil {
				return nil, err
			}
		}
		return nil, errWorldUnavailable
	}
	terminals := make([]criticTerminal, 0, len(effective))
	for _, e := range effective {
		if err := verifyCriticChain(e, byID); err != nil {
			return nil, err
		}
		terminals = append(terminals, criticTerminal{JudgmentID: e.JudgmentID, Result: e.CriticReview.Result})
	}
	return terminals, nil
}

// verifySupersederChain walks a superseded candidate upwards (toward newer
// nodes) verifying every adjacent pair shares scope, subject identity/hash
// and memory context, until the chain head; the head's older direction is
// then verified by verifyCriticChain. Cycles fail closed.
func verifySupersederChain(c JudgmentFact, byID map[string]JudgmentFact, supersederOf map[string]string) error {
	path := make(map[string]bool)
	cur := c
	for {
		supID, ok := supersederOf[cur.JudgmentID]
		if !ok {
			return verifyCriticChain(cur, byID)
		}
		if path[cur.JudgmentID] {
			return storeError(CodeSchemaInvalid, "critic supersede cycle")
		}
		path[cur.JudgmentID] = true
		sup, ok := byID[supID]
		if !ok {
			return storeError(CodeSchemaInvalid, "critic supersede target missing")
		}
		// sup is the node that supersedes cur, so its supersedes ref must
		// point back at cur and fully match it; judgment_id alone is never
		// trusted.
		if sup.SupersedesJudgmentRef == nil {
			return storeError(CodeSchemaInvalid, "critic supersede target missing")
		}
		if err := validateJudgmentRefTarget(*sup.SupersedesJudgmentRef, cur); err != nil {
			return err
		}
		if !criticNodeMatches(sup, cur) {
			return storeError(CodeSchemaInvalid, "critic supersede chain nodes must share subject and memory context")
		}
		cur = sup
	}
}

// verifyCriticChain walks a node back through its supersede links and
// verifies every pair shares scope, subject identity/hash and memory context.
func verifyCriticChain(j JudgmentFact, byID map[string]JudgmentFact) error {
	path := make(map[string]bool)
	cur := j
	for {
		if cur.SupersedesJudgmentRef == nil {
			return nil
		}
		if path[cur.JudgmentID] {
			return storeError(CodeSchemaInvalid, "critic supersede cycle")
		}
		path[cur.JudgmentID] = true
		next, ok := byID[cur.SupersedesJudgmentRef.JudgmentID]
		if !ok {
			return storeError(CodeSchemaInvalid, "critic supersede target missing")
		}
		if err := validateJudgmentRefTarget(*cur.SupersedesJudgmentRef, next); err != nil {
			return err
		}
		if !criticNodeMatches(next, cur) {
			return storeError(CodeSchemaInvalid, "critic supersede chain nodes must share subject and memory context")
		}
		cur = next
	}
}

// criticNodeMatches compares two critic chain nodes: same scope, subject
// memory identity (type, id, revision, hash) and memory context.
func criticNodeMatches(a, b JudgmentFact) bool {
	if a.Scope != b.Scope {
		return false
	}
	ar, br := a.Subject.MemoryRef, b.Subject.MemoryRef
	if ar == nil || br == nil {
		return false
	}
	if ar.Scope != br.Scope || ar.MemoryType != br.MemoryType || ar.MemoryID != br.MemoryID ||
		ar.Revision != br.Revision || ar.ContentSHA256 != br.ContentSHA256 {
		return false
	}
	return memoryContextsEqual(a.CriticReview.MemoryContext, b.CriticReview.MemoryContext)
}

// validateJudgmentRefTarget verifies that a supersedes_judgment_ref fully
// matches its actual target judgment: scope, judgment_type, judgment_id and
// content_sha256 must all agree. A mismatch is corrupt data and fails closed;
// judgment_id alone is never trusted.
func validateJudgmentRefTarget(ref JudgmentRef, target JudgmentFact) error {
	if ref.Scope != target.Scope {
		return storeError(CodeSchemaInvalid, "critic supersede ref scope does not match its target")
	}
	if ref.JudgmentType != target.JudgmentType {
		return storeError(CodeSchemaInvalid, "critic supersede ref type does not match its target")
	}
	if ref.JudgmentID != target.JudgmentID {
		return storeError(CodeSchemaInvalid, "critic supersede ref id does not match its target")
	}
	if ref.ContentSHA256 != target.ContentSHA256 {
		return storeError(CodeSchemaInvalid, "critic supersede ref hash does not match its target")
	}
	return nil
}

// memoryContextsEqual compares two MemoryContexts through their deterministic
// canonical forms.
func memoryContextsEqual(a, b MemoryContext) bool {
	am, err := a.canonMap()
	if err != nil {
		return false
	}
	bm, err := b.canonMap()
	if err != nil {
		return false
	}
	ab, err := json.Marshal(am)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(bm)
	if err != nil {
		return false
	}
	return string(ab) == string(bb)
}
