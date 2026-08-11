package memory

// MEM-02A failure-first tests: MemoryContext / ObservationProvenance /
// MemoryUsage anchor fields. These tests reference the new anchor fields
// (u.MemoryContext, u.ObservationProvenance, ...) that did not exist before
// MEM-02A, so the package failed to compile until the implementation landed.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testHash2 is a second valid hash used to prove anchor changes alter hashes.
const testHash2 = "sha256_2222222222222222222222222222222222222222222222222222222222222222"

// ---- helpers ----

// anchoredUsage returns a fully anchored MemoryUsage. The ContentHash is
// computed over the anchored canonical form, exactly like production writes.
func anchoredUsage(t *testing.T) MemoryUsage {
	t.Helper()
	u := MemoryUsage{
		SchemaVersion: 1,
		UsageID:       "usage_anchored_01",
		Scope:         ScopeProject,
		MemoryID:      "mem_01K7A9X2",
		Revision:      2,
		UsageStage:    "evaluated",
		EpisodeID:     "episode_01K",
		OccurredAt:    "2026-08-11T10:00:00Z",
		Source:        "local_user",
		CreatedAt:     "2026-08-11T10:05:00Z",
		RetrievalID:   "retrieval_001",
		RootTaskID:    "task_root_007",
		MemoryContext: &MemoryContext{
			ProjectGenerationRef: &ProjectGenerationRef{
				SchemaVersion: 1, Scope: ScopeProject,
				GenerationID: "gen_project_000010", InputManifestID: "gen_project_000010",
				InputManifestSHA256: testHash,
			},
			GlobalGenerationRef: nil,
		},
		ContextSignatureVersion: 1,
		ContextSignature:        testHash,
		ContextDescriptorRef:    "context_01K",
		ObservationProvenance: &ObservationProvenance{
			Source: "agent_reported",
			EvidenceRef: &EvidenceRef{
				Scope: ScopeProject, EvidenceType: "reasonix_event",
				EvidenceID: "event_01K", ContentSHA256: testHash,
			},
			JudgmentRef: nil,
		},
	}
	h, err := u.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	u.ContentSHA256 = h
	return u
}

// legacyMemoryUsageJSON is the exact pre-MEM-02A legacy serialization of
// validUsage() (no anchor fields). Decoding it must yield the same Canonical
// Bytes and Content Hash as before the change.
const legacyMemoryUsageJSON = `{"content_sha256":"sha256_de95269e2651e145145aa7b7c8ca2ef829dad19d247da3fd3761b1c0860ca17e","created_at":"2026-08-11T10:05:00Z","episode_id":"episode_01K","memory_id":"mem_01K7A9X2","occurred_at":"2026-08-11T10:00:00Z","revision":2,"schema_version":1,"scope":"project","source":"local_user","usage_id":"usage_01K","usage_stage":"affected"}`

const legacyMemoryUsageHash = "sha256_32b496a43c735451f810102258150fb94966505a4955fe41c38a6bed1a6811fa"

// ---- 5.1 MemoryContext ----

func TestMemoryContextValidation(t *testing.T) {
	prj := func() *ProjectGenerationRef {
		return &ProjectGenerationRef{
			SchemaVersion: 1, Scope: ScopeProject,
			GenerationID: "gen_project_000010", InputManifestID: "gen_project_000010",
			InputManifestSHA256: testHash,
		}
	}
	glb := func() *GlobalGenerationRef {
		return &GlobalGenerationRef{
			SchemaVersion: 1, Scope: ScopeGlobal,
			GenerationID: "gen_global_000020", InputManifestID: "gen_global_000020",
			InputManifestSHA256: testHash,
		}
	}
	valid := []MemoryContext{
		{ProjectGenerationRef: prj(), GlobalGenerationRef: nil},
		{ProjectGenerationRef: nil, GlobalGenerationRef: glb()},
		{ProjectGenerationRef: prj(), GlobalGenerationRef: glb()},
	}
	for i, c := range valid {
		if err := c.Validate(); err != nil {
			t.Errorf("valid memory context %d rejected: %v", i, err)
		}
	}
	invalid := []struct {
		name string
		mut  func(*MemoryContext)
	}{
		{"both nil", func(c *MemoryContext) { c.ProjectGenerationRef = nil; c.GlobalGenerationRef = nil }},
		{"project scope wrong", func(c *MemoryContext) { c.GlobalGenerationRef = nil; c.ProjectGenerationRef.Scope = ScopeGlobal }},
		{"global scope wrong", func(c *MemoryContext) { c.ProjectGenerationRef = nil; c.GlobalGenerationRef.Scope = ScopeProject }},
		{"generation id traversal", func(c *MemoryContext) { c.GlobalGenerationRef = nil; c.ProjectGenerationRef.GenerationID = "../evil" }},
		{"manifest id empty", func(c *MemoryContext) { c.GlobalGenerationRef = nil; c.ProjectGenerationRef.InputManifestID = "" }},
		{"manifest hash invalid", func(c *MemoryContext) {
			c.GlobalGenerationRef = nil
			c.ProjectGenerationRef.InputManifestSHA256 = "md5_abc"
		}},
	}
	for _, tc := range invalid {
		c := MemoryContext{ProjectGenerationRef: prj(), GlobalGenerationRef: glb()}
		tc.mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: must be rejected", tc.name)
		}
	}
}

func TestMemoryContextCanonicalStable(t *testing.T) {
	prj := &ProjectGenerationRef{
		SchemaVersion: 1, Scope: ScopeProject,
		GenerationID: "gen_project_000010", InputManifestID: "gen_project_000010",
		InputManifestSHA256: testHash,
	}
	glb := &GlobalGenerationRef{
		SchemaVersion: 1, Scope: ScopeGlobal,
		GenerationID: "gen_global_000020", InputManifestID: "gen_global_000020",
		InputManifestSHA256: testHash,
	}
	c := MemoryContext{ProjectGenerationRef: prj, GlobalGenerationRef: glb}
	b1, err := c.canonMap()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := c.canonMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(b1) != 2 {
		t.Fatalf("canonical must output exactly two keys, got %d: %v", len(b1), b1)
	}
	for k := range b2 {
		if _, ok := b1[k]; !ok {
			t.Errorf("key %q missing from repeated canonical output", k)
		}
	}
}

// ---- 5.2 ObservationProvenance ----

func TestObservationProvenanceMatrix(t *testing.T) {
	ev := func() *EvidenceRef {
		return &EvidenceRef{Scope: ScopeProject, EvidenceType: "reasonix_event", EvidenceID: "event_01K", ContentSHA256: testHash}
	}
	conf := func() *JudgmentRef {
		return &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_01K", ContentSHA256: testHash}
	}
	valid := []ObservationProvenance{
		{Source: "agent_reported", EvidenceRef: ev(), JudgmentRef: nil},
		{Source: "runtime_observed", EvidenceRef: ev(), JudgmentRef: nil},
		{Source: "user_confirmed", EvidenceRef: nil, JudgmentRef: conf()},
		{Source: "user_confirmed", EvidenceRef: ev(), JudgmentRef: conf()},
	}
	for i, p := range valid {
		if err := p.Validate(); err != nil {
			t.Errorf("valid provenance %d rejected: %v", i, err)
		}
	}
	invalid := []struct {
		name string
		p    ObservationProvenance
	}{
		{"unknown source", ObservationProvenance{Source: "llm_hallucinated", EvidenceRef: ev()}},
		{"agent missing evidence", ObservationProvenance{Source: "agent_reported", EvidenceRef: nil, JudgmentRef: nil}},
		{"runtime missing evidence", ObservationProvenance{Source: "runtime_observed", EvidenceRef: nil}},
		{"agent carries judgment", ObservationProvenance{Source: "agent_reported", EvidenceRef: ev(), JudgmentRef: conf()}},
		{"runtime carries judgment", ObservationProvenance{Source: "runtime_observed", EvidenceRef: ev(), JudgmentRef: conf()}},
		{"user missing judgment", ObservationProvenance{Source: "user_confirmed", EvidenceRef: ev(), JudgmentRef: nil}},
		{"user judgment not confirmation", ObservationProvenance{Source: "user_confirmed", EvidenceRef: nil, JudgmentRef: &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeFreshnessEvaluation, JudgmentID: "j", ContentSHA256: testHash}}},
		{"evidence traversal id", ObservationProvenance{Source: "agent_reported", EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "reasonix_event", EvidenceID: "../etc", ContentSHA256: testHash}}},
		{"evidence bad hash", ObservationProvenance{Source: "agent_reported", EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "reasonix_event", EvidenceID: "event_01K", ContentSHA256: "abc"}}},
	}
	for _, tc := range invalid {
		if err := tc.p.Validate(); err == nil {
			t.Errorf("%s: must be rejected", tc.name)
		} else {
			for _, banned := range []string{"/Users", "/tmp", ";", "\n", "rm -rf", "Bearer", "sk-"} {
				if strings.Contains(err.Error(), banned) {
					t.Errorf("%s: error leaks %q: %v", tc.name, banned, err)
				}
			}
		}
	}
}

// ---- 5.3 MemoryUsage anchors ----

func TestAnchoredUsageRoundTrip(t *testing.T) {
	u := anchoredUsage(t)
	if err := u.Validate(); err != nil {
		t.Fatalf("anchored usage rejected: %v", err)
	}
	raw, err := u.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeStrict[MemoryUsage](raw)
	if err != nil {
		t.Fatalf("round-trip decode failed: %v", err)
	}
	if got.ContentSHA256 != u.ContentSHA256 {
		t.Errorf("hash not stable across round trip: %s != %s", got.ContentSHA256, u.ContentSHA256)
	}
	if got.MemoryContext == nil || got.ObservationProvenance == nil {
		t.Fatal("anchored fields must survive round trip")
	}
	if got.MemoryContext.ProjectGenerationRef == nil || got.MemoryContext.ProjectGenerationRef.GenerationID != "gen_project_000010" {
		t.Errorf("project generation ref lost: %+v", got.MemoryContext)
	}
	if got.RetrievalID != "retrieval_001" || got.RootTaskID != "task_root_007" ||
		got.ContextSignatureVersion != 1 || got.ContextSignature != testHash ||
		got.ContextDescriptorRef != "context_01K" {
		t.Errorf("anchor scalars lost: %+v", got)
	}
	if got.ObservationProvenance.Source != "agent_reported" || got.ObservationProvenance.EvidenceRef == nil {
		t.Errorf("observation provenance lost: %+v", got.ObservationProvenance)
	}
}

func TestAnchoredUsageStoreRoundTrip(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	u := anchoredUsage(t)
	res, err := s.Put(context.Background(), u)
	if err != nil {
		t.Fatalf("put anchored usage: %v", err)
	}
	if res.Status != WriteCreated {
		t.Fatalf("expected created, got %s", res.Status)
	}
	// Same fact -> NOOP.
	res2, err := s.Put(context.Background(), anchoredUsage(t))
	if err != nil {
		t.Fatal(err)
	}
	if res2.Status != WriteNoop {
		t.Errorf("same anchored usage must be NOOP, got %s", res2.Status)
	}
	got, err := s.Get(context.Background(), FactKindMemoryUsage, "usage_anchored_01")
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeStrict[MemoryUsage](got)
	if err != nil {
		t.Fatal(err)
	}
	if back.ContentSHA256 != u.ContentSHA256 {
		t.Errorf("stored hash mismatch: %s != %s", back.ContentSHA256, u.ContentSHA256)
	}
	if back.MemoryContext == nil || back.MemoryContext.ProjectGenerationRef == nil {
		t.Error("anchored memory context must survive store")
	}
	// List sees it.
	list, err := s.List(context.Background(), FactKindMemoryUsage)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("List must see exactly one usage, got %d", len(list))
	}
}

func TestAnchoredUsageAllOrNone(t *testing.T) {
	base := func() MemoryUsage { return anchoredUsage(t) }
	fields := []struct {
		name string
		mut  func(*MemoryUsage)
	}{
		{"retrieval_id", func(u *MemoryUsage) { u.RetrievalID = "" }},
		{"root_task_id", func(u *MemoryUsage) { u.RootTaskID = "" }},
		{"memory_context", func(u *MemoryUsage) { u.MemoryContext = nil }},
		{"context_signature_version", func(u *MemoryUsage) { u.ContextSignatureVersion = 0 }},
		{"context_signature", func(u *MemoryUsage) { u.ContextSignature = "" }},
		{"context_descriptor_ref", func(u *MemoryUsage) { u.ContextDescriptorRef = "" }},
		{"observation_provenance", func(u *MemoryUsage) { u.ObservationProvenance = nil }},
	}
	for _, tc := range fields {
		u := base()
		tc.mut(&u)
		if err := u.Validate(); err == nil {
			t.Errorf("partial anchor (%s) must be rejected", tc.name)
		}
	}
	// Partial-but-nonempty scalar with a nil context must also fail.
	u := base()
	u.MemoryContext = nil
	if err := u.Validate(); err == nil {
		t.Error("anchored scalars with nil memory_context must be rejected")
	}
}

func TestAnchoredUsageRejectsUnsafeIDs(t *testing.T) {
	base := func() MemoryUsage { return anchoredUsage(t) }
	cases := []struct {
		name string
		mut  func(*MemoryUsage)
	}{
		{"retrieval traversal", func(u *MemoryUsage) { u.RetrievalID = "../retrieval" }},
		{"root task traversal", func(u *MemoryUsage) { u.RootTaskID = "task/../root" }},
		{"context descriptor traversal", func(u *MemoryUsage) { u.ContextDescriptorRef = "/etc/passwd" }},
		{"bad signature version", func(u *MemoryUsage) { u.ContextSignatureVersion = 2 }},
		{"bad signature hash", func(u *MemoryUsage) { u.ContextSignature = "sha256_zz" }},
	}
	for _, tc := range cases {
		u := base()
		tc.mut(&u)
		if err := u.Validate(); err == nil {
			t.Errorf("%s: must be rejected", tc.name)
		}
	}
}

func TestAnchoredAnchorChangeAltersHash(t *testing.T) {
	base := func() MemoryUsage { return anchoredUsage(t) }
	fields := []struct {
		name string
		mut  func(*MemoryUsage)
	}{
		{"retrieval_id", func(u *MemoryUsage) { u.RetrievalID = "retrieval_002" }},
		{"root_task_id", func(u *MemoryUsage) { u.RootTaskID = "task_root_008" }},
		{"generation id", func(u *MemoryUsage) { u.MemoryContext.ProjectGenerationRef.GenerationID = "gen_project_000011" }},
		{"context_signature", func(u *MemoryUsage) { u.ContextSignature = testHash2 }},
		{"context_descriptor_ref", func(u *MemoryUsage) { u.ContextDescriptorRef = "context_02K" }},
		{"provenance evidence", func(u *MemoryUsage) { u.ObservationProvenance.EvidenceRef.EvidenceID = "event_02K" }},
	}
	prev := base()
	hPrev, err := prev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range fields {
		u := base()
		tc.mut(&u)
		h, err := u.ContentHash()
		if err != nil {
			t.Fatal(err)
		}
		if h == hPrev {
			t.Errorf("%s: anchor change must alter content hash", tc.name)
		}
	}
}

func TestObservationProvenanceUnknownSourceRedacted(t *testing.T) {
	// The unknown-source error must not echo attacker-controlled input.
	p := ObservationProvenance{
		Source: "sk-verysecret/../../etc/passwd; rm -rf /tmp",
		EvidenceRef: &EvidenceRef{
			Scope: ScopeProject, EvidenceType: "reasonix_event",
			EvidenceID: "event_01K", ContentSHA256: testHash,
		},
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("unknown source must be rejected")
	}
	for _, banned := range []string{"sk-verysecret", "/etc/passwd", "rm -rf", "/tmp"} {
		if strings.Contains(err.Error(), banned) {
			t.Errorf("unknown-source error leaks %q: %v", banned, err)
		}
	}
}

func TestMemoryContextBothNilRejected(t *testing.T) {
	// An empty JSON object for memory_context must be rejected, never
	// treated as a valid context.
	u := anchoredUsage(t)
	u.MemoryContext = &MemoryContext{}
	if err := u.Validate(); err == nil {
		t.Error("empty memory_context object must be rejected")
	}
	// Canonicalization of a partial-anchor usage must fail cleanly (no panic).
	u2 := anchoredUsage(t)
	u2.MemoryContext = nil
	if _, err := u2.CanonicalBytes(); err == nil {
		t.Error("canonicalization of partial anchors must fail, not panic")
	}
	if _, err := u2.ContentHash(); err == nil {
		t.Error("content hash of partial anchors must fail, not panic")
	}
}

// ---- 5.3 legacy compatibility ----

func TestLegacyUsageGoldenLock(t *testing.T) {
	u, err := DecodeStrict[MemoryUsage]([]byte(legacyMemoryUsageJSON))
	if err != nil {
		t.Fatalf("legacy JSON must stay decodable: %v", err)
	}
	if err := u.Validate(); err != nil {
		t.Fatalf("legacy usage must validate: %v", err)
	}
	cb, err := u.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(cb) != legacyMemoryUsageJSON {
		t.Errorf("legacy canonical bytes changed:\n got %s\nwant %s", cb, legacyMemoryUsageJSON)
	}
	h, err := u.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if h != legacyMemoryUsageHash {
		t.Errorf("legacy content hash changed: %s != %s", h, legacyMemoryUsageHash)
	}
}

func TestLegacyUsageRejectsPartialAnchors(t *testing.T) {
	// An anchored field alone (retrieval_id) without the rest must fail.
	in := `{"schema_version":1,"usage_id":"usage_partial","scope":"project","memory_id":"mem_01K7A9X2","revision":2,"usage_stage":"affected","occurred_at":"2026-08-11T10:00:00Z","source":"local_user","content_sha256":"sha256_1111111111111111111111111111111111111111111111111111111111111111","created_at":"2026-08-11T10:05:00Z","retrieval_id":"retrieval_001"}`
	if _, err := DecodeStrict[MemoryUsage]([]byte(in)); err == nil {
		t.Fatal("partial anchor JSON must be rejected")
	}
}

func TestLegacyAnchoredIdentityConflict(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	legacy := validUsage()
	res, err := s.Put(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != WriteCreated {
		t.Fatal("legacy usage must be created")
	}
	anchored := anchoredUsage(t)
	anchored.UsageID = legacy.UsageID // same identity, different content
	h, err := anchored.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	anchored.ContentSHA256 = h
	_, err = s.Put(context.Background(), anchored)
	if ErrorCode(err) != CodeIdentityConflict {
		t.Errorf("same usage_id with different anchors must conflict, got %v", err)
	}
}

func TestLegacyUsageDerivedStatsUnchanged(t *testing.T) {
	// Legacy usage continues to feed the existing basic usage_count /
	// last_used_at derivation, and anchors must not change Lifecycle/Health.
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)
	legacy := validUsage()
	if _, err := s.Put(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	state, err := DeriveState(context.Background(), s, DerivedStateRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	var found *DerivedMemoryState
	for i := range state.States {
		if state.States[i].MemoryID == rev.MemoryID {
			found = &state.States[i]
		}
	}
	if found == nil {
		t.Fatal("derived entry missing for legacy usage memory")
	}
	// outcome_attributed requires ≥3 helps & ≥2 episodes: one legacy usage
	// alone must not promote; the anchored fields must not leak into stats.
	if found.Lifecycle == "active" {
		t.Errorf("single legacy usage must not promote lifecycle, got %s", found.Lifecycle)
	}
	if found.Usage.UsageCount < 1 {
		t.Errorf("legacy usage must still count in basic stats: %+v", found.Usage)
	}
}

// ---- 5.4 determinism & safety ----

func TestAnchoredUsageDeterministicEncoding(t *testing.T) {
	a := anchoredUsage(t)
	b1, err := a.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := a.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Error("repeat encoding must be byte-identical")
	}
}

func TestAnchoredUsageStrictDecoder(t *testing.T) {
	in := legacyMemoryUsageJSON
	// Inject an unknown top-level field.
	trimmed := strings.TrimSuffix(in, "}")
	unknown := trimmed + `,"anchor_unknown":true}`
	if _, err := DecodeStrict[MemoryUsage]([]byte(unknown)); err == nil {
		t.Fatal("unknown field must be rejected")
	}
}

func TestAnchoredUsageFilePersistence(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	u := anchoredUsage(t)
	if _, err := s.Put(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	// The stored file must be readable from disk and remain stable.
	path := filepath.Join(root, "facts", "memory-usages", "usage_anchored_01.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeStrict[MemoryUsage](data)
	if err != nil {
		t.Fatal(err)
	}
	if back.ContentSHA256 != u.ContentSHA256 {
		t.Errorf("persisted hash mismatch")
	}
}
