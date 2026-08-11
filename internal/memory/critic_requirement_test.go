package memory

// MEM-02-02 failure-first tests: Critic Requirement evaluation. The
// critic_review judgment subtype is NOT registered in the frozen MEM-01A
// Judgment union (JudgmentType is a strict enum), so EvaluateCriticRequirement
// must always report unavailable and never satisfy the evidence_validated
// critic condition. Malformed or unknown judgment payloads still fail closed.

import (
	"context"
	"encoding/json"
	"os"
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

func TestEvaluateCriticRequirementUnavailable(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)

	res, err := EvaluateCriticRequirement(context.Background(), s, CriticRequirementRequest{
		Scope: ScopeProject, MemoryID: rev.MemoryID, Revision: rev.Revision,
	})
	if err != nil {
		t.Fatalf("evaluate critic requirement: %v", err)
	}
	if res.Satisfied {
		t.Error("critic requirement must never be satisfied while critic_review is unregistered")
	}
	if res.Status != CriticRequirementUnavailable {
		t.Errorf("status = %q, want unavailable", res.Status)
	}
	if res.UsagePolicy != UsagePolicyEvidenceValidated {
		t.Errorf("usage policy = %q", res.UsagePolicy)
	}
}

func TestEvaluateCriticRequirementDerivedStateStaysProbation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)

	res, err := DeriveState(context.Background(), s, DerivedStateRequest{
		Scope: ScopeProject, Now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var state *DerivedMemoryState
	for i := range res.States {
		if res.States[i].MemoryID == rev.MemoryID {
			state = &res.States[i]
		}
	}
	if state == nil {
		t.Fatal("derived state missing the evidence_validated memory")
	}
	if state.Lifecycle != LifecycleProbation {
		t.Errorf("evidence_validated without critic must stay probation, got %s", state.Lifecycle)
	}
}

func TestEvaluateCriticRequirementRejectsUnknownJudgment(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)

	// A forged judgment with an unregistered subtype must fail closed when
	// the evaluator scans the judgment set, never be silently ignored.
	forged := `{
		"schema_version": 1,
		"judgment_id": "judgment_critic_forged",
		"judgment_type": "critic_review",
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
		t.Fatalf("plant forged judgment: %v", err)
	}
	_, err := EvaluateCriticRequirement(context.Background(), s, CriticRequirementRequest{
		Scope: ScopeProject, MemoryID: rev.MemoryID, Revision: rev.Revision,
	})
	if err == nil {
		t.Fatal("unregistered critic_review subtype must fail closed")
	}
	if ErrorCode(err) != CodeSchemaInvalid && ErrorCode(err) != CodeUnknownField && ErrorCode(err) != CodeHashMismatch {
		t.Errorf("expected schema/unknown/hash failure, got %v", err)
	}
}

func TestEvaluateCriticRequirementRejectsUnknownField(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)

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
	// Inject an unknown field into the payload JSON.
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
		t.Fatalf("plant judgment with unknown field: %v", err)
	}
	if _, err := EvaluateCriticRequirement(context.Background(), s, CriticRequirementRequest{
		Scope: ScopeProject, MemoryID: rev.MemoryID, Revision: rev.Revision,
	}); err == nil {
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

func TestEvaluateCriticRequirementScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)

	// A global scope request against a project store must fail closed.
	if _, err := EvaluateCriticRequirement(context.Background(), s, CriticRequirementRequest{
		Scope: ScopeGlobal, MemoryID: rev.MemoryID, Revision: rev.Revision,
	}); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope critic evaluation must fail closed, got %v", err)
	}
	// Missing revision must fail closed.
	if _, err := EvaluateCriticRequirement(context.Background(), s, CriticRequirementRequest{
		Scope: ScopeProject, MemoryID: "mem_missing", Revision: 1,
	}); ErrorCode(err) != CodeNotFound {
		t.Fatalf("missing revision must fail closed, got %v", err)
	}
}
