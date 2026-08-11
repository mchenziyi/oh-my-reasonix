package memory

// MEM-02-06: Freshness / Revalidation 评估（只读）。
//
// 时间老化与有效性隔离：本评估器只派生 fresh|aging|needs_revalidation 候选，
// 永不产生 frozen/superseded/archived，永不修改 Revision，永不写 Judgment 或
// 任何事实。freshness_evaluation Judgment 已冻结的字段（memory_ref、result、
// evaluated_at、freshness_policy_ref、basis_refs）被严格校验；PolicyRef 的
// content_sha256 是唯一 Policy Hash 锚，不增加重复字段或第二事实源。
// 相同 Now + Policy + Facts 输出字节稳定；Policy 漂移、未来 evaluated_at 只
// 作为诊断项并回退到冻结时间窗，绝不猜测；hash 漂移与损坏一律 fail closed。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// RevalidationResult is the fixed vocabulary of the revalidation evaluator.
type RevalidationResult string

const (
	RevalidationFresh             RevalidationResult = "fresh"
	RevalidationAging             RevalidationResult = "aging"
	RevalidationNeedsRevalidation RevalidationResult = "needs_revalidation"
)

// RevalidationRequest selects the world and policy for the evaluation.
type RevalidationRequest struct {
	Scope              Scope
	Now                time.Time
	FreshnessPolicyRef PolicyRef
}

// RevalidationCandidate is one derived, auditable revalidation candidate.
type RevalidationCandidate struct {
	MemoryID    string             `json:"memory_id"`
	Revision    int                `json:"revision"`
	Result      RevalidationResult `json:"result"`
	Reason      string             `json:"reason"`
	EvaluatedAt string             `json:"evaluated_at"`
}

// RevalidationDiagnostic reports a protocol inconsistency without leaking
// sensitive details.
type RevalidationDiagnostic struct {
	Code     string `json:"code"`
	MemoryID string `json:"memory_id"`
	Detail   string `json:"detail"`
}

// RevalidationReport is the derived, byte-stable output.
type RevalidationReport struct {
	Scope       string                   `json:"scope"`
	Now         string                   `json:"now"`
	PolicyRef   PolicyRef                `json:"policy_ref"`
	Candidates  []RevalidationCandidate  `json:"candidates"`
	Diagnostics []RevalidationDiagnostic `json:"diagnostics"`
}

const (
	diagFutureEvaluatedAt           = "future_evaluated_at"
	diagFutureJudgment              = "future_judgment"
	diagFutureEvidence              = "future_evidence"
	diagEvaluationExpired           = "evaluation_expired"
	diagPolicyDrift                 = "policy_drift"
	diagConflictingFreshnessResults = "conflicting_freshness_judgments"
)

// EvaluateRevalidation derives revalidation candidates for every memory's
// latest revision in the store's scope. It is read-only and deterministic.
func EvaluateRevalidation(ctx context.Context, store *FactStore, req RevalidationRequest) (*RevalidationReport, error) {
	if store == nil {
		return nil, storeError(CodeDerivedInvalidInput, "revalidation requires a store")
	}
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "revalidation scope must be project or global")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match revalidation scope")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "revalidation requires an explicit now")
	}
	if err := req.FreshnessPolicyRef.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid freshness policy ref")
	}
	if req.FreshnessPolicyRef.PolicyType != PolicyTypeFreshness {
		return nil, storeError(CodeDerivedInvalidInput, "freshness policy ref must be a freshness policy")
	}
	// The policy must exist and its content hash must match exactly: a
	// drifted or missing policy fails closed, it is never substituted.
	ps := NewPolicyStore(store)
	policy, err := ps.GetPolicy(ctx, req.FreshnessPolicyRef)
	if err != nil {
		return nil, err
	}
	if policy.Config.Freshness == nil {
		return nil, storeError(CodeSchemaInvalid, "policy has no freshness config")
	}
	if err := checkNotFuture(policy.CreatedAt, req.Now); err != nil {
		return nil, err
	}
	cfg := *policy.Config.Freshness

	revisions, err := listLatestRevisions(ctx, store)
	if err != nil {
		return nil, err
	}
	judgments, err := loadAllJudgments(ctx, store)
	if err != nil {
		return nil, err
	}
	evidences, err := listEvidences(ctx, store)
	if err != nil {
		return nil, err
	}

	rep := &RevalidationReport{
		Scope:     string(req.Scope),
		Now:       req.Now.UTC().Format(time.RFC3339Nano),
		PolicyRef: req.FreshnessPolicyRef,
	}
	for _, rev := range revisions {
		if err := checkNotFuture(rev.CreatedAt, req.Now); err != nil {
			return nil, err
		}
		cand, diags, err := deriveRevalidation(ctx, store, rev, judgments, evidences, cfg, req.Now, req.FreshnessPolicyRef)
		if err != nil {
			return nil, err
		}
		rep.Candidates = append(rep.Candidates, cand)
		rep.Diagnostics = append(rep.Diagnostics, diags...)
	}
	sort.Slice(rep.Candidates, func(i, j int) bool {
		if rep.Candidates[i].MemoryID != rep.Candidates[j].MemoryID {
			return rep.Candidates[i].MemoryID < rep.Candidates[j].MemoryID
		}
		return rep.Candidates[i].Revision < rep.Candidates[j].Revision
	})
	sort.Slice(rep.Diagnostics, func(i, j int) bool {
		if rep.Diagnostics[i].MemoryID != rep.Diagnostics[j].MemoryID {
			return rep.Diagnostics[i].MemoryID < rep.Diagnostics[j].MemoryID
		}
		return rep.Diagnostics[i].Code < rep.Diagnostics[j].Code
	})
	return rep, nil
}

// deriveRevalidation computes one candidate. A live freshness judgment is
// adopted only when it is not superseded, its evaluated_at is not in the
// future and its policy ref matches the requested policy; otherwise it is
// surfaced as a diagnostic and the frozen time window drives the result.
func deriveRevalidation(ctx context.Context, store *FactStore, rev MemoryRevision, judgments []JudgmentFact, evidences []MemoryEvidenceGeneration, cfg PolicyConfigFreshness, now time.Time, policyRef PolicyRef) (RevalidationCandidate, []RevalidationDiagnostic, error) {
	cand := RevalidationCandidate{
		MemoryID: rev.MemoryID,
		Revision: rev.Revision,
	}
	var diags []RevalidationDiagnostic

	j, judgmentDiags, err := selectFreshnessJudgment(ctx, store, judgments, evidences, rev, cfg, now, policyRef)
	if err != nil {
		return RevalidationCandidate{}, nil, err
	}
	diags = append(diags, judgmentDiags...)
	if j != nil {
		cand.Result = RevalidationResult(j.FreshnessEvaluation.Result)
		cand.EvaluatedAt = j.FreshnessEvaluation.EvaluatedAt
		cand.Reason = "judgment_driven"
		return cand, diags, nil
	}

	// Window-driven fallback: age of the latest activity.
	last := parseFactTime(rev.CreatedAt)
	allowedEvidence := make(map[string]bool, len(cfg.RevalidationEvidenceTypes))
	for _, evidenceType := range cfg.RevalidationEvidenceTypes {
		allowedEvidence[evidenceType] = true
	}
	for i := range evidences {
		ev := evidences[i]
		if ev.MemoryID != rev.MemoryID || ev.Revision != rev.Revision {
			continue
		}
		matches := false
		for _, ref := range ev.EvidenceRefs {
			if ref.Scope != rev.Scope {
				return RevalidationCandidate{}, nil, storeError(CodeScopeMismatch, "revalidation evidence scope does not match revision scope")
			}
			matches = matches || allowedEvidence[ref.EvidenceType]
		}
		if !matches {
			continue
		}
		t := parseFactTime(ev.CreatedAt)
		if t.After(now) {
			diags = append(diags, RevalidationDiagnostic{Code: diagFutureEvidence, MemoryID: rev.MemoryID, Detail: "revalidation evidence lies in the future"})
			continue
		}
		if t.After(last) {
			last = t
		}
	}
	ageDays := int(now.Sub(last).Hours() / 24)
	if ageDays < cfg.EvaluationWindowDays {
		cand.Result = RevalidationFresh
	} else if ageDays < cfg.AgingAfterDays {
		cand.Result = RevalidationAging
	} else {
		cand.Result = RevalidationNeedsRevalidation
	}
	if ageDays >= cfg.StaleAfterDays {
		cand.Reason = "stale_window"
	} else {
		cand.Reason = "window_driven"
	}
	return cand, diags, nil
}

// samePolicyRef compares two policy refs field by field.
func samePolicyRef(a, b PolicyRef) bool {
	return a.PolicyID == b.PolicyID && a.PolicyType == b.PolicyType && a.ContentSHA256 == b.ContentSHA256
}

// selectFreshnessJudgment validates the complete relevant freshness graph and
// returns one usable terminal judgment. Structural corruption fails closed;
// temporal or policy inapplicability is diagnosed and falls back to windows.
func selectFreshnessJudgment(ctx context.Context, store *FactStore, judgments []JudgmentFact, evidences []MemoryEvidenceGeneration, rev MemoryRevision, cfg PolicyConfigFreshness, now time.Time, policyRef PolicyRef) (*JudgmentFact, []RevalidationDiagnostic, error) {
	byID := make(map[string]JudgmentFact, len(judgments))
	for _, j := range judgments {
		byID[j.JudgmentID] = j
	}

	relevant := make(map[string]JudgmentFact)
	for _, j := range judgments {
		if j.JudgmentType != JudgmentTypeFreshnessEvaluation || j.FreshnessEvaluation == nil {
			continue
		}
		subjectRelated := j.Subject.MemoryRef != nil && referencesRevisionIdentity(*j.Subject.MemoryRef, rev)
		payloadRelated := referencesRevisionIdentity(j.FreshnessEvaluation.MemoryRef, rev)
		if !subjectRelated && !payloadRelated {
			continue
		}
		if j.Scope != rev.Scope || j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil ||
			!sameMemoryRef(*j.Subject.MemoryRef, j.FreshnessEvaluation.MemoryRef) || !sameMemoryVersion(*j.Subject.MemoryRef, rev) {
			return nil, nil, storeError(CodeSchemaInvalid, "freshness judgment memory identity mismatch")
		}
		relevant[j.JudgmentID] = j
	}

	for _, j := range relevant {
		if j.SupersedesJudgmentRef == nil {
			continue
		}
		target, ok := byID[j.SupersedesJudgmentRef.JudgmentID]
		if !ok {
			return nil, nil, storeError(CodeSchemaInvalid, "freshness supersede target is missing")
		}
		if err := validateJudgmentRefTarget(*j.SupersedesJudgmentRef, target); err != nil {
			return nil, nil, err
		}
		if _, ok := relevant[target.JudgmentID]; !ok {
			return nil, nil, storeError(CodeSchemaInvalid, "freshness supersede chain identity mismatch")
		}
	}
	if err := validateFreshnessGraph(relevant); err != nil {
		return nil, nil, err
	}

	var diags []RevalidationDiagnostic
	eligible := make(map[string]JudgmentFact, len(relevant))
	for id, j := range relevant {
		created := parseFactTime(j.CreatedAt)
		evaluated := parseFactTime(j.FreshnessEvaluation.EvaluatedAt)
		future := false
		if created.After(now) {
			diags = append(diags, RevalidationDiagnostic{Code: diagFutureJudgment, MemoryID: rev.MemoryID, Detail: "freshness judgment lies in the future"})
			future = true
		}
		if evaluated.After(now) {
			diags = append(diags, RevalidationDiagnostic{Code: diagFutureEvaluatedAt, MemoryID: rev.MemoryID, Detail: "freshness judgment evaluated_at lies in the future"})
			future = true
		}
		if future {
			continue
		}
		if !samePolicyRef(j.FreshnessEvaluation.FreshnessPolicyRef, policyRef) {
			diags = append(diags, RevalidationDiagnostic{Code: diagPolicyDrift, MemoryID: rev.MemoryID, Detail: "freshness judgment cites a different freshness policy"})
			continue
		}
		if int(now.Sub(evaluated).Hours()/24) >= cfg.EvaluationWindowDays {
			diags = append(diags, RevalidationDiagnostic{Code: diagEvaluationExpired, MemoryID: rev.MemoryID, Detail: "freshness judgment evaluation window expired"})
			continue
		}
		if err := validateFreshnessBasis(ctx, store, j, judgments, evidences, rev); err != nil {
			return nil, nil, err
		}
		eligible[id] = j
	}

	superseded := make(map[string]bool)
	for _, j := range eligible {
		if j.SupersedesJudgmentRef != nil {
			if _, ok := eligible[j.SupersedesJudgmentRef.JudgmentID]; ok {
				superseded[j.SupersedesJudgmentRef.JudgmentID] = true
			}
		}
	}
	terminals := make([]JudgmentFact, 0, len(eligible))
	for id, j := range eligible {
		if !superseded[id] {
			terminals = append(terminals, j)
		}
	}
	if len(terminals) == 0 {
		return nil, diags, nil
	}
	result := terminals[0].FreshnessEvaluation.Result
	for _, j := range terminals[1:] {
		if j.FreshnessEvaluation.Result != result {
			diags = append(diags, RevalidationDiagnostic{Code: diagConflictingFreshnessResults, MemoryID: rev.MemoryID, Detail: "live freshness judgments conflict"})
			return nil, diags, nil
		}
	}
	sort.Slice(terminals, func(i, k int) bool { return judgmentNewer(terminals[i], terminals[k]) })
	return &terminals[0], diags, nil
}

func sameMemoryRef(a, b MemoryRef) bool {
	return a.Scope == b.Scope && a.MemoryType == b.MemoryType && a.MemoryID == b.MemoryID &&
		a.Revision == b.Revision && a.ContentSHA256 == b.ContentSHA256
}

func sameMemoryVersion(ref MemoryRef, rev MemoryRevision) bool {
	return ref.Scope == rev.Scope && ref.MemoryType == rev.MemoryType && ref.MemoryID == rev.MemoryID &&
		ref.Revision == rev.Revision && ref.ContentSHA256 == rev.ContentSHA256
}

func referencesRevisionIdentity(ref MemoryRef, rev MemoryRevision) bool {
	return ref.MemoryID == rev.MemoryID && ref.Revision == rev.Revision
}

func validateFreshnessGraph(relevant map[string]JudgmentFact) error {
	for start := range relevant {
		seen := map[string]bool{}
		for id := start; id != ""; {
			if seen[id] {
				return storeError(CodeSchemaInvalid, "freshness supersede chain contains a cycle")
			}
			seen[id] = true
			j := relevant[id]
			if j.SupersedesJudgmentRef == nil {
				break
			}
			id = j.SupersedesJudgmentRef.JudgmentID
		}
	}
	return nil
}

func validateFreshnessBasis(ctx context.Context, store *FactStore, judgment JudgmentFact, judgments []JudgmentFact, evidences []MemoryEvidenceGeneration, rev MemoryRevision) error {
	judgmentByID := make(map[string]JudgmentFact, len(judgments))
	for _, j := range judgments {
		judgmentByID[j.JudgmentID] = j
	}
	evidenceSet := make(map[string]bool)
	for _, generation := range evidences {
		for _, ref := range generation.EvidenceRefs {
			evidenceSet[evidenceKey(ref)] = true
		}
	}
	for _, basis := range judgment.FreshnessEvaluation.BasisRefs {
		switch {
		case basis.MemoryRef != nil:
			if !store.scopeMatches(basis.MemoryRef.Scope) {
				return storeError(CodeScopeMismatch, "freshness basis memory ref has the wrong scope")
			}
			data, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", basis.MemoryRef.MemoryID, basis.MemoryRef.Revision))
			if err != nil {
				return err
			}
			target, err := DecodeStrict[MemoryRevision](data)
			if err != nil {
				return classifyDecodeError(err)
			}
			if !sameMemoryVersion(*basis.MemoryRef, target) {
				return storeError(CodeSchemaInvalid, "freshness basis memory ref does not match its target")
			}
		case basis.EvidenceRef != nil:
			if !store.scopeMatches(basis.EvidenceRef.Scope) {
				return storeError(CodeScopeMismatch, "freshness basis evidence ref has the wrong scope")
			}
			if !evidenceSet[evidenceKey(*basis.EvidenceRef)] {
				return storeError(CodeSchemaInvalid, "freshness basis evidence ref is missing")
			}
		case basis.JudgmentRef != nil:
			target, ok := judgmentByID[basis.JudgmentRef.JudgmentID]
			if !ok {
				return storeError(CodeSchemaInvalid, "freshness basis judgment ref is missing")
			}
			if err := validateJudgmentRefTarget(*basis.JudgmentRef, target); err != nil {
				return err
			}
		case basis.PolicyRef != nil:
			if _, err := NewPolicyStore(store).GetPolicy(ctx, *basis.PolicyRef); err != nil {
				return err
			}
		}
	}
	return nil
}

// listLatestRevisions loads every revision and keeps the latest revision
// per memory id.
func listLatestRevisions(ctx context.Context, store *FactStore) ([]MemoryRevision, error) {
	keys, err := store.List(ctx, FactKindMemoryRevision)
	if err != nil {
		return nil, err
	}
	byID := map[string]MemoryRevision{}
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindMemoryRevision, key)
		if err != nil {
			return nil, err
		}
		rev, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		if cur, ok := byID[rev.MemoryID]; !ok || rev.Revision > cur.Revision {
			byID[rev.MemoryID] = rev
		}
	}
	out := make([]MemoryRevision, 0, len(byID))
	for _, rev := range byID {
		out = append(out, rev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MemoryID < out[j].MemoryID })
	return out, nil
}

func listJudgments(ctx context.Context, store *FactStore) ([]JudgmentFact, error) {
	keys, err := store.List(ctx, FactKindJudgment)
	if err != nil {
		return nil, err
	}
	out := make([]JudgmentFact, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindJudgment, key)
		if err != nil {
			return nil, err
		}
		j, err := DecodeStrict[JudgmentFact](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		out = append(out, j)
	}
	return out, nil
}

func listEvidences(ctx context.Context, store *FactStore) ([]MemoryEvidenceGeneration, error) {
	keys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryEvidenceGeneration, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
		if err != nil {
			return nil, err
		}
		ev, err := DecodeStrict[MemoryEvidenceGeneration](data)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// EncodeCanonical renders the report deterministically (for tests and
// byte-stability checks).
func (r RevalidationReport) EncodeCanonical() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
