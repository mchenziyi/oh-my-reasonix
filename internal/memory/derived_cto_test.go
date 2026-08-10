package memory

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
)

// ---- CTO review regression: outcome_attributed lifecycle thresholds ----
//
// Frozen semantics per the CTO review:
//   - initial: probation + healthy
//   - active: >= 3 counted_as_help, >= 2 independent episodes, no unresolved
//     counted_as_harm
//   - degraded: first attributed harm
//   - frozen: >= 3 counted_as_harm AND negative rate >= 60%
//   - external_failure counts as usage only, never help/harm, never
//     degraded/frozen
//   - success never cancels failure; insufficient evidence stays probation

// addOutcome writes one usage + outcome pair for the memory.
func addOutcome(t *testing.T, s *FactStore, rev MemoryRevision, usageID, episodeID string, occurredAt, effect string, external bool) {
	t.Helper()
	u := mkUsageFull(t, usageID, rev.MemoryID, rev.Revision, occurredAt, "affected", episodeID)
	put(t, s, u)
	o := mkOutcome(t, "outcome_"+usageID, u.UsageID, rev.MemoryID, rev.Revision, effect, external)
	put(t, s, o)
}

func TestCTOThresholdsZeroHelp(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_zero", "cto-zero", 1)
	putRevEvidence(t, s, mem)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleProbation || st.Health != HealthHealthy {
		t.Errorf("zero help = %s/%s, want probation/healthy", st.Lifecycle, st.Health)
	}
}

func TestCTOThresholdsOneHelpNotActive(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_one", "cto-one", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_one", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("one help must not be active, got %s", st.Lifecycle)
	}
	if st.Health != HealthHealthy {
		t.Errorf("one help (no negative evidence) = %s, want healthy", st.Health)
	}
}

func TestCTOThresholdsTwoHelpOneEpisodeNotActive(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_two", "cto-two", 1)
	putRevEvidence(t, s, mem)
	// Two helps but both from the same episode: episode independence fails.
	addOutcome(t, s, mem, "usage_cto_two_a", "episode_same", "2026-08-11T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_two_b", "episode_same", "2026-08-12T10:00:00Z", "helped", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("two helps from one episode must not be active, got %s", st.Lifecycle)
	}
}

func TestCTOThresholdsActiveHealthy(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_active", "cto-active", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_active_1", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_active_2", "episode_a", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_active_3", "episode_b", "2026-08-13T10:00:00Z", "helped", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleActive || st.Health != HealthHealthy {
		t.Errorf("3 helps / 2 episodes = %s/%s, want active/healthy", st.Lifecycle, st.Health)
	}
}

func TestCTOThresholdsOneHarmDegraded(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_harm", "cto-harm", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_harm_1", "episode_a", "2026-08-11T10:00:00Z", "harmed", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleDegraded || st.Health != HealthDegraded {
		t.Errorf("one harm = %s/%s, want degraded/degraded", st.Lifecycle, st.Health)
	}
}

func TestCTOThresholdsAutoFrozen(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_frozen", "cto-frozen", 1)
	putRevEvidence(t, s, mem)
	// 3 harms + 2 helps => negative rate 3/5 = 0.6 >= 0.6 => auto frozen.
	addOutcome(t, s, mem, "usage_cto_f_h1", "episode_a", "2026-08-11T10:00:00Z", "harmed", false)
	addOutcome(t, s, mem, "usage_cto_f_h2", "episode_b", "2026-08-12T10:00:00Z", "harmed", false)
	addOutcome(t, s, mem, "usage_cto_f_h3", "episode_c", "2026-08-13T10:00:00Z", "harmed", false)
	addOutcome(t, s, mem, "usage_cto_f_p1", "episode_d", "2026-08-14T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_f_p2", "episode_e", "2026-08-15T10:00:00Z", "helped", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleFrozen {
		t.Errorf("3 harms @60%% rate = %s, want frozen", st.Lifecycle)
	}
	if st.Health != HealthDegraded {
		t.Errorf("auto-frozen with negative evidence = %s, want degraded", st.Health)
	}
	// Frozen must be excluded from the normal index.
	if len(res.RootIndex.Entries) != 0 || res.RootIndex.FrozenCount != 1 {
		t.Errorf("auto-frozen memory must be excluded from the index, entries=%d frozen=%d", len(res.RootIndex.Entries), res.RootIndex.FrozenCount)
	}
}

func TestCTOThresholdsExternalFailure(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_external", "cto-external", 1)
	putRevEvidence(t, s, mem)
	// Five external harms: usage only, never degraded/frozen, never counted.
	for i := 1; i <= 5; i++ {
		addOutcome(t, s, mem, "usage_cto_ext_"+strconv.Itoa(i), "episode_x", "2026-08-11T10:00:00Z", "harmed", true)
	}
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle == LifecycleDegraded || st.Lifecycle == LifecycleFrozen {
		t.Errorf("external failures must not degrade/freeze, got %s", st.Lifecycle)
	}
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("external-only memory = %s, want probation", st.Lifecycle)
	}
	if st.Usage.CountedHarmCount != 0 || st.Health != HealthHealthy {
		t.Errorf("external failures must not count as harm or degrade health, got harm=%d health=%s", st.Usage.CountedHarmCount, st.Health)
	}
	if st.Usage.UsageCount != 5 {
		t.Errorf("external failures still count as usage, got %d", st.Usage.UsageCount)
	}
}

func TestCTOThresholdsSuccessNeverCancelsFailure(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_mixed", "cto-mixed", 1)
	putRevEvidence(t, s, mem)
	// Enough helps for active, but one harm stays: degraded wins.
	addOutcome(t, s, mem, "usage_cto_m_h", "episode_a", "2026-08-11T10:00:00Z", "harmed", false)
	addOutcome(t, s, mem, "usage_cto_m_p1", "episode_b", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_m_p2", "episode_c", "2026-08-13T10:00:00Z", "helped", false)
	addOutcome(t, s, mem, "usage_cto_m_p3", "episode_d", "2026-08-14T10:00:00Z", "helped", false)
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Lifecycle != LifecycleDegraded {
		t.Errorf("3 helps + 1 harm = %s, want degraded (success never cancels failure)", st.Lifecycle)
	}
}

func TestCTOHealthIndependentCombinations(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// active + degraded: enough helps, but an override-corrected harm leaves
	// historical negative evidence.
	ad := mkStrategy("mem_cto_health_ad", "cto-health-ad", 1)
	putRevEvidence(t, s, ad)
	addOutcome(t, s, ad, "usage_cto_ad_h", "episode_a", "2026-08-11T10:00:00Z", "harmed", false)
	addOutcome(t, s, ad, "usage_cto_ad_p1", "episode_b", "2026-08-12T10:00:00Z", "helped", false)
	addOutcome(t, s, ad, "usage_cto_ad_p2", "episode_c", "2026-08-13T10:00:00Z", "helped", false)
	addOutcome(t, s, ad, "usage_cto_ad_p3", "episode_d", "2026-08-14T10:00:00Z", "helped", false)
	// Correct the harm to neutral: counted harm becomes 0 (active possible)
	// but the historical harmed outcome keeps health degraded.
	ov := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_cto_health_ad", JudgmentType: JudgmentTypeAttributionOverride,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_usage_cto_ad_h"},
		Source:              JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "re-evaluated"},
		BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-15T00:00:00Z",
	}
	ov = fillJudgmentHash(ov)
	put(t, s, ov)

	// frozen + healthy: governance freeze without any negative evidence.
	fh := mkStrategy("mem_cto_health_fh", "cto-health-fh", 1)
	putRevEvidence(t, s, fh)
	put(t, s, governanceEvent("gov_cto_fh", fh, "manual_freeze", "2026-08-10T00:00:00Z"))

	res := derive(t, s, deriveReq())
	stAD := stateByID(t, res, ad.MemoryID)
	if stAD.Lifecycle != LifecycleActive || stAD.Health != HealthDegraded {
		t.Errorf("override-corrected harm = %s/%s, want active/degraded", stAD.Lifecycle, stAD.Health)
	}
	stFH := stateByID(t, res, fh.MemoryID)
	if stFH.Lifecycle != LifecycleFrozen || stFH.Health != HealthHealthy {
		t.Errorf("governance frozen without negative evidence = %s/%s, want frozen/healthy", stFH.Lifecycle, stFH.Health)
	}
}

func TestCTOUsageStageFiltering(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_stage", "cto-stage", 1)
	putRevEvidence(t, s, mem)
	// retrieved/read/adopted usages are usage but never attribution evidence.
	u1 := mkUsageFull(t, "usage_cto_stage_r", mem.MemoryID, 1, "2026-08-11T10:00:00Z", "retrieved", "episode_a")
	put(t, s, u1)
	put(t, s, mkOutcome(t, "outcome_cto_stage_r", u1.UsageID, mem.MemoryID, 1, "helped", false))
	u2 := mkUsageFull(t, "usage_cto_stage_e", mem.MemoryID, 1, "2026-08-12T10:00:00Z", "evaluated", "episode_b")
	put(t, s, u2)
	put(t, s, mkOutcome(t, "outcome_cto_stage_e", u2.UsageID, mem.MemoryID, 1, "helped", false))

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.UsageCount != 2 {
		t.Errorf("usage_count = %d, want 2 (all stages count)", st.Usage.UsageCount)
	}
	if st.Usage.CountedHelpCount != 1 {
		t.Errorf("counted_help = %d, want 1 (only affected/evaluated attribute)", st.Usage.CountedHelpCount)
	}
	if st.Lifecycle != LifecycleProbation {
		t.Errorf("1 attributed help = %s, want probation", st.Lifecycle)
	}
}

func TestCTORepeatUsageIdempotent(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_repeat", "cto-repeat", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_repeat", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	// Repeating the same usage (same id) is a NOOP and never double counts.
	u := mkUsageFull(t, "usage_cto_repeat", mem.MemoryID, 1, "2026-08-11T10:00:00Z", "affected", "episode_a")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_usage_cto_repeat", u.UsageID, mem.MemoryID, 1, "helped", false))
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.UsageCount != 1 || st.Usage.CountedHelpCount != 1 {
		t.Errorf("repeat must not double count: count=%d help=%d", st.Usage.UsageCount, st.Usage.CountedHelpCount)
	}
}

func TestCTOOverrideSupersedeStillCorrect(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_ov", "cto-ov", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_ov", "episode_a", "2026-08-11T10:00:00Z", "harmed", false)
	first := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_cto_ov_1", JudgmentType: JudgmentTypeAttributionOverride,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_usage_cto_ov"},
		Source:              JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "first"},
		BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-12T00:00:00Z",
	}
	first = fillJudgmentHash(first)
	put(t, s, first)
	second := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_cto_ov_2", JudgmentType: JudgmentTypeAttributionOverride,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_usage_cto_ov"},
		Source:                JudgmentSource{SourceType: "user", SourceID: "local_user"},
		AttributionOverride:   &AttributionOverridePayload{PreviousEffect: "neutral", NewEffect: "helped", Reason: "second"},
		SupersedesJudgmentRef: &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: first.JudgmentID, ContentSHA256: first.ContentSHA256},
		BasisRefs:             []BasisRef{}, CreatedAt: "2026-08-13T00:00:00Z",
	}
	second = fillJudgmentHash(second)
	put(t, s, second)

	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.CountedHelpCount != 1 || st.Usage.CountedHarmCount != 0 {
		t.Errorf("supersede chain must apply newest override: help=%d harm=%d", st.Usage.CountedHelpCount, st.Usage.CountedHarmCount)
	}
}

func TestCTOStillFailClosedAndDeterministic(t *testing.T) {
	// Scope, revision and orphan references still fail closed; identical
	// inputs derive identical bytes.
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_fc", "cto-fc", 1)
	putRevEvidence(t, s, mem)
	addOutcome(t, s, mem, "usage_cto_fc", "episode_a", "2026-08-11T10:00:00Z", "helped", false)

	// Wrong revision reference fails closed.
	u := mkUsageFull(t, "usage_cto_fc_bad", mem.MemoryID, 9, "2026-08-11T10:00:00Z", "affected", "episode_a")
	put(t, s, u)
	if _, err := DeriveState(context.Background(), s, deriveReq()); err == nil {
		t.Error("revision-level orphan must fail closed")
	} else if !IsSensitiveError(err) {
		t.Errorf("error must be redacted, got %T", err)
	}
	// Remove the bad usage again (immutable store: open a fresh one).
	root2 := tempRoot(t)
	s2 := openProject(t, root2, Options{})
	putRevEvidence(t, s2, mem)
	addOutcome(t, s2, mem, "usage_cto_fc", "episode_a", "2026-08-11T10:00:00Z", "helped", false)
	r1 := derive(t, s2, deriveReq())
	b1, _ := json.Marshal(r1)
	r2 := derive(t, s2, deriveReq())
	b2, _ := json.Marshal(r2)
	if string(b1) != string(b2) {
		t.Error("identical inputs must derive identical bytes")
	}
}

// A harmed outcome from an earlier-stage usage (retrieved) is not verified
// negative evidence: it never degrades health and never counts as harm.
func TestCTOStageFilteredHarmNotNegativeEvidence(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	mem := mkStrategy("mem_cto_stage_harm", "cto-stage-harm", 1)
	putRevEvidence(t, s, mem)
	u := mkUsageFull(t, "usage_cto_stage_harm", mem.MemoryID, 1, "2026-08-11T10:00:00Z", "retrieved", "episode_a")
	put(t, s, u)
	put(t, s, mkOutcome(t, "outcome_cto_stage_harm", u.UsageID, mem.MemoryID, 1, "harmed", false))
	res := derive(t, s, deriveReq())
	st := stateByID(t, res, mem.MemoryID)
	if st.Usage.CountedHarmCount != 0 {
		t.Errorf("retrieved-stage harm must not count, got %d", st.Usage.CountedHarmCount)
	}
	if st.Lifecycle != LifecycleProbation || st.Health != HealthHealthy {
		t.Errorf("retrieved-stage harm = %s/%s, want probation/healthy", st.Lifecycle, st.Health)
	}
}
