package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"time"
)

type EpisodicDoctorFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

type EpisodicDoctorReport struct {
	SchemaVersion          int                     `json:"schema_version"`
	Scope                  Scope                   `json:"scope"`
	GenerationID           string                  `json:"generation_id"`
	Healthy                bool                    `json:"healthy"`
	Rebuildable            bool                    `json:"rebuildable"`
	ExpectedCompiledSHA256 string                  `json:"expected_compiled_sha256"`
	Findings               []EpisodicDoctorFinding `json:"findings"`
}

func (r EpisodicDoctorReport) EncodeCanonical() ([]byte, error) { return json.Marshal(r) }

func CheckEpisodicGeneration(ctx context.Context, store *FactStore, p EpisodicScopeContext) (*EpisodicDoctorReport, error) {
	report := &EpisodicDoctorReport{SchemaVersion: 1, Scope: p.Scope, GenerationID: p.GenerationID, Healthy: true, Rebuildable: false, Findings: []EpisodicDoctorFinding{}}
	if store == nil || p.Validate() != nil || !store.scopeMatches(p.Scope) {
		return nil, storeError(CodeLibrarianInvalidContext, "episodic doctor context is invalid")
	}
	gs := NewGenerationStore(store).(*generationStore)
	dir, err := gs.publishedGenDir(ctx, p.GenerationID)
	if err != nil {
		return nil, err
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil || doc.GenerationID != p.GenerationID || doc.Scope != p.Scope || doc.CompilerVersion != EpisodicCompilerVersion {
		doctorFinding(report, "generation_invalid", "error", "episodic generation metadata is invalid")
		return report, nil
	}
	b, err := store.Get(ctx, FactKindGenerationInputManifest, p.InputManifestID)
	if err != nil {
		return nil, err
	}
	mf, err := DecodeStrict[GenerationInputManifest](b)
	if err != nil || mf.GenerationID != p.GenerationID || mf.InputManifestSHA256 != p.InputManifestSHA256 {
		doctorFinding(report, "manifest_invalid", "error", "episodic manifest is invalid")
		return report, nil
	}
	episodeRefs, contextRefs, now, err := episodicRefsFromManifest(ctx, store, p, mf)
	if err != nil {
		doctorFinding(report, "input_invalid", "error", "episodic manifest input is invalid")
		return report, nil
	}
	expected, err := CompileEpisodic(ctx, EpisodicCompileRequest{Scope: p.Scope, GenerationID: p.GenerationID, CompilerVersion: EpisodicCompilerVersion, EvaluationTime: now, EpisodeRefs: episodeRefs, ContextRefs: contextRefs, Store: store})
	if err != nil {
		doctorFinding(report, "rebuild_failed", "error", "episodic generation cannot be rebuilt")
		return report, nil
	}
	report.Rebuildable = true
	report.ExpectedCompiledSHA256 = expected.CompiledSHA256
	actualHash, err := gs.compiledOutputHash(ctx, dir)
	if err != nil {
		doctorFinding(report, "output_unsafe", "error", "episodic output is unsafe")
		return report, nil
	}
	if actualHash != expected.CompiledSHA256 || doc.CompiledOutputSHA256 != expected.CompiledSHA256 {
		doctorFinding(report, "compiled_hash_drift", "error", "episodic compiled output hash drifted")
	}
	for path, want := range expected.Outputs {
		got, err := readLibrarianFile(ctx, store, p.GenerationID, path)
		if err != nil {
			doctorFinding(report, "derived_output_missing", "warning", "episodic derived output is missing")
			continue
		}
		if !bytes.Equal(got, want) {
			doctorFinding(report, "derived_output_drift", "error", "episodic derived output differs from rebuild")
		}
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Detail < b.Detail
	})
	report.Healthy = len(report.Findings) == 0
	return report, nil
}

func episodicRefsFromManifest(ctx context.Context, store *FactStore, p EpisodicScopeContext, mf GenerationInputManifest) ([]EpisodeRef, []ContextDescriptorRef, time.Time, error) {
	episodes := []EpisodeRef{}
	contexts := []ContextDescriptorRef{}
	latest, err := time.Parse(time.RFC3339Nano, mf.CreatedAt)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	for _, in := range mf.Inputs {
		switch in.FactType {
		case "episode":
			b, err := store.Get(ctx, FactKindEpisode, in.FactID)
			if err != nil {
				return nil, nil, time.Time{}, err
			}
			f, err := DecodeStrict[EpisodeFact](b)
			if err != nil || f.Scope != p.Scope || f.ContentSHA256 != in.ContentSHA256 || f.SchemaVersion != in.FactSchemaVersion {
				return nil, nil, time.Time{}, storeError(CodeSchemaInvalid, "episode manifest input mismatch")
			}
			created, _ := time.Parse(time.RFC3339Nano, f.CreatedAt)
			if created.After(latest) {
				latest = created
			}
			episodes = append(episodes, EpisodeRef{Scope: f.Scope, EpisodeID: f.EpisodeID, ContentSHA256: f.ContentSHA256})
		case "context_descriptor":
			b, err := store.Get(ctx, FactKindContextDescriptor, in.FactID)
			if err != nil {
				return nil, nil, time.Time{}, err
			}
			f, err := DecodeStrict[ContextDescriptorFact](b)
			if err != nil || f.Scope != p.Scope || f.ContentSHA256 != in.ContentSHA256 || f.SchemaVersion != in.FactSchemaVersion {
				return nil, nil, time.Time{}, storeError(CodeSchemaInvalid, "context manifest input mismatch")
			}
			created, _ := time.Parse(time.RFC3339Nano, f.CreatedAt)
			if created.After(latest) {
				latest = created
			}
			contexts = append(contexts, ContextDescriptorRef{Scope: f.Scope, ContextDescriptorID: f.ContextDescriptorID, ContentSHA256: f.ContentSHA256})
		default:
			return nil, nil, time.Time{}, storeError(CodeSchemaInvalid, "episodic manifest contains an unsupported input")
		}
	}
	return episodes, contexts, latest, nil
}

func doctorFinding(r *EpisodicDoctorReport, code, severity, detail string) {
	r.Findings = append(r.Findings, EpisodicDoctorFinding{Code: code, Severity: severity, Detail: detail})
	r.Healthy = false
}
