package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- helpers ----

var deriveNow = time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

func deriveReq() DerivedStateRequest {
	return DerivedStateRequest{
		Scope: ScopeProject,
		Now:   deriveNow,
	}
}

func derive(t *testing.T, s *FactStore, req DerivedStateRequest) *DerivedStateResult {
	t.Helper()
	res, err := DeriveState(context.Background(), s, req)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	return res
}

func stateByID(t *testing.T, res *DerivedStateResult, id string) DerivedMemoryState {
	t.Helper()
	for _, st := range res.States {
		if st.MemoryID == id {
			return st
		}
	}
	t.Fatalf("state for %q not found (have %d states)", id, len(res.States))
	return DerivedMemoryState{}
}

// mkStrategy returns a fresh outcome_attributed strategy revision with the
// given identity pieces; every call gets a distinct memory.
func mkStrategy(id, canonicalKey string, rev int) MemoryRevision {
	r := validRevision()
	r.MemoryID = id
	r.CanonicalKey = canonicalKey
	r.Revision = rev
	r.Title = "Strategy " + id
	r.Summary = "Derived state test strategy."
	r = fillRevisionHash(r)
	return r
}

func mkUsage(t *testing.T, id, memoryID string, rev int, occurredAt string) MemoryUsage {
	return mkUsageFull(t, id, memoryID, rev, occurredAt, "affected", "episode_def")
}

func mkUsageFull(t *testing.T, id, memoryID string, rev int, occurredAt, stage, episodeID string) MemoryUsage {
	u := MemoryUsage{
		SchemaVersion: 1,
		UsageID:       id,
		Scope:         ScopeProject,
		MemoryID:      memoryID,
		Revision:      rev,
		UsageStage:    stage,
		EpisodeID:     episodeID,
		OccurredAt:    occurredAt,
		Source:        "local_user",
		CreatedAt:     occurredAt,
	}
	u = fillUsageHash(u)
	return u
}

func mkOutcome(t *testing.T, id, usageID, memoryID string, rev int, effect string, external bool) Outcome {
	o := Outcome{
		SchemaVersion:   1,
		OutcomeID:       id,
		Scope:           ScopeProject,
		UsageID:         usageID,
		MemoryID:        memoryID,
		Revision:        rev,
		Effect:          effect,
		ExternalFailure: external,
		CreatedAt:       "2026-08-11T11:00:00Z",
	}
	o = fillOutcomeHash(o)
	return o
}

func putRevEvidence(t *testing.T, s *FactStore, rev MemoryRevision) {
	t.Helper()
	putRevEvidenceAt(t, s, rev, "2026-08-07T00:00:00Z")
}

func putRevEvidenceAt(t *testing.T, s *FactStore, rev MemoryRevision, evCreatedAt string) {
	t.Helper()
	ev := validEvidenceGeneration()
	ev.MemoryID = rev.MemoryID
	ev.Revision = rev.Revision
	ev.EvidenceRefs = []EvidenceRef{{Scope: rev.Scope, EvidenceType: "test_result", EvidenceID: "tr_" + rev.MemoryID, ContentSHA256: testHash}}
	ev.CreatedAt = evCreatedAt
	ev = fillEvidenceHash(ev)
	putRevisionEvidence(t, s, rev, ev)
}

// ---- lifecycle: outcome_attributed ----

func TestDeriveLifecycleOutcomeAttributed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// No outcomes yet: probation, never guessed active.
	noOutcome := mkStrategy("mem_strategy_none", "no-outcome", 1)
	putRevEvidence(t, s, noOutcome)

	// Helped outcomes across two independent episodes: active (>=3 helps,
	// >=2 episodes, no harm).
	helped := mkStrategy("mem_strategy_helped", "helped", 1)
	putRevEvidence(t, s, helped)
	u1 := mkUsageFull(t, "usage_helped_1", helped.MemoryID, 1, "2026-08-11T10:00:00Z", "affected", "episode_h1")
	put(t, s, u1)
	put(t, s, mkOutcome(t, "outcome_helped_1", u1.UsageID, helped.MemoryID, 1, "helped", false))
	u1b := mkUsageFull(t, "usage_helped_2", helped.MemoryID, 1, "2026-08-12T10:00:00Z", "evaluated", "episode_h1")
	put(t, s, u1b)
	put(t, s, mkOutcome(t, "outcome_helped_2", u1b.UsageID, helped.MemoryID, 1, "helped", false))
	u1c := mkUsageFull(t, "usage_helped_3", helped.MemoryID, 1, "2026-08-13T10:00:00Z", "affected", "episode_h2")
	put(t, s, u1c)
	put(t, s, mkOutcome(t, "outcome_helped_3", u1c.UsageID, helped.MemoryID, 1, "helped", false))

	// Harmed outcome (even with help): degraded, success never cancels harm.
	mixed := mkStrategy("mem_strategy_mixed", "mixed", 1)
	putRevEvidence(t, s, mixed)
	u2 := mkUsage(t, "usage_mixed_1", mixed.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u2)
	o2 := mkOutcome(t, "outcome_mixed_help", u2.UsageID, mixed.MemoryID, 1, "helped", false)
	put(t, s, o2)
	u3 := mkUsage(t, "usage_mixed_2", mixed.MemoryID, 1, "2026-08-11T11:00:00Z")
	put(t, s, u3)
	o3 := mkOutcome(t, "outcome_mixed_harm", u3.UsageID, mixed.MemoryID, 1, "harmed", false)
	put(t, s, o3)

	// Third-party failure is never auto-attributed.
	external := mkStrategy("mem_strategy_external", "external", 1)
	putRevEvidence(t, s, external)
	u4 := mkUsage(t, "usage_ext_1", external.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u4)
	o4 := mkOutcome(t, "outcome_ext_harm", u4.UsageID, external.MemoryID, 1, "harmed", true)
	put(t, s, o4)

	res := derive(t, s, deriveReq())

	if got := stateByID(t, res, noOutcome.MemoryID).Lifecycle; got != LifecycleProbation {
		t.Errorf("no-outcome lifecycle = %s, want probation", got)
	}
	if got := stateByID(t, res, helped.MemoryID).Lifecycle; got != LifecycleActive {
		t.Errorf("helped lifecycle = %s, want active", got)
	}
	if got := stateByID(t, res, mixed.MemoryID).Lifecycle; got != LifecycleDegraded {
		t.Errorf("mixed lifecycle = %s, want degraded (harm never cancels)", got)
	}
	ext := stateByID(t, res, external.MemoryID)
	if ext.Lifecycle != LifecycleProbation {
		t.Errorf("external-failure lifecycle = %s, want probation (not auto-attributed)", ext.Lifecycle)
	}
	if ext.Usage.CountedHarmCount != 0 {
		t.Errorf("external failure must not count harm, got %d", ext.Usage.CountedHarmCount)
	}
}

// ---- lifecycle: evidence_validated ----

func TestDeriveLifecycleEvidenceValidated(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	withEv := validRevision()
	withEv.MemoryID = "mem_pattern_with_ev"
	withEv.MemoryType = MemoryTypePattern
	withEv.UsagePolicy = UsagePolicyEvidenceValidated
	withEv.CanonicalKey = "pattern-with-ev"
	withEv.Title = "Pattern With Evidence"
	withEv.Summary = "Pattern backed by an evidence generation."
	withEv = fillRevisionHash(withEv)
	putRevEvidence(t, s, withEv)

	noEv := validRevision()
	noEv.MemoryID = "mem_pattern_no_ev"
	noEv.MemoryType = MemoryTypePattern
	noEv.UsagePolicy = UsagePolicyEvidenceValidated
	noEv.CanonicalKey = "pattern-no-ev"
	noEv.Title = "Pattern Without Evidence"
	noEv.Summary = "Pattern without an evidence generation."
	noEv = fillRevisionHash(noEv)
	ev := validEvidenceGeneration()
	ev.MemoryID = noEv.MemoryID
	ev.Revision = noEv.Revision
	ev.EvidenceRefs = nil
	ev = fillEvidenceHash(ev)
	putRevisionEvidence(t, s, noEv, ev)

	res := derive(t, s, deriveReq())
	if got := stateByID(t, res, withEv.MemoryID).Lifecycle; got != LifecycleProbation {
		t.Errorf("evidence-backed lifecycle = %s, want probation (Critic protocol gap)", got)
	}
	if got := stateByID(t, res, noEv.MemoryID).Lifecycle; got != LifecycleProbation {
		t.Errorf("empty-evidence lifecycle = %s, want probation", got)
	}
}

// ---- lifecycle: explicit_confirmation ----

func TestDeriveLifecycleExplicitConfirmation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	confirmed := validRevision()
	confirmed.MemoryID = "mem_decision_ok"
	confirmed.MemoryType = MemoryTypeDecision
	confirmed.UsagePolicy = UsagePolicyExplicitConfirmation
	confirmed.Revision = 1
	confirmed.CanonicalKey = "decide-ok"
	confirmed.Title = "Confirmed Decision"
	confirmed.Summary = "Decision confirmed by a user."
	jud := validConfirmationJudgment()
	jud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: confirmed.MemoryID, Revision: 1, ContentSHA256: confirmed.ContentSHA256}
	jud = fillJudgmentHash(jud)
	put(t, s, jud)
	confirmed.ConfirmationSourceRef = &ConfirmationSourceRef{
		Scope: jud.Scope, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: jud.JudgmentID, ContentSHA256: jud.ContentSHA256,
	}
	confirmed = fillRevisionHash(confirmed)
	put(t, s, confirmed)

	// No confirmation: the ref points at a judgment that was never written,
	// so the confirmation is unverifiable -> degraded (architecture 11.5).
	unconfirmed := validRevision()
	unconfirmed.MemoryID = "mem_decision_no"
	unconfirmed.MemoryType = MemoryTypeDecision
	unconfirmed.UsagePolicy = UsagePolicyExplicitConfirmation
	unconfirmed.CanonicalKey = "decide-no"
	unconfirmed.Title = "Unconfirmed Decision"
	unconfirmed.Summary = "Decision without confirmation."
	unconfirmed.ConfirmationSourceRef = &ConfirmationSourceRef{
		Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: "judgment_never_written", ContentSHA256: testHash,
	}
	unconfirmed = fillRevisionHash(unconfirmed)
	put(t, s, unconfirmed)

	// Revoked confirmation with no replacement revision: frozen (never a
	// simplified degraded). The revoked judgment must supersede a
	// prior confirmation per the frozen judgment schema.
	revokedJud := validConfirmationJudgment()
	revokedJud.JudgmentID = "judgment_revoked"
	revokedJud.Subject.MemoryRef = &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: "mem_decision_revoked", Revision: 1, ContentSHA256: testHash}
	revokedJud.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: ScopeProject}
	revokedJud.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_original", ContentSHA256: testHash}
	revokedJud = fillJudgmentHash(revokedJud)
	put(t, s, revokedJud)
	revoked := validRevision()
	revoked.MemoryID = "mem_decision_revoked"
	revoked.MemoryType = MemoryTypeDecision
	revoked.UsagePolicy = UsagePolicyExplicitConfirmation
	revoked.Revision = 1
	revoked.CanonicalKey = "decide-revoked"
	revoked.Title = "Revoked Decision"
	revoked.Summary = "Decision whose confirmation was revoked."
	revoked.ConfirmationSourceRef = &ConfirmationSourceRef{
		Scope: revokedJud.Scope, JudgmentType: JudgmentTypeConfirmation,
		JudgmentID: revokedJud.JudgmentID, ContentSHA256: revokedJud.ContentSHA256,
	}
	revoked = fillRevisionHash(revoked)
	put(t, s, revoked)

	res := derive(t, s, deriveReq())
	if got := stateByID(t, res, confirmed.MemoryID).Lifecycle; got != LifecycleActive {
		t.Errorf("confirmed lifecycle = %s, want active", got)
	}
	if got := stateByID(t, res, unconfirmed.MemoryID).Lifecycle; got != LifecycleDegraded {
		t.Errorf("unconfirmed lifecycle = %s, want degraded (unverifiable ref)", got)
	}
	if got := stateByID(t, res, revoked.MemoryID).Lifecycle; got != LifecycleFrozen {
		t.Errorf("revoked lifecycle = %s, want frozen (not degraded)", got)
	}
}

// ---- governance: pin/unpin/freeze/unfreeze/archive ----

func TestDeriveGovernance(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	frozen := mkStrategy("mem_strategy_frozen", "frozen", 1)
	putRevEvidence(t, s, frozen)
	put(t, s, governanceEvent("gov_freeze", frozen, "manual_freeze", "2026-08-10T00:00:00Z"))

	unfrozen := mkStrategy("mem_strategy_unfrozen", "unfrozen", 1)
	putRevEvidence(t, s, unfrozen)
	put(t, s, governanceEvent("gov_freeze2", unfrozen, "manual_freeze", "2026-08-10T00:00:00Z"))
	put(t, s, governanceEvent("gov_unfreeze", unfrozen, "unfreeze", "2026-08-11T00:00:00Z"))

	pinned := mkStrategy("mem_strategy_pinned", "pinned", 1)
	putRevEvidence(t, s, pinned)
	put(t, s, governanceEvent("gov_pin", pinned, "pin", "2026-08-10T00:00:00Z"))

	unpinned := mkStrategy("mem_strategy_unpinned", "unpinned", 1)
	putRevEvidence(t, s, unpinned)
	put(t, s, governanceEvent("gov_pin2", unpinned, "pin", "2026-08-10T00:00:00Z"))
	put(t, s, governanceEvent("gov_unpin", unpinned, "unpin", "2026-08-11T00:00:00Z"))

	archived := mkStrategy("mem_strategy_archived", "archived", 1)
	putRevEvidence(t, s, archived)
	put(t, s, governanceEvent("gov_archive", archived, "archive", "2026-08-10T00:00:00Z"))
	// Late unfreeze/pin on an archived memory must not revive it (terminal).
	put(t, s, governanceEvent("gov_late_pin", archived, "pin", "2026-08-12T00:00:00Z"))
	put(t, s, governanceEvent("gov_late_unfreeze", archived, "unfreeze", "2026-08-12T01:00:00Z"))

	res := derive(t, s, deriveReq())

	fs := stateByID(t, res, frozen.MemoryID)
	if fs.Lifecycle != LifecycleFrozen || !fs.Frozen {
		t.Errorf("frozen memory = %s frozen=%v, want frozen/true", fs.Lifecycle, fs.Frozen)
	}
	us := stateByID(t, res, unfrozen.MemoryID)
	if us.Frozen || us.Lifecycle == LifecycleFrozen {
		t.Errorf("unfrozen memory must not stay frozen, got %s frozen=%v", us.Lifecycle, us.Frozen)
	}
	// With no outcomes the strategy is naturally probation after the freeze
	// is lifted — never frozen.
	if us.Lifecycle != LifecycleProbation {
		t.Errorf("unfrozen memory lifecycle = %s, want probation (no outcomes yet)", us.Lifecycle)
	}
	if got := stateByID(t, res, pinned.MemoryID).Pinned; !got {
		t.Error("pinned memory must be pinned")
	}
	if got := stateByID(t, res, unpinned.MemoryID).Pinned; got {
		t.Error("unpinned memory must not be pinned")
	}
	as := stateByID(t, res, archived.MemoryID)
	if as.Lifecycle != LifecycleArchived || !as.Archived {
		t.Errorf("archived memory = %s archived=%v, want archived/true", as.Lifecycle, as.Archived)
	}
	if as.Pinned {
		t.Error("archived memory must stay unpinned (late pin ignored)")
	}
}

func governanceEvent(id string, rev MemoryRevision, op, at string) GovernanceEvent {
	g := GovernanceEvent{
		SchemaVersion: 1,
		EventID:       id,
		Scope:         rev.Scope,
		MemoryID:      rev.MemoryID,
		Revision:      rev.Revision,
		Operation:     op,
		Reason:        "test governance",
		Source:        "local_user",
		BasisRefs:     []BasisRef{},
		CreatedAt:     at,
	}
	if op == "unfreeze" {
		// The frozen schema requires unfreeze to carry memory/evidence/
		// judgment basis refs (never policy refs).
		g.BasisRefs = []BasisRef{{MemoryRef: &MemoryRef{
			Scope: rev.Scope, MemoryType: rev.MemoryType,
			MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
		}}}
	}
	return g
}

// ---- superseded ----

func TestDeriveSuperseded(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// Same memory id, two revisions; the current one is active, the older is
	// superseded when derived explicitly.
	base := mkStrategy("mem_strategy_chain", "chain", 1)
	putRevEvidence(t, s, base)
	cur := mkStrategy("mem_strategy_chain", "chain", 2)
	cur.Title = "Chain v2"
	cur = fillRevisionHash(cur)
	putRevEvidence(t, s, cur)

	// Latest revision is the current state.
	res := derive(t, s, deriveReq())
	latest := stateByID(t, res, base.MemoryID)
	if latest.Revision != 2 {
		t.Fatalf("current revision = %d, want 2", latest.Revision)
	}
	if latest.Lifecycle == LifecycleSuperseded {
		t.Error("current revision must not be superseded")
	}

	// Explicit older revision derives as superseded.
	req := deriveReq()
	req.Revision = 1
	res = derive(t, s, req)
	old := stateByID(t, res, base.MemoryID)
	if old.Revision != 1 {
		t.Fatalf("requested revision = %d, want 1", old.Revision)
	}
	if old.Lifecycle != LifecycleSuperseded {
		t.Errorf("older revision lifecycle = %s, want superseded", old.Lifecycle)
	}
}

// ---- freshness ----

func TestDeriveFreshness(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// Judgment-driven freshness: the latest freshness_evaluation judgment
	// wins over the time window.
	judged := mkStrategy("mem_strategy_judged", "judged", 1)
	putRevEvidence(t, s, judged)
	freshEval := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_fresh_1",
		JudgmentType:  JudgmentTypeFreshnessEvaluation,
		Scope:         ScopeProject,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef:   &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: judged.MemoryID, Revision: 1, ContentSHA256: judged.ContentSHA256},
		},
		Source: JudgmentSource{SourceType: "user", SourceID: "local_user"},
		FreshnessEvaluation: &FreshnessEvaluationPayload{
			MemoryRef:   MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: judged.MemoryID, Revision: 1, ContentSHA256: judged.ContentSHA256},
			Result:      "needs_revalidation",
			EvaluatedAt: "2026-08-14T00:00:00Z",
			FreshnessPolicyRef: PolicyRef{
				PolicyType: PolicyTypeFreshness,
				PolicyID:   "freshness_policy_v1", ContentSHA256: testHash,
			},
			BasisRefs: []BasisRef{},
		},
		BasisRefs: []BasisRef{},
		CreatedAt: "2026-08-14T00:00:00Z",
	}
	freshEval = fillJudgmentHash(freshEval)
	put(t, s, freshEval)

	// Window-driven freshness: fresh / aging / needs_revalidation by age of
	// the last usage (default policy: 90 / 180 / 365 days). Evidence
	// timestamps are pinned before the usages so the usage is the latest
	// activity.
	freshMem := mkStrategy("mem_strategy_fresh_win", "fresh-window", 1)
	putRevEvidenceAt(t, s, freshMem, "2026-08-06T00:00:00Z")
	u := mkUsage(t, "usage_fresh_win", freshMem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)

	agingMem := mkStrategy("mem_strategy_aging_win", "aging-window", 1)
	putRevEvidenceAt(t, s, agingMem, "2026-01-01T00:00:00Z")
	u2 := mkUsage(t, "usage_aging_win", agingMem.MemoryID, 1, "2026-03-01T10:00:00Z")
	put(t, s, u2)

	staleMem := mkStrategy("mem_strategy_stale_win", "stale-window", 1)
	putRevEvidenceAt(t, s, staleMem, "2024-12-01T00:00:00Z")
	u3 := mkUsage(t, "usage_stale_win", staleMem.MemoryID, 1, "2025-01-10T10:00:00Z")
	put(t, s, u3)

	res := derive(t, s, deriveReq())
	if got := stateByID(t, res, judged.MemoryID).Freshness; got != FreshnessNeedsRevalidation {
		t.Errorf("judged freshness = %s, want needs_revalidation", got)
	}
	if got := stateByID(t, res, freshMem.MemoryID).Freshness; got != FreshnessFresh {
		t.Errorf("recent usage freshness = %s, want fresh", got)
	}
	if got := stateByID(t, res, agingMem.MemoryID).Freshness; got != FreshnessAging {
		t.Errorf("aging usage freshness = %s, want aging", got)
	}
	if got := stateByID(t, res, staleMem.MemoryID).Freshness; got != FreshnessNeedsRevalidation {
		t.Errorf("stale usage freshness = %s, want needs_revalidation", got)
	}
	// Freshness must never produce frozen/archived/superseded.
	for _, id := range []string{judged.MemoryID, freshMem.MemoryID, agingMem.MemoryID, staleMem.MemoryID} {
		st := stateByID(t, res, id)
		if st.Lifecycle == LifecycleFrozen || st.Lifecycle == LifecycleArchived || st.Lifecycle == LifecycleSuperseded {
			t.Errorf("freshness must not set lifecycle %s on %s", st.Lifecycle, id)
		}
	}
}

// ---- usage stats ----

func TestDeriveUsageStats(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_stats", "stats", 1)
	putRevEvidence(t, s, mem)

	u1 := mkUsage(t, "usage_stats_1", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u1)
	put(t, s, mkOutcome(t, "outcome_stats_1", u1.UsageID, mem.MemoryID, 1, "helped", false))
	u2 := mkUsage(t, "usage_stats_2", mem.MemoryID, 1, "2026-08-12T10:00:00Z")
	put(t, s, u2)
	put(t, s, mkOutcome(t, "outcome_stats_2", u2.UsageID, mem.MemoryID, 1, "helped", false))
	u3 := mkUsage(t, "usage_stats_3", mem.MemoryID, 1, "2026-08-13T10:00:00Z")
	put(t, s, u3)
	put(t, s, mkOutcome(t, "outcome_stats_3", u3.UsageID, mem.MemoryID, 1, "harmed", false))
	// External failure outcome: counts as a usage but never as help/harm.
	u4 := mkUsage(t, "usage_stats_4", mem.MemoryID, 1, "2026-08-14T10:00:00Z")
	put(t, s, u4)
	put(t, s, mkOutcome(t, "outcome_stats_4", u4.UsageID, mem.MemoryID, 1, "harmed", true))

	// Repeat the same usage (same id): idempotent, never double counted.
	put(t, s, u1)

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	us := st.Usage
	if us.UsageCount != 4 {
		t.Errorf("usage_count = %d, want 4 (repeat is idempotent)", us.UsageCount)
	}
	if us.CountedHelpCount != 2 {
		t.Errorf("counted_help_count = %d, want 2", us.CountedHelpCount)
	}
	if us.CountedHarmCount != 1 {
		t.Errorf("counted_harm_count = %d, want 1 (external failure excluded)", us.CountedHarmCount)
	}
	if us.LastUsedAt != "2026-08-14T10:00:00Z" {
		t.Errorf("last_used_at = %q, want the latest usage", us.LastUsedAt)
	}
}

func TestDeriveUsageOverrideAndInsufficient(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_override", "override", 1)
	putRevEvidence(t, s, mem)
	u := mkUsage(t, "usage_override_1", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	o := mkOutcome(t, "outcome_override_1", u.UsageID, mem.MemoryID, 1, "harmed", false)
	put(t, s, o)
	// Attribution override corrects harmed -> neutral.
	ov := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_override",
		JudgmentType:  JudgmentTypeAttributionOverride,
		Scope:         ScopeProject,
		Subject:       JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: o.OutcomeID},
		Source:        JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride: &AttributionOverridePayload{
			PreviousEffect: "harmed",
			NewEffect:      "neutral",
			Reason:         "third-party outage, corrected",
		},
		BasisRefs: []BasisRef{},
		CreatedAt: "2026-08-12T00:00:00Z",
	}
	ov = fillJudgmentHash(ov)
	put(t, s, ov)

	// Usage without any outcome: insufficient evidence marker.
	noOutcome := mkStrategy("mem_strategy_no_outcome", "no-outcome-stats", 1)
	putRevEvidence(t, s, noOutcome)
	u2 := mkUsage(t, "usage_no_outcome_1", noOutcome.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u2)

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.CountedHarmCount != 0 {
		t.Errorf("override must cancel harm, got %d", st.Usage.CountedHarmCount)
	}
	if st.Usage.InsufficientEvidence {
		t.Error("corrected outcome must not be insufficient")
	}
	no := stateByID(t, res, noOutcome.MemoryID)
	if !no.Usage.InsufficientEvidence {
		t.Error("usage without outcome must be flagged insufficient_evidence")
	}
	if no.Lifecycle != LifecycleProbation {
		t.Errorf("usage-without-outcome lifecycle = %s, want probation", no.Lifecycle)
	}
}

// ---- stable sort order ----

func TestDeriveSortOrderAndDeterminism(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// active (3 helped outcomes across 2 episodes) beats probation.
	hot := mkStrategy("mem_strategy_hot", "hot", 1)
	putRevEvidence(t, s, hot)
	addOutcome(t, s, hot, "usage_hot_1", "episode_a", "2026-08-13T10:00:00Z", "helped", false)
	addOutcome(t, s, hot, "usage_hot_2", "episode_a", "2026-08-14T10:00:00Z", "helped", false)
	addOutcome(t, s, hot, "usage_hot_3", "episode_b", "2026-08-15T10:00:00Z", "helped", false)

	// probation (no outcomes).
	cold := mkStrategy("mem_strategy_cold", "cold", 1)
	putRevEvidence(t, s, cold)

	// pinned probation outranks unpinned active? No: pinned lifts within
	// its evidence class; pinned cold stays behind unpinned active.
	pinnedCold := mkStrategy("mem_strategy_pinned_cold", "pinned-cold", 1)
	putRevEvidence(t, s, pinnedCold)
	put(t, s, governanceEvent("gov_pc", pinnedCold, "pin", "2026-08-10T00:00:00Z"))

	res := derive(t, s, deriveReq())
	// active first, then pinned probation, then probation.
	var order []string
	for _, st := range res.States {
		order = append(order, st.MemoryID)
	}
	if len(order) != 3 {
		t.Fatalf("want 3 states, got %v", order)
	}
	if order[0] != hot.MemoryID {
		t.Errorf("active must sort before probation, got %v", order)
	}
	if order[1] != pinnedCold.MemoryID || order[2] != cold.MemoryID {
		t.Errorf("pinned must lift within its evidence class, got %v", order)
	}

	// Byte-stable across runs.
	b1, _ := json.Marshal(res)
	res2 := derive(t, s, deriveReq())
	b2, _ := json.Marshal(res2)
	if string(b1) != string(b2) {
		t.Error("derived state must be byte-stable for identical inputs")
	}
}

// ---- determinism and rebuild ----

func TestDeriveDeterministicRebuild(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_rebuild", "rebuild", 1)
	putRevEvidence(t, s, mem)
	u := mkUsage(t, "usage_rebuild_1", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_rebuild_1", u.UsageID, mem.MemoryID, 1, "helped", false))

	r1 := derive(t, s, deriveReq())
	b1, _ := json.Marshal(r1)
	// Rebuilding from the same facts (a fresh store with identical facts)
	// must produce identical bytes — derived state is fully reconstructible.
	root2 := tempRoot(t)
	s2 := openProject(t, root2, Options{})
	putRevEvidence(t, s2, mem)
	put(t, s2, u)
	put(t, s2, mkOutcome(t, "outcome_rebuild_1", u.UsageID, mem.MemoryID, 1, "helped", false))
	r2 := derive(t, s2, deriveReq())
	b2, _ := json.Marshal(r2)
	if string(b1) != string(b2) {
		t.Error("derived state must rebuild identically from the same facts")
	}
}

// ---- indexes ----

func TestDeriveIndexes(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	active := mkStrategy("mem_strategy_idx_active", "idx-active", 1)
	putRevEvidence(t, s, active)
	addOutcome(t, s, active, "usage_idx_a1", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	addOutcome(t, s, active, "usage_idx_a2", "episode_a", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, active, "usage_idx_a3", "episode_b", "2026-08-13T10:00:00Z", "helped", false)

	frozen := mkStrategy("mem_strategy_idx_frozen", "idx-frozen", 1)
	putRevEvidence(t, s, frozen)
	put(t, s, governanceEvent("gov_idx_f", frozen, "manual_freeze", "2026-08-10T00:00:00Z"))

	archived := mkStrategy("mem_strategy_idx_arch", "idx-archived", 1)
	putRevEvidence(t, s, archived)
	put(t, s, governanceEvent("gov_idx_a", archived, "archive", "2026-08-10T00:00:00Z"))

	res := derive(t, s, deriveReq())

	// Root index: frozen/archived excluded from normal entries, counted
	// separately; page paths are deterministic wiki links.
	if len(res.RootIndex.Entries) != 1 {
		t.Fatalf("root index must contain only non-frozen/archived entries, got %d", len(res.RootIndex.Entries))
	}
	e := res.RootIndex.Entries[0]
	if e.MemoryID != active.MemoryID || e.CanonicalKey != "idx-active" || e.Lifecycle != LifecycleActive {
		t.Errorf("root entry mismatch: %+v", e)
	}
	if e.PagePath != "wiki/strategies/idx-active.md" {
		t.Errorf("page path = %q, want wiki/strategies/idx-active.md", e.PagePath)
	}
	if res.RootIndex.FrozenCount != 1 || res.RootIndex.ArchivedCount != 1 {
		t.Errorf("frozen/archived counts = %d/%d, want 1/1", res.RootIndex.FrozenCount, res.RootIndex.ArchivedCount)
	}

	// Local index: deterministic sharding by the index policy split order.
	if len(res.LocalIndex.Shards) == 0 {
		t.Error("local index must have shards")
	}
	total := 0
	for _, entries := range res.LocalIndex.Shards {
		total += len(entries)
	}
	if total != 1 {
		t.Errorf("local index must list the active entry once, got %d across shards", total)
	}

	// Project store: global index stays empty.
	if res.GlobalIndex.Scope != "" || len(res.GlobalIndex.Entries) != 0 {
		t.Errorf("project store must not produce a global index, got %+v", res.GlobalIndex)
	}
}

func TestDeriveEmptyIndex(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	res := derive(t, s, deriveReq())
	if len(res.States) != 0 {
		t.Errorf("empty store must have no states, got %d", len(res.States))
	}
	if len(res.RootIndex.Entries) != 0 || len(res.LocalIndex.Shards) != 0 {
		t.Errorf("empty store must have empty indexes: %+v", res.RootIndex)
	}
}

func TestDerivePinnedLiftsInIndex(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	a := mkStrategy("mem_strategy_ia", "idx-a", 1)
	putRevEvidence(t, s, a)
	addOutcome(t, s, a, "usage_ia_1", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	addOutcome(t, s, a, "usage_ia_2", "episode_a", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, a, "usage_ia_3", "episode_b", "2026-08-13T10:00:00Z", "helped", false)
	b := mkStrategy("mem_strategy_ib", "idx-b", 1)
	putRevEvidence(t, s, b)
	addOutcome(t, s, b, "usage_ib_1", "episode_c", "2026-08-11T10:00:00Z", "helped", false)
	addOutcome(t, s, b, "usage_ib_2", "episode_c", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, b, "usage_ib_3", "episode_d", "2026-08-12T12:00:00Z", "helped", false)
	// Both active; pin the one whose last help is older: it must lift above
	// the newer one within the same lifecycle class.
	put(t, s, governanceEvent("gov_ib", b, "pin", "2026-08-10T00:00:00Z"))

	res := derive(t, s, deriveReq())
	if len(res.RootIndex.Entries) != 2 {
		t.Fatalf("want 2 index entries, got %d", len(res.RootIndex.Entries))
	}
	if res.RootIndex.Entries[0].MemoryID != b.MemoryID {
		t.Errorf("pinned entry must lift to the top, got %+v", res.RootIndex.Entries[0])
	}
}

// ---- scope isolation ----

func TestDeriveScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	sp := openProject(t, root, Options{})
	rev := mkStrategy("mem_strategy_proj", "proj", 1)
	putRevEvidence(t, sp, rev)
	u := mkUsage(t, "usage_proj", rev.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, sp, u)
	put(t, sp, mkOutcome(t, "outcome_proj", u.UsageID, rev.MemoryID, 1, "helped", false))

	gres := derive(t, sp, deriveReq())
	if len(gres.States) != 1 || gres.States[0].Scope != ScopeProject {
		t.Fatalf("project store must derive only project states, got %+v", gres.States)
	}

	// Global store derives only global facts; a project-scoped request is
	// rejected (the store is scope-bound, never silently cross-reads).
	rootG := tempRoot(t)
	sg := openProject(t, rootG, Options{})
	_ = sg
	gs, err := OpenGlobal(rootG, Options{})
	if err != nil {
		t.Fatal(err)
	}
	grev := mkStrategy("mem_strategy_global", "global", 1)
	grev.Scope = ScopeGlobal
	grev = fillRevisionHash(grev)
	putRevEvidence(t, gs, grev)
	greq := deriveReq()
	greq.Scope = ScopeGlobal
	gresG := derive(t, gs, greq)
	if len(gresG.States) != 1 || gresG.States[0].Scope != ScopeGlobal {
		t.Fatalf("global store must derive only global states, got %+v", gresG.States)
	}
	if gresG.GlobalIndex.Scope != "global" || len(gresG.GlobalIndex.Entries) != 1 {
		t.Errorf("global store must fill the global index, got %+v", gresG.GlobalIndex)
	}
}

// ---- orphan references fail closed ----

func TestDeriveOrphanReferencesFailClosed(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, s *FactStore)
	}{
		{"usage without revision", func(t *testing.T, s *FactStore) {
			put(t, s, mkUsage(t, "usage_orphan", "mem_strategy_missing", 1, "2026-08-11T10:00:00Z"))
		}},
		{"outcome without usage", func(t *testing.T, s *FactStore) {
			rev := mkStrategy("mem_strategy_oo", "oo", 1)
			putRevEvidence(t, s, rev)
			put(t, s, mkOutcome(t, "outcome_orphan", "usage_missing", rev.MemoryID, 1, "helped", false))
		}},
		{"outcome without revision", func(t *testing.T, s *FactStore) {
			put(t, s, mkUsage(t, "usage_orphan2", "mem_strategy_missing", 1, "2026-08-11T10:00:00Z"))
			put(t, s, mkOutcome(t, "outcome_orphan2", "usage_orphan2", "mem_strategy_missing", 1, "helped", false))
		}},
		{"override without outcome", func(t *testing.T, s *FactStore) {
			rev := mkStrategy("mem_strategy_ov", "ov", 1)
			putRevEvidence(t, s, rev)
			ov := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_orphan", JudgmentType: JudgmentTypeAttributionOverride,
				Scope:               ScopeProject,
				Subject:             JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_missing"},
				Source:              JudgmentSource{SourceType: "user", SourceID: "local_user"},
				AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "corrected"},
				BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-12T00:00:00Z",
			}
			ov = fillJudgmentHash(ov)
			put(t, s, ov)
		}},
		{"governance without revision", func(t *testing.T, s *FactStore) {
			put(t, s, GovernanceEvent{
				SchemaVersion: 1, EventID: "gov_orphan", Scope: ScopeProject,
				MemoryID: "mem_strategy_missing", Revision: 1, Operation: "archive",
				Reason: "test", Source: "local_user", BasisRefs: []BasisRef{},
				CreatedAt: "2026-08-10T00:00:00Z",
			})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tempRoot(t)
			s := openProject(t, root, Options{})
			c.seed(t, s)
			if _, err := DeriveState(context.Background(), s, deriveReq()); err == nil {
				t.Error("orphan reference must fail closed")
			} else if !IsSensitiveError(err) {
				t.Errorf("error must be a redacted StoreError, got %T %v", err, err)
			}
		})
	}
}

// ---- corrupt fact fails closed ----

func TestDeriveCorruptFactFailClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := mkStrategy("mem_strategy_corrupt", "corrupt", 1)
	putRevEvidence(t, s, rev)
	// Corrupt the stored revision bytes (hash drift / garbage).
	path := filepath.Join(root, "facts", "memory-revisions", rev.MemoryID, "1.json")
	if err := os.WriteFile(path, []byte(`{"garbage": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DeriveState(context.Background(), s, deriveReq()); err == nil {
		t.Error("corrupt fact must fail closed")
	}
}

// ---- concurrent reads ----

func TestDeriveConcurrentReads(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_conc", "conc", 1)
	putRevEvidence(t, s, mem)
	u := mkUsage(t, "usage_conc", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_conc", u.UsageID, mem.MemoryID, 1, "helped", false))

	const n = 8
	results := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			res, err := DeriveState(context.Background(), s, deriveReq())
			if err != nil {
				errs <- err
				return
			}
			b, _ := json.Marshal(res)
			results <- string(b)
		}()
	}
	var want string
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			t.Fatalf("concurrent derive failed: %v", err)
		case b := <-results:
			if want == "" {
				want = b
			} else if b != want {
				t.Fatal("concurrent derives must agree")
			}
		}
	}
}

// ---- derived snapshot adapter (read-only OKF input) ----

func TestDeriveSnapshotAdapter(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_snap", "snap", 1)
	putRevEvidence(t, s, mem)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	b1 := st.SnapshotBytes()

	// Deterministic.
	b2 := stateByID(t, res, mem.MemoryID).SnapshotBytes()
	if string(b1) != string(b2) {
		t.Error("snapshot must be deterministic")
	}
	if len(b1) == 0 {
		t.Fatal("snapshot must not be empty")
	}

	// A fact change must change the snapshot (deriveable explanation).
	u := mkUsage(t, "usage_snap", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_snap", u.UsageID, mem.MemoryID, 1, "helped", false))
	res2 := derive(t, s, deriveReq())
	b3 := stateByID(t, res2, mem.MemoryID).SnapshotBytes()
	if string(b1) == string(b3) {
		t.Error("fact changes must change the derived snapshot")
	}
}

// ---- multi-process isolation ----

func TestDeriveMultiProcessIsolation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_mp", "mp", 1)
	putRevEvidence(t, s, mem)
	u := mkUsage(t, "usage_mp", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_mp", u.UsageID, mem.MemoryID, 1, "helped", false))

	// Parent-process derivation.
	res := derive(t, s, deriveReq())
	want, _ := json.Marshal(res)

	// Child-process derivation on the same store must agree byte for byte.
	out := runChildDerive(t, root)
	if out != string(want) {
		t.Error("multi-process derivation must produce identical bytes")
	}
}

func runChildDerive(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestDeriveMultiProcessIsolation")
	cmd.Env = append(os.Environ(), "MEM_DERIVE_HELPER=1", "MEM_DERIVE_ROOT="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child derive failed: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.HasPrefix(s, "status=ok\n") {
		t.Fatalf("child derive error: %s", s)
	}
	return strings.TrimSuffix(strings.TrimPrefix(s, "status=ok\n"), "\n")
}

// ---- attribution override supersede chain ----

func TestDeriveUsageOverrideSupersedeChain(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_chain_ov", "chain-ov", 1)
	putRevEvidence(t, s, mem)
	u := mkUsage(t, "usage_chain_ov", mem.MemoryID, 1, "2026-08-11T10:00:00Z")
	put(t, s, u)
	o := mkOutcome(t, "outcome_chain_ov", u.UsageID, mem.MemoryID, 1, "harmed", false)
	put(t, s, o)

	// First override: harmed -> neutral.
	first := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_ov_first", JudgmentType: JudgmentTypeAttributionOverride,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: o.OutcomeID},
		Source:              JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "unclear"},
		BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-12T00:00:00Z",
	}
	first = fillJudgmentHash(first)
	put(t, s, first)
	// Second override supersedes the first and lands on helped: only the
	// newest non-superseded override may count.
	second := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_ov_second", JudgmentType: JudgmentTypeAttributionOverride,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: o.OutcomeID},
		Source:                JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride:   &AttributionOverridePayload{PreviousEffect: "neutral", NewEffect: "helped", Reason: "re-evaluated"},
		SupersedesJudgmentRef: &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: first.JudgmentID, ContentSHA256: first.ContentSHA256},
		BasisRefs:             []BasisRef{}, CreatedAt: "2026-08-13T00:00:00Z",
	}
	second = fillJudgmentHash(second)
	put(t, s, second)

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.CountedHelpCount != 1 || st.Usage.CountedHarmCount != 0 {
		t.Errorf("supersede chain must apply the newest override, got help=%d harm=%d", st.Usage.CountedHelpCount, st.Usage.CountedHarmCount)
	}
	// One counted help is below the active threshold, but the override chain
	// itself is what is under test here.
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("lifecycle = %s, want probation (1 help below threshold)", st.Lifecycle)
	}
}

// ---- revision-level orphan reference ----

func TestDeriveOrphanRevisionFailClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_strategy_orphan_rev", "orphan-rev", 1)
	putRevEvidence(t, s, mem)
	// The memory exists but revision 7 was never written.
	u := mkUsage(t, "usage_orphan_rev", mem.MemoryID, 7, "2026-08-11T10:00:00Z")
	put(t, s, u)
	if _, err := DeriveState(context.Background(), s, deriveReq()); err == nil {
		t.Error("usage referencing a nonexistent revision must fail closed")
	} else if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Errorf("want derived_invalid_input, got %v", err)
	}
}
