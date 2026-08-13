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

func TestMemoryWebExportRequiresExplicitNowAndOutput(t *testing.T) {
	project := t.TempDir()
	if _, err := mem.OpenProject(memoryStoreRoot(project), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"web", "export", "--project-dir", project, "--output", filepath.Join(project, "memory.html")}); err == nil || !strings.Contains(err.Error(), "now is required") {
		t.Fatalf("web export must require explicit now, got %v", err)
	}
	if err := runMemory([]string{"web", "export", "--project-dir", project, "--now", "2026-08-14T00:00:00Z"}); err == nil || !strings.Contains(err.Error(), "requires --output") {
		t.Fatalf("web export must require output, got %v", err)
	}
}

func TestMemoryWebExportRejectsUnsafeOutput(t *testing.T) {
	project, outside := t.TempDir(), t.TempDir()
	if _, err := mem.OpenProject(memoryStoreRoot(project), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(project, "memory.html")
	if err := os.Symlink(filepath.Join(outside, "outside.html"), link); err != nil {
		t.Fatal(err)
	}
	err := runMemory([]string{"web", "export", "--project-dir", project, "--now", "2026-08-14T00:00:00Z", "--output", link})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("web export must reject symlink output, got %v", err)
	}
}

func TestMemoryMigrationPlanFileIsStrictAndReadOnly(t *testing.T) {
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	if _, err := mem.OpenProject(memoryStoreRoot(sourceDir), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.OpenProject(memoryStoreRoot(targetDir), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	planFile := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(planFile, []byte(`{"schema_version":1,"operation":"migration_preview","source_scope":"project","target_scope":"project","generation_id":"gen_01K7A9X2SOURCE","input_manifest_sha256":"`+testHashForCLI+`","fact_count":0,"snapshot_required":true,"steps":["preview"],"eligible":true,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"migration", "doctor", "--source-dir", sourceDir, "--target-dir", targetDir, "--scope", "project", "--plan-file", planFile}); err == nil {
		t.Fatal("unknown plan fields must be rejected before migration work")
	}
}

func TestMemoryStatusRequiresExplicitNow(t *testing.T) {
	err := runMemory([]string{"status", "--memory-id", "mem_status_01", "--project-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "now is required") {
		t.Fatalf("memory status must require an explicit --now, got %v", err)
	}
}

func TestMemoryDerivedCommandsRequireExplicitNow(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"report", []string{"report", "--project-dir", t.TempDir()}},
		{"review get", []string{"get", "--project-dir", t.TempDir(), "--memory-id", "mem_status_01", "--revision", "1", "--review-mode"}},
		{"outcome override", []string{"outcome", "override", "outcome_01", "--project-dir", t.TempDir(), "--previous-effect", "helped", "--new-effect", "neutral", "--reason", "review"}},
		{"governance", []string{"freeze", "--project-dir", t.TempDir(), "--memory-id", "mem_status_01", "--revision", "1", "--reason", "test"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runMemory(tc.args)
			if err == nil || !strings.Contains(err.Error(), "now is required") {
				t.Fatalf("%s must require an explicit --now, got %v", tc.name, err)
			}
		})
	}
}

func TestMemoryCompileRequiresExplicitRequest(t *testing.T) {
	if err := runMemory([]string{"compile", "--project-dir", t.TempDir()}); err == nil || !strings.Contains(err.Error(), "--request") {
		t.Fatalf("compile must require an explicit request file, got %v", err)
	}
}

func TestMemoryIndexRebuildRequiresExplicitRequest(t *testing.T) {
	err := runMemory([]string{"index", "rebuild", "--project-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "requires --request") {
		t.Fatalf("index rebuild must require an explicit request, got %v", err)
	}
}

func TestMemoryIndexPublishRequiresRequestAndKey(t *testing.T) {
	if err := runMemory([]string{"index", "publish", "--project-dir", t.TempDir()}); err == nil || !strings.Contains(err.Error(), "requires --request") {
		t.Fatalf("index publish must require a request, got %v", err)
	}
	if err := runMemory([]string{"index", "publish", "--request", "/tmp/missing-index-request.json", "--project-dir", t.TempDir()}); err == nil || !strings.Contains(err.Error(), "request is unavailable") {
		t.Fatalf("index publish must reject an unavailable request before key validation, got %v", err)
	}
}

func TestMemoryEpisodeCommandsRequireFixedContext(t *testing.T) {
	if err := runMemory([]string{"episode", "list"}); err == nil || !strings.Contains(err.Error(), "context-file is required") {
		t.Fatalf("episode list must require fixed context, got %v", err)
	}
	if err := runMemory([]string{"episode", "show"}); err == nil || !strings.Contains(err.Error(), "episode-id is required") {
		t.Fatalf("episode show must require episode id, got %v", err)
	}
}

func TestMemoryIndexDoctorRequiresPinnedInputs(t *testing.T) {
	err := runMemory([]string{"index", "doctor", "--project-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "requires --index and --request") {
		t.Fatalf("index doctor must require explicit inputs, got %v", err)
	}
}

func TestMemoryHistoryAndUsageRequireMemoryID(t *testing.T) {
	if err := runMemory([]string{"history"}); err == nil || !strings.Contains(err.Error(), "requires memory id") {
		t.Fatalf("history must require memory id, got %v", err)
	}
	if err := runMemory([]string{"usage"}); err == nil || !strings.Contains(err.Error(), "requires memory id") {
		t.Fatalf("usage must require memory id, got %v", err)
	}
}

func TestMemoryHistoryAndUsageReadExistingStore(t *testing.T) {
	project := t.TempDir()
	if _, err := mem.OpenProject(memoryStoreRoot(project), mem.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"history", "usage"} {
		t.Run(command, func(t *testing.T) {
			out, err := captureRunOutput(func() error {
				return runMemory([]string{command, "mem_missing", "--project-dir", project, "--json"})
			})
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Scope     mem.Scope         `json:"scope"`
				MemoryID  string            `json:"memory_id"`
				Revisions []json.RawMessage `json:"revisions"`
				Usages    []json.RawMessage `json:"usages"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Scope != mem.ScopeProject || result.MemoryID != "mem_missing" {
				t.Fatalf("unexpected result identity: %+v", result)
			}
			if command == "history" && result.Revisions == nil {
				t.Fatal("history must return an empty array")
			}
			if command == "usage" && result.Usages == nil {
				t.Fatal("usage must return an empty array")
			}
		})
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
	err := runMemory([]string{"outcome", "override", "outcome_missing", "--project-dir", project, "--previous-effect", "helped", "--new-effect", "neutral", "--reason", "review", "--now", "2026-08-13T00:00:00Z"})
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
	if err := runMemory([]string{"freeze", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--reason", "test", "--now", "2026-08-14T00:00:00Z", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"get", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1"}); err == nil {
		t.Fatal("normal get must reject frozen memory")
	}
	review, err := captureRunOutput(func() error {
		return runMemory([]string{"get", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--include-frozen", "--review-mode", "--now", "2026-08-14T00:00:00Z", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(review, `"frozen":true`) {
		t.Fatalf("review read did not expose frozen state: %s", review)
	}
	if err := runMemory([]string{"outcome", "override", outcome.OutcomeID, "--project-dir", project, "--previous-effect", "helped", "--new-effect", "neutral", "--reason", "review", "--now", "2026-08-14T00:00:00Z", "--json"}); err != nil {
		t.Fatal(err)
	}
	if err := runMemory([]string{"unfreeze", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--reason", "corrected", "--now", "2026-08-14T00:00:00Z", "--json"}); err != nil {
		t.Fatal(err)
	}
	status, err := captureRunOutput(func() error {
		return runMemory([]string{"status", "--project-dir", project, "--memory-id", rev.MemoryID, "--revision", "1", "--now", "2026-08-14T00:00:00Z", "--json"})
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
