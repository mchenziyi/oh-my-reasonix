package memory

import (
	"context"
	"testing"
	"time"
)

func TestCommitAttributionOverrideAppendsAndIsIdempotent(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	outcomes, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 1, 2, 3, 0, time.UTC)
	req := AttributionOverrideRequest{Store: store, OutcomeID: outcomes[0].OutcomeID, PreviousEffect: "helped", NewEffect: "harmed", Reason: "reviewed evidence", SourceType: "local_user", SourceID: "user_1", Now: now}
	first, err := CommitAttributionOverride(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitAttributionOverride(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != WriteCreated || second.Status != WriteNoop || first.Judgment.JudgmentID != second.Judgment.JudgmentID {
		t.Fatalf("not idempotent: %+v %+v", first, second)
	}
	b, err := store.Get(context.Background(), FactKindOutcome, req.OutcomeID)
	if err != nil {
		t.Fatal(err)
	}
	o, _ := DecodeStrict[Outcome](b)
	if o.Effect != "helped" {
		t.Fatalf("override mutated outcome: %s", o.Effect)
	}
}

func TestAttributionOverrideRequiresCurrentEffect(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	outcomes, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CommitOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitAttributionOverride(context.Background(), AttributionOverrideRequest{Store: store, OutcomeID: outcomes[0].OutcomeID, PreviousEffect: "helped", NewEffect: "harmed", Reason: "x", SourceType: "local_user", SourceID: "user_1", Now: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAttributionOverride(context.Background(), AttributionOverrideRequest{Store: store, OutcomeID: outcomes[0].OutcomeID, PreviousEffect: "helped", NewEffect: "neutral", Reason: "x", SourceType: "local_user", SourceID: "user_1", Now: time.Now().UTC()}); err == nil {
		t.Fatal("expected broken chain/current state to be rejected")
	}
}
