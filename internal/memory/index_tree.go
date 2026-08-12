package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// IndexTree is a deterministic, derived routing view. It is never persisted
// as a fact and can be rebuilt from the same states and Index Policy.
type IndexTree struct {
	SchemaVersion int          `json:"schema_version"`
	Scope         Scope        `json:"scope"`
	PolicyRef     *PolicyRef   `json:"policy_ref,omitempty"`
	Root          *IndexNode   `json:"root"`
	Pages         []*IndexNode `json:"pages"`
	FrozenCount   int          `json:"frozen_count"`
	ArchivedCount int          `json:"archived_count"`
}

type IndexNode struct {
	Path       string       `json:"path"`
	Depth      int          `json:"depth"`
	EntryCount int          `json:"entry_count"`
	ByteCount  int          `json:"byte_count"`
	Entries    []IndexEntry `json:"entries,omitempty"`
	Routes     []IndexRoute `json:"routes,omitempty"`
	children   []*IndexNode
}

type IndexRoute struct {
	Dimension  string `json:"dimension"`
	Value      string `json:"value"`
	EntryCount int    `json:"entry_count"`
	Path       string `json:"path"`
}

// CompileIndexTree builds the bounded index topology. The byte bound is
// measured from the exact canonical JSON representation of each node; OKF
// renderers may apply an additional, stricter bound to their final pages.
func CompileIndexTree(scope Scope, states []DerivedMemoryState, policy PolicyConfigIndex) (*IndexTree, error) {
	return compileIndexTree(scope, states, policy, nil, nil)
}

func compileIndexTree(scope Scope, states []DerivedMemoryState, policy PolicyConfigIndex, policyRef *PolicyRef, pagePaths map[string]string) (*IndexTree, error) {
	if err := scope.Validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid index scope")
	}
	if err := policy.validate(); err != nil {
		return nil, storeError(CodeDerivedInvalidInput, "invalid index policy")
	}
	if policyRef != nil {
		if err := policyRef.Validate(); err != nil || policyRef.PolicyType != PolicyTypeIndex {
			return nil, storeError(CodeDerivedInvalidInput, "invalid index policy reference")
		}
		copyRef := *policyRef
		policyRef = &copyRef
	}
	tree := &IndexTree{SchemaVersion: SchemaVersion, Scope: scope, PolicyRef: policyRef}
	entries := make([]IndexEntry, 0, len(states))
	seen := map[string]bool{}
	for _, state := range states {
		if state.Scope != scope {
			return nil, storeError(CodeScopeMismatch, "index state crosses scope")
		}
		switch state.Lifecycle {
		case LifecycleFrozen:
			tree.FrozenCount++
			continue
		case LifecycleArchived:
			tree.ArchivedCount++
			continue
		}
		entry, ok := indexEntry(state)
		if !ok {
			return nil, storeError(CodeDerivedInvalidInput, "index entry is not renderable")
		}
		if path := pagePaths[state.MemoryID+"\x00"+itoa(state.Revision)]; path != "" {
			entry.PagePath = path
		}
		key := string(entry.Scope) + "\x00" + entry.MemoryID + "\x00" + itoa(entry.Revision)
		if seen[key] {
			return nil, storeError(CodeDerivedInvalidInput, "duplicate index entry")
		}
		seen[key] = true
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return indexEntryLess(entries[i], entries[j]) })
	root, err := compileIndexNode(entries, policy, "wiki/index.md", 0, 0, 0)
	if err != nil {
		return nil, err
	}
	tree.Root = root
	collectIndexPages(root, &tree.Pages)
	sort.Slice(tree.Pages, func(i, j int) bool { return tree.Pages[i].Path < tree.Pages[j].Path })
	return tree, nil
}

func collectIndexPages(node *IndexNode, pages *[]*IndexNode) {
	if node == nil {
		return
	}
	*pages = append(*pages, node)
	for _, child := range node.children {
		collectIndexPages(child, pages)
	}
}

func compileIndexNode(entries []IndexEntry, policy PolicyConfigIndex, path string, depth, cursor, prefixLen int) (*IndexNode, error) {
	leaf := &IndexNode{Path: path, Depth: depth, EntryCount: len(entries), Entries: append([]IndexEntry{}, entries...)}
	if fitsIndexNode(leaf, policy) {
		return leaf, nil
	}
	if depth >= policy.MaxShardDepth {
		return nil, storeError(CodeIndexPolicyUnsatisfied, "index exceeds the configured shard depth")
	}

	for i := cursor; i < len(policy.SplitOrder); i++ {
		dimension := policy.SplitOrder[i]
		usedPrefix := prefixLen
		buckets := indexBuckets(entries, dimension, policy.OverflowBucket, usedPrefix+1)
		if dimension == "stable_id_prefix" {
			for len(buckets) < 2 && usedPrefix < maxMemoryIDLength(entries) {
				usedPrefix++
				buckets = indexBuckets(entries, dimension, policy.OverflowBucket, usedPrefix+1)
			}
		}
		if len(buckets) < 2 {
			continue
		}
		values := make([]string, 0, len(buckets))
		for value := range buckets {
			values = append(values, value)
		}
		sort.Strings(values)
		branch := &IndexNode{Path: path, Depth: depth, EntryCount: len(entries)}
		nextCursor := i + 1
		if dimension == "stable_id_prefix" {
			// Prefix fan-out may reuse the same final dimension at the next
			// depth with one additional character.
			nextCursor = i
		}
		for _, value := range values {
			childPath := indexChildPath(path, dimension, value)
			child, err := compileIndexNode(buckets[value], policy, childPath, depth+1, nextCursor, usedPrefix+1)
			if err != nil {
				return nil, err
			}
			branch.Routes = append(branch.Routes, IndexRoute{Dimension: dimension, Value: value, EntryCount: child.EntryCount, Path: child.Path})
			branch.children = append(branch.children, child)
		}
		if !fitsIndexNode(branch, policy) {
			return nil, storeError(CodeIndexPolicyUnsatisfied, "index routing page exceeds the configured byte limit")
		}
		return branch, nil
	}
	return nil, storeError(CodeIndexPolicyUnsatisfied, "index entries cannot be split within the policy")
}

func maxMemoryIDLength(entries []IndexEntry) int {
	max := 0
	for _, entry := range entries {
		if n := len(strings.TrimPrefix(entry.MemoryID, "mem_")); n > max {
			max = n
		}
	}
	return max
}

func indexBuckets(entries []IndexEntry, dimension, other string, prefixLen int) map[string][]IndexEntry {
	result := map[string][]IndexEntry{}
	for _, entry := range entries {
		value := other
		switch dimension {
		case "memory_type":
			value = string(entry.MemoryType)
		case "stable_id_prefix":
			id := strings.TrimPrefix(entry.MemoryID, "mem_")
			if id != "" {
				n := prefixLen
				if n > len(id) {
					n = len(id)
				}
				value = id[:n]
			}
		}
		result[value] = append(result[value], entry)
	}
	return result
}

func indexChildPath(parent, dimension, value string) string {
	base := strings.TrimSuffix(parent, "/index.md")
	if parent == "wiki/index.md" {
		base = "wiki/index"
	}
	return base + "/" + dimension + "/" + value + "/index.md"
}

func fitsIndexNode(node *IndexNode, policy PolicyConfigIndex) bool {
	items := len(node.Entries)
	if len(node.Routes) > items {
		items = len(node.Routes)
	}
	if items > policy.MaxEntriesPerPage {
		return false
	}
	for i := 0; i < 8; i++ {
		data, err := json.Marshal(node)
		if err != nil {
			return false
		}
		if node.ByteCount == len(data) {
			return len(data) <= policy.MaxPageBytes && len(renderIndexMarkdown(node)) <= policy.MaxPageBytes
		}
		node.ByteCount = len(data)
	}
	data, _ := json.Marshal(node)
	return len(data) <= policy.MaxPageBytes && len(renderIndexMarkdown(node)) <= policy.MaxPageBytes
}

func compileIndexOutputs(outputs map[string][]byte, tree *IndexTree, policy PolicyConfigIndex) error {
	if tree == nil || tree.Root == nil {
		return storeError(CodeOKFCompileError, "index tree is missing")
	}
	if tree.PolicyRef == nil {
		return storeError(CodeOKFCompileError, "index policy reference is missing")
	}
	if err := validateIndexTree(tree, policy); err != nil {
		return err
	}
	var visit func(*IndexNode) error
	visit = func(node *IndexNode) error {
		page := renderIndexMarkdown(node)
		if len(page) > policy.MaxPageBytes {
			return storeError(CodeIndexPolicyUnsatisfied, "rendered index page exceeds the configured byte limit")
		}
		if _, exists := outputs[node.Path]; exists {
			return storeError(CodeOKFCompileError, "index output path collides")
		}
		outputs[node.Path] = page
		for _, child := range node.children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(tree.Root); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return storeError(CodeOKFCompileError, "index tree cannot be encoded")
	}
	outputs["state/index-tree.json"] = b
	return nil
}

// DiagnoseIndexTree strictly checks one machine-readable derived index. It
// never repairs or writes and returns only stable, redacted diagnostics.
func DiagnoseIndexTree(data []byte, policy PolicyConfigIndex) []Diagnostic {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var tree IndexTree
	if err := dec.Decode(&tree); err != nil {
		return []Diagnostic{{Code: CodeIndexPolicyUnsatisfied, Detail: "index tree is not valid strict JSON"}}
	}
	if err := validateIndexTree(&tree, policy); err != nil {
		return []Diagnostic{{Code: CodeIndexPolicyUnsatisfied, Detail: "index tree violates its policy"}}
	}
	return nil
}

func validateIndexTree(tree *IndexTree, policy PolicyConfigIndex) error {
	if tree == nil || tree.Root == nil || tree.Root.Path != "wiki/index.md" || tree.FrozenCount < 0 || tree.ArchivedCount < 0 {
		return storeError(CodeIndexPolicyUnsatisfied, "index tree envelope is invalid")
	}
	if tree.PolicyRef != nil {
		if err := tree.PolicyRef.Validate(); err != nil || tree.PolicyRef.PolicyType != PolicyTypeIndex {
			return storeError(CodeIndexPolicyUnsatisfied, "index policy reference is invalid")
		}
	}
	pages := map[string]*IndexNode{}
	for _, page := range tree.Pages {
		if page == nil || page.Path == "" || pages[page.Path] != nil {
			return storeError(CodeIndexPolicyUnsatisfied, "index page identity is invalid")
		}
		pages[page.Path] = page
		if (len(page.Entries) > 0) == (len(page.Routes) > 0) {
			if page.EntryCount != 0 {
				return storeError(CodeIndexPolicyUnsatisfied, "index page payload is invalid")
			}
		}
		copyNode := *page
		copyNode.ByteCount = 0
		if !fitsIndexNode(&copyNode, policy) || copyNode.ByteCount != page.ByteCount {
			return storeError(CodeIndexPolicyUnsatisfied, "index page exceeds its policy")
		}
	}
	if pages[tree.Root.Path] == nil {
		return storeError(CodeIndexPolicyUnsatisfied, "root index page is missing")
	}
	rootBytes, _ := json.Marshal(tree.Root)
	pageBytes, _ := json.Marshal(pages[tree.Root.Path])
	if !bytes.Equal(rootBytes, pageBytes) {
		return storeError(CodeIndexPolicyUnsatisfied, "root index projection is inconsistent")
	}
	seenEntries := map[string]bool{}
	for _, page := range tree.Pages {
		if len(page.Entries) > 0 {
			if page.EntryCount != len(page.Entries) {
				return storeError(CodeIndexPolicyUnsatisfied, "index leaf count is invalid")
			}
		} else if len(page.Routes) > 0 {
			total := 0
			seenRoutes := map[string]bool{}
			for _, route := range page.Routes {
				key := route.Dimension + "\x00" + route.Value
				if route.Dimension == "" || route.Value == "" || seenRoutes[key] {
					return storeError(CodeIndexPolicyUnsatisfied, "index route identity is invalid")
				}
				seenRoutes[key] = true
				total += route.EntryCount
			}
			if total != page.EntryCount {
				return storeError(CodeIndexPolicyUnsatisfied, "index branch count is invalid")
			}
		}
		for _, route := range page.Routes {
			child := pages[route.Path]
			if child == nil || child.EntryCount != route.EntryCount || child.Depth != page.Depth+1 {
				return storeError(CodeIndexPolicyUnsatisfied, "index route target is invalid")
			}
		}
		for _, entry := range page.Entries {
			key := string(entry.Scope) + "\x00" + entry.MemoryID + "\x00" + itoa(entry.Revision)
			if seenEntries[key] {
				return storeError(CodeIndexPolicyUnsatisfied, "index entry is duplicated")
			}
			seenEntries[key] = true
		}
	}
	reachable := map[string]bool{}
	queue := []*IndexNode{pages[tree.Root.Path]}
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		if reachable[page.Path] {
			return storeError(CodeIndexPolicyUnsatisfied, "index route graph is not a tree")
		}
		reachable[page.Path] = true
		for _, route := range page.Routes {
			queue = append(queue, pages[route.Path])
		}
	}
	if len(reachable) != len(pages) {
		return storeError(CodeIndexPolicyUnsatisfied, "index contains an unreachable page")
	}
	return nil
}

func renderIndexMarkdown(node *IndexNode) []byte {
	var b strings.Builder
	b.WriteString("# OKF Wiki Index\n\n")
	fmt.Fprintf(&b, "Entries: %d\n\n", node.EntryCount)
	if node.EntryCount == 0 {
		b.WriteString("No memories in this generation.\n")
		return []byte(b.String())
	}
	for _, route := range node.Routes {
		fmt.Fprintf(&b, "- [%s=%s](%s) (%d)\n", route.Dimension, route.Value, route.Path, route.EntryCount)
	}
	for _, entry := range node.Entries {
		fmt.Fprintf(&b, "- [%s](%s)\n", entry.CanonicalKey, entry.PagePath)
	}
	return []byte(b.String())
}
