package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

type EpisodicScopeContext struct {
	Scope               Scope  `json:"scope"`
	ScopeID             string `json:"scope_id"`
	GenerationID        string `json:"generation_id"`
	InputManifestID     string `json:"input_manifest_id"`
	InputManifestSHA256 string `json:"input_manifest_sha256"`
	IndexPath           string `json:"index_path"`
}

func (p EpisodicScopeContext) Validate() error {
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	for _, x := range []struct{ v, n string }{{p.ScopeID, "scope_id"}, {p.GenerationID, "generation_id"}, {p.InputManifestID, "input_manifest_id"}} {
		if err := validateID(x.v, x.n); err != nil {
			return err
		}
	}
	if err := validateHash(p.InputManifestSHA256, "input_manifest_sha256"); err != nil {
		return err
	}
	if p.IndexPath != "state/episodes/index.json" {
		return errors.New("episodic scope: invalid index path")
	}
	return nil
}

func verifyEpisodicScope(ctx context.Context, store *FactStore, p EpisodicScopeContext) error {
	if store == nil || p.Validate() != nil || !store.scopeMatches(p.Scope) {
		return storeError(CodeLibrarianInvalidContext, "pinned episodic scope is invalid")
	}
	gs := NewGenerationStore(store).(*generationStore)
	dir, err := gs.publishedGenDir(ctx, p.GenerationID)
	if err != nil {
		return err
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil || doc.GenerationID != p.GenerationID || doc.Scope != p.Scope || (doc.CompilerVersion != EpisodicCompilerVersion && doc.CompilerVersion != CompositeCompilerVersion) {
		return storeError(CodeLibrarianInvalidContext, "pinned episodic generation is invalid")
	}
	if err := gs.verifyCompiledOutputIntegrity(ctx, dir, doc); err != nil {
		return err
	}
	b, err := store.Get(ctx, FactKindGenerationInputManifest, p.InputManifestID)
	if err != nil {
		return err
	}
	m, err := DecodeStrict[GenerationInputManifest](b)
	if err != nil || m.GenerationID != p.GenerationID || m.InputManifestSHA256 != p.InputManifestSHA256 {
		return storeError(CodeLibrarianInvalidContext, "pinned episodic manifest is invalid")
	}
	return nil
}

func ReadEpisodicIndex(ctx context.Context, store *FactStore, p EpisodicScopeContext) (*EpisodicIndex, error) {
	if err := verifyEpisodicScope(ctx, store, p); err != nil {
		return nil, err
	}
	b, err := readLibrarianFile(ctx, store, p.GenerationID, p.IndexPath)
	if err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "episodic index is unreadable")
	}
	var idx EpisodicIndex
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&idx); err != nil || dec.Decode(&struct{}{}) != io.EOF || idx.SchemaVersion != 1 || idx.CompilerVersion != EpisodicCompilerVersion || idx.GenerationID != p.GenerationID {
		return nil, storeError(CodeLibrarianInvalidContext, "episodic index is invalid")
	}
	seen := map[string]bool{}
	last := ""
	for _, e := range idx.Entries {
		if e.EpisodeRef.Validate() != nil || e.EpisodeRef.Scope != p.Scope || seen[e.EpisodeRef.EpisodeID] || e.CardPath != "state/episodes/cards/"+e.EpisodeRef.EpisodeID+".json" || validateHash(e.CardSHA256, "card_sha256") != nil {
			return nil, storeError(CodeLibrarianInvalidContext, "episodic index entry is invalid")
		}
		key := episodicEntryKey(e)
		if last != "" && key < last {
			return nil, storeError(CodeLibrarianInvalidContext, "episodic index is not sorted")
		}
		last = key
		seen[e.EpisodeRef.EpisodeID] = true
	}
	return &idx, nil
}

func ReadEpisodeCard(ctx context.Context, store *FactStore, p EpisodicScopeContext, ref EpisodeRef) (*EpisodeCard, error) {
	idx, err := ReadEpisodicIndex(ctx, store, p)
	if err != nil {
		return nil, err
	}
	var entry *EpisodicIndexEntry
	for i := range idx.Entries {
		if idx.Entries[i].EpisodeRef == ref {
			entry = &idx.Entries[i]
			break
		}
	}
	if entry == nil {
		return nil, storeError(CodeLibrarianInvalidContext, "episode is not present in fixed index")
	}
	b, err := readLibrarianFile(ctx, store, p.GenerationID, entry.CardPath)
	if err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "episode card is unreadable")
	}
	var card EpisodeCard
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&card); err != nil || dec.Decode(&struct{}{}) != io.EOF || card.EpisodeRef != ref || card.GenerationID != p.GenerationID || card.CompilerVersion != EpisodicCompilerVersion || card.CardSHA256 != entry.CardSHA256 {
		return nil, storeError(CodeLibrarianInvalidContext, "episode card is invalid")
	}
	saved := card.CardSHA256
	card.CardSHA256 = ""
	raw, _ := json.Marshal(card)
	if hashOf(raw) != saved {
		return nil, storeError(CodeLibrarianInvalidContext, "episode card hash mismatch")
	}
	card.CardSHA256 = saved
	return &card, nil
}

type EpisodeCardSelection struct {
	EpisodeRef    EpisodeRef `json:"episode_ref"`
	ScopeID       string     `json:"scope_id"`
	CardSHA256    string     `json:"card_sha256"`
	RelevanceRank int        `json:"relevance_rank"`
	Why           string     `json:"why"`
}
type EpisodicRecallReceipt struct {
	SchemaVersion      int                    `json:"schema_version"`
	RetrievalID        string                 `json:"retrieval_id"`
	Status             string                 `json:"status"`
	EpisodeCards       []EpisodeCardSelection `json:"episode_cards"`
	VisitedIndexScopes []Scope                `json:"visited_index_scopes"`
	RequiresParentRead bool                   `json:"requires_parent_read"`
}

func ValidateEpisodicReceipt(ctx context.Context, stores map[Scope]*FactStore, pinned map[Scope]EpisodicScopeContext, r EpisodicRecallReceipt) error {
	if r.SchemaVersion != 1 || validateID(r.RetrievalID, "retrieval_id") != nil {
		return storeError(CodeLibrarianInvalidContext, "episodic receipt envelope is invalid")
	}
	valid := r.Status == "found" || r.Status == "no_candidate" || r.Status == "unknown" || r.Status == "unavailable"
	if !valid || (r.Status == "found") != (len(r.EpisodeCards) > 0) || r.RequiresParentRead != (r.Status == "found") {
		return storeError(CodeLibrarianInvalidContext, "episodic receipt status is invalid")
	}
	seen := map[string]bool{}
	for i, s := range r.EpisodeCards {
		if s.RelevanceRank != i+1 || validateID(s.ScopeID, "scope_id") != nil || validateHash(s.CardSHA256, "card_sha256") != nil || s.Why == "" || !utf8.ValidString(s.Why) || validateText(s.Why, maxLibrarianTextBytes, "why", true) != nil {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt selection is invalid")
		}
		p, ok := pinned[s.EpisodeRef.Scope]
		st := stores[s.EpisodeRef.Scope]
		if !ok || st == nil || p.ScopeID != s.ScopeID {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt scope is unavailable")
		}
		key := string(s.EpisodeRef.Scope) + "\x00" + s.EpisodeRef.EpisodeID
		if seen[key] {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt repeats an episode")
		}
		seen[key] = true
		card, err := ReadEpisodeCard(ctx, st, p, s.EpisodeRef)
		if err != nil || card.CardSHA256 != s.CardSHA256 {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt card is invalid")
		}
	}
	scopes := append([]Scope(nil), r.VisitedIndexScopes...)
	sort.Slice(scopes, func(i, j int) bool { return scopes[i] < scopes[j] })
	visited := map[Scope]bool{}
	for i, s := range scopes {
		if i > 0 && s == scopes[i-1] {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt repeats a visited scope")
		}
		if _, ok := pinned[s]; !ok {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt visited scope is unavailable")
		}
		visited[s] = true
	}
	for _, selection := range r.EpisodeCards {
		if !visited[selection.EpisodeRef.Scope] {
			return storeError(CodeLibrarianInvalidContext, "episodic receipt selection scope was not visited")
		}
	}
	return nil
}
