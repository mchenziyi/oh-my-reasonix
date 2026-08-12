package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEpisodicDoctorClean(t *testing.T) {
	s, p, _ := publishedEpisodic(t)
	r, err := CheckEpisodicGeneration(context.Background(), s, p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Healthy || !r.Rebuildable || r.ExpectedCompiledSHA256 == "" || len(r.Findings) != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestEpisodicDoctorDetectsMissingDerivedOutput(t *testing.T) {
	s, p, ref := publishedEpisodic(t)
	path := filepath.Join(s.root, "generations", p.GenerationID, "state", "episodes", "cards", ref.EpisodeID+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r, err := CheckEpisodicGeneration(context.Background(), s, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Healthy || !r.Rebuildable || !doctorHas(r, "derived_output_missing") {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func TestEpisodicDoctorDetectsDrift(t *testing.T) {
	s, p, _ := publishedEpisodic(t)
	path := filepath.Join(s.root, "generations", p.GenerationID, "wiki", "episodes", "index.md")
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := CheckEpisodicGeneration(context.Background(), s, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Healthy || !doctorHas(r, "compiled_hash_drift") || !doctorHas(r, "derived_output_drift") {
		t.Fatalf("unexpected report: %+v", r)
	}
}

func doctorHas(r *EpisodicDoctorReport, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
