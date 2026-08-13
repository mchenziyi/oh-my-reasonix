package memory

// MEM-02-07: Evidence / Judgment 关联一致性与 Doctor（只读）。
//
// 集中发现孤儿引用、断链、Hash 漂移与协议不一致：Judgment → Memory/Outcome/
// Policy/Judgment 引用完整性、supersede 链环、跨 Scope 引用、错误 Subject、
// 损坏事实。输出稳定诊断码与脱敏摘要；只读，不自动修复、不删除、不改
// CURRENT，不创建第二事实源。

import (
	"context"
	"encoding/json"
	"sort"
)

// ConsistencyCheck is one redacted finding.
type ConsistencyCheck struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // "error" | "warning"
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Detail   string `json:"detail"`
}

// ConsistencyReport is the deterministic, byte-stable doctor output.
type ConsistencyReport struct {
	Scope    string             `json:"scope"`
	Healthy  bool               `json:"healthy"`
	Findings []ConsistencyCheck `json:"findings"`
}

// EncodeCanonical renders the report deterministically.
func (r ConsistencyReport) EncodeCanonical() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ConsistencyRequest selects the scope to audit.
type ConsistencyRequest struct {
	Scope Scope
}

const (
	findingOrphanMemoryRef     = "orphan_memory_ref"
	findingOrphanOutcomeRef    = "orphan_outcome_ref"
	findingOrphanPolicyRef     = "orphan_policy_ref"
	findingOrphanJudgmentRef   = "orphan_judgment_ref"
	findingOrphanEvidenceRef   = "orphan_evidence_ref"
	findingReferenceMismatch   = "reference_mismatch"
	findingCrossScopeReference = "cross_scope_reference"
	findingSupersedeCycle      = "supersede_cycle"
	findingSubjectMismatch     = "subject_payload_mismatch"
	findingAttributionMismatch = "attribution_override_mismatch"
	findingCorruptFact         = "corrupt_fact"
)

// CheckConsistency audits every fact in the store's scope. It never writes,
// deletes or repairs anything; broken facts surface as findings.
func CheckConsistency(ctx context.Context, store *FactStore, req ConsistencyRequest) (*ConsistencyReport, error) {
	if req.Scope != ScopeProject && req.Scope != ScopeGlobal {
		return nil, storeError(CodeDerivedInvalidInput, "consistency scope must be project or global")
	}
	if !store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "store scope does not match consistency scope")
	}

	report := &ConsistencyReport{Scope: string(req.Scope), Healthy: true}

	revisions, err := loadRevisions(ctx, store, report)
	if err != nil {
		return nil, err
	}
	judgments, err := loadJudgmentFacts(ctx, store, report)
	if err != nil {
		return nil, err
	}
	outcomes, err := loadOutcomes(ctx, store, report)
	if err != nil {
		return nil, err
	}
	policies, err := loadPolicies(ctx, store, report)
	if err != nil {
		return nil, err
	}
	evidenceGenerations, err := loadEvidenceGenerations(ctx, store, report)
	if err != nil {
		return nil, err
	}

	revSet := map[string]bool{} // memory_id@revision
	revRefSet := map[string]bool{}
	for _, r := range revisions {
		revSet[fmtMemID(r.MemoryID, r.Revision)] = true
		ref := MemoryRef{Scope: r.Scope, MemoryType: r.MemoryType, MemoryID: r.MemoryID, Revision: r.Revision, ContentSHA256: r.ContentSHA256}
		revRefSet[memoryRefKey(ref)] = true
	}
	evidenceSet := map[string]bool{}
	for _, generation := range evidenceGenerations {
		for _, ref := range generation.EvidenceRefs {
			evidenceSet[evidenceKey(ref)] = true
		}
	}
	outcomeSet := map[string]bool{}
	outcomeByID := map[string]Outcome{}
	for _, o := range outcomes {
		outcomeSet[o.OutcomeID] = true
		outcomeByID[o.OutcomeID] = o
	}
	// policy facts keyed by policy_id + type + content hash (the PolicyRef
	// carries no version, so the exact hash is the anchor).
	polSet := map[string]bool{}
	for _, p := range policies {
		polSet[fmtPolicyKey(p.PolicyID, p.PolicyType, p.ContentSHA256)] = true
	}
	judgmentIDs := map[string]JudgmentType{}
	judgmentByID := map[string]JudgmentFact{}
	for _, j := range judgments {
		judgmentIDs[j.JudgmentID] = j.JudgmentType
		judgmentByID[j.JudgmentID] = j
	}
	// supersede refs: judgment_id -> superseded judgment_id
	supersedes := map[string]string{}
	for _, j := range judgments {
		if j.SupersedesJudgmentRef != nil {
			supersedes[j.JudgmentID] = j.SupersedesJudgmentRef.JudgmentID
		}
	}

	// 1. Judgment reference integrity.
	for _, j := range judgments {
		switch j.Subject.SubjectType {
		case "memory_revision":
			if j.Subject.MemoryRef == nil {
				add(report, findingSubjectMismatch, "error", "judgment", j.JudgmentID, "memory_revision subject without memory_ref")
				continue
			}
			if j.Subject.MemoryRef.Scope != j.Scope {
				add(report, findingCrossScopeReference, "error", "judgment", j.JudgmentID, "subject memory scope differs from judgment scope")
			}
			if !revSet[fmtMemID(j.Subject.MemoryRef.MemoryID, j.Subject.MemoryRef.Revision)] {
				add(report, findingOrphanMemoryRef, "error", "judgment", j.JudgmentID, "subject memory revision does not exist")
			}
		case "memory_outcome":
			if !outcomeSet[j.Subject.OutcomeID] {
				add(report, findingOrphanOutcomeRef, "error", "judgment", j.JudgmentID, "subject outcome does not exist")
			}
		case "context":
			if j.Subject.MemoryRef != nil && j.Subject.MemoryRef.Scope != j.Scope {
				add(report, findingCrossScopeReference, "error", "judgment", j.JudgmentID, "subject memory scope differs from judgment scope")
			}
		case "evidence":
			// Existence of evidence facts is not yet verifiable (no
			// registered evidence kind); format was schema-validated.
		}
		// Payload vs subject consistency.
		if j.FreshnessEvaluation != nil && j.Subject.SubjectType == "memory_revision" && j.Subject.MemoryRef != nil {
			pm := j.FreshnessEvaluation.MemoryRef
			if pm.MemoryID != j.Subject.MemoryRef.MemoryID || pm.Revision != j.Subject.MemoryRef.Revision {
				add(report, findingSubjectMismatch, "error", "judgment", j.JudgmentID, "freshness payload memory_ref differs from subject")
			}
		}
		// Policy refs must exist and match the required type.
		if j.FreshnessEvaluation != nil {
			checkPolicyRef(report, j, j.FreshnessEvaluation.FreshnessPolicyRef, PolicyTypeFreshness, polSet)
		}
		if j.EvidenceTrust != nil {
			checkPolicyRef(report, j, j.EvidenceTrust.TrustPolicyRef, PolicyTypeTrust, polSet)
		}
		if j.ContentClassification != nil {
			checkPolicyRef(report, j, j.ContentClassification.ClassifierPolicyRef, PolicyTypeContentClassifier, polSet)
		}
		if j.ConflictReview != nil {
			if j.Subject.MemoryRef != nil && !revRefSet[memoryRefKey(*j.Subject.MemoryRef)] {
				add(report, findingReferenceMismatch, "error", "judgment", j.JudgmentID, "conflict subject memory revision does not exactly match")
			}
			for _, ref := range j.ConflictReview.CounterpartMemoryRefs {
				if !revRefSet[memoryRefKey(ref)] {
					add(report, findingOrphanMemoryRef, "error", "judgment", j.JudgmentID, "conflict counterpart memory revision does not exist or does not match")
				}
			}
			for _, ref := range j.ConflictReview.EvidenceRefs {
				if !evidenceSet[evidenceKey(ref)] {
					add(report, findingOrphanEvidenceRef, "error", "judgment", j.JudgmentID, "conflict evidence reference does not exist")
				}
			}
		}
		// Supersede targets must exist and share the judgment type. The
		// target's actual registered type is authoritative: a lying ref
		// cannot bypass the check by declaring a different type.
		if j.SupersedesJudgmentRef != nil {
			targetType, ok := judgmentIDs[j.SupersedesJudgmentRef.JudgmentID]
			if !ok {
				add(report, findingOrphanJudgmentRef, "error", "judgment", j.JudgmentID, "supersede target judgment does not exist")
			} else if targetType != j.JudgmentType {
				add(report, findingSubjectMismatch, "error", "judgment", j.JudgmentID, "supersede target judgment type differs from the referencing judgment")
			} else if j.JudgmentType == JudgmentTypeConflictReview {
				target := judgmentByID[j.SupersedesJudgmentRef.JudgmentID]
				if validateJudgmentRefTarget(*j.SupersedesJudgmentRef, target) != nil || !conflictNodesEqual(j, target) {
					add(report, findingReferenceMismatch, "error", "judgment", j.JudgmentID, "conflict supersede reference does not exactly match its target")
				}
			}
		}
		if j.JudgmentType == JudgmentTypeAttributionOverride && j.AttributionOverride != nil {
			previous := ""
			if j.SupersedesJudgmentRef == nil {
				if o, ok := outcomeByID[j.Subject.OutcomeID]; ok {
					previous = o.Effect
				}
			} else if target, ok := judgmentByID[j.SupersedesJudgmentRef.JudgmentID]; ok && target.AttributionOverride != nil {
				previous = target.AttributionOverride.NewEffect
			}
			if previous == "" || previous != j.AttributionOverride.PreviousEffect {
				add(report, findingAttributionMismatch, "error", "judgment", j.JudgmentID, "attribution override previous effect does not match its target outcome")
			}
		}
	}

	// 2. Supersede cycles (guarded traversal, never loops).
	state := map[string]int{} // 0=unvisited 1=visiting 2=done
	var visit func(id string)
	visit = func(id string) {
		switch state[id] {
		case 1:
			add(report, findingSupersedeCycle, "error", "judgment", id, "supersede chain contains a cycle")
			return
		case 2:
			return
		}
		state[id] = 1
		if next, ok := supersedes[id]; ok {
			visit(next)
		}
		state[id] = 2
	}
	for id := range supersedes {
		visit(id)
	}

	// 3. Deterministic order.
	sort.Slice(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Detail < b.Detail
	})
	report.Healthy = len(report.Findings) == 0
	return report, nil
}

func checkPolicyRef(report *ConsistencyReport, j JudgmentFact, ref PolicyRef, want PolicyType, polSet map[string]bool) {
	if ref.PolicyID == "" {
		return // schema validation already rejected this; defensive skip
	}
	// The payload contract enforces the policy type at schema time, so only
	// the exact-hash existence check can drift here.
	if !polSet[fmtPolicyKey(ref.PolicyID, ref.PolicyType, ref.ContentSHA256)] {
		add(report, findingOrphanPolicyRef, "error", "judgment", j.JudgmentID, "policy ref does not match any policy fact")
	}
}

// fmtPolicyKey keys policy facts by id + type + content hash: PolicyRef
// carries no version, so the exact hash is the only anchor.
func fmtPolicyKey(id string, pt PolicyType, hash string) string {
	return id + "|" + string(pt) + "|" + hash
}

// isCorruptCode reports whether a store error means the fact itself is
// invalid (corrupt JSON, unknown field, schema violation, hash drift),
// which the doctor surfaces as a finding; environment-level failures (path,
// symlink, permissions) still abort the audit.
func isCorruptCode(code Code) bool {
	switch code {
	case CodeInvalidJSON, CodeUnknownField, CodeSchemaInvalid, CodeHashMismatch, CodeCorruptFile:
		return true
	}
	return false
}

func add(report *ConsistencyReport, code, severity, kind, id, detail string) {
	report.Findings = append(report.Findings, ConsistencyCheck{
		Code: code, Severity: severity, Kind: kind, ID: id, Detail: detail,
	})
}

func fmtMemID(id string, n int) string {
	return id + "@" + itoa(n)
}

// loadJudgmentFacts loads and strict-decodes every judgment; a corrupt fact
// surfaces as a finding instead of aborting the whole audit.
func loadJudgmentFacts(ctx context.Context, store *FactStore, report *ConsistencyReport) ([]JudgmentFact, error) {
	keys, err := store.List(ctx, FactKindJudgment)
	if err != nil {
		return nil, err
	}
	out := make([]JudgmentFact, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindJudgment, key)
		if err != nil {
			if isCorruptCode(ErrorCode(err)) {
				add(report, findingCorruptFact, "error", "judgment", key, "judgment fact fails strict validation")
				continue
			}
			return nil, err
		}
		j, err := DecodeStrict[JudgmentFact](data)
		if err != nil {
			add(report, findingCorruptFact, "error", "judgment", key, "judgment fact fails strict validation")
			continue
		}
		out = append(out, j)
	}
	return out, nil
}

func loadRevisions(ctx context.Context, store *FactStore, report *ConsistencyReport) ([]MemoryRevision, error) {
	keys, err := store.List(ctx, FactKindMemoryRevision)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryRevision, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindMemoryRevision, key)
		if err != nil {
			if isCorruptCode(ErrorCode(err)) {
				add(report, findingCorruptFact, "error", "memory_revision", key, "revision fact fails strict validation")
				continue
			}
			return nil, err
		}
		r, err := DecodeStrict[MemoryRevision](data)
		if err != nil {
			add(report, findingCorruptFact, "error", "memory_revision", key, "revision fact fails strict validation")
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func loadOutcomes(ctx context.Context, store *FactStore, report *ConsistencyReport) ([]Outcome, error) {
	keys, err := store.List(ctx, FactKindOutcome)
	if err != nil {
		return nil, err
	}
	out := make([]Outcome, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindOutcome, key)
		if err != nil {
			if isCorruptCode(ErrorCode(err)) {
				add(report, findingCorruptFact, "error", "outcome", key, "outcome fact fails strict validation")
				continue
			}
			return nil, err
		}
		o, err := DecodeStrict[Outcome](data)
		if err != nil {
			add(report, findingCorruptFact, "error", "outcome", key, "outcome fact fails strict validation")
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func loadPolicies(ctx context.Context, store *FactStore, report *ConsistencyReport) ([]PolicyFact, error) {
	keys, err := store.List(ctx, FactKindPolicy)
	if err != nil {
		return nil, err
	}
	out := make([]PolicyFact, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindPolicy, key)
		if err != nil {
			if isCorruptCode(ErrorCode(err)) {
				add(report, findingCorruptFact, "error", "policy", key, "policy fact fails strict validation")
				continue
			}
			return nil, err
		}
		p, err := DecodeStrict[PolicyFact](data)
		if err != nil {
			add(report, findingCorruptFact, "error", "policy", key, "policy fact fails strict validation")
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func loadEvidenceGenerations(ctx context.Context, store *FactStore, report *ConsistencyReport) ([]MemoryEvidenceGeneration, error) {
	keys, err := store.List(ctx, FactKindMemoryEvidenceGeneration)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryEvidenceGeneration, 0, len(keys))
	for _, key := range keys {
		data, err := store.Get(ctx, FactKindMemoryEvidenceGeneration, key)
		if err != nil {
			if isCorruptCode(ErrorCode(err)) {
				add(report, findingCorruptFact, "error", "memory_evidence_generation", key, "evidence generation fails strict validation")
				continue
			}
			return nil, err
		}
		generation, err := DecodeStrict[MemoryEvidenceGeneration](data)
		if err != nil {
			add(report, findingCorruptFact, "error", "memory_evidence_generation", key, "evidence generation fails strict validation")
			continue
		}
		out = append(out, generation)
	}
	return out, nil
}
