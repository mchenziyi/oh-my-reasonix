package memory

import (
	"context"
	"fmt"
	"time"
)

type ConflictRequirementStatus string

const (
	ConflictRequirementClear       ConflictRequirementStatus = "clear"
	ConflictRequirementUnresolved  ConflictRequirementStatus = "unresolved"
	ConflictRequirementUnavailable ConflictRequirementStatus = "unavailable"
)

type ConflictRequirementRequest struct {
	Scope                 Scope
	MemoryID              string
	Revision              int
	ExpectedMemoryContext MemoryContext
	ProjectStore          *FactStore
	GlobalStore           *FactStore
	Now                   time.Time
}

type ConflictRequirementResult struct {
	Status    ConflictRequirementStatus
	Satisfied bool
}

func EvaluateConflictRequirement(ctx context.Context, store *FactStore, req ConflictRequirementRequest) (*ConflictRequirementResult, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "conflict evaluation scope must be project or global")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match conflict evaluation scope")
	}
	if err := validateID(req.MemoryID, "memory_id"); err != nil || req.Revision < 1 {
		return nil, storeError(CodeDerivedInvalidInput, "invalid conflict evaluation target")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "conflict evaluation requires an explicit now")
	}
	if err := req.ExpectedMemoryContext.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid expected memory context")
	}
	if (req.Scope == ScopeProject && req.ExpectedMemoryContext.ProjectGenerationRef == nil) ||
		(req.Scope == ScopeGlobal && req.ExpectedMemoryContext.GlobalGenerationRef == nil) {
		return nil, storeError(CodeScopeMismatch, "target scope is absent from expected memory context")
	}
	revData, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", req.MemoryID, req.Revision))
	if err != nil {
		return nil, err
	}
	rev, err := DecodeStrict[MemoryRevision](revData)
	if err != nil {
		return nil, classifyDecodeError(err)
	}
	if err := rejectFutureFactTime(rev.CreatedAt, req.Now, "memory revision lies in the future"); err != nil {
		return nil, err
	}
	criticRequest := CriticRequirementRequest{
		Scope: req.Scope, MemoryID: req.MemoryID, Revision: req.Revision,
		ExpectedMemoryContext: req.ExpectedMemoryContext,
		ProjectStore:          req.ProjectStore, GlobalStore: req.GlobalStore, Now: req.Now,
	}
	if err := verifyExpectedMemoryContext(ctx, criticRequest); err != nil {
		if err == errWorldUnavailable {
			return conflictUnavailable(), nil
		}
		return nil, err
	}
	fixedWorld, err := loadFixedMemoryWorld(ctx, req.ExpectedMemoryContext, req.Scope, req.ProjectStore, req.GlobalStore)
	if err != nil {
		if err == errWorldUnavailable {
			return conflictUnavailable(), nil
		}
		return nil, err
	}
	targetRef := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	if err := fixedWorld.requireRevision(targetRef); err != nil {
		if err == errWorldUnavailable {
			return conflictUnavailable(), nil
		}
		return nil, err
	}
	evidenceSet, _, err := fixedWorld.collectEvidence(ctx, req.MemoryID, req.Revision, req.Now)
	if err != nil {
		return nil, err
	}
	judgments, err := loadAllJudgments(ctx, store)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]JudgmentFact, len(judgments))
	for _, j := range judgments {
		byID[j.JudgmentID] = j
	}
	if err := validateAllConflictEdges(byID); err != nil {
		return nil, err
	}
	candidates := make([]JudgmentFact, 0)
	for _, j := range judgments {
		if j.JudgmentType == JudgmentTypeConflictReview && j.Scope == req.Scope && j.Subject.MemoryRef != nil &&
			j.Subject.MemoryRef.MemoryID == req.MemoryID && j.Subject.MemoryRef.Revision == req.Revision {
			actualRef := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
			if !memoryRefsEqual(*j.Subject.MemoryRef, actualRef) {
				return nil, storeError(CodeHashMismatch, "conflict subject reference mismatch")
			}
		}
		if conflictMatches(j, req, rev) {
			created, parseErr := time.Parse(time.RFC3339Nano, j.CreatedAt)
			if parseErr != nil || created.After(req.Now) {
				return nil, storeError(CodeEvaluationFutureReference, "conflict judgment lies in the future")
			}
			candidates = append(candidates, j)
		}
	}
	if len(candidates) == 0 {
		return conflictUnavailable(), nil
	}
	terminals, err := conflictTerminals(candidates, byID)
	if err != nil {
		return nil, err
	}
	hasConflict, hasUnavailable, hasClear := false, false, false
	for _, j := range terminals {
		if err := validateConflictReferences(ctx, req, rev, j, evidenceSet); err != nil {
			return nil, err
		}
		switch j.ConflictReview.Result {
		case "conflict":
			hasConflict = true
		case "unavailable":
			hasUnavailable = true
		case "clear":
			if j.ConflictReview.EvaluationScope == "sampled_audit" {
				hasUnavailable = true
			} else {
				hasClear = true
			}
		}
	}
	if hasConflict {
		return &ConflictRequirementResult{Status: ConflictRequirementUnresolved}, nil
	}
	if hasUnavailable || !hasClear {
		return conflictUnavailable(), nil
	}
	return &ConflictRequirementResult{Status: ConflictRequirementClear, Satisfied: true}, nil
}

func conflictUnavailable() *ConflictRequirementResult {
	return &ConflictRequirementResult{Status: ConflictRequirementUnavailable}
}

func conflictMatches(j JudgmentFact, req ConflictRequirementRequest, rev MemoryRevision) bool {
	if j.JudgmentType != JudgmentTypeConflictReview || j.Scope != req.Scope || j.ConflictReview == nil || j.Subject.MemoryRef == nil {
		return false
	}
	r := *j.Subject.MemoryRef
	want := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	return memoryRefsEqual(r, want) && memoryContextsEqual(j.ConflictReview.MemoryContext, req.ExpectedMemoryContext)
}

func validateAllConflictEdges(byID map[string]JudgmentFact) error {
	for _, j := range byID {
		if j.JudgmentType != JudgmentTypeConflictReview || j.SupersedesJudgmentRef == nil {
			continue
		}
		target, ok := byID[j.SupersedesJudgmentRef.JudgmentID]
		if !ok {
			return storeError(CodeSchemaInvalid, "conflict supersede target missing")
		}
		if err := validateJudgmentRefTarget(*j.SupersedesJudgmentRef, target); err != nil {
			return err
		}
		if !conflictNodesEqual(j, target) {
			return storeError(CodeSchemaInvalid, "conflict supersede chain nodes must share subject and memory context")
		}
	}
	return nil
}

func conflictTerminals(candidates []JudgmentFact, byID map[string]JudgmentFact) ([]JudgmentFact, error) {
	superseded := make(map[string]bool)
	for _, j := range byID {
		if j.JudgmentType == JudgmentTypeConflictReview && j.SupersedesJudgmentRef != nil {
			superseded[j.SupersedesJudgmentRef.JudgmentID] = true
		}
	}
	terminals := make([]JudgmentFact, 0)
	for _, j := range candidates {
		if superseded[j.JudgmentID] {
			continue
		}
		if err := verifyConflictChain(j, byID); err != nil {
			return nil, err
		}
		terminals = append(terminals, j)
	}
	if len(terminals) == 0 {
		return nil, storeError(CodeSchemaInvalid, "conflict supersede cycle")
	}
	return terminals, nil
}

func verifyConflictChain(j JudgmentFact, byID map[string]JudgmentFact) error {
	seen := make(map[string]bool)
	for j.SupersedesJudgmentRef != nil {
		if seen[j.JudgmentID] {
			return storeError(CodeSchemaInvalid, "conflict supersede cycle")
		}
		seen[j.JudgmentID] = true
		next, ok := byID[j.SupersedesJudgmentRef.JudgmentID]
		if !ok {
			return storeError(CodeSchemaInvalid, "conflict supersede target missing")
		}
		if err := validateJudgmentRefTarget(*j.SupersedesJudgmentRef, next); err != nil {
			return err
		}
		if !conflictNodesEqual(j, next) {
			return storeError(CodeSchemaInvalid, "conflict supersede chain nodes must share subject and memory context")
		}
		j = next
	}
	return nil
}

func conflictNodesEqual(a, b JudgmentFact) bool {
	return a.Scope == b.Scope && a.Subject.MemoryRef != nil && b.Subject.MemoryRef != nil &&
		memoryRefsEqual(*a.Subject.MemoryRef, *b.Subject.MemoryRef) &&
		a.ConflictReview != nil && b.ConflictReview != nil &&
		memoryContextsEqual(a.ConflictReview.MemoryContext, b.ConflictReview.MemoryContext)
}

func validateConflictReferences(ctx context.Context, req ConflictRequirementRequest, target MemoryRevision, j JudgmentFact, evidenceSet map[string]bool) error {
	allowedEvidence := make(map[string]bool, len(evidenceSet))
	for key := range evidenceSet {
		allowedEvidence[key] = true
	}
	for _, ref := range j.ConflictReview.CounterpartMemoryRefs {
		if memoryRefsEqual(ref, MemoryRef{Scope: target.Scope, MemoryType: target.MemoryType, MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256}) {
			return storeError(CodeSchemaInvalid, "conflict counterpart references its subject")
		}
		refStore := req.ProjectStore
		if ref.Scope == ScopeGlobal {
			refStore = req.GlobalStore
		}
		if (ref.Scope == ScopeProject && req.ExpectedMemoryContext.ProjectGenerationRef == nil) ||
			(ref.Scope == ScopeGlobal && req.ExpectedMemoryContext.GlobalGenerationRef == nil) {
			return storeError(CodeScopeMismatch, "conflict counterpart scope is absent from expected memory context")
		}
		if refStore == nil || !refStore.scopeMatches(ref.Scope) {
			return storeError(CodeScopeMismatch, "conflict counterpart store unavailable")
		}
		world, err := loadFixedMemoryWorld(ctx, req.ExpectedMemoryContext, ref.Scope, req.ProjectStore, req.GlobalStore)
		if err != nil {
			return err
		}
		if err := world.requireRevision(ref); err != nil {
			return err
		}
		data, err := refStore.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", ref.MemoryID, ref.Revision))
		if err != nil {
			return err
		}
		actual, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			return classifyDecodeError(err)
		}
		actualRef := MemoryRef{Scope: actual.Scope, MemoryType: actual.MemoryType, MemoryID: actual.MemoryID, Revision: actual.Revision, ContentSHA256: actual.ContentSHA256}
		if !memoryRefsEqual(ref, actualRef) {
			return storeError(CodeHashMismatch, "conflict counterpart reference mismatch")
		}
		if err := rejectFutureFactTime(actual.CreatedAt, req.Now, "conflict counterpart lies in the future"); err != nil {
			return err
		}
		counterpartEvidence, _, err := world.collectEvidence(ctx, ref.MemoryID, ref.Revision, req.Now)
		if err != nil {
			return err
		}
		for key := range counterpartEvidence {
			allowedEvidence[key] = true
		}
	}
	for _, ref := range j.ConflictReview.EvidenceRefs {
		if !allowedEvidence[evidenceKey(ref)] {
			return storeError(CodeSchemaInvalid, "conflict evidence reference is outside the reviewed evidence sets")
		}
	}
	return nil
}

func rejectFutureFactTime(raw string, now time.Time, detail string) error {
	created, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return storeError(CodeSchemaInvalid, "invalid fact timestamp")
	}
	if created.After(now) {
		return storeError(CodeEvaluationFutureReference, detail)
	}
	return nil
}
