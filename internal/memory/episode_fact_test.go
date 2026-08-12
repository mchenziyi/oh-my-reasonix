package memory

import (
	"context"
	"encoding/json"
	"testing"
)

func validContextDescriptor(t *testing.T, scope Scope) ContextDescriptorFact {
	t.Helper()
	c := ContextDescriptorFact{SchemaVersion: 1, ContextDescriptorID: "context_01", Scope: scope, ContextSignatureVersion: 1, ComponentRefs: []string{"memory"}, OperationRefs: []string{"compile"}, TaskClassRefs: []string{"build"}, Environment: ContextEnvironment{OS: "darwin", Arch: "arm64", Language: "go", Tool: "omr"}, CreatedAt: "2026-08-12T00:00:00Z"}
	d, err := c.descriptorMap()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := canonicalJSON(d)
	c.CanonicalSHA256 = hashOf(b)
	h, err := c.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	c.ContentSHA256 = h
	return c
}

func validEpisode(t *testing.T, c ContextDescriptorFact) EpisodeFact {
	t.Helper()
	e := EpisodeFact{SchemaVersion: 1, EpisodeID: "episode_01", Scope: c.Scope, RootTaskID: "task_01", ContextDescriptorRef: ContextDescriptorRef{Scope: c.Scope, ContextDescriptorID: c.ContextDescriptorID, ContentSHA256: c.ContentSHA256}, TaskClassRefs: []string{"build"}, ComponentRefs: []string{"memory"}, OperationRefs: []string{"compile"}, FailureConceptRefs: []string{}, TaskResult: "succeeded", TaskResultEvidenceRefs: []EvidenceRef{}, EvidenceRefs: []EvidenceRef{}, OccurredAt: "2026-08-12T00:00:00Z", CreatedAt: "2026-08-12T00:00:01Z"}
	h, err := e.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	e.ContentSHA256 = h
	return e
}

func canonicalJSON(v any) ([]byte, error) { return json.Marshal(v) }

func TestEpisodeAndContextRoundTrip(t *testing.T) {
	c := validContextDescriptor(t, ScopeProject)
	e := validEpisode(t, c)
	for _, f := range []Fact{c, e} {
		if err := f.Validate(); err != nil {
			t.Fatal(err)
		}
		b, _ := f.EncodeCanonical()
		if len(b) == 0 {
			t.Fatal("empty encoding")
		}
	}
	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Fact{c, e} {
		if _, err := s.Put(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Get(context.Background(), FactKindEpisode, e.EpisodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), FactKindContextDescriptor, c.ContextDescriptorID); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodeFailedRequiresEvidence(t *testing.T) {
	c := validContextDescriptor(t, ScopeProject)
	e := validEpisode(t, c)
	e.TaskResult = "failed"
	e.ContentSHA256, _ = e.ContentHash()
	if err := e.Validate(); err == nil {
		t.Fatal("failed episode without evidence must fail")
	}
}

func TestEpisodeContextScopeIsolation(t *testing.T) {
	c := validContextDescriptor(t, ScopeProject)
	e := validEpisode(t, c)
	e.ContextDescriptorRef.Scope = ScopeGlobal
	e.ContentSHA256, _ = e.ContentHash()
	if err := e.Validate(); err == nil {
		t.Fatal("cross-scope context must fail")
	}
}
