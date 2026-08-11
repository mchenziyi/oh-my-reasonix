package memory

// MEM-02B failure-first tests: upgraded EvaluateCriticRequirement. The tests
// reference the new request fields (ExpectedMemoryContext / ProjectStore /
// GlobalStore / Now) and the critic_review subtype, so the package failed to
// compile until MEM-02B landed.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validEvidenceValidatedRevision() MemoryRevision {
	rev := validRevision()
	rev.MemoryID = "mem_critic_001"
	rev.MemoryType = MemoryTypePattern
	rev.UsagePolicy = UsagePolicyEvidenceValidated
	rev.CanonicalKey = "critic-pattern"
	rev.Title = "Critic Pattern"
	rev.Summary = "Evidence-validated pattern awaiting critic protocol."
	return fillRevisionHash(rev)
}

// criticWorld commits one OKF generation (fixed manifest created_at
// 2026-08-10), puts an evidence_validated revision plus its evidence
// generation, and returns the stores.
func criticWorld(t *testing.T, key string) (string, *FactStore, *GenerationTx, MemoryRevision, MemoryEvidenceGeneration) {
	t.Helper()
	root := tempRoot(t)
	tx, s, _ := commitOKFGeneration(t, root, key, nil)
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)
	ev := MemoryEvidenceGeneration{
		SchemaVersion:      1,
		MemoryID:           rev.MemoryID,
		Revision:           rev.Revision,
		EvidenceGeneration: 1,
		EvidenceRefs:       []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}},
		TransactionID:      "tx_critic",
		CreatedAt:          "2026-08-11T00:00:00Z",
	}
	h, err := ev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	ev.EvidenceSetSHA256 = h
	if _, err := s.Put(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	return root, s, tx, rev, ev
}

// expectedContext builds the request context pinned to tx's generation,
// reading the real permanent manifest hash.
func expectedContext(t *testing.T, s *FactStore, tx *GenerationTx) MemoryContext {
	t.Helper()
	mfData, err := s.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfData)
	if err != nil {
		t.Fatal(err)
	}
	return MemoryContext{
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion: 1, Scope: ScopeProject,
			GenerationID: tx.GenerationID, InputManifestID: mf.GenerationID,
			InputManifestSHA256: mf.InputManifestSHA256,
		},
		GlobalGenerationRef: nil,
	}
}

func criticReq(rev MemoryRevision, ctx MemoryContext) CriticRequirementRequest {
	return CriticRequirementRequest{
		Scope:                 rev.Scope,
		MemoryID:              rev.MemoryID,
		Revision:              rev.Revision,
		ExpectedMemoryContext: ctx,
		ProjectStore:          nil, // set by caller when needed
		Now:                   time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	}
}

// putCritic stores a critic_review judgment for the revision with the given
// result, supersede target and optional evidence override.
func putCritic(t *testing.T, s *FactStore, rev MemoryRevision, mc MemoryContext, id, result string, supersedes *JudgmentRef, evidence *EvidenceRef) JudgmentFact {
	t.Helper()
	base := validCriticEvidence()
	req := []EvidenceRef{}
	basis := []BasisRef{}
	if evidence != nil {
		req = append(req, *evidence)
		basis = append(basis, BasisRef{EvidenceRef: evidence})
	}
	if evidence == nil && result == "passed" {
		// passed requires evidence; use the world evidence by default.
		ev := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}
		req = append(req, ev)
		basis = append(basis, BasisRef{EvidenceRef: &ev})
	}
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    id,
		JudgmentType:  JudgmentTypeCriticReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		CriticReview: &CriticReviewPayload{
			Result:               result,
			EvaluationScope:      "generation_full_scan",
			MemoryContext:        mc,
			RequiredEvidenceRefs: req,
		},
		BasisRefs:             basis,
		SupersedesJudgmentRef: supersedes,
		CreatedAt:             "2026-08-11T00:00:00Z",
	}
	j = fillCriticHash(j)
	if _, err := s.Put(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	_ = base
	return j
}

func evalCritic(t *testing.T, s *FactStore, req CriticRequirementRequest) (*CriticRequirementResult, error) {
	t.Helper()
	return EvaluateCriticRequirement(context.Background(), s, req)
}

// ---- 5.1 no matching critic ----

func TestCriticRequirementNoCriticUnavailable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_none")
	res, err := evalCritic(t, s, criticReq(rev, expectedContext(t, s, tx)))
	if err != nil {
		t.Fatal(err)
	}
	if res.Satisfied || res.Status != CriticRequirementUnavailable {
		t.Errorf("no critic must be unavailable, got %s satisfied=%v", res.Status, res.Satisfied)
	}
	if res.UsagePolicy != UsagePolicyEvidenceValidated {
		t.Errorf("usage policy = %q", res.UsagePolicy)
	}
}

// ---- 5.2 fixed world ----

func TestCriticRequirementPassedSatisfied(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_pass")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_p1", "passed", nil, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementPassed || !res.Satisfied {
		t.Errorf("passed critic must satisfy, got %s satisfied=%v", res.Status, res.Satisfied)
	}
}

func TestCriticRequirementContextMismatchUnavailable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_ctxmismatch")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_cm", "passed", nil, nil)
	// Different expected context (different generation id).
	other := MemoryContext{
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion: 1, Scope: ScopeProject,
			GenerationID: "gen_project_000099", InputManifestID: "gen_project_000099",
			InputManifestSHA256: testHash,
		},
		GlobalGenerationRef: nil,
	}
	req := criticReq(rev, other)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementUnavailable || res.Satisfied {
		t.Errorf("context mismatch must be unavailable, got %s", res.Status)
	}
}

func TestCriticRequirementZeroNowRejected(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_zeronow")
	mc := expectedContext(t, s, tx)
	req := criticReq(rev, mc)
	req.Now = time.Time{}
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now must fail with derived_invalid_input, got %v", err)
	}
}

func TestCriticRequirementFutureGenerationFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_future")
	mc := expectedContext(t, s, tx)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	req.Now = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) // before manifest created_at
	if _, err := evalCritic(t, s, req); ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("future generation must fail closed, got %v", err)
	}
}

func TestCriticRequirementMissingStoreUnavailable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_nostore")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_ns", "passed", nil, nil)
	req := criticReq(rev, mc)
	// ProjectStore left nil: the world cannot be verified -> unavailable.
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementUnavailable || res.Satisfied {
		t.Errorf("missing store must be unavailable, got %s", res.Status)
	}
}

func TestCriticRequirementCURRENTSwitchDoesNotAffect(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_current")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_cur", "passed", nil, nil)
	// Commit a second generation on top of the first; CURRENT moves, the
	// pinned ref does not.
	commitOKFGeneration(t, root, "critic_current_2", &tx.GenerationID)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementPassed || !res.Satisfied {
		t.Errorf("CURRENT switch must not affect pinned world, got %s", res.Status)
	}
}

func TestCriticRequirementRebuildAfterCleanup(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_rebuild")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_rb", "passed", nil, nil)
	// Delete the published generation directory: the permanent manifest
	// must allow exact rebuild.
	if err := os.RemoveAll(filepath.Join(root, "generations", tx.GenerationID)); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementPassed || !res.Satisfied {
		t.Errorf("rebuild from manifest must satisfy, got %s", res.Status)
	}
}

func TestCriticRequirementUnavailableAfterManifestCleanup(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_unavail")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_un", "passed", nil, nil)
	if err := os.RemoveAll(filepath.Join(root, "generations", tx.GenerationID)); err != nil {
		t.Fatal(err)
	}
	// Remove the permanent manifest too: the world cannot be rebuilt.
	if err := os.Remove(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementUnavailable || res.Satisfied {
		t.Errorf("no rebuild path must be unavailable, got %s", res.Status)
	}
}

func TestCriticRequirementTamperedGenerationFailClosed(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_tamper")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_tp", "passed", nil, nil)
	genPath := filepath.Join(root, "generations", tx.GenerationID, "generation.json")
	doc, err := readJSONFile[generationDoc](genPath)
	if err != nil {
		t.Fatal(err)
	}
	doc.OutputGenerationSHA256 = testHash
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(genPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("tampered generation must fail closed, got %v", err)
	}
}

// ---- 5.3 evidence ----

func TestCriticRequirementPassedEvidenceMissingUnavailable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_evmissing")
	mc := expectedContext(t, s, tx)
	// required evidence is in basis but NOT in the revision's evidence set.
	missing := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_999", ContentSHA256: testHash}
	putCritic(t, s, rev, mc, "judgment_critic_em", "passed", nil, &missing)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementUnavailable || res.Satisfied {
		t.Errorf("missing required evidence must be unavailable, got %s", res.Status)
	}
}

func TestCriticRequirementCorruptEvidenceFailClosed(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_evcorrupt")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_ec", "passed", nil, nil)
	// Corrupt the evidence generation fact on disk.
	path := filepath.Join(root, "facts", "memory-evidence-generations", rev.MemoryID,
		fmt.Sprintf("%d", rev.Revision), "1.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("corrupt evidence must fail closed")
	}
}

// ---- 5.4 supersede ----

func TestCriticRequirementSupersedePassedToFailed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_sup1")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_s1a", "passed", nil, nil)
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256}
	putCritic(t, s, rev, mc, "judgment_critic_s1b", "failed", sup, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementFailed || res.Satisfied {
		t.Errorf("superseded passed -> failed expected failed, got %s", res.Status)
	}
}

func TestCriticRequirementSupersedeFailedToPassed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_sup2")
	mc := expectedContext(t, s, tx)
	f1 := putCritic(t, s, rev, mc, "judgment_critic_s2a", "failed", nil, nil)
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: f1.JudgmentID, ContentSHA256: f1.ContentSHA256}
	putCritic(t, s, rev, mc, "judgment_critic_s2b", "passed", sup, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementPassed || !res.Satisfied {
		t.Errorf("superseded failed -> passed expected passed, got %s", res.Status)
	}
}

func TestCriticRequirementSupersedeChainMismatchFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_supmismatch")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_sma", "passed", nil, nil)
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256}
	// Second node evaluates a different revision: the chain must fail closed.
	otherRev := validEvidenceValidatedRevision()
	otherRev.MemoryID = "mem_critic_002"
	otherRev = fillRevisionHash(otherRev)
	put(t, s, otherRev)
	j := putCritic(t, s, otherRev, mc, "judgment_critic_smb", "failed", sup, nil)
	_ = j
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede chain with mismatched subject must fail closed")
	}
}

func TestCriticRequirementSupersedeCycleFailClosed(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_cycle")
	mc := expectedContext(t, s, tx)
	// Immutability prevents mutating a stored judgment, so the cycle is
	// forged on disk: j1 supersedes j2 and j2 supersedes j1.
	j1 := putCritic(t, s, rev, mc, "judgment_critic_c1", "passed", nil, nil)
	j2 := putCritic(t, s, rev, mc, "judgment_critic_c2", "passed", nil, nil)
	j1.SupersedesJudgmentRef = &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: j2.JudgmentID, ContentSHA256: j2.ContentSHA256}
	j2.SupersedesJudgmentRef = &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: j1.JudgmentID, ContentSHA256: j1.ContentSHA256}
	// Re-hash both with their new supersede links and plant them raw.
	j1raw := fillCriticHash(j1)
	j2raw := fillCriticHash(j2)
	j2raw.SupersedesJudgmentRef = &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: j1raw.JudgmentID, ContentSHA256: j1raw.ContentSHA256}
	j2raw = fillCriticHash(j2raw)
	raw1, err := j1raw.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	raw2, err := j2raw.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plantRawFact(root, FactKindJudgment, j1raw.JudgmentID, raw1); err != nil {
		t.Fatal(err)
	}
	if _, err := plantRawFact(root, FactKindJudgment, j2raw.JudgmentID, raw2); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede cycle must fail closed")
	}
}

// putCriticRaw builds a critic judgment without writing it (for forged
// on-disk scenarios).
func putCriticRaw(t *testing.T, rev MemoryRevision, mc MemoryContext, id, result string, supersedes *JudgmentRef) JudgmentFact {
	t.Helper()
	ev := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    id,
		JudgmentType:  JudgmentTypeCriticReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		CriticReview: &CriticReviewPayload{
			Result:               result,
			EvaluationScope:      "generation_full_scan",
			MemoryContext:        mc,
			RequiredEvidenceRefs: []EvidenceRef{ev},
		},
		BasisRefs:             []BasisRef{{EvidenceRef: &ev}},
		SupersedesJudgmentRef: supersedes,
		CreatedAt:             "2026-08-11T00:00:00Z",
	}
	return fillCriticHash(j)
}

func TestCriticRequirementParallelConflictUnavailable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_parconf")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_pca", "passed", nil, nil)
	putCritic(t, s, rev, mc, "judgment_critic_pcb", "failed", nil, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementUnavailable || res.Satisfied {
		t.Errorf("parallel conflicting terminals must be unavailable, got %s", res.Status)
	}
}

func TestCriticRequirementParallelAgreementStable(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_paragree")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_paa", "passed", nil, nil)
	putCritic(t, s, rev, mc, "judgment_critic_pab", "passed", nil, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CriticRequirementPassed || !res.Satisfied {
		t.Errorf("parallel agreeing terminals must be passed, got %s", res.Status)
	}
}

// ---- 5.5 lifecycle isolation ----

func TestCriticRequirementSupersedeOutsideContextFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_supoutside")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_soa", "passed", nil, nil)
	// A superseder evaluating a different MemoryContext violates the frozen
	// chain constraint (plan 2.3): the chain must fail closed.
	mc2 := MemoryContext{
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion: 1, Scope: ScopeProject,
			GenerationID: "gen_project_000042", InputManifestID: "gen_project_000042",
			InputManifestSHA256: testHash,
		},
		GlobalGenerationRef: nil,
	}
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256}
	putCritic(t, s, rev, mc2, "judgment_critic_sob", "passed", sup, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede chain with mismatched memory context must fail closed (plan 2.3)")
	}
}

func TestCriticRequirementLifecycleStaysProbation(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_lifecycle")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_lc", "passed", nil, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Satisfied {
		t.Fatal("precondition: critic must be satisfied")
	}
	derived, err := DeriveState(context.Background(), s, DerivedStateRequest{
		Scope: ScopeProject, Now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var state *DerivedMemoryState
	for i := range derived.States {
		if derived.States[i].MemoryID == rev.MemoryID {
			state = &derived.States[i]
		}
	}
	if state == nil {
		t.Fatal("derived state missing the evidence_validated memory")
	}
	// Conflict Fact is not frozen: evidence_validated must stay probation
	// even though the critic condition itself is satisfied.
	if state.Lifecycle != LifecycleProbation {
		t.Errorf("evidence_validated must stay probation despite critic passed, got %s", state.Lifecycle)
	}
}

func TestCriticRequirementFailedNeverActive(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_notactive")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_na", "failed", nil, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Satisfied {
		t.Error("failed critic must never satisfy")
	}
}

// ---- 5.6 safety & scope ----

func TestCriticRequirementCrossScopeFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_scope")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_sc", "passed", nil, nil)
	req := criticReq(rev, mc)
	req.Scope = ScopeGlobal
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope request must fail closed, got %v", err)
	}
}

func TestCriticRequirementRedactedErrors(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_redact")
	mc := expectedContext(t, s, tx)
	putCritic(t, s, rev, mc, "judgment_critic_rd", "passed", nil, nil)
	req := criticReq(rev, mc)
	req.MemoryID = "../../etc/passwd; rm -rf /tmp"
	req.ProjectStore = s
	_, err := evalCritic(t, s, req)
	if err == nil {
		t.Fatal("unsafe memory id must be rejected")
	}
	for _, banned := range []string{"/etc/passwd", "rm -rf", "/tmp", "../../"} {
		if strings.Contains(err.Error(), banned) {
			t.Errorf("error leaks %q: %v", banned, err)
		}
	}
}

func TestCriticRequirementRejectsUnknownJudgment(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_unknown")
	// A forged judgment with an unknown subtype must fail closed during the
	// scan, never be silently ignored.
	forged := `{
		"schema_version": 1,
		"judgment_id": "judgment_critic_forged",
		"judgment_type": "critic_hallucinated",
		"scope": "project",
		"subject": {"subject_type": "memory_revision", "memory_ref": {
			"scope": "project", "memory_type": "pattern",
			"memory_id": "mem_critic_001", "revision": 1,
			"content_sha256": "` + rev.ContentSHA256 + `"}},
		"source": {"source_type": "fixture_oracle", "source_id": "fixture_001"},
		"content_sha256": "sha256_fake",
		"created_at": "2026-08-11T00:00:00Z"
	}`
	if _, err := plantRawFact(root, FactKindJudgment, "judgment_critic_forged", []byte(forged)); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, expectedContext(t, s, tx))
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("unregistered judgment_type must fail closed during scan")
	}
}

func TestCriticRequirementRejectsUnknownField(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_unknownfield")
	j := validConfirmationJudgment()
	j.JudgmentID = "judgment_unknown_field_critic"
	j.Subject.MemoryRef = &MemoryRef{
		Scope: rev.Scope, MemoryType: rev.MemoryType,
		MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
	}
	j = fillJudgmentHash(j)
	raw, err := j.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["unknown_judgment_field"] = true
	injected, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plantRawFact(root, FactKindJudgment, j.JudgmentID, injected); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, expectedContext(t, s, tx))
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("judgment with unknown field must fail closed during scan")
	}
}

// plantRawFact writes raw bytes directly into the fact tree, bypassing the
// store's validation, to simulate a tampered/forged fact file.
func plantRawFact(root string, kind FactKind, key string, data []byte) (string, error) {
	comps, err := validateFactKey(key)
	if err != nil {
		return "", err
	}
	path, err := secureJoin(root, factPathComps(kind, comps), true, true)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func TestCriticRequirementMissingRevisionFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_missingrev")
	mc := expectedContext(t, s, tx)
	req := criticReq(rev, mc)
	req.MemoryID = "mem_missing"
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); ErrorCode(err) != CodeNotFound {
		t.Fatalf("missing revision must fail closed, got %v", err)
	}
}

// ---- MEM-02B CTO review regressions ----

// putCriticWithType stores a critic whose subject MemoryRef uses the given
// memory_type (possibly different from the real revision type).
func putCriticWithType(t *testing.T, s *FactStore, rev MemoryRevision, mc MemoryContext, id, result string, supersedes *JudgmentRef, memType MemoryType) JudgmentFact {
	t.Helper()
	ev := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    id,
		JudgmentType:  JudgmentTypeCriticReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: memType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		CriticReview: &CriticReviewPayload{
			Result:               result,
			EvaluationScope:      "generation_full_scan",
			MemoryContext:        mc,
			RequiredEvidenceRefs: []EvidenceRef{ev},
		},
		BasisRefs:             []BasisRef{{EvidenceRef: &ev}},
		SupersedesJudgmentRef: supersedes,
		CreatedAt:             "2026-08-11T00:00:00Z",
	}
	j = fillCriticHash(j)
	if _, err := s.Put(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	return j
}

// TestCriticRequirementMemoryTypeMismatchNotMatch: a critic whose subject
// memory_type differs from the real revision must not match, so the
// condition stays unavailable and never satisfied.
func TestCriticRequirementMemoryTypeMismatchNotMatch(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_typemismatch")
	mc := expectedContext(t, s, tx)
	// rev is MemoryTypePattern; the critic claims MemoryTypeStrategy with
	// the same memory_id/revision/content_sha256.
	putCriticWithType(t, s, rev, mc, "judgment_critic_tm", "passed", nil, MemoryTypeStrategy)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	res, err := evalCritic(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Satisfied || res.Status != CriticRequirementUnavailable {
		t.Errorf("critic with wrong memory_type must not match, got %s satisfied=%v", res.Status, res.Satisfied)
	}
}

// TestCriticRequirementSupersedeMemoryTypeMismatchFailClosed: any supersede
// chain node with a different subject memory_type must fail closed.
func TestCriticRequirementSupersedeMemoryTypeMismatchFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_typemismatch_sup")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_tms1", "passed", nil, nil)
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256}
	putCriticWithType(t, s, rev, mc, "judgment_critic_tms2", "failed", sup, MemoryTypeStrategy)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede chain with mismatched memory_type must fail closed")
	}
}

// TestCriticRequirementSupersedeRefHashMismatchFailClosed: a supersede ref
// with a wrong content_sha256 must fail closed, never trust judgment_id alone.
func TestCriticRequirementSupersedeRefHashMismatchFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_refhash")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_rh1", "passed", nil, nil)
	sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: testHash2}
	putCritic(t, s, rev, mc, "judgment_critic_rh2", "failed", sup, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede ref with wrong content_sha256 must fail closed")
	}
}

// TestCriticRequirementSupersedeRefScopeMismatchFailClosed: a supersede ref
// with a wrong scope must fail closed.
func TestCriticRequirementSupersedeRefScopeMismatchFailClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "critic_refscope")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_rs1", "passed", nil, nil)
	sup := &JudgmentRef{Scope: ScopeGlobal, JudgmentType: JudgmentTypeCriticReview, JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256}
	putCritic(t, s, rev, mc, "judgment_critic_rs2", "failed", sup, nil)
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede ref with wrong scope must fail closed")
	}
}

// TestCriticRequirementSupersedeRefTypeMismatchFailClosed: a supersede ref
// with a wrong judgment_type must fail closed (forged on disk; the schema
// already rejects it via Put).
func TestCriticRequirementSupersedeRefTypeMismatchFailClosed(t *testing.T) {
	root, s, tx, rev, _ := criticWorld(t, "critic_reftype")
	mc := expectedContext(t, s, tx)
	p1 := putCritic(t, s, rev, mc, "judgment_critic_rt1", "passed", nil, nil)
	// Build a forged critic superseding p1 with a wrong ref type.
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_critic_rt2",
		JudgmentType:  JudgmentTypeCriticReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		CriticReview: &CriticReviewPayload{
			Result:               "failed",
			EvaluationScope:      "generation_full_scan",
			MemoryContext:        mc,
			RequiredEvidenceRefs: []EvidenceRef{},
		},
		BasisRefs: nil,
		SupersedesJudgmentRef: &JudgmentRef{
			Scope: rev.Scope, JudgmentType: JudgmentTypeConfirmation,
			JudgmentID: p1.JudgmentID, ContentSHA256: p1.ContentSHA256,
		},
		CreatedAt: "2026-08-11T00:00:00Z",
	}
	j = fillCriticHash(j)
	raw, err := j.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plantRawFact(root, FactKindJudgment, j.JudgmentID, raw); err != nil {
		t.Fatal(err)
	}
	req := criticReq(rev, mc)
	req.ProjectStore = s
	if _, err := evalCritic(t, s, req); err == nil {
		t.Fatal("supersede ref with wrong judgment_type must fail closed")
	}
}

// TestCriticRequirementParallelSupersedersWithMismatchedBranchFailClosed:
// when the same old critic is superseded by both a valid node (same subject
// and context) and an invalid node (different memory_type/context whose ref
// identity is fully correct), the invalid branch must fail closed instead of
// being bypassed by the candidate filter. The judgment write order is
// reversed between the two subtests to prove the result does not depend on
// map/directory traversal order.
func TestCriticRequirementParallelSupersedersWithMismatchedBranchFailClosed(t *testing.T) {
	for _, order := range []string{"valid_first", "invalid_first"} {
		t.Run(order, func(t *testing.T) {
			_, s, tx, rev, _ := criticWorld(t, "critic_parsup_"+order)
			mc := expectedContext(t, s, tx)
			old := putCritic(t, s, rev, mc, "judgment_critic_ps_old", "passed", nil, nil)
			sup := &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeCriticReview, JudgmentID: old.JudgmentID, ContentSHA256: old.ContentSHA256}
			validNew := func() JudgmentFact {
				return putCritic(t, s, rev, mc, "judgment_critic_ps_valid", "passed", sup, nil)
			}
			invalidNew := func() JudgmentFact {
				// Same ref identity (scope/type/id/hash correct) but a
				// different subject memory_type: chain-node mismatch.
				return putCriticWithType(t, s, rev, mc, "judgment_critic_ps_invalid", "passed", sup, MemoryTypeStrategy)
			}
			if order == "valid_first" {
				validNew()
				invalidNew()
			} else {
				invalidNew()
				validNew()
			}
			req := criticReq(rev, mc)
			req.ProjectStore = s
			_, err := evalCritic(t, s, req)
			if err == nil {
				t.Fatal("parallel superseders with a mismatched branch must fail closed")
			}
			if ErrorCode(err) != CodeSchemaInvalid {
				t.Fatalf("mismatched supersede branch must fail with schema_invalid, got %v", err)
			}
		})
	}
}
