package memory

import (
	"context"
	"encoding/json"
	"testing"
)

func attributionFixture(t *testing.T, stage string) (*FactStore, AttributionReceipt, OutcomeCandidate) {
	t.Helper()
	store, lr, usageReceipt, _ := usageCaptureFixture(t)
	usageReceipt.Usages[0].UsageStage = stage
	result, err := CommitMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: usageReceipt})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Get(context.Background(), FactKindEpisode, usageReceipt.EpisodeRef.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := DecodeStrict[EpisodeFact](b)
	if err != nil {
		t.Fatal(err)
	}
	evidence := EvidenceRef{Scope: episode.Scope, EvidenceType: "episode", EvidenceID: episode.EpisodeID, ContentSHA256: episode.ContentSHA256}
	candidate := OutcomeCandidate{UsageID: result.UsageIDs[0], TaskOutcome: episode.TaskResult, MemoryEffect: "helped", Attribution: "confirmed", Critic: "not_required", EvidenceRefs: []EvidenceRef{evidence}}
	return store, AttributionReceipt{SchemaVersion: 1, EpisodeRef: usageReceipt.EpisodeRef, RootTaskID: usageReceipt.RootTaskID, Candidates: []OutcomeCandidate{candidate}}, candidate
}

func TestOutcomeLegacyCanonicalUnchanged(t *testing.T) {
	o := Outcome{SchemaVersion: 1, OutcomeID: "outcome_legacy", Scope: ScopeProject, UsageID: "usage_legacy", MemoryID: "mem_legacy", Revision: 1, Effect: "neutral", ContentSHA256: testHash, CreatedAt: "2026-08-07T00:00:00Z"}
	b, err := o.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"content_sha256":"sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-08-07T00:00:00Z","effect":"neutral","external_failure":false,"memory_id":"mem_legacy","outcome_id":"outcome_legacy","revision":1,"schema_version":1,"scope":"project","usage_id":"usage_legacy"}`
	if string(b) != want {
		t.Fatalf("legacy Outcome changed:\n%s", b)
	}
}

func TestBuildOutcomesCountingGate(t *testing.T) {
	store, receipt, candidate := attributionFixture(t, "evaluated")
	got, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CountedAsHelp == nil || !*got[0].CountedAsHelp || *got[0].CountedAsHarm || !*got[0].Evaluated {
		t.Fatalf("confirmed help not counted correctly: %+v", got)
	}

	receipt.Candidates[0].MemoryEffect = "harmed"
	receipt.Candidates[0].Critic = "unsupported"
	got, err = BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Effect != "unknown" || *got[0].CountedAsHarm {
		t.Fatalf("unsupported harm must degrade to unknown: %+v", got[0])
	}

	receipt.Candidates[0] = candidate
	got, err = BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt, ExternalFailure: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].ExternalFailure || *got[0].CountedAsHelp || *got[0].CountedAsHarm {
		t.Fatalf("external failure must never score: %+v", got[0])
	}
}

func TestBuildOutcomesRequiresEvaluatedUsage(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "affected")
	got, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if *got[0].Evaluated || *got[0].CountedAsHelp || *got[0].CountedAsHarm {
		t.Fatalf("affected-only usage must not score: %+v", got[0])
	}
}

func TestBuildOutcomesConfirmedWithoutEvidenceDegradesUnknown(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	receipt.Candidates[0].EvidenceRefs = []EvidenceRef{}
	got, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Effect != "unknown" || *got[0].CountedAsHelp || *got[0].CountedAsHarm {
		t.Fatalf("evidence-free confirmation must degrade to unknown: %+v", got[0])
	}
}

func TestCommitOutcomesIdempotent(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	req := AttributionRequest{Store: store, Receipt: receipt}
	first, err := CommitOutcomes(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitOutcomes(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 1 || second.Noop != 1 || first.OutcomeIDs[0] != second.OutcomeIDs[0] {
		t.Fatalf("retry was not idempotent: %+v %+v", first, second)
	}
}

func TestCommitOutcomesConflictWritesNothing(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	built, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	conflict := built[0]
	conflict.Effect = "neutral"
	conflict.CountedAsHelp = boolPtr(false)
	conflict.ContentSHA256 = ""
	h, err := conflict.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	conflict.ContentSHA256 = h
	put(t, store, conflict)
	if _, err := CommitOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); ErrorCode(err) != CodeIdentityConflict {
		t.Fatalf("conflict must fail before writes: %v", err)
	}
	keys, err := store.List(context.Background(), FactKindOutcome)
	if err != nil || len(keys) != 1 {
		t.Fatalf("conflict changed outcome set: %v %v", keys, err)
	}
}

func TestCommittedOutcomeFeedsDerivedStats(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	if _, err := CommitOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); err != nil {
		t.Fatal(err)
	}
	keys, err := store.List(context.Background(), FactKindMemoryRevision)
	if err != nil || len(keys) == 0 {
		t.Fatalf("missing revision fixture: %v %v", keys, err)
	}
	b, err := store.Get(context.Background(), FactKindMemoryRevision, keys[0])
	if err != nil {
		t.Fatal(err)
	}
	rev, err := DecodeStrict[MemoryRevision](b)
	if err != nil {
		t.Fatal(err)
	}
	states, err := DeriveState(context.Background(), store, DerivedStateRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	var state *DerivedMemoryState
	for i := range states.States {
		if states.States[i].MemoryID == rev.MemoryID && states.States[i].Revision == rev.Revision {
			state = &states.States[i]
			break
		}
	}
	if state == nil || state.Usage.CountedHelpCount != 1 || state.Usage.CountedHarmCount != 0 {
		t.Fatalf("enriched outcome did not feed unique scoring protocol: %+v", state)
	}
}

func TestAttributionReceiptStrictJSON(t *testing.T) {
	_, receipt, _ := attributionFixture(t, "evaluated")
	b, _ := receipt.EncodeCanonical()
	if _, err := DecodeStrict[AttributionReceipt](b); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["outcome_id"] = "model_controlled"
	injected, _ := json.Marshal(m)
	if _, err := DecodeStrict[AttributionReceipt](injected); err == nil {
		t.Fatal("model-controlled outcome_id must be rejected")
	}
}

func TestEnrichedOutcomeRejectsPartialAnchors(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	got, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	o := got[0]
	o.CountedAsHarm = nil
	if err := o.Validate(); err == nil {
		t.Fatal("partial enriched Outcome must be rejected")
	}
	if _, err := o.CanonicalBytes(); err == nil {
		t.Fatal("partial enriched Outcome canonicalization must fail without panic")
	}
}

func TestBuildOutcomesRejectsEvidenceOutsideEpisode(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	receipt.Candidates[0].EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "tool_result", EvidenceID: "evidence_outside", ContentSHA256: testHash}}
	if _, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); ErrorCode(err) != CodeAttributionCaptureInvalid {
		t.Fatalf("outside evidence must fail closed: %v", err)
	}
}

func TestBuildOutcomesRejectsNonFailureConceptCause(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	b, err := store.Get(context.Background(), FactKindMemoryUsage, receipt.Candidates[0].UsageID)
	if err != nil {
		t.Fatal(err)
	}
	u, err := DecodeStrict[MemoryUsage](b)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Candidates[0].FailureCauseMemoryRef = &MemoryRef{Scope: u.Scope, MemoryType: MemoryTypeStrategy, MemoryID: u.MemoryID, Revision: u.Revision, ContentSHA256: testHash}
	if _, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); ErrorCode(err) != CodeAttributionCaptureInvalid {
		t.Fatalf("non failure-concept cause must fail closed: %v", err)
	}
}
