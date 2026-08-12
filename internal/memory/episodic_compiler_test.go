package memory

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func episodicFixture(t *testing.T) (*FactStore, ContextDescriptorFact, EpisodeFact) {
	t.Helper()
	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := validContextDescriptor(t, ScopeProject)
	e := validEpisode(t, c)
	for _, f := range []Fact{c, e} {
		if _, err := s.Put(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	return s, c, e
}

func episodicRequest(s *FactStore, c ContextDescriptorFact, e EpisodeFact) EpisodicCompileRequest {
	return EpisodicCompileRequest{Scope: ScopeProject, GenerationID: "gen_episode_01", CompilerVersion: EpisodicCompilerVersion, EvaluationTime: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), EpisodeRefs: []EpisodeRef{{Scope: e.Scope, EpisodeID: e.EpisodeID, ContentSHA256: e.ContentSHA256}}, ContextRefs: []ContextDescriptorRef{{Scope: c.Scope, ContextDescriptorID: c.ContextDescriptorID, ContentSHA256: c.ContentSHA256}}, Store: s}
}

func TestCompileEpisodicDeterministic(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	a, err := CompileEpisodic(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompileEpisodic(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.CompiledSHA256 != b.CompiledSHA256 || !reflect.DeepEqual(a.Outputs, b.Outputs) {
		t.Fatal("rebuild must be byte stable")
	}
	for _, p := range []string{"state/episodes/cards/episode_01.json", "wiki/episodes/cards/episode_01.md", "state/episodes/index.json", "wiki/episodes/index.md"} {
		if len(a.Outputs[p]) == 0 {
			t.Fatalf("missing %s", p)
		}
	}
	for _, data := range a.Outputs {
		if bytes.Contains(data, []byte("/Users/")) || bytes.Contains(data, []byte("sanitized_summary")) {
			t.Fatal("unsafe content in output")
		}
	}
}

func TestCompileEpisodicDuplicateInputDedupes(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	req.EpisodeRefs = append(req.EpisodeRefs, req.EpisodeRefs[0])
	req.ContextRefs = append(req.ContextRefs, req.ContextRefs[0])
	res, err := CompileEpisodic(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inputs) != 2 {
		t.Fatalf("inputs=%d", len(res.Inputs))
	}
}

func TestCompileEpisodicRejectsMissingContext(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	req.ContextRefs = nil
	if _, err := CompileEpisodic(context.Background(), req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("got %v", err)
	}
}

func TestCompileEpisodicRejectsFuture(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	req.EvaluationTime = time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	if _, err := CompileEpisodic(context.Background(), req); ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("got %v", err)
	}
}

func TestCompileEpisodicRejectsZeroNow(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	req.EvaluationTime = time.Time{}
	if _, err := CompileEpisodic(context.Background(), req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("got %v", err)
	}
}

func TestCompileEpisodicEntryLimit(t *testing.T) {
	s, c, e := episodicFixture(t)
	req := episodicRequest(s, c, e)
	req.EpisodeRefs = make([]EpisodeRef, maxEpisodicEntries+1)
	for i := range req.EpisodeRefs {
		req.EpisodeRefs[i] = EpisodeRef{Scope: ScopeProject, EpisodeID: fmt.Sprintf("episode_%04d", i), ContentSHA256: e.ContentSHA256}
	}
	if _, err := CompileEpisodic(context.Background(), req); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("got %v", err)
	}
}

func TestEpisodicCompilerGenerationContract(t *testing.T) {
	s, c, e := episodicFixture(t)
	gs := NewGenerationStore(s)
	tx, err := gs.Begin(context.Background(), BeginGenerationRequest{Scope: ScopeProject, CompilerVersion: EpisodicCompilerVersion, CanonicalizationVersion: 1, SchemaVersion: 1, IdempotencyKey: "episodic_contract"})
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
	if doc.CompilerVersion != EpisodicCompilerVersion || doc.CompiledOutputSHA256 != res.CompiledSHA256 {
		t.Fatal("published generation does not match compiler result")
	}
}
