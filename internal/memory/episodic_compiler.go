package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	EpisodicCompilerVersion = "mnemosyne-episodic-compiler/1"
	maxEpisodicEntries      = 1024
)

type EpisodicCompileRequest struct {
	Scope           Scope
	GenerationID    string
	CompilerVersion string
	EvaluationTime  time.Time
	EpisodeRefs     []EpisodeRef
	ContextRefs     []ContextDescriptorRef
	Store           *FactStore
}

type EpisodicCompileResult struct {
	Outputs        map[string][]byte
	CompiledSHA256 string
	Inputs         []ManifestInput
}

type EpisodeCard struct {
	SchemaVersion        int                  `json:"schema_version"`
	EpisodeRef           EpisodeRef           `json:"episode_ref"`
	ContextDescriptorRef ContextDescriptorRef `json:"context_descriptor_ref"`
	EvidenceSetSHA256    string               `json:"evidence_set_sha256"`
	CompilerVersion      string               `json:"compiler_version"`
	GenerationID         string               `json:"generation_id"`
	OccurredAt           string               `json:"occurred_at"`
	ComponentRefs        []string             `json:"component_refs"`
	OperationRefs        []string             `json:"operation_refs"`
	TaskClassRefs        []string             `json:"task_class_refs"`
	FailureConceptRefs   []string             `json:"failure_concept_refs"`
	TaskResult           string               `json:"task_result"`
	CardSHA256           string               `json:"card_sha256"`
}

type EpisodicIndexEntry struct {
	EpisodeRef         EpisodeRef `json:"episode_ref"`
	ComponentRefs      []string   `json:"component_refs"`
	OperationRefs      []string   `json:"operation_refs"`
	TaskClassRefs      []string   `json:"task_class_refs"`
	FailureConceptRefs []string   `json:"failure_concept_refs"`
	TimeBucket         string     `json:"time_bucket"`
	TaskResult         string     `json:"task_result"`
	CardPath           string     `json:"card_path"`
	CardSHA256         string     `json:"card_sha256"`
}

type EpisodicIndex struct {
	SchemaVersion   int                  `json:"schema_version"`
	CompilerVersion string               `json:"compiler_version"`
	GenerationID    string               `json:"generation_id"`
	Entries         []EpisodicIndexEntry `json:"entries"`
}

func CompileEpisodic(ctx context.Context, req EpisodicCompileRequest) (*EpisodicCompileResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeOKFCompileError, "episodic compile cancelled")
	}
	if req.Store == nil || req.Scope.Validate() != nil || !req.Store.scopeMatches(req.Scope) {
		return nil, storeError(CodeOKFInvalidInput, "invalid episodic compile scope")
	}
	if req.CompilerVersion != EpisodicCompilerVersion {
		return nil, storeError(CodeGenerationCompilerUnavailable, "episodic compiler is unavailable")
	}
	if err := validateID(req.GenerationID, "generation_id"); err != nil || req.EvaluationTime.IsZero() {
		return nil, storeError(CodeOKFInvalidInput, "invalid episodic compile request")
	}
	if n, err := uniqueEpisodeRefCount(req.EpisodeRefs); err != nil {
		return nil, err
	} else if n > maxEpisodicEntries {
		return nil, storeError(CodeOKFCompileError, "episodic index exceeds entry limit")
	}
	contexts, inputs, err := loadContextDescriptors(ctx, req)
	if err != nil {
		return nil, err
	}
	episodes, moreInputs, err := loadEpisodes(ctx, req, contexts)
	if err != nil {
		return nil, err
	}
	if len(episodes) > maxEpisodicEntries {
		return nil, storeError(CodeOKFCompileError, "episodic index exceeds entry limit")
	}
	inputs = append(inputs, moreInputs...)
	inputs, err = dedupeManifestInputs(inputs)
	if err != nil {
		return nil, storeError(CodeOKFInvalidInput, "episodic inputs conflict")
	}

	res := &EpisodicCompileResult{Outputs: map[string][]byte{}, Inputs: inputs}
	entries := make([]EpisodicIndexEntry, 0, len(episodes))
	for _, e := range episodes {
		card, err := buildEpisodeCard(e, req.GenerationID)
		if err != nil {
			return nil, err
		}
		jsonBytes, _ := json.MarshalIndent(card, "", "  ")
		path := "state/episodes/cards/" + e.EpisodeID + ".json"
		res.Outputs[path] = jsonBytes
		res.Outputs["wiki/episodes/cards/"+e.EpisodeID+".md"] = renderEpisodeCard(card)
		entries = append(entries, EpisodicIndexEntry{EpisodeRef: card.EpisodeRef, ComponentRefs: card.ComponentRefs, OperationRefs: card.OperationRefs, TaskClassRefs: card.TaskClassRefs, FailureConceptRefs: card.FailureConceptRefs, TimeBucket: card.OccurredAt[:7], TaskResult: card.TaskResult, CardPath: path, CardSHA256: card.CardSHA256})
	}
	sort.Slice(entries, func(i, j int) bool { return episodicEntryKey(entries[i]) < episodicEntryKey(entries[j]) })
	idx := EpisodicIndex{SchemaVersion: 1, CompilerVersion: req.CompilerVersion, GenerationID: req.GenerationID, Entries: entries}
	res.Outputs["state/episodes/index.json"], _ = json.MarshalIndent(idx, "", "  ")
	res.Outputs["wiki/episodes/index.md"] = renderEpisodicIndex(idx)
	for _, data := range res.Outputs {
		if int64(len(data)) > maxCompiledOutputBytes {
			return nil, storeError(CodeOKFCompileError, "episodic output exceeds size limit")
		}
	}
	res.CompiledSHA256 = compiledOutputHash(res.Outputs)
	return res, nil
}

func uniqueEpisodeRefCount(refs []EpisodeRef) (int, error) {
	seen := map[string]string{}
	for _, r := range refs {
		if err := r.Validate(); err != nil {
			return 0, storeError(CodeOKFInvalidInput, "invalid episode reference")
		}
		if h, ok := seen[r.EpisodeID]; ok && h != r.ContentSHA256 {
			return 0, storeError(CodeOKFInvalidInput, "episode references conflict")
		}
		seen[r.EpisodeID] = r.ContentSHA256
	}
	return len(seen), nil
}

func loadContextDescriptors(ctx context.Context, req EpisodicCompileRequest) (map[string]ContextDescriptorFact, []ManifestInput, error) {
	out := map[string]ContextDescriptorFact{}
	inputs := []ManifestInput{}
	for _, r := range req.ContextRefs {
		if err := r.Validate(); err != nil || r.Scope != req.Scope {
			return nil, nil, storeError(CodeOKFInvalidInput, "invalid context descriptor reference")
		}
		key := r.ContextDescriptorID
		if old, ok := out[key]; ok {
			if old.ContentSHA256 != r.ContentSHA256 {
				return nil, nil, storeError(CodeOKFInvalidInput, "context descriptor references conflict")
			}
			continue
		}
		b, err := req.Store.Get(ctx, FactKindContextDescriptor, key)
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "context descriptor is missing or unreadable")
		}
		f, err := DecodeStrict[ContextDescriptorFact](b)
		if err != nil || f.Scope != r.Scope || f.ContextDescriptorID != r.ContextDescriptorID || f.ContentSHA256 != r.ContentSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "context descriptor identity mismatch")
		}
		if after(f.CreatedAt, req.EvaluationTime) {
			return nil, nil, storeError(CodeEvaluationFutureReference, "context descriptor is from the future")
		}
		out[key] = f
		inputs = append(inputs, ManifestInput{FactType: "context_descriptor", FactID: f.ContextDescriptorID, FactSchemaVersion: f.SchemaVersion, ContentSHA256: f.ContentSHA256})
	}
	return out, inputs, nil
}

func loadEpisodes(ctx context.Context, req EpisodicCompileRequest, contexts map[string]ContextDescriptorFact) ([]EpisodeFact, []ManifestInput, error) {
	out := []EpisodeFact{}
	inputs := []ManifestInput{}
	ids := map[string]string{}
	roots := map[string]bool{}
	for _, r := range req.EpisodeRefs {
		if err := r.Validate(); err != nil || r.Scope != req.Scope {
			return nil, nil, storeError(CodeOKFInvalidInput, "invalid episode reference")
		}
		if h, ok := ids[r.EpisodeID]; ok {
			if h != r.ContentSHA256 {
				return nil, nil, storeError(CodeOKFInvalidInput, "episode references conflict")
			}
			continue
		}
		b, err := req.Store.Get(ctx, FactKindEpisode, r.EpisodeID)
		if err != nil {
			return nil, nil, storeError(CodeOKFInvalidInput, "episode is missing or unreadable")
		}
		f, err := DecodeStrict[EpisodeFact](b)
		if err != nil || f.Scope != r.Scope || f.EpisodeID != r.EpisodeID || f.ContentSHA256 != r.ContentSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "episode identity mismatch")
		}
		if roots[f.RootTaskID] {
			return nil, nil, storeError(CodeOKFInvalidInput, "root task has multiple episodes")
		}
		roots[f.RootTaskID] = true
		if after(f.CreatedAt, req.EvaluationTime) || after(f.OccurredAt, req.EvaluationTime) {
			return nil, nil, storeError(CodeEvaluationFutureReference, "episode is from the future")
		}
		c, ok := contexts[f.ContextDescriptorRef.ContextDescriptorID]
		if !ok || c.Scope != f.ContextDescriptorRef.Scope || c.ContentSHA256 != f.ContextDescriptorRef.ContentSHA256 {
			return nil, nil, storeError(CodeOKFInvalidInput, "episode context is not an explicit input")
		}
		if !subset(f.ComponentRefs, c.ComponentRefs) || !subset(f.OperationRefs, c.OperationRefs) || !subset(f.TaskClassRefs, c.TaskClassRefs) {
			return nil, nil, storeError(CodeOKFInvalidInput, "episode classification does not match context")
		}
		ids[r.EpisodeID] = r.ContentSHA256
		out = append(out, f)
		inputs = append(inputs, ManifestInput{FactType: "episode", FactID: f.EpisodeID, FactSchemaVersion: f.SchemaVersion, ContentSHA256: f.ContentSHA256})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EpisodeID < out[j].EpisodeID })
	return out, inputs, nil
}

func after(s string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, s)
	return err != nil || t.After(now)
}

func buildEpisodeCard(e EpisodeFact, gen string) (EpisodeCard, error) {
	refs := append([]EvidenceRef{}, e.EvidenceRefs...)
	maps := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		m, _ := r.canonMap()
		maps = append(maps, m)
	}
	sort.Slice(maps, func(i, j int) bool {
		a, _ := json.Marshal(maps[i])
		b, _ := json.Marshal(maps[j])
		return string(a) < string(b)
	})
	eb, _ := json.Marshal(maps)
	c := EpisodeCard{SchemaVersion: 1, EpisodeRef: EpisodeRef{Scope: e.Scope, EpisodeID: e.EpisodeID, ContentSHA256: e.ContentSHA256}, ContextDescriptorRef: e.ContextDescriptorRef, EvidenceSetSHA256: hashOf(eb), CompilerVersion: EpisodicCompilerVersion, GenerationID: gen, OccurredAt: e.OccurredAt, ComponentRefs: sortedStrings(e.ComponentRefs), OperationRefs: sortedStrings(e.OperationRefs), TaskClassRefs: sortedStrings(e.TaskClassRefs), FailureConceptRefs: sortedStrings(e.FailureConceptRefs), TaskResult: e.TaskResult}
	b, _ := json.Marshal(c)
	c.CardSHA256 = hashOf(b)
	return c, nil
}

func subset(items, universe []string) bool {
	set := map[string]bool{}
	for _, v := range universe {
		set[v] = true
	}
	for _, v := range items {
		if !set[v] {
			return false
		}
	}
	return true
}

func episodicEntryKey(e EpisodicIndexEntry) string {
	return strings.Join(e.ComponentRefs, "\x00") + "\x01" + strings.Join(e.OperationRefs, "\x00") + "\x01" + strings.Join(e.TaskClassRefs, "\x00") + "\x01" + strings.Join(e.FailureConceptRefs, "\x00") + "\x01" + e.TimeBucket + "\x01" + e.TaskResult + "\x01" + e.EpisodeRef.EpisodeID
}
func renderEpisodeCard(c EpisodeCard) []byte {
	return []byte(fmt.Sprintf("# Episode %s\n\n- Result: `%s`\n- Occurred: `%s`\n- Context: `%s`\n- Card SHA256: `%s`\n", c.EpisodeRef.EpisodeID, c.TaskResult, c.OccurredAt, c.ContextDescriptorRef.ContextDescriptorID, c.CardSHA256))
}
func renderEpisodicIndex(idx EpisodicIndex) []byte {
	var b strings.Builder
	b.WriteString("# Episodic Index\n\n")
	for _, e := range idx.Entries {
		fmt.Fprintf(&b, "- `%s` — `%s` — `%s`\n", e.EpisodeRef.EpisodeID, e.TaskResult, e.CardPath)
	}
	return []byte(b.String())
}
