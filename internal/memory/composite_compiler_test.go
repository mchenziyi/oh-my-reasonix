package memory

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func compositeFixture(t *testing.T) (*FactStore, MemoryRevision, MemoryEvidenceGeneration, ContextDescriptorFact, EpisodeFact) {
	t.Helper()
	s, c, e := episodicFixture(t)
	r, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, s, r, ev)
	return s, r, ev, c, e
}
func compositeRequest(r MemoryRevision, ev MemoryEvidenceGeneration, c ContextDescriptorFact, e EpisodeFact, gen string) CompositeCompileRequest {
	return CompositeCompileRequest{Scope: ScopeProject, GenerationID: gen, EvaluationTime: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), OKF: okfRequest(r, ev), EpisodeRefs: []EpisodeRef{{Scope: e.Scope, EpisodeID: e.EpisodeID, ContentSHA256: e.ContentSHA256}}, ContextRefs: []ContextDescriptorRef{{Scope: c.Scope, ContextDescriptorID: c.ContextDescriptorID, ContentSHA256: c.ContentSHA256}}}
}

func TestCompositeCompileDeterministic(t *testing.T) {
	s, r, ev, c, e := compositeFixture(t)
	req := compositeRequest(r, ev, c, e, "gen_composite_01")
	a, err := CompileComposite(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompileComposite(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.CompiledSHA256 != b.CompiledSHA256 || !reflect.DeepEqual(a.Outputs, b.Outputs) {
		t.Fatal("composite output is not deterministic")
	}
	for _, p := range []string{"wiki/index.md", "state/index-tree.json", "state/episodes/index.json", "wiki/episodes/index.md"} {
		if len(a.Outputs[p]) == 0 {
			t.Fatalf("missing %s", p)
		}
	}
	if !bytes.Contains(a.Outputs["wiki/index.md"], []byte("Episodic Index")) {
		t.Fatal("root route missing")
	}
}

func TestCompositeGenerationServesBothReaders(t *testing.T) {
	s, r, ev, c, e := compositeFixture(t)
	gs := NewGenerationStore(s)
	tx, err := gs.Begin(context.Background(), BeginGenerationRequest{Scope: ScopeProject, CompilerVersion: CompositeCompilerVersion, CanonicalizationVersion: 1, SchemaVersion: 1, IdempotencyKey: "composite_contract"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Fact{r, ev, policyOf(PolicyTypeIndex), c, e} {
		if err := gs.PrepareFact(context.Background(), tx, f); err != nil {
			t.Fatal(err)
		}
	}
	res, err := CompileComposite(context.Background(), s, compositeRequest(r, ev, c, e, tx.GenerationID))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, res.Inputs)); err != nil {
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
	doc, err := readJSONFile[generationDoc](filepath.Join(s.root, "generations", tx.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CompiledOutputSHA256 != res.CompiledSHA256 {
		t.Fatal("compiled hash mismatch")
	}
	b, err := s.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	mf, err := DecodeStrict[GenerationInputManifest](b)
	if err != nil {
		t.Fatal(err)
	}
	p := EpisodicScopeContext{Scope: ScopeProject, ScopeID: "project_test", GenerationID: tx.GenerationID, InputManifestID: tx.GenerationID, InputManifestSHA256: mf.InputManifestSHA256, IndexPath: "state/episodes/index.json"}
	if _, err := ReadEpisodicIndex(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	if _, err := readLibrarianFile(context.Background(), s, tx.GenerationID, "state/index-tree.json"); err != nil {
		t.Fatal(err)
	}
	pinned, err := PinCurrentEpisodicContext(context.Background(), s, "project_test")
	if err != nil {
		t.Fatal(err)
	}
	if pinned.GenerationID != tx.GenerationID || pinned.InputManifestSHA256 != mf.InputManifestSHA256 {
		t.Fatal("CURRENT was not pinned to the committed composite generation")
	}
	if _, err := ReadEpisodicIndex(context.Background(), s, *pinned); err != nil {
		t.Fatal(err)
	}
}
