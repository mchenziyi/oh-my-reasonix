package memory

import (
	"encoding/json"
	"testing"
)

func indexState(id string, mt MemoryType) DerivedMemoryState {
	return DerivedMemoryState{
		Scope: ScopeProject, MemoryID: id, MemoryType: mt,
		CanonicalKey: "key-" + id, Revision: 1, ContentSHA256: testHash,
		Lifecycle: LifecycleProbation, Health: HealthHealthy, Freshness: FreshnessFresh,
	}
}

func indexPolicy(entries, bytes, depth int) PolicyConfigIndex {
	return PolicyConfigIndex{
		MaxEntriesPerPage: entries, MaxPageBytes: bytes, MaxShardDepth: depth,
		SplitOrder:     []string{"component", "operation", "memory_type", "stable_id_prefix"},
		OverflowBucket: "other", Version: 1,
	}
}

func TestCompileIndexTreeDeterministicFanout(t *testing.T) {
	states := []DerivedMemoryState{
		indexState("mem_a1", MemoryTypeStrategy), indexState("mem_b1", MemoryTypePattern),
		indexState("mem_c1", MemoryTypeDecision), indexState("mem_d1", MemoryTypeStrategy),
	}
	p := indexPolicy(3, 4096, 4)
	a, err := CompileIndexTree(ScopeProject, states, p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{states[3], states[1], states[0], states[2]}, p)
	if err != nil {
		t.Fatal(err)
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ab) != string(bb) {
		t.Fatal("index tree must be byte-stable across input order")
	}
	if len(a.Root.Entries) != 0 || len(a.Root.Routes) < 2 {
		t.Fatalf("root must contain routes only after fanout: %+v", a.Root)
	}
	if len(a.Pages) < 3 {
		t.Fatalf("machine view must contain root and leaf pages: %d", len(a.Pages))
	}
}

func TestCompileIndexTreeNoUnboundedOverflow(t *testing.T) {
	states := []DerivedMemoryState{
		indexState("mem_aa", MemoryTypeStrategy),
		indexState("mem_ab", MemoryTypeStrategy),
		indexState("mem_ac", MemoryTypeStrategy),
	}
	_, err := CompileIndexTree(ScopeProject, states, indexPolicy(1, 4096, 1))
	if ErrorCode(err) != CodeIndexPolicyUnsatisfied {
		t.Fatalf("unbounded overflow must fail closed, got %v", err)
	}
}

func TestCompileIndexTreeCountsExcludedStates(t *testing.T) {
	active := indexState("mem_a", MemoryTypeStrategy)
	frozen := indexState("mem_f", MemoryTypeStrategy)
	frozen.Lifecycle = LifecycleFrozen
	archived := indexState("mem_r", MemoryTypeStrategy)
	archived.Lifecycle = LifecycleArchived
	tree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{frozen, active, archived}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	if tree.FrozenCount != 1 || tree.ArchivedCount != 1 || len(tree.Root.Entries) != 1 {
		t.Fatalf("unexpected exclusion counts: %+v", tree)
	}
}

func TestCompileIndexTreeUTF8ByteLimit(t *testing.T) {
	state := indexState("mem_utf8", MemoryTypeStrategy)
	state.CanonicalKey = "中文中文中文中文"
	_, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{state}, indexPolicy(10, 64, 4))
	if ErrorCode(err) != CodeIndexPolicyUnsatisfied {
		t.Fatalf("oversized single entry must fail, got %v", err)
	}
}

func TestDiagnoseIndexTreeDetectsDrift(t *testing.T) {
	tree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{indexState("mem_a", MemoryTypeStrategy)}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(tree)
	if got := DiagnoseIndexTree(data, indexPolicy(10, 4096, 4)); len(got) != 0 {
		t.Fatalf("clean tree diagnosed: %+v", got)
	}
	var drifted IndexTree
	if err := json.Unmarshal(data, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted.Pages[0].EntryCount++
	data, _ = json.Marshal(drifted)
	if got := DiagnoseIndexTree(data, indexPolicy(10, 4096, 4)); len(got) != 1 || got[0].Code != CodeIndexPolicyUnsatisfied {
		t.Fatalf("drift not diagnosed: %+v", got)
	}
}

func TestDiagnoseIndexTreeRejectsUnreachablePage(t *testing.T) {
	tree, err := CompileIndexTree(ScopeProject, []DerivedMemoryState{indexState("mem_a", MemoryTypeStrategy)}, indexPolicy(10, 4096, 4))
	if err != nil {
		t.Fatal(err)
	}
	tree.Pages = append(tree.Pages, &IndexNode{Path: "wiki/index/orphan/index.md", Depth: 1})
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got := DiagnoseIndexTree(data, indexPolicy(10, 4096, 4)); len(got) != 1 || got[0].Code != CodeIndexPolicyUnsatisfied {
		t.Fatalf("unreachable page must be rejected: %+v", got)
	}
}

func TestCompileIndexTreeLargeFixtureNoLoss(t *testing.T) {
	states := make([]DerivedMemoryState, 0, 500)
	for i := 0; i < 500; i++ {
		id := "mem_large_" + itoa(1000+i)
		states = append(states, indexState(id, MemoryTypeStrategy))
	}
	tree, err := CompileIndexTree(ScopeProject, states, indexPolicy(200, 65536, 4))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, page := range tree.Pages {
		if len(page.Entries) > 200 || len(page.Routes) > 200 || len(renderIndexMarkdown(page)) > 65536 {
			t.Fatalf("page exceeds policy: %+v", page)
		}
		for _, entry := range page.Entries {
			if seen[entry.MemoryID] {
				t.Fatalf("duplicate %s", entry.MemoryID)
			}
			seen[entry.MemoryID] = true
		}
	}
	if len(seen) != 500 {
		t.Fatalf("lost entries: got %d", len(seen))
	}
}
