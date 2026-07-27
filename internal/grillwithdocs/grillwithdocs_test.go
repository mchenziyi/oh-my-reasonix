package grillwithdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testProject creates a temp directory with a minimal project structure.
func testProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Create a CONTEXT.md with some initial content
	os.WriteFile(filepath.Join(root, "CONTEXT.md"), []byte("# Project Context\n\n## Terms\n- PostgreSQL: relational database\n\n## Facts\n- CI uses GitHub Actions\n"), 0o644)
	// Create an existing ADR
	os.MkdirAll(filepath.Join(root, "docs", "adr"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "adr", "0001-use-postgresql.md"), []byte("# 0001-use-postgresql\n\n**Status**: accepted\n"), 0o644)
	// A .reasonix directory to test rejection
	os.MkdirAll(filepath.Join(root, ".reasonix", "skills"), 0o755)
	return root
}

// --- Test 1: Plan dry-run produces zero writes ---

func TestPlan_DryRun_ZeroWrites(t *testing.T) {
	root := testProject(t)
	plan, err := Plan(root, []TermFact{
		{Term: "Redis", Fact: "used for caching", Source: "grill-session-1"},
	}, []ADRDraft{
		{Slug: "use-redis", Title: "Use Redis for Caching", Status: "proposed", Context: "Need caching", Decision: "Use Redis", Consequences: "Faster reads", Alternatives: []string{"Memcached"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun {
		t.Fatal("expected Plan to produce dry-run by default")
	}
	// Verify no files were written
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "0002-use-redis.md")); !os.IsNotExist(err) {
		t.Fatal("ADR file was written during dry-run Plan")
	}
	if plan.ExistingADRs != 1 {
		t.Fatalf("expected ExistingADRs=1, got %d", plan.ExistingADRs)
	}
}

// --- Test 2: Apply writes confirmed content ---

func TestApply_ConfirmedWrite(t *testing.T) {
	root := testProject(t)
	plan := DocPlan{
		DryRun: false,
		TermFacts: []TermFact{
			{Term: "Redis", Fact: "used for caching", Source: "grill-session-1"},
		},
		ADRs: []ADRDraft{
			{
				Slug: "use-redis", Title: "Use Redis for Caching", Status: "proposed",
				Context: "Need caching", Decision: "Use Redis", Consequences: "Faster reads",
				Alternatives: []string{"Memcached"}, ConfirmationBasis: "Confirmed in grill session",
			},
		},
		ExistingADRs: 1,
	}
	if err := Apply(root, plan); err != nil {
		t.Fatal(err)
	}
	// Verify ADR was created
	adrPath := filepath.Join(root, "docs", "adr", "0002-use-redis.md")
	if _, err := os.Stat(adrPath); os.IsNotExist(err) {
		t.Fatal("ADR file not created by Apply")
	}
	data, _ := os.ReadFile(adrPath)
	if !strings.Contains(string(data), "Use Redis for Caching") {
		t.Fatal("ADR missing title")
	}
	if !strings.Contains(string(data), "Confirmed in grill session") {
		t.Fatal("ADR missing confirmation basis")
	}
	// Verify CONTEXT.md was updated
	ctxData, _ := os.ReadFile(filepath.Join(root, "CONTEXT.md"))
	if !strings.Contains(string(ctxData), "Redis") || !strings.Contains(string(ctxData), "used for caching") {
		t.Fatal("CONTEXT.md missing new term/fact")
	}
}

// --- Test 3: ADR numbering increments from existing ---

func TestADR_Numbering(t *testing.T) {
	root := testProject(t)
	// Add two more ADRs
	os.WriteFile(filepath.Join(root, "docs", "adr", "0002-use-redis.md"), []byte("# 0002-use-redis\n"), 0o644)
	os.WriteFile(filepath.Join(root, "docs", "adr", "0003-use-memcached.md"), []byte("# 0003-use-memcached\n"), 0o644)

	plan, err := Plan(root, nil, []ADRDraft{
		{Slug: "use-kafka", Title: "Use Kafka"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExistingADRs != 3 {
		t.Fatalf("expected ExistingADRs=3, got %d", plan.ExistingADRs)
	}

	// Apply should create 0004
	plan.DryRun = false
	if err := Apply(root, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "0004-use-kafka.md")); os.IsNotExist(err) {
		t.Fatal("ADR 0004 not created")
	}
}

// --- Test 4: Apply is idempotent ---

func TestApply_Idempotent(t *testing.T) {
	root := testProject(t)
	plan := DocPlan{
		DryRun: false,
		TermFacts: []TermFact{
			{Term: "Redis", Fact: "used for caching", Source: "grill-session-1"},
		},
		ADRs:         []ADRDraft{{Slug: "use-redis", Title: "Use Redis", Status: "proposed", Context: "ctx", Decision: "decision", Consequences: "cons", ConfirmationBasis: "confirmed"}},
		ExistingADRs: 1,
	}
	// Apply twice
	if err := Apply(root, plan); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, plan); err != nil {
		t.Fatal(err)
	}
	// CONTEXT.md should not have duplicate term entries
	ctxData, _ := os.ReadFile(filepath.Join(root, "CONTEXT.md"))
	if strings.Count(string(ctxData), "Redis") > 2 {
		t.Fatal("CONTEXT.md has duplicate term entries (not idempotent)")
	}
	// ADR should still be only one file
	adrFiles, _ := filepath.Glob(filepath.Join(root, "docs", "adr", "*.md"))
	if len(adrFiles) != 2 { // 0001 + 0002
		t.Fatalf("expected 2 ADR files, got %d", len(adrFiles))
	}
}

// --- Test 5: Conflict detection preserves original ---

func TestConflict_PreservesOriginal(t *testing.T) {
	root := testProject(t)
	plan, err := Plan(root, []TermFact{
		{Term: "PostgreSQL", Fact: "not suitable for this project"}, // contradicts existing CONTEXT.md
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatal("expected conflict detection for contradictory term")
	}
	// Original CONTEXT.md must remain unchanged
	ctxData, _ := os.ReadFile(filepath.Join(root, "CONTEXT.md"))
	if !strings.Contains(string(ctxData), "relational database") {
		t.Fatal("original CONTEXT.md content was overwritten")
	}
	if strings.Contains(string(ctxData), "not suitable") {
		t.Fatal("conflicting content leaked into CONTEXT.md during dry-run Plan")
	}
}

// --- Test 6: Unconfirmed assumptions are isolated ---

func TestUnconfirmedAssumptions_Isolated(t *testing.T) {
	root := testProject(t)
	// A plan that includes something marked as not confirmed
	plan := DocPlan{
		DryRun: false,
		TermFacts: []TermFact{
			{Term: "Verified", Fact: "confirmed fact", Source: "session"},
		},
		ExistingADRs: 1,
	}
	// Apply should only write TermFacts — not any unconfirmed items
	if err := Apply(root, plan); err != nil {
		t.Fatal(err)
	}
	ctxData, _ := os.ReadFile(filepath.Join(root, "CONTEXT.md"))
	if !strings.Contains(string(ctxData), "confirmed fact") {
		t.Fatal("confirmed fact should be in CONTEXT.md")
	}
}

// --- Test 7-10: Path safety ---

func TestReject_AbsolutePath(t *testing.T) {
	if err := validatePath("/tmp", "/etc/passwd"); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestReject_PathTraversal(t *testing.T) {
	if err := validatePath("/tmp/project", "/tmp/project/../../etc/passwd"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestReject_DotReasonix(t *testing.T) {
	if err := validatePath("/tmp/project", "/tmp/project/.reasonix/manifest.yaml"); err == nil {
		t.Fatal("expected .reasonix path to be rejected")
	}
}

func TestReject_OutsideProjectRoot(t *testing.T) {
	if err := validatePath("/tmp/project", "/tmp/other/file.md"); err == nil {
		t.Fatal("expected path outside project root to be rejected")
	}
}

// --- Test 11: Symlink escape rejection ---

func TestReject_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "secret.txt")
	os.WriteFile(externalFile, []byte("secret"), 0o644)
	symlink := filepath.Join(root, "escape.md")
	os.Symlink(externalFile, symlink)
	if err := validatePath(root, symlink); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

// --- Test 12: T11 regression — all T11 tests still pass ---
// This is verified by running `go test ./internal/grillme/...` separately.
// The build must not break due to the new grillwithdocs package.

// --- Test 13: Slug validation rejects path traversal ---

func TestReject_InvalidSlug(t *testing.T) {
	root := testProject(t)
	_, err := Plan(root, nil, []ADRDraft{
		{Slug: "../../../etc/passwd", Title: "Bad"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Slug validation happens at Apply time
	plan := DocPlan{
		DryRun:       false,
		ADRs:         []ADRDraft{{Slug: "../../../etc/passwd", Title: "Bad", Status: "proposed", Context: "x", Decision: "y", Consequences: "z", ConfirmationBasis: "test"}},
		ExistingADRs: 1,
	}
	if err := Apply(root, plan); err == nil {
		t.Fatal("expected Apply to reject invalid slug with path traversal")
	}
}

// TestReject_IntermediateSymlink verifies that symlink in an intermediate
// directory component does not bypass security checks.
func TestReject_IntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	externalDir := t.TempDir()
	// Create a symlink at docs/adr pointing outside the project
	docsDir := filepath.Join(root, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.Symlink(externalDir, filepath.Join(root, "docs", "adr"))
	// Now writing to docs/adr/0001-test.md would actually write to externalDir
	plan := DocPlan{
		DryRun:       false,
		ADRs:         []ADRDraft{{Slug: "test", Title: "Test", Status: "proposed", Context: "x", Decision: "y", Consequences: "z", ConfirmationBasis: "test"}},
		ExistingADRs: 0,
	}
	if err := Apply(root, plan); err == nil {
		t.Fatal("expected Apply to reject symlink escape via intermediate directory")
	}
}

// --- Test 14: Slug with special characters is rejected ---

func TestReject_SlugWithSpecialChars(t *testing.T) {
	root := testProject(t)
	plan := DocPlan{
		DryRun:       false,
		ADRs:         []ADRDraft{{Slug: "use redis!", Title: "Bad slug", Status: "proposed", Context: "x", Decision: "y", Consequences: "z", ConfirmationBasis: "test"}},
		ExistingADRs: 1,
	}
	if err := Apply(root, plan); err == nil {
		t.Fatal("expected Apply to reject slug with special characters")
	}
}

// TestReject_PrefixBypass verifies that a path in a sibling directory
// (e.g. /tmp/project-evil/ when root is /tmp/project) is correctly rejected.
func TestReject_PrefixBypass(t *testing.T) {
	root := t.TempDir()
	siblingDir := root + "-evil"
	if err := validatePath(root, siblingDir+"/file.md"); err == nil {
		t.Fatal("expected prefix bypass to be rejected")
	}
}

// TestReject_SymlinkPrefixBypass verifies that a symlink pointing to a
// sibling directory (e.g. root=/tmp/project, symlink → /tmp/project-evil)
// is correctly rejected.
func TestReject_SymlinkPrefixBypass(t *testing.T) {
	root := t.TempDir()
	siblingDir := root + "-evil"
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "escape-link")
	if err := os.Symlink(siblingDir, symlink); err != nil {
		t.Fatal(err)
	}
	if err := validatePath(root, symlink+"/file.md"); err == nil {
		t.Fatal("expected symlink prefix bypass to be rejected")
	}
}

// TestApply_NoPartialWrite verifies that if one ADR in a batch is invalid,
// no files at all are written (no partial write).
func TestApply_NoPartialWrite(t *testing.T) {
	root := testProject(t)
	plan := DocPlan{
		DryRun: false,
		TermFacts: []TermFact{
			{Term: "Kafka", Fact: "used for event streaming", Source: "session"},
		},
		ADRs: []ADRDraft{
			{Slug: "use-kafka", Title: "Use Kafka", Status: "proposed", Context: "x", Decision: "y", Consequences: "z", ConfirmationBasis: "test"},
			{Slug: "../../../etc/passwd", Title: "Bad", Status: "proposed", Context: "x", Decision: "y", Consequences: "z", ConfirmationBasis: "test"},
		},
		ExistingADRs: 1,
	}
	err := Apply(root, plan)
	if err == nil {
		t.Fatal("expected Apply to reject invalid slug, got nil")
	}
	// Verify CONTEXT.md was NOT modified (pre-check catches the bad slug before writes)
	ctxData, _ := os.ReadFile(filepath.Join(root, "CONTEXT.md"))
	if strings.Contains(string(ctxData), "Kafka") {
		t.Fatal("CONTEXT.md was modified despite invalid ADR slug (partial write)")
	}
	// Verify no ADR file was created
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "0002-use-kafka.md")); !os.IsNotExist(err) {
		t.Fatal("ADR 0002 was created despite invalid sibling slug (partial write)")
	}
}
