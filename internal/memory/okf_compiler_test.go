package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// ---- helpers ----

func putRevisionEvidence(t *testing.T, s *FactStore, rev MemoryRevision, ev MemoryEvidenceGeneration) {
	t.Helper()
	if _, err := s.Put(context.Background(), rev); err != nil {
		t.Fatalf("put revision: %v", err)
	}
	if _, err := s.Put(context.Background(), ev); err != nil {
		t.Fatalf("put evidence: %v", err)
	}
	if _, err := s.Put(context.Background(), policyOf(PolicyTypeIndex)); err != nil {
		t.Fatalf("put index policy: %v", err)
	}
}

func okfRequest(rev MemoryRevision, ev MemoryEvidenceGeneration) OKFCompileRequest {
	return OKFCompileRequest{
		Scope: rev.Scope, IndexPolicyRef: policyRefOf(policyOf(PolicyTypeIndex)),
		Revisions: []MemoryRevisionRef{{
			MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
		}},
		Evidence: []MemoryEvidenceRef{{
			MemoryID: ev.MemoryID, Revision: ev.Revision,
			EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256,
		}},
	}
}

func policyRefOf(policy PolicyFact) PolicyRef {
	return PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: policy.ContentSHA256}
}

func compileOKF(t *testing.T, s *FactStore, req OKFCompileRequest) *OKFCompileResult {
	t.Helper()
	res, err := CompileOKF(context.Background(), s, req)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return res
}

// ---- determinism & hash ----

func TestOKFCompileDeterministic(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	req := okfRequest(rev, ev)

	res1 := compileOKF(t, s, req)
	res2 := compileOKF(t, s, req)
	if res1.CompiledSHA256 != res2.CompiledSHA256 {
		t.Error("compiled output hash must be stable across compiles")
	}
	if len(res1.Outputs) != len(res2.Outputs) {
		t.Fatalf("output sets differ: %d vs %d", len(res1.Outputs), len(res2.Outputs))
	}
	for path, b1 := range res1.Outputs {
		b2, ok := res2.Outputs[path]
		if !ok {
			t.Errorf("output %q missing from second compile", path)
			continue
		}
		if string(b1) != string(b2) {
			t.Errorf("output %q differs across compiles", path)
		}
	}
	if res1.CompiledSHA256 == "" {
		t.Error("non-empty generation must have a compiled output hash")
	}
	// The page must be a well-formed UTF-8 markdown document.
	page, ok := res1.Outputs["wiki/strategies/verify-before-upgrade-retry.md"]
	if !ok {
		t.Fatalf("page missing, got paths: %v", outputPaths(res1))
	}
	if !strings.HasPrefix(string(page), "---\n") || !strings.Contains(string(page), "\n---\n") {
		t.Error("page must have YAML frontmatter delimiters")
	}
}

func TestOKFIndexPolicyIsManifestInput(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	res := compileOKF(t, s, okfRequest(rev, ev))
	want := policyOf(PolicyTypeIndex)
	found := false
	for _, input := range res.Inputs {
		if input.FactType == "policy" && input.FactID == want.PolicyID+"@1" && input.ContentSHA256 == want.ContentSHA256 {
			found = true
		}
	}
	if !found {
		t.Fatal("exact index policy fact must be recorded in manifest inputs")
	}
	if _, ok := res.Outputs["state/index-tree.json"]; !ok {
		t.Fatal("machine index tree output is missing")
	}
	var tree IndexTree
	if err := json.Unmarshal(res.Outputs["state/index-tree.json"], &tree); err != nil {
		t.Fatal(err)
	}
	if tree.PolicyRef == nil || *tree.PolicyRef != policyRefOf(want) {
		t.Fatalf("machine index tree must pin the exact policy reference: %+v", tree.PolicyRef)
	}
}

func TestOKFIndexUsesFixedDerivedWorld(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	base := compileOKF(t, s, okfRequest(rev, ev))
	// A later governance fact must not affect a replay pinned to the original
	// manifest input set.
	gov := validGovernanceEvent()
	gov.MemoryID, gov.Revision = rev.MemoryID, rev.Revision
	gov.Operation = "manual_freeze"
	if _, err := s.Put(context.Background(), gov); err != nil {
		t.Fatal(err)
	}
	req := okfRequest(rev, ev)
	req.DerivationInputs = base.Inputs
	replayed := compileOKF(t, s, req)
	if replayed.CompiledSHA256 != base.CompiledSHA256 {
		t.Fatal("post-generation facts leaked into fixed derived world")
	}
}

func TestOKFIndexExcludesFrozenAndUsesRenderedPagePath(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	gov := validGovernanceEvent()
	gov.MemoryID, gov.Revision, gov.Operation = rev.MemoryID, rev.Revision, "manual_freeze"
	if _, err := s.Put(context.Background(), gov); err != nil {
		t.Fatal(err)
	}
	res := compileOKF(t, s, okfRequest(rev, ev))
	var tree IndexTree
	if err := json.Unmarshal(res.Outputs["state/index-tree.json"], &tree); err != nil {
		t.Fatal(err)
	}
	if tree.FrozenCount != 1 || len(tree.Root.Entries) != 0 {
		t.Fatalf("frozen memory leaked into normal index: %+v", tree)
	}
	if _, ok := res.Outputs["wiki/strategies/"+rev.CanonicalKey+".md"]; !ok {
		t.Fatal("frozen knowledge page must remain auditable")
	}
}

func outputPaths(res *OKFCompileResult) []string {
	var out []string
	for p := range res.Outputs {
		out = append(out, p)
	}
	return out
}

// ---- input validation (fail closed) ----

func TestOKFCompileMissingEvidence(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	putRevisionEvidence(t, s, rev, validEvidenceGeneration())

	req := OKFCompileRequest{
		Scope:     rev.Scope,
		Revisions: []MemoryRevisionRef{{MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}},
		// Evidence deliberately absent.
	}
	if _, err := CompileOKF(context.Background(), s, req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("missing evidence must fail closed, got %v", err)
	}
}

func TestOKFCompileEvidenceRevisionMismatch(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	putRevisionEvidence(t, s, rev, validEvidenceGeneration())

	// The evidence reference points at a different revision than the
	// revision reference.
	req := OKFCompileRequest{
		Scope:     rev.Scope,
		Revisions: []MemoryRevisionRef{{MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}},
		Evidence: []MemoryEvidenceRef{{
			MemoryID: rev.MemoryID, Revision: rev.Revision + 1,
			EvidenceGeneration: 3, EvidenceSetSHA256: validEvidenceGeneration().EvidenceSetSHA256,
		}},
	}
	if _, err := CompileOKF(context.Background(), s, req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("evidence revision mismatch must fail closed, got %v", err)
	}
}

func TestOKFCompileHashDrift(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	req := okfRequest(rev, ev)
	req.Revisions[0].ContentSHA256 = "sha256_" + strings.Repeat("f", 64)
	if _, err := CompileOKF(context.Background(), s, req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("revision hash drift must fail closed, got %v", err)
	}

	req2 := okfRequest(rev, ev)
	req2.Evidence[0].EvidenceSetSHA256 = "sha256_" + strings.Repeat("e", 64)
	if _, err := CompileOKF(context.Background(), s, req2); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("evidence hash drift must fail closed, got %v", err)
	}
}

func TestOKFCompileCrossScope(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	putRevisionEvidence(t, s, rev, validEvidenceGeneration())

	req := okfRequest(rev, validEvidenceGeneration())
	req.Scope = ScopeGlobal
	if _, err := CompileOKF(context.Background(), s, req); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope compile must fail closed, got %v", err)
	}
}

func TestOKFCompileDuplicateRefs(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	req := okfRequest(rev, ev)
	req.Revisions = append(req.Revisions, req.Revisions[0])
	if _, err := CompileOKF(context.Background(), s, req); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("duplicate revision refs must fail closed, got %v", err)
	}

	req2 := okfRequest(rev, ev)
	req2.Evidence = append(req2.Evidence, req2.Evidence[0])
	if _, err := CompileOKF(context.Background(), s, req2); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("duplicate evidence refs must fail closed, got %v", err)
	}

	// Two evidence generations for the same revision are also a duplicate.
	req3 := okfRequest(rev, ev)
	req3.Evidence = append(req3.Evidence, MemoryEvidenceRef{
		MemoryID: ev.MemoryID, Revision: ev.Revision,
		EvidenceGeneration: ev.EvidenceGeneration + 1, EvidenceSetSHA256: ev.EvidenceSetSHA256,
	})
	if _, err := CompileOKF(context.Background(), s, req3); ErrorCode(err) != CodeOKFInvalidInput {
		t.Fatalf("two evidence refs for one revision must fail closed, got %v", err)
	}
}

// ---- canonical key collision ----

func TestOKFCompileCanonicalCollision(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	base := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, base, ev)

	mkRev := func(id string, rev int) (MemoryRevision, MemoryEvidenceGeneration) {
		r := validRevision()
		r.MemoryID = id
		r.Revision = rev
		r = fillRevisionHash(r)
		e := validEvidenceGeneration()
		e.MemoryID = id
		e.Revision = rev
		e.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_" + id, ContentSHA256: testHash}}
		e = fillEvidenceHash(e)
		return r, e
	}
	// Same canonical_key ("verify-before-upgrade-retry") across three
	// different memories: filenames fall back to --component then
	// --short-memory-id.
	rev2, ev2 := mkRev("mem_second", 1)
	rev3, ev3 := mkRev("mem_third", 1)
	putRevisionEvidence(t, s, rev2, ev2)
	putRevisionEvidence(t, s, rev3, ev3)

	req := OKFCompileRequest{
		Scope: ScopeProject, IndexPolicyRef: policyRefOf(policyOf(PolicyTypeIndex)),
		Revisions: []MemoryRevisionRef{
			{MemoryID: base.MemoryID, Revision: base.Revision, ContentSHA256: base.ContentSHA256},
			{MemoryID: rev2.MemoryID, Revision: rev2.Revision, ContentSHA256: rev2.ContentSHA256},
			{MemoryID: rev3.MemoryID, Revision: rev3.Revision, ContentSHA256: rev3.ContentSHA256},
		},
		Evidence: []MemoryEvidenceRef{
			{MemoryID: ev.MemoryID, Revision: ev.Revision, EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256},
			{MemoryID: ev2.MemoryID, Revision: ev2.Revision, EvidenceGeneration: ev2.EvidenceGeneration, EvidenceSetSHA256: ev2.EvidenceSetSHA256},
			{MemoryID: ev3.MemoryID, Revision: ev3.Revision, EvidenceGeneration: ev3.EvidenceGeneration, EvidenceSetSHA256: ev3.EvidenceSetSHA256},
		},
	}
	res := compileOKF(t, s, req)
	if _, ok := res.Outputs["wiki/strategies/verify-before-upgrade-retry.md"]; !ok {
		t.Error("first page must keep the plain canonical key")
	}
	if _, ok := res.Outputs["wiki/strategies/verify-before-upgrade-retry--strategy.md"]; !ok {
		t.Error("second page must use the --component fallback")
	}
	shortFound := false
	for p := range res.Outputs {
		// shortMemoryID strips the "mem_" prefix, so the third fallback is
		// "--second" / "--third" here.
		if strings.HasPrefix(p, "wiki/strategies/verify-before-upgrade-retry--") &&
			!strings.HasPrefix(p, "wiki/strategies/verify-before-upgrade-retry--strategy") {
			shortFound = true
		}
	}
	if !shortFound {
		t.Error("third page must use the --short-memory-id fallback")
	}

	// Four revisions of the SAME memory (stable canonical_key across
	// revisions) exhaust the three fallback names: the fourth cannot be
	// resolved and fails closed, never overwriting.
	base2 := validRevision()
	base2.CanonicalKey = "same-memory-key"
	base2.MemoryID = "mem_same"
	var revs []MemoryRevisionRef
	var evs []MemoryEvidenceRef
	for i := 1; i <= 4; i++ {
		r := base2
		r.Revision = i
		r.Title = fmt.Sprintf("Same Memory %d", i)
		r = fillRevisionHash(r)
		e := validEvidenceGeneration()
		e.MemoryID = "mem_same"
		e.Revision = i
		e.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: fmt.Sprintf("ep_%d", i), ContentSHA256: testHash}}
		e = fillEvidenceHash(e)
		putRevisionEvidence(t, s, r, e)
		revs = append(revs, MemoryRevisionRef{MemoryID: r.MemoryID, Revision: r.Revision, ContentSHA256: r.ContentSHA256})
		evs = append(evs, MemoryEvidenceRef{MemoryID: e.MemoryID, Revision: e.Revision, EvidenceGeneration: e.EvidenceGeneration, EvidenceSetSHA256: e.EvidenceSetSHA256})
	}
	req4 := OKFCompileRequest{Scope: ScopeProject, IndexPolicyRef: policyRefOf(policyOf(PolicyTypeIndex)), Revisions: revs, Evidence: evs}
	if _, err := CompileOKF(context.Background(), s, req4); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("unresolvable collision must fail closed, got %v", err)
	}
}

func fillEvidenceHash(e MemoryEvidenceGeneration) MemoryEvidenceGeneration {
	h, err := e.ContentHash()
	if err != nil {
		panic(err)
	}
	e.EvidenceSetSHA256 = h
	return e
}

// ---- cross-scope relation ----

func TestOKFCompileCrossScopeRelation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	rev.Relations = []MemoryRelation{{
		Predicate: "derived_from",
		Target: MemoryRef{
			Scope: ScopeGlobal, MemoryType: MemoryTypePattern,
			MemoryID: "mem_pattern_other", Revision: 1, ContentSHA256: testHash,
		},
	}}
	rev = fillRevisionHash(rev)
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	if _, err := CompileOKF(context.Background(), s, okfRequest(rev, ev)); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("cross-scope relation must fail closed, got %v", err)
	}
}

// ---- empty generation & index integrity ----

func TestOKFCompileEmptyGeneration(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	policy := policyOf(PolicyTypeIndex)
	if _, err := s.Put(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	req := OKFCompileRequest{Scope: ScopeProject, IndexPolicyRef: policyRefOf(policy)}

	res1 := compileOKF(t, s, req)
	res2 := compileOKF(t, s, req)
	if res1.CompiledSHA256 != res2.CompiledSHA256 {
		t.Error("empty generation output hash must be stable")
	}
	for _, want := range []string{"wiki/index.md", "state/memories.json", "state/relations.json"} {
		if _, ok := res1.Outputs[want]; !ok {
			t.Errorf("empty generation must emit %q, got %v", want, outputPaths(res1))
		}
	}
	if !strings.Contains(string(res1.Outputs["wiki/index.md"]), "No memories") {
		t.Error("empty index must state there are no memories")
	}
}

func TestOKFIndexLinksAllResolve(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	putRevisionEvidence(t, s, rev, validEvidenceGeneration())
	res := compileOKF(t, s, okfRequest(rev, validEvidenceGeneration()))

	index := string(res.Outputs["wiki/index.md"])
	for _, line := range strings.Split(index, "\n") {
		if !strings.Contains(line, "](wiki/") && !strings.Contains(line, "](") {
			continue
		}
		start := strings.Index(line, "](")
		end := strings.Index(line[start:], ")")
		if end < 0 {
			t.Fatalf("malformed link line: %q", line)
		}
		link := line[start+2 : start+end]
		if _, ok := res.Outputs[link]; !ok {
			t.Errorf("index link %q does not resolve to an emitted page", link)
		}
	}
}

// ---- golden page ----

func TestOKFPageGoldenFormat(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	res := compileOKF(t, s, okfRequest(rev, ev))
	page := string(res.Outputs["wiki/strategies/verify-before-upgrade-retry.md"])

	for _, want := range []string{
		"okf_version: \"0.1\"",
		"type: strategy",
		"memory_id: " + rev.MemoryID,
		"canonical_key: verify-before-upgrade-retry",
		"title: \"Verify Before Upgrade Retry\"",
		"summary: \"Check the asset source before retrying an upgrade.\"",
		"lifecycle: not_available",
		"health: not_available",
		"usage_policy: outcome_attributed",
		"revision: 2",
		"evidence_generation: 3",
		"content_sha256: " + rev.ContentSHA256,
		"evidence_set_sha256: " + ev.EvidenceSetSHA256,
		"## Summary",
		"## Applicable Conditions",
		"## Known Boundaries",
		"## Evidence",
		"## Relations",
		"episode_001",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page must contain %q", want)
		}
	}
	// No runtime statistics, lifecycle or machine noise may leak into the
	// page.
	for _, banned := range []string{"created_at", "updated_at", "last_used", "usage_count", "pinned", "/Users", "/var/", "2026-08-07T", "tx_01K"} {
		if strings.Contains(page, banned) {
			t.Errorf("page must not contain %q", banned)
		}
	}
}

// ---- compiler identity & deterministic ordering ----

func TestOKFCompilerVersionContract(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	gs := NewGenerationStore(s)
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	res, err := CompileOKF(context.Background(), s, okfRequest(rev, ev))
	if err != nil {
		t.Fatal(err)
	}
	req := BeginGenerationRequest{
		Scope:                   ScopeProject,
		CompilerVersion:         OKFCompilerVersion,
		CanonicalizationVersion: OKFCanonicalizationVersion,
		SchemaVersion:           1,
		IdempotencyKey:          "okf_contract",
	}
	tx, err := gs.Begin(context.Background(), req)
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
	// The registered OKF compiler identity must have driven the whole
	// transaction; the published document records it.
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", tx.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CompilerVersion != OKFCompilerVersion || doc.CanonicalizationVersion != OKFCanonicalizationVersion {
		t.Errorf("generation must record the frozen OKF compiler identity, got %s@%d", doc.CompilerVersion, doc.CanonicalizationVersion)
	}
}

func TestOKFLegacyCompilerRemainsRebuildable(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev, ev := validRevision(), validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	req := okfRequest(rev, ev)
	res, err := compileOKFLegacy(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Outputs["state/index-tree.json"]; ok {
		t.Fatal("v1 compiler must not gain v2 output")
	}
	if !strings.Contains(string(res.Outputs["wiki/index.md"]), rev.Title) {
		t.Fatal("v1 index rendering changed")
	}
	if !generationCompilerAvailable(OKFCompilerVersionV1, OKFCanonicalizationVersion) {
		t.Fatal("v1 compiler registry entry was removed")
	}
}

func TestOKFDuplicateSortKeysDeterministic(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// Two evidence refs that share the sort key (type/id/hash) but differ in
	// scope, and two relations that share a predicate but target distinct
	// generation members: the total sort key keeps the output deterministic.
	// Relation targets must be members of the generation inputs.
	rev := validRevision()
	ev := validEvidenceGeneration()
	ev.EvidenceRefs = []EvidenceRef{
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_same", ContentSHA256: testHash},
		{Scope: ScopeGlobal, EvidenceType: "episode", EvidenceID: "ep_same", ContentSHA256: testHash},
	}
	ev = fillEvidenceHash(ev)

	mkTarget := func(id string, revNo int) (MemoryRevision, MemoryEvidenceGeneration) {
		r := validRevision()
		r.MemoryID = id
		r.MemoryType = MemoryTypePattern
		r.UsagePolicy = UsagePolicyEvidenceValidated
		r.CanonicalKey = "pattern-" + id
		r.Revision = revNo
		r.Title = "Pattern " + id
		r.Summary = "Target pattern for determinism."
		r = fillRevisionHash(r)
		e := validEvidenceGeneration()
		e.MemoryID = id
		e.Revision = revNo
		e.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_" + id, ContentSHA256: testHash}}
		e = fillEvidenceHash(e)
		putRevisionEvidence(t, s, r, e)
		return r, e
	}
	tA, eA := mkTarget("mem_pattern_a", 1)
	tB, eB := mkTarget("mem_pattern_b", 2)

	rev.Relations = []MemoryRelation{
		{Predicate: "derived_from", Target: MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypePattern, MemoryID: tA.MemoryID, Revision: tA.Revision, ContentSHA256: tA.ContentSHA256}},
		{Predicate: "derived_from", Target: MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypePattern, MemoryID: tB.MemoryID, Revision: tB.Revision, ContentSHA256: tB.ContentSHA256}},
	}
	rev = fillRevisionHash(rev)
	putRevisionEvidence(t, s, rev, ev)

	req := okfRequest(rev, ev)
	req.Revisions = append(req.Revisions,
		MemoryRevisionRef{MemoryID: tA.MemoryID, Revision: tA.Revision, ContentSHA256: tA.ContentSHA256},
		MemoryRevisionRef{MemoryID: tB.MemoryID, Revision: tB.Revision, ContentSHA256: tB.ContentSHA256},
	)
	req.Evidence = append(req.Evidence,
		MemoryEvidenceRef{MemoryID: eA.MemoryID, Revision: eA.Revision, EvidenceGeneration: eA.EvidenceGeneration, EvidenceSetSHA256: eA.EvidenceSetSHA256},
		MemoryEvidenceRef{MemoryID: eB.MemoryID, Revision: eB.Revision, EvidenceGeneration: eB.EvidenceGeneration, EvidenceSetSHA256: eB.EvidenceSetSHA256},
	)

	res1 := compileOKF(t, s, req)
	res2 := compileOKF(t, s, req)
	if res1.CompiledSHA256 != res2.CompiledSHA256 {
		t.Error("duplicate sort-key entries must still compile deterministically")
	}
	page := string(res1.Outputs["wiki/strategies/verify-before-upgrade-retry.md"])
	if strings.Count(page, "- ep_same (episode)") != 2 {
		t.Errorf("both evidence refs must appear deterministically, page has %d", strings.Count(page, "- ep_same (episode)"))
	}
	// Both relations survive into the deterministic relations view.
	var relations struct {
		Relations []map[string]any `json:"relations"`
	}
	if err := json.Unmarshal(res1.Outputs["state/relations.json"], &relations); err != nil {
		t.Fatalf("relations.json must be valid JSON: %v", err)
	}
	if len(relations.Relations) != 2 {
		t.Fatalf("relations.json must list both relations, got %d", len(relations.Relations))
	}
}

func TestOKFStateViews(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	// The relation target must be a member of this generation's inputs; it
	// is added explicitly below instead of being an implicit dangling link.
	target := validRevision()
	target.MemoryID = "mem_pattern_prompt_drift"
	target.MemoryType = MemoryTypePattern
	target.UsagePolicy = UsagePolicyEvidenceValidated
	target.CanonicalKey = "prompt-drift"
	target.Revision = 2
	target.Title = "Prompt Drift"
	target.Summary = "The working prompt drifts from the written memory."
	target = fillRevisionHash(target)
	targetEv := validEvidenceGeneration()
	targetEv.MemoryID = target.MemoryID
	targetEv.Revision = target.Revision
	targetEv.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_drift", ContentSHA256: testHash}}
	targetEv = fillEvidenceHash(targetEv)
	putRevisionEvidence(t, s, target, targetEv)

	rev.Relations = []MemoryRelation{{
		Predicate: "derived_from",
		Target: MemoryRef{
			Scope: ScopeProject, MemoryType: MemoryTypePattern,
			MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256,
		},
	}}
	rev = fillRevisionHash(rev)
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	req := okfRequest(rev, ev)
	req.Revisions = append(req.Revisions, MemoryRevisionRef{
		MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256,
	})
	req.Evidence = append(req.Evidence, MemoryEvidenceRef{
		MemoryID: targetEv.MemoryID, Revision: targetEv.Revision,
		EvidenceGeneration: targetEv.EvidenceGeneration, EvidenceSetSHA256: targetEv.EvidenceSetSHA256,
	})
	res := compileOKF(t, s, req)

	var memories struct {
		Memories []map[string]any `json:"memories"`
	}
	if err := json.Unmarshal(res.Outputs["state/memories.json"], &memories); err != nil {
		t.Fatalf("memories.json must be valid JSON: %v", err)
	}
	if len(memories.Memories) != 2 {
		t.Fatalf("memories.json must list both generation members, got %d", len(memories.Memories))
	}
	ids := map[string]bool{}
	hashes := map[string]bool{}
	for _, m := range memories.Memories {
		if id, ok := m["memory_id"].(string); ok {
			ids[id] = true
		}
		if h, ok := m["content_sha256"].(string); ok {
			hashes[h] = true
		}
	}
	if !ids[rev.MemoryID] || !ids[target.MemoryID] {
		t.Errorf("memories.json must list both members, got %v", memories.Memories)
	}
	if !hashes[rev.ContentSHA256] || !hashes[target.ContentSHA256] {
		t.Errorf("memories.json must carry each member's content hash, got %v", memories.Memories)
	}

	var relations struct {
		Relations []map[string]any `json:"relations"`
	}
	if err := json.Unmarshal(res.Outputs["state/relations.json"], &relations); err != nil {
		t.Fatalf("relations.json must be valid JSON: %v", err)
	}
	if len(relations.Relations) != 1 {
		t.Fatalf("relations.json must list exactly one relation, got %d", len(relations.Relations))
	}
	if relations.Relations[0]["predicate"] != "derived_from" {
		t.Errorf("relation predicate mismatch: %v", relations.Relations[0])
	}
	trg, ok := relations.Relations[0]["target"].(map[string]any)
	if !ok || trg["memory_id"] != target.MemoryID || trg["revision"] != float64(target.Revision) {
		t.Errorf("relation target mismatch: %v", relations.Relations[0]["target"])
	}
}

// ---- relation target must be a generation member ----

func TestOKFCompileRelationTargetMustBeInInputs(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	target := validRevision()
	target.MemoryID = "mem_pattern_target"
	target.MemoryType = MemoryTypePattern
	target.UsagePolicy = UsagePolicyEvidenceValidated
	target.CanonicalKey = "pattern-target"
	target.Revision = 1
	target.Title = "Target Pattern"
	target.Summary = "The relation target."
	target = fillRevisionHash(target)
	targetEv := validEvidenceGeneration()
	targetEv.MemoryID = target.MemoryID
	targetEv.Revision = target.Revision
	targetEv.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_target", ContentSHA256: testHash}}
	targetEv = fillEvidenceHash(targetEv)
	putRevisionEvidence(t, s, target, targetEv)

	// withRelation builds a source revision whose single relation points at
	// the target, optionally mutating the reference; every source uses a
	// distinct memory id so the staged identity never collides.
	withRelation := func(id string, mut func(*MemoryRef)) (MemoryRevision, MemoryEvidenceGeneration) {
		r := validRevision()
		r.MemoryID = id
		r.Relations = []MemoryRelation{{Predicate: "derived_from", Target: MemoryRef{
			Scope: ScopeProject, MemoryType: MemoryTypePattern,
			MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256,
		}}}
		if mut != nil {
			mut(&r.Relations[0].Target)
		}
		r = fillRevisionHash(r)
		e := validEvidenceGeneration()
		e.MemoryID = id
		e.Revision = r.Revision
		e.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_src_" + id, ContentSHA256: testHash}}
		e = fillEvidenceHash(e)
		putRevisionEvidence(t, s, r, e)
		return r, e
	}
	reqFor := func(r MemoryRevision, e MemoryEvidenceGeneration) OKFCompileRequest {
		req := okfRequest(r, e)
		req.Revisions = append(req.Revisions, MemoryRevisionRef{
			MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256,
		})
		req.Evidence = append(req.Evidence, MemoryEvidenceRef{
			MemoryID: targetEv.MemoryID, Revision: targetEv.Revision,
			EvidenceGeneration: targetEv.EvidenceGeneration, EvidenceSetSHA256: targetEv.EvidenceSetSHA256,
		})
		return req
	}

	// Valid: the target is a member of the generation inputs.
	valid, validEv := withRelation("mem_strategy_valid", nil)
	if _, err := CompileOKF(context.Background(), s, reqFor(valid, validEv)); err != nil {
		t.Fatalf("in-generation target must compile: %v", err)
	}

	// Missing target: no implicit links to pages that are not in this
	// generation's input set.
	missing, missingEv := withRelation("mem_strategy_missing", func(ref *MemoryRef) { ref.MemoryID = "mem_pattern_nonexistent" })
	if _, err := CompileOKF(context.Background(), s, reqFor(missing, missingEv)); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("missing relation target must fail closed, got %v", err)
	}

	// Wrong revision.
	wrongRev, wrongRevEv := withRelation("mem_strategy_wrongrev", func(ref *MemoryRef) { ref.Revision = target.Revision + 1 })
	if _, err := CompileOKF(context.Background(), s, reqFor(wrongRev, wrongRevEv)); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("wrong revision target must fail closed, got %v", err)
	}

	// Wrong content hash.
	wrongHash, wrongHashEv := withRelation("mem_strategy_wronghash", func(ref *MemoryRef) { ref.ContentSHA256 = testHash })
	if _, err := CompileOKF(context.Background(), s, reqFor(wrongHash, wrongHashEv)); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("wrong hash target must fail closed, got %v", err)
	}

	// Wrong memory type.
	wrongType, wrongTypeEv := withRelation("mem_strategy_wrongtype", func(ref *MemoryRef) { ref.MemoryType = MemoryTypeStrategy })
	if _, err := CompileOKF(context.Background(), s, reqFor(wrongType, wrongTypeEv)); ErrorCode(err) != CodeOKFCompileError {
		t.Fatalf("wrong type target must fail closed, got %v", err)
	}
}
