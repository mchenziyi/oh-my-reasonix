package memory

import (
	"context"
	"testing"
)

func publishedEpisodic(t *testing.T) (*FactStore, EpisodicScopeContext, EpisodeRef) {
	t.Helper()
	s, c, e := episodicFixture(t)
	gs := NewGenerationStore(s)
	tx, err := gs.Begin(context.Background(), BeginGenerationRequest{Scope: ScopeProject, CompilerVersion: EpisodicCompilerVersion, CanonicalizationVersion: 1, SchemaVersion: 1, IdempotencyKey: "episodic_read"})
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareFact(context.Background(), tx, c); err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareFact(context.Background(), tx, e); err != nil {
		t.Fatal(err)
	}
	req := episodicRequest(s, c, e)
	req.GenerationID = tx.GenerationID
	res, err := CompileEpisodic(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mf := manifestFor(tx, res.Inputs)
	if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
		t.Fatal(err)
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, res.Outputs); err != nil {
		t.Fatal(err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	b, err := s.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeStrict[GenerationInputManifest](b)
	if err != nil {
		t.Fatal(err)
	}
	p := EpisodicScopeContext{Scope: ScopeProject, ScopeID: "project_test", GenerationID: tx.GenerationID, InputManifestID: tx.GenerationID, InputManifestSHA256: saved.InputManifestSHA256, IndexPath: "state/episodes/index.json"}
	return s, p, EpisodeRef{Scope: e.Scope, EpisodeID: e.EpisodeID, ContentSHA256: e.ContentSHA256}
}

func TestReadFixedEpisodicIndexAndCard(t *testing.T) {
	s, p, ref := publishedEpisodic(t)
	idx, err := ReadEpisodicIndex(context.Background(), s, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("entries=%d", len(idx.Entries))
	}
	card, err := ReadEpisodeCard(context.Background(), s, p, ref)
	if err != nil {
		t.Fatal(err)
	}
	if card.EpisodeRef != ref {
		t.Fatal("wrong card")
	}
}

func TestValidateEpisodicReceipt(t *testing.T) {
	s, p, ref := publishedEpisodic(t)
	card, err := ReadEpisodeCard(context.Background(), s, p, ref)
	if err != nil {
		t.Fatal(err)
	}
	r := EpisodicRecallReceipt{SchemaVersion: 1, RetrievalID: "retrieval_01", Status: "found", EpisodeCards: []EpisodeCardSelection{{EpisodeRef: ref, ScopeID: p.ScopeID, CardSHA256: card.CardSHA256, RelevanceRank: 1, Why: "same structured task context"}}, VisitedIndexScopes: []Scope{ScopeProject}, RequiresParentRead: true}
	if err := ValidateEpisodicReceipt(context.Background(), map[Scope]*FactStore{ScopeProject: s}, map[Scope]EpisodicScopeContext{ScopeProject: p}, r); err != nil {
		t.Fatal(err)
	}
	r.EpisodeCards[0].CardSHA256 = hashOf([]byte("wrong"))
	if err := ValidateEpisodicReceipt(context.Background(), map[Scope]*FactStore{ScopeProject: s}, map[Scope]EpisodicScopeContext{ScopeProject: p}, r); err == nil {
		t.Fatal("wrong card hash must fail")
	}
}

func TestEpisodicReceiptStatusMatrix(t *testing.T) {
	for _, status := range []string{"no_candidate", "unknown", "unavailable"} {
		r := EpisodicRecallReceipt{SchemaVersion: 1, RetrievalID: "retrieval_01", Status: status, EpisodeCards: []EpisodeCardSelection{}, VisitedIndexScopes: []Scope{}, RequiresParentRead: false}
		if err := ValidateEpisodicReceipt(context.Background(), nil, nil, r); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
}
