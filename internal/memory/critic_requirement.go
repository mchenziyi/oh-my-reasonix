package memory

// MEM-02-02: Critic Requirement 评估（只读）。
//
// 架构 11.5 要求 evidence_validated 晋升 Active 需要 "Critic 通过"，且架构
// 6.2.1 明确 Critic Judgment 需 "对应协议定义固定 subtype payload"。MEM-01A
// 冻结的 JudgmentType 严格枚举与 JudgmentFact 判别联合均未注册 critic_review
// subtype（协议缺口，见 MEM-02 计划 Schema 变更提案）。因此本模块只实现
// 只读验证器：Critic 条件恒为 unavailable、永不满足，evidence_validated
// 派生生命周期保持 probation；扫描期间任何损坏、未知字段或未注册
// judgment_type 一律 fail closed（不静默忽略）。不新增 subtype、不修改
// 冻结 Schema、不写事实。

import (
	"context"
	"fmt"
)

// CriticRequirementStatus is the derived status of the evidence_validated
// critic condition. Only unavailable exists today: the critic_review
// subtype is unregistered, so no critic evidence can ever be produced.
type CriticRequirementStatus string

const (
	CriticRequirementUnavailable CriticRequirementStatus = "unavailable"
)

// CriticRequirementRequest selects the revision whose critic condition is
// evaluated.
type CriticRequirementRequest struct {
	Scope    Scope
	MemoryID string
	Revision int
}

// CriticRequirementResult is derived, read-only data: it is never written
// back to any fact.
type CriticRequirementResult struct {
	Status      CriticRequirementStatus
	Satisfied   bool
	UsagePolicy UsagePolicy
}

// EvaluateCriticRequirement checks whether the evidence_validated critic
// condition can be satisfied for one revision. Because critic_review is not
// registered in the frozen Judgment union, the result is always
// unavailable + not satisfied; any judgment that is corrupt, carries an
// unknown field or uses an unregistered judgment_type fails closed instead
// of being skipped.
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

	revData, err := store.Get(ctx, FactKindMemoryRevision, fmt.Sprintf("%s/%d", req.MemoryID, req.Revision))
	if err != nil {
		return nil, err
	}
	rev, err := DecodeStrict[MemoryRevision](revData)
	if err != nil {
		return nil, classifyDecodeError(err)
	}

	// Scan the judgment set with full strict validation. Any corrupt,
	// unknown-field or unregistered-type judgment must fail closed: the
	// evaluator never guesses past a broken fact set.
	keys, err := store.List(ctx, FactKindJudgment)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		jData, err := store.Get(ctx, FactKindJudgment, key)
		if err != nil {
			return nil, err
		}
		if _, err := DecodeStrict[JudgmentFact](jData); err != nil {
			return nil, classifyDecodeError(err)
		}
	}

	// critic_review is not a registered JudgmentType, so no critic evidence
	// exists; the condition is unavailable, never satisfied.
	return &CriticRequirementResult{
		Status:      CriticRequirementUnavailable,
		Satisfied:   false,
		UsagePolicy: rev.UsagePolicy,
	}, nil
}
