package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func retrievalScope(scope Scope, generation string) *RetrievalScopeContext {
	return &RetrievalScopeContext{
		Scope: scope, ScopeID: "scope_" + string(scope), GenerationID: generation,
		InputManifestID: generation, InputManifestSHA256: testHash,
		RootIndexPath: "wiki/index.md", IndexTreePath: "state/index-tree.json",
	}
}

func TestRetrievalContextValidation(t *testing.T) {
	valid := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_01K", Project: retrievalScope(ScopeProject, "gen_project")}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	globalOnly := valid
	globalOnly.Project = nil
	globalOnly.Global = retrievalScope(ScopeGlobal, "gen_global")
	if err := globalOnly.Validate(); err != nil {
		t.Fatal(err)
	}
	empty := valid
	empty.Project = nil
	if err := empty.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := valid
	bad.Project.Scope = ScopeGlobal
	if err := bad.Validate(); err == nil {
		t.Fatal("project slot accepted a global scope")
	}
}

func TestLibrarianRequestValidation(t *testing.T) {
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_01K", Project: retrievalScope(ScopeProject, "gen_project")}
	ref := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01K", Revision: 1, ContentSHA256: testHash}
	request := LibrarianRequest{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, TaskSummary: "fix the failing regression", ExplicitMemoryRefs: []MemoryRef{ref}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.ExcludedMemoryRefs = []MemoryRef{ref}
	if err := request.Validate(); ErrorCode(err) != CodeLibrarianInvalidContext {
		t.Fatalf("explicit and excluded overlap must fail: %v", err)
	}
	request.ExcludedMemoryRefs = nil
	request.TaskSummary = ""
	if err := request.Validate(); ErrorCode(err) != CodeLibrarianInvalidContext {
		t.Fatalf("empty task summary must fail: %v", err)
	}
}

func TestLibrarianReceiptRejectsFrozenAndWrongPage(t *testing.T) {
	active := indexState("mem_active", MemoryTypeStrategy)
	frozen := indexState("mem_frozen", MemoryTypeStrategy)
	frozen.Lifecycle = LifecycleFrozen
	tree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{active, frozen}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_01K", Project: retrievalScope(ScopeProject, "gen_project")}
	receipt := LibrarianReceipt{
		SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx,
		Status: LibrarianStatusFound, RequiresParentRead: true,
		RecommendedPages: []LibrarianCandidate{{MemoryRef: memoryRefFromState(active), PagePath: "wiki/strategies/key-mem_active.md", RelevanceRank: 1, Why: "relevant"}},
	}
	// The compiler-generated path is the only accepted path.
	receipt.RecommendedPages[0].PagePath = tree.Root.Entries[0].PagePath
	if err := ValidateLibrarianReceipt(receipt, map[Scope]*IndexTree{ScopeProject: tree}); err != nil {
		t.Fatal(err)
	}
	receipt.RecommendedPages[0].PagePath = "wiki/strategies/wrong.md"
	if err := ValidateLibrarianReceipt(receipt, map[Scope]*IndexTree{ScopeProject: tree}); ErrorCode(err) != CodeLibrarianInvalidReceipt {
		t.Fatalf("wrong page must fail closed: %v", err)
	}
	receipt.RecommendedPages[0] = LibrarianCandidate{MemoryRef: memoryRefFromState(frozen), PagePath: "wiki/strategies/key-mem_frozen.md", RelevanceRank: 1, Why: "frozen"}
	if err := ValidateLibrarianReceipt(receipt, map[Scope]*IndexTree{ScopeProject: tree}); ErrorCode(err) != CodeLibrarianInvalidReceipt {
		t.Fatalf("frozen candidate must fail closed: %v", err)
	}
}

func TestLibrarianReceiptStatusMatrix(t *testing.T) {
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_01K"}
	for _, status := range []LibrarianStatus{LibrarianStatusNoCandidate, LibrarianStatusUnknown, LibrarianStatusUnavailable} {
		r := LibrarianReceipt{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, Status: status, RequiresParentRead: true}
		if err := ValidateLibrarianReceipt(r, nil); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
	found := LibrarianReceipt{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, Status: LibrarianStatusFound, RequiresParentRead: true}
	if err := ValidateLibrarianReceipt(found, nil); ErrorCode(err) != CodeLibrarianInvalidReceipt {
		t.Fatalf("empty found must fail: %v", err)
	}
}

func TestLibrarianCandidateOrderUsesRelevanceThenScope(t *testing.T) {
	projectState := indexState("mem_project", MemoryTypeStrategy)
	globalState := indexState("mem_global", MemoryTypeStrategy)
	globalState.Scope = ScopeGlobal
	projectTree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{projectState}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	globalTree, err := CompileIndexTree(ScopeGlobal, []DerivedMemoryState{globalState}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_order", Project: retrievalScope(ScopeProject, "gen_project"), Global: retrievalScope(ScopeGlobal, "gen_global")}
	candidates := []LibrarianCandidate{
		{MemoryRef: memoryRefFromState(globalState), PagePath: globalTree.Root.Entries[0].PagePath, RelevanceRank: 1, Why: "same semantic rank"},
		{MemoryRef: memoryRefFromState(projectState), PagePath: projectTree.Root.Entries[0].PagePath, RelevanceRank: 1, Why: "same semantic rank"},
	}
	ordered, err := OrderLibrarianCandidates(ctx.RetrievalID, ctx, candidates, map[Scope]*IndexTree{ScopeProject: projectTree, ScopeGlobal: globalTree})
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].MemoryRef.Scope != ScopeProject {
		t.Fatalf("project candidate must win a structural tie: %+v", ordered)
	}
}

func TestLibrarianReceiptForRequestRejectsExcludedAndPrioritizesExplicit(t *testing.T) {
	projectState := indexState("mem_project", MemoryTypeStrategy)
	globalState := indexState("mem_global", MemoryTypeStrategy)
	globalState.Scope = ScopeGlobal
	projectTree, _ := CompileIndexTree(ScopeProject, []DerivedMemoryState{projectState}, indexPolicy(10, 4096, 4))
	globalTree, _ := CompileIndexTree(ScopeGlobal, []DerivedMemoryState{globalState}, indexPolicy(10, 4096, 4))
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_request", Project: retrievalScope(ScopeProject, "gen_project"), Global: retrievalScope(ScopeGlobal, "gen_global")}
	projectCandidate := LibrarianCandidate{MemoryRef: memoryRefFromState(projectState), PagePath: projectTree.Root.Entries[0].PagePath, RelevanceRank: 1, Why: "project"}
	globalCandidate := LibrarianCandidate{MemoryRef: memoryRefFromState(globalState), PagePath: globalTree.Root.Entries[0].PagePath, RelevanceRank: 1, Why: "explicit"}
	request := LibrarianRequest{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, TaskSummary: "test explicit ordering", ExplicitMemoryRefs: []MemoryRef{globalCandidate.MemoryRef}}
	receipt := LibrarianReceipt{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, Status: LibrarianStatusFound, RecommendedPages: []LibrarianCandidate{globalCandidate, projectCandidate}, RequiresParentRead: true}
	trees := map[Scope]*IndexTree{ScopeProject: projectTree, ScopeGlobal: globalTree}
	if err := ValidateLibrarianReceiptForRequest(request, receipt, trees); err != nil {
		t.Fatalf("explicit memory must win a structural tie: %v", err)
	}
	request.ExplicitMemoryRefs = nil
	request.ExcludedMemoryRefs = []MemoryRef{globalCandidate.MemoryRef}
	if err := ValidateLibrarianReceiptForRequest(request, receipt, trees); ErrorCode(err) != CodeLibrarianInvalidReceipt {
		t.Fatalf("excluded memory must fail closed: %v", err)
	}
}

func TestBuildRetrievalContextPinsCurrentWithoutInventingGlobal(t *testing.T) {
	projectRoot := tempRoot(t)
	tx, project, _ := commitOKFGeneration(t, projectRoot, "librarian_project", nil)
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	got, err := BuildRetrievalContext(context.Background(), RetrievalContextRequest{
		RetrievalID: "retrieval_pair", ProjectStore: project, ProjectScopeID: "scope_project_01K", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project == nil || got.Global != nil || got.Project.Scope != ScopeProject {
		t.Fatalf("generation pair not pinned: %+v", got)
	}
	b, err := json.Marshal(got)
	if err != nil || len(b) == 0 {
		t.Fatal("context must encode")
	}
	tree, err := LoadLibrarianIndexTree(context.Background(), project, *got.Project)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Scope != ScopeProject || tree.PolicyRef == nil {
		t.Fatalf("pinned tree not loaded: %+v", tree)
	}
	if len(tree.Root.Entries) == 0 {
		t.Fatal("fixture must expose a memory page")
	}
	entry := tree.Root.Entries[0]
	ref := MemoryRef{Scope: entry.Scope, MemoryType: entry.MemoryType, MemoryID: entry.MemoryID, Revision: entry.Revision, ContentSHA256: entry.ContentSHA256}
	resolved, page, err := ResolveLibrarianMemoryPage(context.Background(), project, *got.Project, ref)
	if err != nil || len(page) == 0 || resolved.PagePath != entry.PagePath {
		t.Fatalf("fixed memory page not resolved: %v %+v", err, resolved)
	}
	index, err := ReadLibrarianIndex(context.Background(), project, *got.Project, "wiki/index.md")
	if err != nil || len(index) == 0 {
		t.Fatalf("fixed root index not read: %v", err)
	}
	if _, err := ReadLibrarianIndex(context.Background(), project, *got.Project, "../CURRENT"); ErrorCode(err) != CodeLibrarianInvalidContext {
		t.Fatalf("path traversal must fail before reading: %v", err)
	}
	base := tx.GenerationID
	commitOKFGeneration(t, projectRoot, "librarian_project_next", &base)
	if _, err := LoadLibrarianIndexTree(context.Background(), project, *got.Project); err != nil {
		t.Fatalf("pinned retrieval drifted after CURRENT changed: %v", err)
	}
}

func TestBuildRetrievalContextRequiresCallerScopeIdentity(t *testing.T) {
	_, project, _ := commitOKFGeneration(t, tempRoot(t), "librarian_scope_id", nil)
	_, err := BuildRetrievalContext(context.Background(), RetrievalContextRequest{
		RetrievalID: "retrieval_scope", ProjectStore: project, Now: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeLibrarianInvalidContext {
		t.Fatalf("missing project scope identity must fail closed: %v", err)
	}
}

func TestLibrarianReaderRejectsSymlinkPage(t *testing.T) {
	tx, project, _ := commitOKFGeneration(t, tempRoot(t), "librarian_symlink", nil)
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	ctx, err := BuildRetrievalContext(context.Background(), RetrievalContextRequest{
		RetrievalID: "retrieval_symlink", ProjectStore: project, ProjectScopeID: "scope_project_01K", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadLibrarianIndexTree(context.Background(), project, *ctx.Project)
	if err != nil || len(tree.Root.Entries) == 0 {
		t.Fatalf("fixture index unavailable: %v", err)
	}
	entry := tree.Root.Entries[0]
	pagePath := filepath.Join(project.root, "generations", tx.GenerationID, filepath.FromSlash(entry.PagePath))
	external := filepath.Join(tempRoot(t), "external.md")
	original, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pagePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, pagePath); err != nil {
		t.Fatal(err)
	}
	ref := MemoryRef{Scope: entry.Scope, MemoryType: entry.MemoryType, MemoryID: entry.MemoryID, Revision: entry.Revision, ContentSHA256: entry.ContentSHA256}
	if _, _, err := ResolveLibrarianMemoryPage(context.Background(), project, *ctx.Project, ref); err == nil {
		t.Fatal("symlink page must fail closed even when target bytes match")
	}
	after, err := os.ReadFile(external)
	if err != nil || string(after) != string(original) {
		t.Fatal("external symlink target was modified")
	}
}

func TestLibrarianRecommendedRejectsNeedsRevalidation(t *testing.T) {
	state := indexState("mem_revalidate", MemoryTypeStrategy)
	state.Freshness = FreshnessNeedsRevalidation
	tree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{state}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	ctx := RetrievalContext{SchemaVersion: SchemaVersion, RetrievalID: "retrieval_01K", Project: retrievalScope(ScopeProject, "gen_project")}
	candidate := LibrarianCandidate{MemoryRef: memoryRefFromState(state), PagePath: tree.Root.Entries[0].PagePath, RelevanceRank: 1, Why: "requires revalidation"}
	receipt := LibrarianReceipt{SchemaVersion: SchemaVersion, RetrievalID: ctx.RetrievalID, MemoryContext: ctx, Status: LibrarianStatusFound, RecommendedPages: []LibrarianCandidate{candidate}, RequiresParentRead: true}
	if err := ValidateLibrarianReceipt(receipt, map[Scope]*IndexTree{ScopeProject: tree}); ErrorCode(err) != CodeLibrarianInvalidReceipt {
		t.Fatalf("needs_revalidation must not be recommended: %v", err)
	}
	receipt.RecommendedPages = nil
	receipt.OptionalPages = []LibrarianCandidate{candidate}
	if err := ValidateLibrarianReceipt(receipt, map[Scope]*IndexTree{ScopeProject: tree}); err != nil {
		t.Fatalf("needs_revalidation may be optional: %v", err)
	}
}

func memoryRefFromState(s DerivedMemoryState) MemoryRef {
	return MemoryRef{Scope: s.Scope, MemoryType: s.MemoryType, MemoryID: s.MemoryID, Revision: s.Revision, ContentSHA256: s.ContentSHA256}
}
