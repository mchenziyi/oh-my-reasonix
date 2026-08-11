package memory

// MEM-02-06: Freshness / Revalidation 评估（只读）。
//
// 时间老化与有效性隔离：本评估器只派生 fresh|aging|needs_revalidation 候选，
// 永不产生 frozen/superseded/archived，永不修改 Revision，永不写 Judgment 或
// 任何事实。freshness_evaluation Judgment 已冻结的字段（memory_ref、result、
// evaluated_at、freshness_policy_ref、basis_refs）被严格校验；计划要求的
// freshness_policy_sha256 / content_classification_ref 未冻结（提案 6.5），
// 本阶段以 freshness_policy_ref 的 Policy Fact 精确匹配作为 policy 锚。
// 相同 Now + Policy + Facts 输出字节稳定；Policy 漂移、未来 evaluated_at 只
// 作为诊断项并回退到冻结时间窗，绝不猜测；hash 漂移与损坏一律 fail closed。

import (
	"context"
	"encoding/json"
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
	diagFutureEvaluatedAt = "future_evaluated_at"
	diagPolicyDrift       = "policy_drift"
)

// EvaluateRevalidation derives revalidation candidates for every memory's
// latest revision in the store's scope. It is read-only and deterministic.
func EvaluateRevalidation(ctx context.Context, store *FactStore, req RevalidationRequest) (*RevalidationReport, error) {
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
	cfg := *policy.Config.Freshness

	revisions, err := listLatestRevisions(ctx, store)
	if err != nil {
		return nil, err
	}
	judgments, err := listJudgments(ctx, store)
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
		cand, diags := deriveRevalidation(rev, judgments, evidences, cfg, req.Now, req.FreshnessPolicyRef)
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
func deriveRevalidation(rev MemoryRevision, judgments []JudgmentFact, evidences []MemoryEvidenceGeneration, cfg PolicyConfigFreshness, now time.Time, policyRef PolicyRef) (RevalidationCandidate, []RevalidationDiagnostic) {
	cand := RevalidationCandidate{
		MemoryID: rev.MemoryID,
		Revision: rev.Revision,
	}
	var diags []RevalidationDiagnostic

	if j := latestLiveFreshnessJudgment(judgments, rev); j != nil {
		evaluated, err := time.Parse(time.RFC3339Nano, j.FreshnessEvaluation.EvaluatedAt)
		if err != nil {
			diags = append(diags, RevalidationDiagnostic{Code: "invalid_evaluated_at", MemoryID: rev.MemoryID, Detail: "freshness judgment timestamp is invalid"})
		} else if evaluated.After(now) {
			diags = append(diags, RevalidationDiagnostic{Code: diagFutureEvaluatedAt, MemoryID: rev.MemoryID, Detail: "freshness judgment evaluated_at lies in the future"})
		} else if !samePolicyRef(j.FreshnessEvaluation.FreshnessPolicyRef, policyRef) {
			diags = append(diags, RevalidationDiagnostic{Code: diagPolicyDrift, MemoryID: rev.MemoryID, Detail: "freshness judgment cites a different freshness policy"})
		} else {
			cand.Result = RevalidationResult(j.FreshnessEvaluation.Result)
			cand.EvaluatedAt = j.FreshnessEvaluation.EvaluatedAt
			cand.Reason = "judgment_driven"
			return cand, diags
		}
	}

	// Window-driven fallback: age of the latest activity.
	last := parseFactTime(rev.CreatedAt)
	for i := range evidences {
		ev := evidences[i]
		if ev.MemoryID != rev.MemoryID || ev.Revision != rev.Revision {
			continue
		}
		if t := parseFactTime(ev.CreatedAt); t.After(last) {
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
	cand.Reason = "window_driven"
	return cand, diags
}

// samePolicyRef compares two policy refs field by field.
func samePolicyRef(a, b PolicyRef) bool {
	return a.PolicyID == b.PolicyID && a.PolicyType == b.PolicyType && a.ContentSHA256 == b.ContentSHA256
}

// latestLiveFreshnessJudgment returns the newest freshness judgment for a
// revision that is not superseded by any other freshness judgment.
func latestLiveFreshnessJudgment(judgments []JudgmentFact, rev MemoryRevision) *JudgmentFact {
	superBy := map[string]bool{}
	for _, j := range judgments {
		if j.JudgmentType != JudgmentTypeFreshnessEvaluation {
			continue
		}
		if j.SupersedesJudgmentRef != nil {
			superBy[j.SupersedesJudgmentRef.JudgmentID] = true
		}
	}
	var best *JudgmentFact
	for i := range judgments {
		j := &judgments[i]
		if j.JudgmentType != JudgmentTypeFreshnessEvaluation || superBy[j.JudgmentID] {
			continue
		}
		if j.Subject.SubjectType != "memory_revision" || j.Subject.MemoryRef == nil {
			continue
		}
		if j.Subject.MemoryRef.MemoryID != rev.MemoryID || j.Subject.MemoryRef.Revision != rev.Revision {
			continue
		}
		if j.FreshnessEvaluation == nil {
			continue
		}
		if best == nil || judgmentNewer(*j, *best) {
			best = j
		}
	}
	return best
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
