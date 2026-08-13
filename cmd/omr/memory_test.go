package main

import (
	"context"
	"encoding/json"
	"errors"
	mem "github.com/mchenziyi/oh-my-reasonix/internal/memory"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenExistingMemoryStoreDoesNotCreateMissingStore(t *testing.T) {
	project := t.TempDir()
	if _, err := openExistingMemoryStore(project, "project"); err == nil {
		t.Fatal("missing memory store must fail")
	}
	if _, err := os.Stat(memoryStoreRoot(project)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only command created a memory store")
	}
}

func TestMemoryPairedBenchmarkCLI(t *testing.T) {
	dir := t.TempDir()
	fixture := mem.PairedBenchmarkFixture{
		SchemaVersion: 1,
		FixtureID:     "paired_cli_fixture",
		Cases: []mem.PairedBenchmarkCase{{
			CaseID: "case_1",
			Mnemosyne: mem.PairedBenchmarkArm{
				RetrievalHits: 1, RetrievalCandidates: 1, Reads: 1, Adoptions: 1,
				DownstreamSuccess: 1, DownstreamTotal: 1,
			},
			Native: mem.PairedBenchmarkArm{RetrievalCandidates: 1, Reads: 1, DownstreamTotal: 1},
		}},
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := captureRunOutput(func() error {
		return runMemory([]string{"benchmark", "paired", "--paired-fixture", path, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var report mem.PairedBenchmarkReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("CLI output must be JSON: %v", err)
	}
	if report.FixtureID != fixture.FixtureID || report.CaseCount != 1 || report.EvidenceStatus != "insufficient_evidence" {
		t.Fatalf("unexpected paired report: %+v", report)
	}
}

func TestMemoryRetrievalAuditRequiresExplicitNowAndDoesNotCreateStore(t *testing.T) {
	project := t.TempDir()
	if err := runMemory([]string{"retrieval", "audit", "evaluation_01", "--project-dir", project}); err == nil {
		t.Fatal("retrieval audit must require explicit --now")
	}
	if _, err := os.Stat(memoryStoreRoot(project)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retrieval audit created a missing store")
	}
}

func TestMemoryStatusRequiresExplicitNow(t *testing.T) {
	err := runMemory([]string{"status", "--memory-id", "mem_status_01", "--project-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "now is required") {
		t.Fatalf("memory status must require an explicit --now, got %v", err)
	}
}

func TestReadBoundedJSONFileRejectsUnsafeInput(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "context.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(link); err == nil {
		t.Fatal("symlink input must fail")
	}
	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, []byte(strings.Repeat("x", 1<<20+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedJSONFile(large); err == nil {
		t.Fatal("oversized input must fail")
	}
}

func TestStrictJSONRejectsUnknownAndTrailingData(t *testing.T) {
	var doc episodicContextDocument
	if err := strictJSON([]byte(`{"schema_version":1,"unknown":true}`), &doc); err == nil {
		t.Fatal("unknown field must fail")
	}
	if err := strictJSON([]byte(`{"schema_version":1}{}`), &doc); err == nil {
		t.Fatal("trailing JSON must fail")
	}
}

func TestExtractStringFlagForms(t *testing.T) {
	for _, args := range [][]string{{"--episode-id", "episode_1", "--json"}, {"--episode-id=episode_1", "--json"}} {
		got, rest, err := extractStringFlag(args, "episode-id")
		if err != nil || got != "episode_1" || len(rest) != 1 || rest[0] != "--json" {
			t.Fatalf("unexpected result: %q %v %v", got, rest, err)
		}
	}
}

func TestMemoryCLIRejectsIncompleteRequests(t *testing.T) {
	if err := runMemory([]string{"get", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing memory id/revision must fail")
	}
	for _, op := range []string{"pin", "unpin", "freeze", "unfreeze", "archive"} {
		if err := runMemory([]string{op, "--project-dir", t.TempDir()}); err == nil {
			t.Fatalf("missing governance arguments must fail for %s", op)
		}
	}
	if err := runMemory([]string{"episodic", "card", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing episode id must fail")
	}
	if err := runMemory([]string{"episodic", "validate-receipt", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing receipt file must fail")
	}
	if err := runMemory([]string{"usage", "capture", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing usage receipt files must fail")
	}
	if err := runMemory([]string{"outcome", "capture", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing attribution receipt must fail")
	}
	if err := runMemory([]string{"doctor", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing memory store must fail for consistency doctor")
	}
	if err := runMemory([]string{"status", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing memory id must fail for status")
	}
	if err := runMemory([]string{"report", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("missing memory store must fail for report")
	}
	if err := runMemory([]string{"migration", "preview"}); err == nil {
		t.Fatal("migration preview must require source, target and generation")
	}
	if err := runMemory([]string{"migration", "doctor"}); err == nil {
		t.Fatal("migration doctor must require source, target and generation")
	}
	if err := runMemory([]string{"migration", "copy"}); err == nil {
		t.Fatal("migration copy must require source, target and generation")
	}
	if err := runMemory([]string{"generalize", "apply"}); err == nil {
		t.Fatal("generalize apply must require explicit plan and target")
	}
	if err := runMemory([]string{"promotion", "apply"}); err == nil {
		t.Fatal("promotion apply must require explicit plan, policy and target")
	}
	if err := runMemory([]string{"promotion", "candidate", "put"}); err == nil {
		t.Fatal("promotion candidate put must require global store and input")
	}
	if err := runMemory([]string{"promotion", "candidate", "apply"}); err == nil {
		t.Fatal("promotion candidate apply must require global store and input")
	}
	if err := runMemory([]string{"promotion", "generation", "publish"}); err == nil {
		t.Fatal("promotion generation publish must require global store and input")
	}
	if err := runMemory([]string{"rollback", "generation_1", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("rollback must require explicit audit fields")
	}
	if err := runMemory([]string{"repair", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("repair must not create a missing memory store")
	}
	if err := runMemory([]string{"list", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("list must not create a missing memory store")
	}
	if err := runMemory([]string{"show", "fact_key", "--project-dir", t.TempDir()}); err == nil {
		t.Fatal("show must require a fact kind")
	}
}

func TestMemoryPromotionCandidatePutCLI(t *testing.T) {
	project := t.TempDir()
	global := t.TempDir()
	if _, err := mem.OpenGlobal(memoryStoreRoot(global), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	candidate := mem.GlobalPromotionCandidate{
		SchemaVersion: mem.SchemaVersion, CandidateID: "promotion_cli_candidate", Status: "collecting",
		UsagePolicy: mem.UsagePolicyOutcomeAttributed,
		SourceMemoryRefs: []mem.MemoryRef{
			{Scope: mem.ScopeProject, MemoryType: "strategy", MemoryID: "memory_cli_a", Revision: 1, ContentSHA256: testHashForCLI},
			{Scope: mem.ScopeProject, MemoryType: "strategy", MemoryID: "memory_cli_b", Revision: 1, ContentSHA256: testHashForCLI},
		},
		SourceProjectFamilyFingerprints: []string{testHashForCLI, "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		OutcomeRefs:                     []string{"outcome_cli"}, EvidenceRefs: []mem.EvidenceRef{}, CriticJudgmentRefs: []mem.JudgmentRef{},
		ProposedAppliesWhen: []mem.ApplicabilityCondition{}, ProposedDoesNotApplyWhen: []mem.ApplicabilityCondition{},
	}
	var err error
	candidate.ContentSHA256, err = candidate.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(project, "candidate.json")
	b, err := candidate.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRunOutput(func() error {
		return runMemory([]string{"promotion", "candidate", "put", "--global-dir", global, "--input", input, "--json"})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRunOutput(func() error {
		return runMemory([]string{"promotion", "candidate", "put", "--global-dir", global, "--input", input, "--json"})
	}); err != nil {
		t.Fatalf("replaying the same candidate must be idempotent: %v", err)
	}
	store, err := mem.OpenGlobal(memoryStoreRoot(global), mem.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), mem.FactKindPromotionCandidate, candidate.CandidateID); err != nil {
		t.Fatalf("candidate was not persisted: %v", err)
	}
}

const testHashForCLI = "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestMemoryConsistencyDoctorCLIHealthyJSON(t *testing.T) {
	project := t.TempDir()
	if _, err := mem.OpenProject(memoryStoreRoot(project), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	out, err := captureRunOutput(func() error { return runMemory([]string{"doctor", "--project-dir", project, "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var report mem.ConsistencyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Healthy || len(report.Findings) != 0 || report.Scope != "project" {
		t.Fatalf("unexpected doctor report: %+v", report)
	}
}

func TestMemoryOutcomeOverrideAcceptsPositionalIDBeforeFlags(t *testing.T) {
	project := t.TempDir()
	err := runMemory([]string{"outcome", "override", "outcome_missing", "--project-dir", project, "--previous-effect", "helped", "--new-effect", "neutral", "--reason", "review"})
	if err == nil || strings.Contains(err.Error(), "outcome-id, previous-effect") {
		t.Fatalf("positional outcome id must not stop flag parsing: %v", err)
	}
}

func TestMemoryGovernanceFakeCLICycle(t *testing.T) {
	project := t.TempDir()
	store, err := mem.OpenProject(memoryStoreRoot(project), mem.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := mem.MemoryRevision{SchemaVersion: 1, MemoryID: "mem_cli_cycle", MemoryType: mem.MemoryTypeStrategy, Scope: mem.ScopeProject, CanonicalKey: "cli-cycle", Revision: 1, UsagePolicy: mem.UsagePolicyOutcomeAttributed, Title: "CLI cycle", Summary: "test", CreatedAt: "2026-08-13T00:00:00Z"}
	h, err := rev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	rev.ContentSHA256 = h
	if _, err := store.Put(nilContext(), rev); err != nil {
		t.Fatal(err)
	}
	usage := mem.MemoryUsage{SchemaVersion: 1, UsageID: "usage_cli_cycle", Scope: mem.ScopeProject, MemoryID: rev.MemoryID, Revision: 1, UsageStage: "affected", OccurredAt: "2026-08-13T00:00:00Z", Source: "test", CreatedAt: "2026-08-13T00:00:00Z"}
	h, err = usage.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	usage.ContentSHA256 = h
	if _, err := store.Put(nilContext(), usage); err != nil {
		t.Fatal(err)
	}
	outcome := mem.Outcome{SchemaVersion: 1, OutcomeID: "outcome_cli_cycle", Scope: mem.ScopeProject, UsageID: "usage_cli_cycle", MemoryID: rev.MemoryID, Revision: 1, Effect: "helped", CreatedAt: "2026-08-13T00:00:00Z"}
	h, err = outcome.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	outcome.ContentSHA256 = h
	if _, err := store.Put(nilContext(), outcome); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"freeze", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--reason", "test", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"get", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1"}); err == nil {
		t.Fatal("normal get must reject frozen memory")
	}
	review, err := captureRunOutput(func() error {
		return runMemory([]string{"get", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--include-frozen", "--review-mode", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(review, `"frozen":true`) {
		t.Fatalf("review read did not expose frozen state: %s", review)
	}
	if err := runMemory([]string{"outcome", "override", outcome.OutcomeID, "--project-dir", project, "--previous-effect", "helped", "--new-effect", "neutral", "--reason", "review", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"unfreeze", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--reason", "corrected", "--json"}); err != nil {
		t.Fatal(err)
	}
	status, err := captureRunOutput(func() error {
		return runMemory([]string{"status", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--now", "2026-08-13T00:00:00Z", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(status, `"lifecycle":"frozen"`) {
		t.Fatalf("unfreeze did not clear frozen lifecycle: %s", status)
	}
}

// nilContext keeps the test focused on the CLI cycle; FactStore accepts a
// context only to honor cancellation, and a background context is sufficient.
func nilContext() context.Context { return context.Background() }

func TestMemoryUsageCLIRejectsUnknownFieldsBeforeStoreAccess(t *testing.T) {
	dir := t.TempDir()
	librarian := filepath.Join(dir, "librarian.json")
	usage := filepath.Join(dir, "usage.json")
	if err := os.WriteFile(librarian, []byte(`{"schema_version":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usage, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"usage", "capture", "--project-dir", dir, "--librarian-receipt", librarian, "--usage-receipt", usage}); err == nil {
		t.Fatal("unknown field must fail")
	}
	if _, err := os.Stat(memoryStoreRoot(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid input created a memory store")
	}
}
