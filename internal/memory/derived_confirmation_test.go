package memory

import (
	"context"
	"testing"
)

// ---- confirmationStatus boundary fail-closed ----
//
// The confirmation_source_ref anchor and every superseding node must be a
// confirmation for exactly this memory revision (same scope, memory_id,
// revision). Any mismatch yields unverifiable (degraded) or a fail-closed
// error — never active, never frozen.

// mkBoundaryDecision builds an explicit_confirmation decision without
// storing it, so the caller can attach a confirmation ref before Put.
func mkBoundaryDecision(t *testing.T, id string, revNo int) MemoryRevision {
	t.Helper()
	rev := validRevision()
	rev.MemoryID = id
	rev.MemoryType = MemoryTypeDecision
	rev.UsagePolicy = UsagePolicyExplicitConfirmation
	rev.Revision = revNo
	rev.CanonicalKey = "dec-" + id + "-" + itoa(revNo)
	rev.Title = "Decision " + id
	rev.Summary = "Boundary decision."
	rev = fillRevisionHash(rev)
	return rev
}

func mkRefJudgment(t *testing.T, s *FactStore, id string, subject MemoryRef, status string, supersedes *JudgmentRef) JudgmentFact {
	t.Helper()
	j := validConfirmationJudgment()
	j.JudgmentID = id
	j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &subject}
	if status == "revoked" {
		j.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	}
	if supersedes != nil {
		j.SupersedesJudgmentRef = supersedes
	}
	j = fillJudgmentHash(j)
	put(t, s, j)
	return j
}

func refTo(j JudgmentFact) *ConfirmationSourceRef {
	return &ConfirmationSourceRef{Scope: j.Scope, JudgmentType: JudgmentTypeConfirmation, JudgmentID: j.JudgmentID, ContentSHA256: j.ContentSHA256}
}

func TestConfirmationAnchorMismatchNeverActivates(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	other := mkStrategy("mem_dec_other", "target-other", 1)
	put(t, s, other)

	// A: the referenced confirmation describes a different memory.
	a := mkBoundaryDecision(t, "mem_dec_wrongmem", 1)
	otherRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: other.MemoryID, Revision: 1, ContentSHA256: other.ContentSHA256}
	jA := mkRefJudgment(t, s, "judgment_anchor_other", otherRef, "confirmed", nil)
	a.ConfirmationSourceRef = refTo(jA)
	a = fillRevisionHash(a)
	put(t, s, a)

	// B: same memory, but the confirmation describes the older revision 1
	// while this revision is 2 (the latest).
	old := mkStrategy("mem_dec_wrongrev", "target-wrongrev", 1)
	put(t, s, old)
	b := mkBoundaryDecision(t, "mem_dec_wrongrev", 2)
	oldRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: old.MemoryID, Revision: 1, ContentSHA256: old.ContentSHA256}
	jB := mkRefJudgment(t, s, "judgment_wrongrev", oldRef, "confirmed", nil)
	b.ConfirmationSourceRef = refTo(jB)
	b = fillRevisionHash(b)
	put(t, s, b)

	// C: a confirmation whose subject is an outcome, not a revision.
	c := mkBoundaryDecision(t, "mem_dec_outcome", 1)
	u := mkUsage(t, "usage_conf_out", c.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	o := mkOutcome(t, "outcome_conf_out", u.UsageID, c.MemoryID, 1, "helped", false)
	put(t, s, o)
	jC := validConfirmationJudgment()
	jC.JudgmentID = "judgment_outcome_subject"
	jC.Subject = JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: o.OutcomeID}
	jC = fillJudgmentHash(jC)
	put(t, s, jC)
	c.ConfirmationSourceRef = refTo(jC)
	c = fillRevisionHash(c)
	put(t, s, c)

	res := derive(t, s, deriveReq())
	for _, id := range []string{a.MemoryID, b.MemoryID, c.MemoryID} {
		st := stateByID(t, res, id)
		if st.Lifecycle == LifecycleActive || st.Lifecycle == LifecycleFrozen {
			t.Errorf("mismatched confirmation anchor must not activate or freeze %s, got %s", id, st.Lifecycle)
		}
		if st.Lifecycle != LifecycleDegraded {
			t.Errorf("mismatched confirmation anchor %s = %s, want degraded (unverifiable)", id, st.Lifecycle)
		}
	}
}

func TestConfirmationCrossScopeSubjectFailClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := mkBoundaryDecision(t, "mem_dec_crossscope", 1)
	// The referenced confirmation describes a memory in the global scope,
	// which does not exist in this project store: the derivation must fail
	// closed (no activation).
	globalRef := MemoryRef{Scope: ScopeGlobal, MemoryType: MemoryTypeDecision, MemoryID: "mem_global_x", Revision: 1, ContentSHA256: testHash}
	j := mkRefJudgment(t, s, "judgment_crossscope", globalRef, "confirmed", nil)
	rev.ConfirmationSourceRef = refTo(j)
	rev = fillRevisionHash(rev)
	put(t, s, rev)

	if _, err := DeriveState(context.Background(), s, deriveReq()); err == nil {
		t.Error("cross-scope confirmation subject must fail closed")
	} else if !IsSensitiveError(err) {
		t.Errorf("error must be redacted, got %T", err)
	}
}

// A superseding node that stops describing the current memory revision must
// invalidate the chain: unverifiable (degraded), never frozen via a foreign
// revocation.
func TestConfirmationSupersedeNodeMismatch(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := mkBoundaryDecision(t, "mem_dec_chain_node", 1)
	other := mkStrategy("mem_dec_chain_other", "target-other", 1)
	put(t, s, other)

	ownRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c1 := mkRefJudgment(t, s, "judgment_chain_c1", ownRef, "confirmed", nil)
	rev.ConfirmationSourceRef = refTo(c1)
	rev = fillRevisionHash(rev)
	put(t, s, rev)

	// c2 revokes c1 but its subject points at a different memory.
	otherRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: other.MemoryID, Revision: 1, ContentSHA256: other.ContentSHA256}
	mkRefJudgment(t, s, "judgment_chain_c2", otherRef, "revoked", &JudgmentRef{
		Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256,
	})

	st := stateByID(t, derive(t, s, deriveReq()), rev.MemoryID)
	if st.Lifecycle == LifecycleFrozen || st.Lifecycle == LifecycleActive {
		t.Errorf("foreign superseding node must not freeze/activate, got %s", st.Lifecycle)
	}
	if st.Lifecycle != LifecycleDegraded {
		t.Errorf("foreign superseding node = %s, want degraded (unverifiable chain)", st.Lifecycle)
	}
}

// A valid same-revision chain still resolves confirmed/revoked semantics.
func TestConfirmationValidChainSemantics(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := mkBoundaryDecision(t, "mem_dec_valid_chain", 1)
	ownRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c1 := mkRefJudgment(t, s, "judgment_valid_c1", ownRef, "confirmed", nil)
	rev.ConfirmationSourceRef = refTo(c1)
	rev = fillRevisionHash(rev)
	put(t, s, rev)

	// c2 revokes c1 and correctly describes the same revision.
	mkRefJudgment(t, s, "judgment_valid_c2", ownRef, "revoked", &JudgmentRef{
		Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256,
	})

	st := stateByID(t, derive(t, s, deriveReq()), rev.MemoryID)
	if st.Lifecycle != LifecycleFrozen {
		t.Errorf("valid revoked chain = %s, want frozen", st.Lifecycle)
	}
}

// When two judgments supersede the same node, the newest one wins: an older
// revocation must not freeze a revision that a newer confirmation re-asserts.
func TestConfirmationMultipleSupersedeNewestWins(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := mkBoundaryDecision(t, "mem_dec_multi_sup", 1)
	ownRef := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c1 := mkRefJudgment(t, s, "judgment_multi_c1", ownRef, "confirmed", nil)
	rev.ConfirmationSourceRef = refTo(c1)
	rev = fillRevisionHash(rev)
	put(t, s, rev)

	// Older revocation supersedes c1...
	oldRevoked := validConfirmationJudgment()
	oldRevoked.JudgmentID = "judgment_multi_old"
	oldRevoked.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &ownRef}
	oldRevoked.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	oldRevoked.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	oldRevoked.CreatedAt = "2026-08-10T00:00:00Z"
	oldRevoked = fillJudgmentHash(oldRevoked)
	put(t, s, oldRevoked)
	// ...newer confirmation supersedes c1 too and must win.
	newConfirmed := validConfirmationJudgment()
	newConfirmed.JudgmentID = "judgment_multi_new"
	newConfirmed.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &ownRef}
	newConfirmed.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	newConfirmed.CreatedAt = "2026-08-12T00:00:00Z"
	newConfirmed = fillJudgmentHash(newConfirmed)
	put(t, s, newConfirmed)

	st := stateByID(t, derive(t, s, deriveReq()), rev.MemoryID)
	if st.Lifecycle != LifecycleActive {
		t.Errorf("newest superseding confirmation must win, got %s (want active)", st.Lifecycle)
	}
}
