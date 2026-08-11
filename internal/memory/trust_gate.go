package memory

// MEM-02C: Evidence Provenance Trust Gate（只读、确定性、可审计）。
//
// EvaluateEvidenceTrust 从不可变 MemoryEvidenceGeneration、精确 PolicyRef 与
// 精确 Content Classification Judgment 派生 trusted|restricted|unverified|
// blocked|unavailable 与 instructional_content_allowed / promotion_eligible。
//
// 约束：
//   - 显式 Now；零值 CodeDerivedInvalidInput，绝不回退 time.Now()；
//   - 精确 Generation key（memory_id/revision/evidence_generation），不扫描
//     最新版本、不读 CURRENT；
//   - Policy 与 Classification 均按 scope/id/type/hash 精确加载，漂移 fail
//     closed，绝不用"最新"替代；
//   - Trust Policy 安全根不可关闭；只接受冻结 acquisition 枚举；旧自由
//     identifier Policy Fact 可读但 Gate fail closed，不改旧 Policy Hash；
//   - 结果不持久化，不自动创建 Judgment，不修改 Lifecycle/Revision/CURRENT；
//   - 错误固定脱敏。
//
// 事实源层级（Architecture v1 5.3）：Generation + Policy + Classification
// Judgment → Trust Gate 派生结果 → EvidenceTrustPayload 所需确定性字段。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TrustGateStatus is the fixed vocabulary of the trust gate.
type TrustGateStatus string

const (
	TrustGateTrusted     TrustGateStatus = "trusted"
	TrustGateRestricted  TrustGateStatus = "restricted"
	TrustGateUnverified  TrustGateStatus = "unverified"
	TrustGateBlocked     TrustGateStatus = "blocked"
	TrustGateUnavailable TrustGateStatus = "unavailable"
)

// TrustGateRequest selects the exact facts and policy for one evaluation.
type TrustGateRequest struct {
	Scope                    Scope
	MemoryID                 string
	Revision                 int
	EvidenceGeneration       int
	EvidenceRef              EvidenceRef
	TrustPolicyRef           PolicyRef
	ContentClassificationRef JudgmentRef
	Now                      time.Time
}

// TrustGateResult is the derived, byte-stable output. It is never persisted.
type TrustGateResult struct {
	Status                      TrustGateStatus `json:"status"`
	InstructionalContentAllowed bool            `json:"instructional_content_allowed"`
	PromotionEligible           bool            `json:"promotion_eligible"`
	EvaluatedAt                 string          `json:"evaluated_at"`
}

// EncodeCanonical returns the stable wire representation of a derived trust
// result. TrustGateResult remains derived data and is never persisted as a
// Fact.
func (r TrustGateResult) EncodeCanonical() ([]byte, error) {
	return json.Marshal(r)
}

// EvaluateEvidenceTrust derives the trust status of one exact evidence
// generation. It is read-only and deterministic: identical facts + policy +
// classification + Now yield byte-identical output.
func EvaluateEvidenceTrust(ctx context.Context, store *FactStore, req TrustGateRequest) (*TrustGateResult, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "trust gate scope must be project or global")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match trust gate scope")
	}
	if req.Now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "trust gate requires an explicit now")
	}
	if err := req.EvidenceRef.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid evidence ref")
	}
	if req.EvidenceRef.Scope != req.Scope {
		return nil, storeError(CodeScopeMismatch, "evidence ref scope does not match trust gate scope")
	}
	if req.Revision < 1 || req.EvidenceGeneration < 1 {
		return nil, storeError(CodeDerivedInvalidInput, "revision and evidence_generation must be >= 1")
	}
	if err := validateID(req.MemoryID, "memory_id"); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid memory id")
	}
	if err := req.TrustPolicyRef.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid trust policy ref")
	}
	if req.TrustPolicyRef.PolicyType != PolicyTypeTrust {
		return nil, storeError(CodeDerivedInvalidInput, "trust policy ref must be a trust policy")
	}
	if err := req.ContentClassificationRef.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid content classification ref")
	}
	if req.ContentClassificationRef.JudgmentType != JudgmentTypeContentClassification {
		return nil, storeError(CodeDerivedInvalidInput, "content classification ref must be a content_classification judgment")
	}
	if req.ContentClassificationRef.Scope != req.Scope {
		return nil, storeError(CodeScopeMismatch, "content classification ref scope does not match trust gate scope")
	}

	// 1. Exact generation: memory_id/revision/evidence_generation.
	key := memoryEvidenceKey(req.MemoryID, req.Revision, req.EvidenceGeneration)
	raw, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
	if err != nil {
		return nil, err
	}
	gen, err := DecodeStrict[MemoryEvidenceGeneration](raw)
	if err != nil {
		return nil, storeError(CodeSchemaInvalid, "evidence generation is corrupt")
	}
	if gen.MemoryID != req.MemoryID || gen.Revision != req.Revision ||
		gen.EvidenceGeneration != req.EvidenceGeneration {
		return nil, storeError(CodeSchemaInvalid, "evidence generation identity does not match its storage key")
	}
	// The target evidence ref must be an exact member of the generation.
	members := make(map[string]bool, len(gen.EvidenceRefs))
	for _, r := range gen.EvidenceRefs {
		members[evidenceRefKey(r)] = true
	}
	if !members[evidenceRefKey(req.EvidenceRef)] {
		return nil, storeError(CodeSchemaInvalid, "evidence ref is not a member of the evidence generation")
	}
	// Legacy provenance validates the exact generation and its time anchor,
	// then stops before Classification/Policy because the required content
	// signals do not exist in the historical schema.
	nowUTC := req.Now.UTC()
	if err := checkNotFuture(gen.CreatedAt, nowUTC); err != nil {
		return nil, err
	}
	if !gen.provenanceComplete() {
		return legacyTrustResult(req.Now), nil
	}

	// 2. Exact classification judgment by scope/type/id/hash.
	judgments, err := listJudgments(ctx, store)
	if err != nil {
		return nil, err
	}
	var classJud *JudgmentFact
	for i := range judgments {
		j := &judgments[i]
		if j.JudgmentID != req.ContentClassificationRef.JudgmentID {
			continue
		}
		if j.Scope != req.ContentClassificationRef.Scope ||
			j.JudgmentType != req.ContentClassificationRef.JudgmentType ||
			j.ContentSHA256 != req.ContentClassificationRef.ContentSHA256 {
			return nil, storeError(CodeHashMismatch, "content classification ref does not match the stored judgment")
		}
		classJud = j
		break
	}
	if classJud == nil {
		return nil, storeError(CodeNotFound, "content classification judgment not found")
	}

	// 3. Classification consistency: subject/payload evidence ref and both
	// booleans must match the generation exactly.
	payload := classJud.ContentClassification
	if payload == nil || classJud.Subject.SubjectType != "evidence" || classJud.Subject.EvidenceRef == nil {
		return nil, storeError(CodeSchemaInvalid, "content classification subject must reference evidence")
	}
	if evidenceRefKey(*classJud.Subject.EvidenceRef) != evidenceRefKey(req.EvidenceRef) ||
		evidenceRefKey(payload.EvidenceRef) != evidenceRefKey(req.EvidenceRef) {
		return nil, storeError(CodeSchemaInvalid, "content classification evidence refs do not match the target evidence")
	}
	if payload.ContainsInstructionalContent != *gen.ContainsInstructionalContent ||
		payload.ContainsSensitiveContent != *gen.ContainsSensitiveContent {
		return nil, storeError(CodeSchemaInvalid, "content classification booleans do not match the evidence generation")
	}

	// 4. Classifier policy exact load; must exist with matching hash.
	ps := NewPolicyStore(store)
	classifier, err := ps.GetPolicy(ctx, payload.ClassifierPolicyRef)
	if err != nil {
		return nil, err
	}
	if classifier.Config.ContentClassifier == nil {
		return nil, storeError(CodeSchemaInvalid, "classifier policy has no content_classifier config")
	}

	// 5. Trust policy exact load + runtime security checks. The policy fact
	// is immutable and hash-anchored; the Gate additionally requires the
	// frozen acquisition enum and refuses legacy free-identifier policies.
	trustPol, err := ps.GetPolicy(ctx, req.TrustPolicyRef)
	if err != nil {
		return nil, err
	}
	if trustPol.Config.Trust == nil {
		return nil, storeError(CodeSchemaInvalid, "trust policy has no trust config")
	}
	cfg := *trustPol.Config.Trust
	if !cfg.RequireProvenance || !cfg.RequireVerificationStatus {
		return nil, storeError(CodeSchemaInvalid, "trust policy safety root is disabled")
	}
	if cfg.ExternalUnverifiedInstructionAllowed {
		return nil, storeError(CodeSchemaInvalid, "trust policy safety root is disabled")
	}
	if len(cfg.AllowedAcquisitionMethods) == 0 {
		return nil, storeError(CodeSchemaInvalid, "trust policy must allow at least one acquisition method")
	}
	allowed := make(map[string]bool, len(cfg.AllowedAcquisitionMethods))
	for _, m := range cfg.AllowedAcquisitionMethods {
		if err := validAcquisitionMethod(m); err != nil {
			// Legacy free-identifier policy: readable, but not acceptable
			// as a security policy. Fail closed, never remap.
			return nil, storeError(CodeSchemaInvalid, "trust policy contains an unfrozen acquisition method")
		}
		allowed[m] = true
	}

	// 6. Time: generation, classification judgment and both policies must
	// not be in the future relative to Now.
	if err := checkNotFuture(classJud.CreatedAt, nowUTC); err != nil {
		return nil, err
	}
	if err := checkNotFuture(classifier.CreatedAt, nowUTC); err != nil {
		return nil, err
	}
	if err := checkNotFuture(trustPol.CreatedAt, nowUTC); err != nil {
		return nil, err
	}

	// 7. Deterministic state matrix (MEM-02C chapter 5).
	res := &TrustGateResult{EvaluatedAt: nowUTC.Format(time.RFC3339Nano)}
	if !allowed[gen.AcquisitionMethod] {
		res.Status = TrustGateBlocked
		return res, nil
	}
	if *gen.ContainsSensitiveContent {
		res.Status = TrustGateBlocked
		return res, nil
	}
	if gen.EvidenceOrigin == "external" && gen.VerificationStatus == "unverified" &&
		*gen.ContainsInstructionalContent {
		res.Status = TrustGateBlocked
		return res, nil
	}
	switch gen.VerificationStatus {
	case "unverified":
		res.Status = TrustGateUnverified
		return res, nil
	case "inferred":
		res.Status = TrustGateRestricted
		return res, nil
	}
	// verified | confirmed with closed references, allowed method and no
	// sensitive content.
	res.Status = TrustGateTrusted
	res.InstructionalContentAllowed = *gen.ContainsInstructionalContent
	res.PromotionEligible = true
	return res, nil
}

func legacyTrustResult(now time.Time) *TrustGateResult {
	return &TrustGateResult{
		Status:      TrustGateUnavailable,
		EvaluatedAt: now.UTC().Format(time.RFC3339Nano),
	}
}

func memoryEvidenceKey(memoryID string, revision, generation int) string {
	return fmt.Sprintf("%s/%d/%d", memoryID, revision, generation)
}

// checkNotFuture rejects facts whose created_at is after now.
func checkNotFuture(createdAt string, now time.Time) error {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return storeError(CodeSchemaInvalid, "fact has an invalid created_at")
	}
	if t.After(now) {
		return storeError(CodeEvaluationFutureReference, "fact is in the future")
	}
	return nil
}
