package memory

import (
	"context"
	"strings"
	"testing"
)

// ---- evidence_validated: Critic protocol gap keeps it on probation ----

// Architecture 11.5 active conditions for evidence_validated require >=3
// independent EvidenceRefs, >=2 independent Root Tasks / formal sources, no
// unresolved conflicts AND a passing Critic Judgment. The Critic judgment
// subtype is not registered (architecture 6.2.3 requires a fixed payload
// protocol that later phases define), so per the CTO decision the active
// condition cannot be satisfied: evidence_validated stays probation even
// with all other conditions met. The root_task_refs field is implemented and
// schema-tested so the future Critic protocol can enable the threshold.
func TestEvidenceValidatedStaysProbationWithoutCritic(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// All conditions except Critic met: 3 independent evidence refs, 2
	// independent root tasks, no contradicts relations.
	ev := validEvidenceGeneration()
	ev.MemoryID = "mem_pattern_critic_gap"
	ev.Revision = 1
	ev.EvidenceRefs = []EvidenceRef{
		{Scope: ScopeProject, EvidenceType: "test_result", EvidenceID: "tr_a", ContentSHA256: testHash},
		{Scope: ScopeProject, EvidenceType: "test_result", EvidenceID: "tr_b", ContentSHA256: testHash},
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_c", ContentSHA256: testHash},
	}
	ev.RootTaskRefs = []string{"task_alpha", "task_beta"}
	ev = fillEvidenceHash(ev)
	rev := validRevision()
	rev.MemoryID = "mem_pattern_critic_gap"
	rev.MemoryType = MemoryTypePattern
	rev.UsagePolicy = UsagePolicyEvidenceValidated
	rev.CanonicalKey = "pattern-critic-gap"
	rev.Title = "Pattern Critic Gap"
	rev.Summary = "Pattern with full evidence but no Critic protocol."
	rev = fillRevisionHash(rev)
	putRevisionEvidence(t, s, rev, ev)

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, rev.MemoryID)
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("evidence_validated without a registered Critic = %s, want probation (protocol gap)", st.Lifecycle)
	}
	if st.Health != HealthHealthy {
		t.Errorf("full-evidence pattern health = %s, want healthy", st.Health)
	}
	// Boundary: a single evidence ref is also probation (and would be even
	// with a Critic).
	ev2 := validEvidenceGeneration()
	ev2.MemoryID = "mem_pattern_single"
	ev2.Revision = 1
	ev2.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "test_result", EvidenceID: "tr_one", ContentSHA256: testHash}}
	ev2.RootTaskRefs = []string{"task_one"}
	ev2 = fillEvidenceHash(ev2)
	rev2 := validRevision()
	rev2.MemoryID = "mem_pattern_single"
	rev2.MemoryType = MemoryTypePattern
	rev2.UsagePolicy = UsagePolicyEvidenceValidated
	rev2.CanonicalKey = "pattern-single"
	rev2.Title = "Pattern Single"
	rev2.Summary = "Single-evidence pattern."
	rev2 = fillRevisionHash(rev2)
	putRevisionEvidence(t, s, rev2, ev2)
	st2 := stateByID(t, derive(t, s, deriveReq()), rev2.MemoryID)
	if st2.Lifecycle != LifecycleProbation {
		t.Errorf("single-evidence pattern = %s, want probation", st2.Lifecycle)
	}
}

// root_task_refs schema: controlled identifiers, unique, strict decode.
func TestRootTaskRefsSchema(t *testing.T) {
	valid := validEvidenceGeneration()
	valid.RootTaskRefs = []string{"task_alpha", "task_beta"}
	valid = fillEvidenceHash(valid)
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid root_task_refs rejected: %v", err)
	}

	bad := []struct {
		name string
		mut  func(*MemoryEvidenceGeneration)
	}{
		{"path traversal", func(e *MemoryEvidenceGeneration) { e.RootTaskRefs = []string{"../evil"} }},
		{"empty", func(e *MemoryEvidenceGeneration) { e.RootTaskRefs = []string{""} }},
		{"duplicate", func(e *MemoryEvidenceGeneration) { e.RootTaskRefs = []string{"task_a", "task_a"} }},
	}
	for _, b := range bad {
		t.Run(b.name, func(t *testing.T) {
			e := validEvidenceGeneration()
			e.RootTaskRefs = []string{"task_a"}
			b.mut(&e)
			e = fillEvidenceHash(e)
			if err := e.Validate(); err == nil {
				t.Error("invalid root_task_refs must be rejected")
			}
		})
	}

	// Idempotency: same identity + same hash NOOP.
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	if _, err := s.Put(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	res, err := s.Put(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != WriteNoop {
		t.Errorf("repeat put with root_task_refs must be NOOP, got %v", res.Status)
	}
}

// ---- explicit_confirmation: architecture 11.5 state matrix ----

func TestExplicitConfirmationStateMatrix(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	mkDecision := func(id, key string) (MemoryRevision, JudgmentFact) {
		rev := validRevision()
		rev.MemoryID = id
		rev.MemoryType = MemoryTypeDecision
		rev.UsagePolicy = UsagePolicyExplicitConfirmation
		rev.Revision = 1
		rev.CanonicalKey = key
		rev.Title = "Decision " + id
		rev.Summary = "Decision under the confirmation protocol."
		jud := validConfirmationJudgment()
		jud.JudgmentID = "judgment_" + id
		jud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: id, Revision: 1, ContentSHA256: rev.ContentSHA256}
		jud = fillJudgmentHash(jud)
		rev.ConfirmationSourceRef = &ConfirmationSourceRef{
			Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation,
			JudgmentID: jud.JudgmentID, ContentSHA256: jud.ContentSHA256,
		}
		rev = fillRevisionHash(rev)
		return rev, jud
	}

	// 1. confirmed + verifiable ref -> active.
	confirmed, cJud := mkDecision("mem_decision_act", "dec-act")
	put(t, s, cJud)
	put(t, s, confirmed)

	// 2. revoked, no replacement revision -> frozen (never degraded).
	revoked, rJud := mkDecision("mem_decision_rev", "dec-rev")
	rJud.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	rJud.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_prior", ContentSHA256: testHash}
	rJud = fillJudgmentHash(rJud)
	put(t, s, rJud)
	put(t, s, revoked)

	// 3. unverifiable ref (judgment never written) -> degraded per the
	// architecture 11.5 "confirmation temporarily unverifiable" row.
	unverifiable, _ := mkDecision("mem_decision_unv", "dec-unv")
	unverifiable.ConfirmationSourceRef = &ConfirmationSourceRef{
		Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: "judgment_never_written", ContentSHA256: testHash,
	}
	unverifiable = fillRevisionHash(unverifiable)
	put(t, s, unverifiable)

	res := derive(t, s, deriveReq())
	if got := stateByID(t, res, confirmed.MemoryID).Lifecycle; got != LifecycleActive {
		t.Errorf("confirmed/verifiable = %s, want active", got)
	}
	revState := stateByID(t, res, revoked.MemoryID)
	if revState.Lifecycle != LifecycleFrozen {
		t.Errorf("revoked without replacement = %s, want frozen (not degraded)", revState.Lifecycle)
	}
	if revState.Health != HealthHealthy {
		t.Errorf("revoked without negative evidence = %s, want healthy", revState.Health)
	}
	if got := stateByID(t, res, unverifiable.MemoryID).Lifecycle; got != LifecycleDegraded {
		t.Errorf("unverifiable ref = %s, want degraded", got)
	}
}

func TestExplicitConfirmationRevokedWithReplacement(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// The old revision is revoked; a replacement revision exists.
	oldJud := validConfirmationJudgment()
	oldJud.JudgmentID = "judgment_old"
	oldJud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: "mem_decision_chain", Revision: 1, ContentSHA256: testHash}
	oldJud = fillJudgmentHash(oldJud)
	put(t, s, oldJud)

	oldRev := validRevision()
	oldRev.MemoryID = "mem_decision_chain"
	oldRev.MemoryType = MemoryTypeDecision
	oldRev.UsagePolicy = UsagePolicyExplicitConfirmation
	oldRev.Revision = 1
	oldRev.CanonicalKey = "dec-chain"
	oldRev.Title = "Chain v1"
	oldRev.Summary = "First revision."
	oldRev.ConfirmationSourceRef = &ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: oldJud.JudgmentID, ContentSHA256: oldJud.ContentSHA256}
	oldRev = fillRevisionHash(oldRev)
	put(t, s, oldRev)

	// Revocation supersedes the old confirmation.
	revJud := validConfirmationJudgment()
	revJud.JudgmentID = "judgment_revoke_chain"
	revJud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: "mem_decision_chain", Revision: 1, ContentSHA256: oldRev.ContentSHA256}
	revJud.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	revJud.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: oldJud.JudgmentID, ContentSHA256: oldJud.ContentSHA256}
	revJud = fillJudgmentHash(revJud)
	put(t, s, revJud)

	// Replacement revision 2 with its own confirmation.
	newJud := validConfirmationJudgment()
	newJud.JudgmentID = "judgment_new"
	newJud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: "mem_decision_chain", Revision: 2, ContentSHA256: testHash}
	newJud = fillJudgmentHash(newJud)
	put(t, s, newJud)
	newRev := validRevision()
	newRev.MemoryID = "mem_decision_chain"
	newRev.MemoryType = MemoryTypeDecision
	newRev.UsagePolicy = UsagePolicyExplicitConfirmation
	newRev.Revision = 2
	newRev.CanonicalKey = "dec-chain"
	newRev.Title = "Chain v2"
	newRev.Summary = "Replacement revision."
	newRev.ConfirmationSourceRef = &ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: newJud.JudgmentID, ContentSHA256: newJud.ContentSHA256}
	newRev = fillRevisionHash(newRev)
	put(t, s, newRev)

	// The old revision is not the latest: with a replacement it derives as
	// superseded (requesting the old revision explicitly).
	reqOld := deriveReq()
	reqOld.Revision = 1
	resOld := derive(t, s, reqOld)
	oldState := stateByID(t, resOld, oldRev.MemoryID)
	if oldState.Lifecycle != LifecycleSuperseded {
		t.Errorf("revoked old revision with a replacement = %s, want superseded", oldState.Lifecycle)
	}
	if got := stateByID(t, derive(t, s, deriveReq()), newRev.MemoryID).Lifecycle; got != LifecycleActive {
		t.Errorf("replacement revision = %s, want active", got)
	}
}

// A revoked confirmation that supersedes the ref'd confirmed one resolves
// through the chain to frozen.
func TestExplicitConfirmationSupersedeChain(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	rev.MemoryID = "mem_decision_chainrev"
	rev.MemoryType = MemoryTypeDecision
	rev.UsagePolicy = UsagePolicyExplicitConfirmation
	rev.Revision = 1
	rev.CanonicalKey = "dec-chainrev"
	rev.Title = "Chain Rev"
	rev.Summary = "Chain resolution."
	c1 := validConfirmationJudgment()
	c1.JudgmentID = "judgment_c1"
	c1.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c1 = fillJudgmentHash(c1)
	put(t, s, c1)
	rev.ConfirmationSourceRef = &ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	rev = fillRevisionHash(rev)
	put(t, s, rev)
	// c2 revokes c1; the ref points at c1, so the chain must resolve to c2.
	c2 := validConfirmationJudgment()
	c2.JudgmentID = "judgment_c2"
	c2.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c2.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	c2.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	c2 = fillJudgmentHash(c2)
	put(t, s, c2)

	res := derive(t, s, deriveReq())
	if got := stateByID(t, res, rev.MemoryID).Lifecycle; got != LifecycleFrozen {
		t.Errorf("ref'd confirmation revoked via chain = %s, want frozen", got)
	}
}

// A supersede cycle between confirmation judgments must fail closed into the
// default state instead of looping forever during derivation.
func TestExplicitConfirmationSupersedeCycleFailClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	rev.MemoryID = "mem_decision_cycle"
	rev.MemoryType = MemoryTypeDecision
	rev.UsagePolicy = UsagePolicyExplicitConfirmation
	rev.Revision = 1
	rev.CanonicalKey = "dec-cycle"
	rev.Title = "Cycle"
	rev.Summary = "Supersede cycle."
	// Supersede cycle: c2 supersedes c1, c3 supersedes c2, c1 supersedes c3.
	// The schema does not existence-check supersede targets, so the cycle is
	// storable; the derived walk must terminate instead of looping.
	c1 := validConfirmationJudgment()
	c1.JudgmentID = "judgment_cyc_a"
	c1.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c1.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_cyc_c", ContentSHA256: testHash}
	c1 = fillJudgmentHash(c1)
	rev.ConfirmationSourceRef = &ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	rev = fillRevisionHash(rev)
	put(t, s, rev)
	c2 := validConfirmationJudgment()
	c2.JudgmentID = "judgment_cyc_b"
	c2.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c2.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c1.JudgmentID, ContentSHA256: c1.ContentSHA256}
	c2 = fillJudgmentHash(c2)
	c3 := validConfirmationJudgment()
	c3.JudgmentID = "judgment_cyc_c"
	c3.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: rev.ContentSHA256}
	c3.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: c2.JudgmentID, ContentSHA256: c2.ContentSHA256}
	c3 = fillJudgmentHash(c3)
	put(t, s, c1)
	put(t, s, c2)
	put(t, s, c3)

	// Derivation must terminate (cycle -> fail closed) and not activate.
	st := stateByID(t, derive(t, s, deriveReq()), rev.MemoryID)
	if st.Lifecycle == LifecycleActive {
		t.Errorf("supersede cycle must not activate, got %s", st.Lifecycle)
	}
}

// The root_task_refs field changed the canonical hash. A document written in
// the old format (without the field) decodes structurally (absent field =
// empty list) but can never carry a hash that matches the new canonical
// form, so the store's hash check rejects it — fail closed, no silent
// reinterpretation of legacy bytes.
func TestRootTaskRefsLegacyDocumentLocked(t *testing.T) {
	hash := "sha256_" + strings.Repeat("0", 64)
	legacy := `{"schema_version":1,"memory_id":"mem_01K7A9X2","revision":2,"evidence_generation":3,"evidence_refs":[],"evidence_set_sha256":"` + hash + `","previous_evidence_generation":null,"transaction_id":"tx_01K","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[MemoryEvidenceGeneration]([]byte(legacy)); err == nil {
		t.Error("legacy-format document must fail the current hash check (fail closed)")
	}
	// The absence of the field is not itself rejected: a current-format
	// document with the field and a correct hash decodes fine.
	e := validEvidenceGeneration()
	e.RootTaskRefs = nil
	e = fillEvidenceHash(e)
	decoded, err := DecodeStrict[MemoryEvidenceGeneration](mustJSON(e))
	if err != nil {
		t.Fatalf("current-format document must decode: %v", err)
	}
	canon, err := decoded.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canon), `"root_task_refs": []`) {
		t.Errorf("canonical form must carry empty root_task_refs, got %s", canon)
	}
}
