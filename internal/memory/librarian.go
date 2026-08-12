package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const maxLibrarianTextBytes = 2048
const maxLibrarianRefs = 256

type RetrievalScopeContext struct {
	Scope               Scope  `json:"scope"`
	ScopeID             string `json:"scope_id"`
	GenerationID        string `json:"generation_id"`
	InputManifestID     string `json:"input_manifest_id"`
	InputManifestSHA256 string `json:"input_manifest_sha256"`
	RootIndexPath       string `json:"root_index_path"`
	IndexTreePath       string `json:"index_tree_path"`
}

func (c RetrievalScopeContext) Validate(expected Scope) error {
	if c.Scope != expected {
		return errors.New("retrieval scope context: unexpected scope")
	}
	if err := validateID(c.ScopeID, "scope_id"); err != nil {
		return fmt.Errorf("retrieval scope context: %w", err)
	}
	if err := validateID(c.GenerationID, "generation_id"); err != nil {
		return fmt.Errorf("retrieval scope context: %w", err)
	}
	if err := validateID(c.InputManifestID, "input_manifest_id"); err != nil {
		return fmt.Errorf("retrieval scope context: %w", err)
	}
	if err := validateHash(c.InputManifestSHA256, "input_manifest_sha256"); err != nil {
		return fmt.Errorf("retrieval scope context: %w", err)
	}
	if c.RootIndexPath != "wiki/index.md" || c.IndexTreePath != "state/index-tree.json" {
		return errors.New("retrieval scope context: invalid route entry")
	}
	return nil
}

type RetrievalContext struct {
	SchemaVersion int                    `json:"schema_version"`
	RetrievalID   string                 `json:"retrieval_id"`
	Project       *RetrievalScopeContext `json:"project"`
	Global        *RetrievalScopeContext `json:"global"`
}

func (c RetrievalContext) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("retrieval context: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(c.RetrievalID, "retrieval_id"); err != nil {
		return fmt.Errorf("retrieval context: %w", err)
	}
	if c.Project != nil {
		if err := c.Project.Validate(ScopeProject); err != nil {
			return err
		}
	}
	if c.Global != nil {
		if err := c.Global.Validate(ScopeGlobal); err != nil {
			return err
		}
	}
	return nil
}

type RetrievalContextRequest struct {
	RetrievalID    string
	ProjectStore   *FactStore
	ProjectScopeID string
	GlobalStore    *FactStore
	GlobalScopeID  string
	Now            time.Time
}

type LibrarianRequest struct {
	SchemaVersion      int              `json:"schema_version"`
	RetrievalID        string           `json:"retrieval_id"`
	MemoryContext      RetrievalContext `json:"memory_context"`
	TaskSummary        string           `json:"task_summary"`
	ExplicitMemoryRefs []MemoryRef      `json:"explicit_memory_refs"`
	ExcludedMemoryRefs []MemoryRef      `json:"excluded_memory_refs"`
}

func (r LibrarianRequest) Validate() error {
	if r.SchemaVersion != SchemaVersion || r.RetrievalID != r.MemoryContext.RetrievalID {
		return storeError(CodeLibrarianInvalidContext, "librarian request envelope is invalid")
	}
	if err := r.MemoryContext.Validate(); err != nil {
		return storeError(CodeLibrarianInvalidContext, "librarian request context is invalid")
	}
	if r.TaskSummary == "" || !utf8.ValidString(r.TaskSummary) || validateText(r.TaskSummary, maxLibrarianTextBytes, "task summary", true) != nil {
		return storeError(CodeLibrarianInvalidContext, "librarian task summary is invalid")
	}
	if len(r.ExplicitMemoryRefs)+len(r.ExcludedMemoryRefs) > maxLibrarianRefs {
		return storeError(CodeLibrarianInvalidContext, "librarian request has too many references")
	}
	seen := map[string]string{}
	for _, set := range []struct {
		name string
		refs []MemoryRef
	}{{"explicit", r.ExplicitMemoryRefs}, {"excluded", r.ExcludedMemoryRefs}} {
		for _, ref := range set.refs {
			if ref.Validate() != nil {
				return storeError(CodeLibrarianInvalidContext, "librarian request reference is invalid")
			}
			if (ref.Scope == ScopeProject && r.MemoryContext.Project == nil) || (ref.Scope == ScopeGlobal && r.MemoryContext.Global == nil) {
				return storeError(CodeLibrarianInvalidContext, "librarian request reference has no fixed generation")
			}
			key := librarianMemoryRefKey(ref)
			if previous, ok := seen[key]; ok {
				if previous != set.name {
					return storeError(CodeLibrarianInvalidContext, "librarian request reference is both explicit and excluded")
				}
				return storeError(CodeLibrarianInvalidContext, "librarian request reference is duplicated")
			}
			seen[key] = set.name
		}
	}
	return nil
}

func BuildRetrievalContext(ctx context.Context, req RetrievalContextRequest) (*RetrievalContext, error) {
	if err := validateID(req.RetrievalID, "retrieval_id"); err != nil || req.Now.IsZero() {
		return nil, storeError(CodeLibrarianInvalidContext, "retrieval context request is invalid")
	}
	out := &RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: req.RetrievalID}
	var err error
	if req.ProjectStore != nil {
		if validateID(req.ProjectScopeID, "project_scope_id") != nil {
			return nil, storeError(CodeLibrarianInvalidContext, "project scope identity is invalid")
		}
		out.Project, err = buildRetrievalScope(ctx, req.ProjectStore, ScopeProject, req.ProjectScopeID, req.RetrievalID+"_project", req.Now)
		if err != nil && ErrorCode(err) != CodeNotFound {
			return nil, err
		}
		if err == nil {
			if _, err = LoadLibrarianIndexTree(ctx, req.ProjectStore, *out.Project); err != nil {
				return nil, err
			}
			if _, err = readLibrarianFile(ctx, req.ProjectStore, out.Project.GenerationID, out.Project.RootIndexPath); err != nil {
				return nil, err
			}
		}
	}
	if req.GlobalStore != nil {
		if validateID(req.GlobalScopeID, "global_scope_id") != nil {
			return nil, storeError(CodeLibrarianInvalidContext, "global scope identity is invalid")
		}
		out.Global, err = buildRetrievalScope(ctx, req.GlobalStore, ScopeGlobal, req.GlobalScopeID, req.RetrievalID+"_global", req.Now)
		if err != nil && ErrorCode(err) != CodeNotFound {
			return nil, err
		}
		if err == nil {
			if _, err = LoadLibrarianIndexTree(ctx, req.GlobalStore, *out.Global); err != nil {
				return nil, err
			}
			if _, err = readLibrarianFile(ctx, req.GlobalStore, out.Global.GenerationID, out.Global.RootIndexPath); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func buildRetrievalScope(ctx context.Context, store *FactStore, scope Scope, scopeID, contextID string, now time.Time) (*RetrievalScopeContext, error) {
	ec, err := BuildEvaluationContext(ctx, store, EvaluationContextRequest{ContextID: contextID, Scope: scope, Now: now})
	if err != nil {
		return nil, err
	}
	result := &RetrievalScopeContext{Scope: scope, ScopeID: scopeID, RootIndexPath: "wiki/index.md", IndexTreePath: "state/index-tree.json"}
	if scope == ScopeProject {
		r := ec.ProjectGenerationRef
		result.GenerationID, result.InputManifestID, result.InputManifestSHA256 = r.GenerationID, r.InputManifestID, r.InputManifestSHA256
	} else {
		r := ec.GlobalGenerationRef
		result.GenerationID, result.InputManifestID, result.InputManifestSHA256 = r.GenerationID, r.InputManifestID, r.InputManifestSHA256
	}
	return result, nil
}

// LoadLibrarianIndexTree reads the machine index from the pinned generation,
// re-verifies the generation output and validates the tree against its exact
// Index Policy. It never reads CURRENT.
func LoadLibrarianIndexTree(ctx context.Context, store *FactStore, pinned RetrievalScopeContext) (*IndexTree, error) {
	if err := pinned.Validate(pinned.Scope); err != nil || !store.scopeMatches(pinned.Scope) {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned retrieval scope is invalid")
	}
	gs, ok := NewGenerationStore(store).(*generationStore)
	if !ok {
		return nil, storeError(CodeLibrarianInvalidContext, "generation reader is unavailable")
	}
	dir, err := gs.publishedGenDir(ctx, pinned.GenerationID)
	if err != nil {
		return nil, err
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil || doc.GenerationID != pinned.GenerationID {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned generation is unreadable")
	}
	if err := gs.verifyCompiledOutputIntegrity(ctx, dir, doc); err != nil {
		return nil, err
	}
	manifestData, err := store.Get(ctx, FactKindGenerationInputManifest, pinned.InputManifestID)
	if err != nil {
		return nil, err
	}
	manifest, err := DecodeStrict[GenerationInputManifest](manifestData)
	if err != nil || manifest.GenerationID != pinned.GenerationID || manifest.InputManifestSHA256 != pinned.InputManifestSHA256 {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned manifest is invalid")
	}
	data, err := readLibrarianFile(ctx, store, pinned.GenerationID, pinned.IndexTreePath)
	if err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned index tree is unreadable")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var tree IndexTree
	if err := dec.Decode(&tree); err != nil || dec.Decode(&struct{}{}) != io.EOF || tree.Scope != pinned.Scope || tree.PolicyRef == nil {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned index tree is invalid")
	}
	policy, err := NewPolicyStore(store).GetPolicy(ctx, *tree.PolicyRef)
	if err != nil || policy.Config.Index == nil {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned index policy is unavailable")
	}
	if err := validateIndexTree(&tree, *policy.Config.Index); err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "pinned index tree violates policy")
	}
	return &tree, nil
}

// ReadLibrarianIndex reads one machine or Markdown index page from the fixed
// generation. The path must be present in the pinned IndexTree.
func ReadLibrarianIndex(ctx context.Context, store *FactStore, pinned RetrievalScopeContext, relativePath string) ([]byte, error) {
	tree, err := LoadLibrarianIndexTree(ctx, store, pinned)
	if err != nil {
		return nil, err
	}
	if relativePath != pinned.IndexTreePath && !indexPathInTree(relativePath, tree) {
		return nil, storeError(CodeLibrarianInvalidContext, "requested index path is outside the fixed generation")
	}
	return readLibrarianFile(ctx, store, pinned.GenerationID, relativePath)
}

// ResolveLibrarianMemoryPage resolves a complete MemoryRef through the pinned
// IndexTree and reads its exact page. Frozen, archived and superseded entries
// are absent from normal IndexTree entries and therefore cannot be resolved.
func ResolveLibrarianMemoryPage(ctx context.Context, store *FactStore, pinned RetrievalScopeContext, ref MemoryRef) (IndexEntry, []byte, error) {
	if ref.Validate() != nil || ref.Scope != pinned.Scope {
		return IndexEntry{}, nil, storeError(CodeLibrarianInvalidContext, "requested memory reference is invalid")
	}
	tree, err := LoadLibrarianIndexTree(ctx, store, pinned)
	if err != nil {
		return IndexEntry{}, nil, err
	}
	for _, page := range tree.Pages {
		for _, entry := range page.Entries {
			if indexEntryMatchesRef(entry, ref) {
				data, err := readLibrarianFile(ctx, store, pinned.GenerationID, entry.PagePath)
				return entry, data, err
			}
		}
	}
	return IndexEntry{}, nil, storeError(CodeNotFound, "memory page is absent from the fixed generation")
}

func readLibrarianFile(ctx context.Context, store *FactStore, generationID, relativePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "librarian read cancelled")
	}
	if !safeGenerationRelativePath(relativePath) {
		return nil, storeError(CodePathUnsafe, "librarian path is unsafe")
	}
	components := append([]string{"generations", generationID}, strings.Split(relativePath, "/")...)
	path, err := secureJoin(store.root, components, false, true)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "librarian page is unreadable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCompiledOutputBytes {
		return nil, storeError(CodeLibrarianInvalidContext, "librarian page is unreadable")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxCompiledOutputBytes+1))
	if err != nil {
		return nil, storeError(CodeLibrarianInvalidContext, "librarian page is unreadable")
	}
	if int64(len(data)) > maxCompiledOutputBytes {
		return nil, storeError(CodeLibrarianInvalidContext, "librarian page exceeds the size limit")
	}
	return data, nil
}

func safeGenerationRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") || filepath.Clean(path) != path {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return strings.HasPrefix(path, "wiki/") || path == "state/index-tree.json" || strings.HasPrefix(path, "state/episodes/")
}

func indexPathInTree(path string, tree *IndexTree) bool {
	if tree == nil {
		return false
	}
	for _, page := range tree.Pages {
		if page.Path == path {
			return true
		}
	}
	return false
}

func indexEntryMatchesRef(entry IndexEntry, ref MemoryRef) bool {
	return entry.Scope == ref.Scope && entry.MemoryType == ref.MemoryType && entry.MemoryID == ref.MemoryID && entry.Revision == ref.Revision && entry.ContentSHA256 == ref.ContentSHA256 && entry.Lifecycle != LifecycleFrozen && entry.Lifecycle != LifecycleArchived && entry.Lifecycle != LifecycleSuperseded
}

type LibrarianStatus string

const (
	LibrarianStatusFound       LibrarianStatus = "found"
	LibrarianStatusNoCandidate LibrarianStatus = "no_candidate"
	LibrarianStatusUnknown     LibrarianStatus = "unknown"
	LibrarianStatusUnavailable LibrarianStatus = "unavailable"
)

type LibrarianCandidate struct {
	MemoryRef     MemoryRef `json:"memory_ref"`
	PagePath      string    `json:"page_path"`
	RelevanceRank int       `json:"relevance_rank"`
	Why           string    `json:"why"`
}

type LibrarianConflict struct {
	Project MemoryRef `json:"project"`
	Global  MemoryRef `json:"global"`
}

type LibrarianReceipt struct {
	SchemaVersion      int                  `json:"schema_version"`
	RetrievalID        string               `json:"retrieval_id"`
	MemoryContext      RetrievalContext     `json:"memory_context"`
	Status             LibrarianStatus      `json:"status"`
	RecommendedPages   []LibrarianCandidate `json:"recommended_pages"`
	OptionalPages      []LibrarianCandidate `json:"optional_pages"`
	Conflicts          []LibrarianConflict  `json:"conflicts"`
	VisitedIndexPaths  []string             `json:"visited_index_paths"`
	FrozenPagesUsed    []MemoryRef          `json:"frozen_pages_used"`
	RequiresParentRead bool                 `json:"requires_parent_read"`
}

func ValidateLibrarianReceipt(r LibrarianReceipt, trees map[Scope]*IndexTree) error {
	return validateLibrarianReceipt(r, trees, nil)
}

// ValidateLibrarianReceiptForRequest additionally enforces the request's
// explicit/excluded references and uses explicit references as the highest
// structural tie-break. It does not interpret task_summary or why.
func ValidateLibrarianReceiptForRequest(request LibrarianRequest, r LibrarianReceipt, trees map[Scope]*IndexTree) error {
	if err := request.Validate(); err != nil || request.RetrievalID != r.RetrievalID || !reflect.DeepEqual(request.MemoryContext, r.MemoryContext) {
		return storeError(CodeLibrarianInvalidReceipt, "librarian request and receipt do not match")
	}
	explicit, excluded := map[string]bool{}, map[string]bool{}
	for _, ref := range request.ExplicitMemoryRefs {
		explicit[librarianMemoryRefKey(ref)] = true
	}
	for _, ref := range request.ExcludedMemoryRefs {
		excluded[librarianMemoryRefKey(ref)] = true
	}
	for _, candidate := range append(append([]LibrarianCandidate{}, r.RecommendedPages...), r.OptionalPages...) {
		if excluded[librarianMemoryRefKey(candidate.MemoryRef)] {
			return storeError(CodeLibrarianInvalidReceipt, "librarian receipt includes an excluded memory")
		}
	}
	return validateLibrarianReceipt(r, trees, explicit)
}

func validateLibrarianReceipt(r LibrarianReceipt, trees map[Scope]*IndexTree, explicit map[string]bool) error {
	if r.SchemaVersion != SchemaVersion || r.RetrievalID != r.MemoryContext.RetrievalID || !r.RequiresParentRead {
		return storeError(CodeLibrarianInvalidReceipt, "librarian receipt envelope is invalid")
	}
	if err := r.MemoryContext.Validate(); err != nil {
		return storeError(CodeLibrarianInvalidReceipt, "librarian receipt context is invalid")
	}
	switch r.Status {
	case LibrarianStatusFound:
		if len(r.RecommendedPages)+len(r.OptionalPages) == 0 {
			return storeError(CodeLibrarianInvalidReceipt, "found receipt has no candidate")
		}
	case LibrarianStatusNoCandidate, LibrarianStatusUnknown, LibrarianStatusUnavailable:
		if len(r.RecommendedPages)+len(r.OptionalPages) != 0 {
			return storeError(CodeLibrarianInvalidReceipt, "non-found receipt has candidates")
		}
	default:
		return storeError(CodeLibrarianInvalidReceipt, "librarian receipt status is invalid")
	}
	if len(r.FrozenPagesUsed) != 0 {
		return storeError(CodeLibrarianInvalidReceipt, "normal retrieval cannot use frozen memory")
	}
	seen := map[string]bool{}
	for _, candidateGroup := range []struct {
		candidates  []LibrarianCandidate
		recommended bool
	}{{r.RecommendedPages, true}, {r.OptionalPages, false}} {
		lastRank := 0
		for _, candidate := range candidateGroup.candidates {
			if candidate.RelevanceRank < 1 || candidate.RelevanceRank > lastRank+1 || len(candidate.Why) == 0 || !utf8.ValidString(candidate.Why) || validateText(candidate.Why, maxLibrarianTextBytes, "candidate reason", true) != nil {
				return storeError(CodeLibrarianInvalidReceipt, "librarian candidate is invalid")
			}
			if candidate.RelevanceRank > lastRank {
				lastRank = candidate.RelevanceRank
			}
			if err := candidate.MemoryRef.Validate(); err != nil {
				return storeError(CodeLibrarianInvalidReceipt, "librarian candidate reference is invalid")
			}
			key := librarianMemoryRefKey(candidate.MemoryRef)
			if seen[key] {
				return storeError(CodeLibrarianInvalidReceipt, "librarian candidate is duplicated")
			}
			seen[key] = true
			entry, ok := findCandidateEntry(candidate, trees[candidate.MemoryRef.Scope])
			if !ok {
				return storeError(CodeLibrarianInvalidReceipt, "librarian candidate is absent from the fixed generation")
			}
			if candidateGroup.recommended && entry.Freshness == FreshnessNeedsRevalidation {
				return storeError(CodeLibrarianInvalidReceipt, "memory needing revalidation cannot be recommended")
			}
		}
	}
	for _, group := range [][]LibrarianCandidate{r.RecommendedPages, r.OptionalPages} {
		ordered, err := orderLibrarianCandidates(r.RetrievalID, r.MemoryContext, group, trees, explicit)
		if err != nil {
			return err
		}
		for i := range ordered {
			if librarianMemoryRefKey(ordered[i].MemoryRef) != librarianMemoryRefKey(group[i].MemoryRef) {
				return storeError(CodeLibrarianInvalidReceipt, "librarian candidates are not deterministically ordered")
			}
		}
	}
	visited := map[string]bool{}
	for _, path := range r.VisitedIndexPaths {
		if !safeLibrarianPath(path) || visited[path] {
			return storeError(CodeLibrarianInvalidReceipt, "visited index path is invalid")
		}
		visited[path] = true
	}
	conflictSeen := map[string]bool{}
	for _, conflict := range r.Conflicts {
		if conflict.Project.Scope != ScopeProject || conflict.Global.Scope != ScopeGlobal || conflict.Project.Validate() != nil || conflict.Global.Validate() != nil {
			return storeError(CodeLibrarianInvalidReceipt, "librarian conflict is invalid")
		}
		if !memoryRefInTree(conflict.Project, trees[ScopeProject]) || !memoryRefInTree(conflict.Global, trees[ScopeGlobal]) {
			return storeError(CodeLibrarianInvalidReceipt, "librarian conflict is absent from the fixed generation")
		}
		projectKey, globalKey := librarianMemoryRefKey(conflict.Project), librarianMemoryRefKey(conflict.Global)
		if conflictSeen[projectKey] || conflictSeen[globalKey] || seen[projectKey] || seen[globalKey] {
			return storeError(CodeLibrarianInvalidReceipt, "librarian conflict reference is duplicated")
		}
		conflictSeen[projectKey], conflictSeen[globalKey] = true, true
	}
	return nil
}

func memoryRefInTree(ref MemoryRef, tree *IndexTree) bool {
	if tree == nil {
		return false
	}
	for _, page := range tree.Pages {
		for _, entry := range page.Entries {
			if indexEntryMatchesRef(entry, ref) {
				return true
			}
		}
	}
	return false
}

func OrderLibrarianCandidates(retrievalID string, memoryContext RetrievalContext, candidates []LibrarianCandidate, trees map[Scope]*IndexTree) ([]LibrarianCandidate, error) {
	return orderLibrarianCandidates(retrievalID, memoryContext, candidates, trees, nil)
}

func orderLibrarianCandidates(retrievalID string, memoryContext RetrievalContext, candidates []LibrarianCandidate, trees map[Scope]*IndexTree, explicit map[string]bool) ([]LibrarianCandidate, error) {
	result := append([]LibrarianCandidate{}, candidates...)
	entries := map[string]IndexEntry{}
	for _, candidate := range result {
		entry, ok := findCandidateEntry(candidate, trees[candidate.MemoryRef.Scope])
		if !ok {
			return nil, storeError(CodeLibrarianInvalidReceipt, "librarian candidate is absent from the fixed generation")
		}
		entries[librarianMemoryRefKey(candidate.MemoryRef)] = entry
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.RelevanceRank != b.RelevanceRank {
			return a.RelevanceRank < b.RelevanceRank
		}
		if explicit[librarianMemoryRefKey(a.MemoryRef)] != explicit[librarianMemoryRefKey(b.MemoryRef)] {
			return explicit[librarianMemoryRefKey(a.MemoryRef)]
		}
		ea, eb := entries[librarianMemoryRefKey(a.MemoryRef)], entries[librarianMemoryRefKey(b.MemoryRef)]
		if rank := librarianScopeRank(ea); rank != librarianScopeRank(eb) {
			return rank < librarianScopeRank(eb)
		}
		if healthRank[ea.Health] != healthRank[eb.Health] {
			return healthRank[ea.Health] < healthRank[eb.Health]
		}
		freshRank := map[Freshness]int{FreshnessFresh: 0, FreshnessAging: 1, FreshnessNeedsRevalidation: 2}
		if freshRank[ea.Freshness] != freshRank[eb.Freshness] {
			return freshRank[ea.Freshness] < freshRank[eb.Freshness]
		}
		return librarianTieKey(retrievalID, generationForScope(memoryContext, ea.Scope), ea.MemoryID) < librarianTieKey(retrievalID, generationForScope(memoryContext, eb.Scope), eb.MemoryID)
	})
	return result, nil
}

func librarianScopeRank(e IndexEntry) int {
	if e.Scope == ScopeProject {
		if e.Pinned {
			return 0
		}
		if e.Lifecycle == LifecycleActive {
			return 1
		}
		return 4
	}
	if e.Pinned {
		return 2
	}
	if e.Lifecycle == LifecycleActive {
		return 3
	}
	return 5
}

func generationForScope(c RetrievalContext, scope Scope) string {
	if scope == ScopeProject && c.Project != nil {
		return c.Project.GenerationID
	}
	if scope == ScopeGlobal && c.Global != nil {
		return c.Global.GenerationID
	}
	return "none"
}

func librarianTieKey(retrievalID, generationID, memoryID string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(retrievalID+generationID+memoryID)))
}

func candidateInTree(candidate LibrarianCandidate, tree *IndexTree) bool {
	_, ok := findCandidateEntry(candidate, tree)
	return ok
}

func findCandidateEntry(candidate LibrarianCandidate, tree *IndexTree) (IndexEntry, bool) {
	if tree == nil {
		return IndexEntry{}, false
	}
	for _, page := range tree.Pages {
		for _, entry := range page.Entries {
			ref := candidate.MemoryRef
			if entry.Scope == ref.Scope && entry.MemoryType == ref.MemoryType && entry.MemoryID == ref.MemoryID && entry.Revision == ref.Revision && entry.ContentSHA256 == ref.ContentSHA256 && entry.PagePath == candidate.PagePath && entry.Lifecycle != LifecycleFrozen && entry.Lifecycle != LifecycleArchived && entry.Lifecycle != LifecycleSuperseded {
				return entry, true
			}
		}
	}
	return IndexEntry{}, false
}

func librarianMemoryRefKey(r MemoryRef) string {
	return string(r.Scope) + "\x00" + string(r.MemoryType) + "\x00" + r.MemoryID + "\x00" + itoa(r.Revision) + "\x00" + r.ContentSHA256
}

func safeLibrarianPath(path string) bool {
	return path == "wiki/index.md" || (strings.HasPrefix(path, "wiki/index/") && strings.HasSuffix(path, "/index.md") && !strings.Contains(path, "..") && !strings.Contains(path, "\\"))
}
