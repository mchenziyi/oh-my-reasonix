package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func usageCaptureFixture(t *testing.T) (*FactStore, LibrarianReceipt, MemoryUsageReceipt, MemoryRef) {
	t.Helper()
	return usageCaptureFixtureAt(t, tempRoot(t))
}

func usageCaptureFixtureAt(t *testing.T, root string) (*FactStore, LibrarianReceipt, MemoryUsageReceipt, MemoryRef) {
	t.Helper()
	_, store, _ := commitOKFGeneration(t, root, "usage_capture_world", nil)
	rctx, err := BuildRetrievalContext(context.Background(), RetrievalContextRequest{
		RetrievalID: "retrieval_capture_01", ProjectStore: store, ProjectScopeID: "project_capture",
		Now: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadLibrarianIndexTree(context.Background(), store, *rctx.Project)
	if err != nil {
		t.Fatal(err)
	}
	entry := tree.Root.Entries[0]
	ref := MemoryRef{Scope: entry.Scope, MemoryType: entry.MemoryType, MemoryID: entry.MemoryID, Revision: entry.Revision, ContentSHA256: entry.ContentSHA256}
	lr := LibrarianReceipt{SchemaVersion: 1, RetrievalID: rctx.RetrievalID, MemoryContext: *rctx, Status: LibrarianStatusFound, RecommendedPages: []LibrarianCandidate{{MemoryRef: ref, PagePath: entry.PagePath, RelevanceRank: 1, Why: "matches the fixed task context"}}, OptionalPages: []LibrarianCandidate{}, Conflicts: []LibrarianConflict{}, VisitedIndexPaths: []string{"wiki/index.md"}, FrozenPagesUsed: []MemoryRef{}, RequiresParentRead: true}
	if err := ValidateLibrarianReceipt(lr, map[Scope]*IndexTree{ScopeProject: tree}); err != nil {
		t.Fatal(err)
	}
	descriptor := validContextDescriptor(t, ScopeProject)
	episode := validEpisode(t, descriptor)
	episode.RootTaskID = "task_capture_01"
	h, err := episode.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	episode.ContentSHA256 = h
	put(t, store, descriptor)
	put(t, store, episode)
	ur := MemoryUsageReceipt{SchemaVersion: 1, RetrievalID: lr.RetrievalID, RootTaskID: episode.RootTaskID, MemoryContext: lr.MemoryContext, EpisodeRef: EpisodeRef{Scope: episode.Scope, EpisodeID: episode.EpisodeID, ContentSHA256: episode.ContentSHA256}, Usages: []MemoryUsageSelection{{MemoryRef: ref, UsageStage: "read"}}}
	return store, lr, ur, ref
}

func TestMemoryUsageCaptureCLIProcess(t *testing.T) {
	project := tempRoot(t)
	storeRoot := filepath.Join(project, ".reasonix", "omr", "memory")
	if err := os.MkdirAll(filepath.Dir(storeRoot), 0o700); err != nil {
		t.Fatal(err)
	}
	store, lr, receipt, _ := usageCaptureFixtureAt(t, storeRoot)
	receipt.Usages[0].UsageStage = "evaluated"
	librarianFile := filepath.Join(project, "librarian.json")
	usageFile := filepath.Join(project, "usage.json")
	lb, _ := json.Marshal(lr)
	ub, _ := receipt.EncodeCanonical()
	if err := os.WriteFile(librarianFile, lb, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usageFile, ub, 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "omr")
	_, source, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	build := exec.Command("go", "build", "-o", binary, "./cmd/omr")
	build.Dir = moduleRoot
	build.Env = append(os.Environ(), "GOCACHE=/tmp/omr-gocache")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	cmd := exec.Command(binary, "memory", "usage", "capture", "--project-dir", project, "--librarian-receipt", librarianFile, "--usage-receipt", usageFile, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capture CLI: %v\n%s", err, out)
	}
	var result CaptureUsageResult
	if err := json.Unmarshal(out, &result); err != nil || result.Created != 1 || len(result.UsageIDs) != 1 {
		t.Fatalf("unexpected CLI result: %s (%v)", out, err)
	}
	keys, err := store.List(context.Background(), FactKindMemoryUsage)
	if err != nil || len(keys) != 1 || keys[0] != result.UsageIDs[0] {
		t.Fatalf("CLI did not persist one usage: %v %v", keys, err)
	}
	attributionFile := filepath.Join(project, "attribution.json")
	attributionEvidence := EvidenceRef{Scope: receipt.EpisodeRef.Scope, EvidenceType: "episode", EvidenceID: receipt.EpisodeRef.EpisodeID, ContentSHA256: receipt.EpisodeRef.ContentSHA256}
	attribution := AttributionReceipt{SchemaVersion: 1, EpisodeRef: receipt.EpisodeRef, RootTaskID: receipt.RootTaskID, Candidates: []OutcomeCandidate{{UsageID: result.UsageIDs[0], TaskOutcome: "succeeded", MemoryEffect: "helped", Attribution: "confirmed", Critic: "not_required", EvidenceRefs: []EvidenceRef{attributionEvidence}}}}
	ab, _ := attribution.EncodeCanonical()
	if err := os.WriteFile(attributionFile, ab, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(binary, "memory", "outcome", "capture", "--project-dir", project, "--attribution-receipt", attributionFile, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("outcome CLI: %v\n%s", err, out)
	}
	var outcomeResult CaptureOutcomeResult
	if err := json.Unmarshal(out, &outcomeResult); err != nil || outcomeResult.Created != 1 || len(outcomeResult.OutcomeIDs) != 1 {
		t.Fatalf("unexpected outcome CLI result: %s (%v)", out, err)
	}
	outcomeKeys, err := store.List(context.Background(), FactKindOutcome)
	if err != nil || len(outcomeKeys) != 1 || outcomeKeys[0] != outcomeResult.OutcomeIDs[0] {
		t.Fatalf("CLI did not persist one outcome: %v %v", outcomeKeys, err)
	}
}

func TestBuildMemoryUsagesFoldsHighestStage(t *testing.T) {
	store, lr, receipt, ref := usageCaptureFixture(t)
	receipt.Usages = []MemoryUsageSelection{{MemoryRef: ref, UsageStage: "read"}, {MemoryRef: ref, UsageStage: "affected"}, {MemoryRef: ref, UsageStage: "adopted"}}
	got, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UsageStage != "affected" {
		t.Fatalf("unexpected folded usage: %+v", got)
	}
	if got[0].UsageID == "" || got[0].RootTaskID != receipt.RootTaskID || got[0].EpisodeID != receipt.EpisodeRef.EpisodeID || got[0].Source != "reasonix_receipt" {
		t.Fatalf("canonical fields missing: %+v", got[0])
	}
	if got[0].ObservationProvenance == nil || got[0].ObservationProvenance.Source != "runtime_observed" || got[0].ObservationProvenance.EvidenceRef.EvidenceType != "episode" {
		t.Fatalf("invalid provenance: %+v", got[0].ObservationProvenance)
	}
	a, _ := got[0].EncodeCanonical()
	again, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := again[0].EncodeCanonical()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same fixed inputs produced different usage bytes")
	}
}

func TestBuildMemoryUsagesAddsRetrievedCandidates(t *testing.T) {
	store, lr, receipt, _ := usageCaptureFixture(t)
	receipt.Usages = []MemoryUsageSelection{}
	got, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UsageStage != "retrieved" {
		t.Fatalf("candidate was not captured as retrieved: %+v", got)
	}
}

func TestBuildMemoryUsagesRejectsUnselectedMemory(t *testing.T) {
	store, lr, receipt, ref := usageCaptureFixture(t)
	ref.MemoryID = "mem_not_selected"
	receipt.Usages = []MemoryUsageSelection{{MemoryRef: ref, UsageStage: "read"}}
	if _, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}); ErrorCode(err) != CodeUsageCaptureInvalid {
		t.Fatalf("unselected memory must fail closed: %v", err)
	}
}

func TestBuildMemoryUsagesRejectsMixedScope(t *testing.T) {
	store, lr, receipt, ref := usageCaptureFixture(t)
	ref.Scope = ScopeGlobal
	receipt.Usages = append(receipt.Usages, MemoryUsageSelection{MemoryRef: ref, UsageStage: "read"})
	if _, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}); ErrorCode(err) != CodeUsageCaptureInvalid {
		t.Fatalf("mixed scope must fail closed: %v", err)
	}
}

func TestCommitMemoryUsagesIdempotent(t *testing.T) {
	store, lr, receipt, _ := usageCaptureFixture(t)
	req := CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}
	first, err := CommitMemoryUsages(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CommitMemoryUsages(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 1 || second.Noop != 1 || second.Created != 0 {
		t.Fatalf("unexpected idempotency results: %+v %+v", first, second)
	}
	keys, err := store.List(context.Background(), FactKindMemoryUsage)
	if err != nil || len(keys) != 1 {
		t.Fatalf("usage duplicated: %v %v", keys, err)
	}
}

func TestCommitMemoryUsagesConflictWritesNothing(t *testing.T) {
	store, lr, receipt, _ := usageCaptureFixture(t)
	built, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	conflict := built[0]
	conflict.UsageStage = "evaluated"
	conflict.ContentSHA256 = ""
	h, err := conflict.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	conflict.ContentSHA256 = h
	put(t, store, conflict)
	if _, err := CommitMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}); ErrorCode(err) != CodeIdentityConflict {
		t.Fatalf("identity conflict must fail before writing: %v", err)
	}
	keys, err := store.List(context.Background(), FactKindMemoryUsage)
	if err != nil || len(keys) != 1 {
		t.Fatalf("conflict changed the usage store: %v %v", keys, err)
	}
}

func TestBuildMemoryUsagesPendingEpisode(t *testing.T) {
	store, lr, receipt, _ := usageCaptureFixture(t)
	receipt.EpisodeRef.EpisodeID = "episode_missing"
	receipt.EpisodeRef.ContentSHA256 = testHash
	if _, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}); ErrorCode(err) != CodeUsageCapturePending {
		t.Fatalf("missing episode must be pending: %v", err)
	}
}

func TestBuildMemoryUsagesDoesNotReadNewCurrent(t *testing.T) {
	store, lr, receipt, _ := usageCaptureFixture(t)
	oldGeneration := lr.MemoryContext.Project.GenerationID
	commitOKFGeneration(t, store.root, "usage_capture_new_world", &oldGeneration)
	got, err := BuildMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].MemoryContext.ProjectGenerationRef.GenerationID != oldGeneration {
		t.Fatalf("capture switched to a newer CURRENT: %+v", got)
	}
}

func TestCapturedUsageDoesNotCreateOutcomeAttribution(t *testing.T) {
	store, lr, receipt, ref := usageCaptureFixture(t)
	if _, err := CommitMemoryUsages(context.Background(), CaptureUsageRequest{Store: store, LibrarianReceipt: lr, UsageReceipt: receipt}); err != nil {
		t.Fatal(err)
	}
	states, err := DeriveState(context.Background(), store, DerivedStateRequest{Scope: ScopeProject, Now: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states.States {
		if state.MemoryID == ref.MemoryID && state.Revision == ref.Revision {
			if state.Usage.UsageCount != 1 || state.Usage.CountedHelpCount != 0 || state.Usage.CountedHarmCount != 0 {
				t.Fatalf("capture incorrectly attributed an outcome: %+v", state.Usage)
			}
			return
		}
	}
	t.Fatal("captured memory state not found")
}

func TestMemoryUsageReceiptStrictDecode(t *testing.T) {
	_, _, receipt, _ := usageCaptureFixture(t)
	b, err := receipt.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStrict[MemoryUsageReceipt](b); err != nil {
		t.Fatal(err)
	}
	injected := append(b[:len(b)-1], []byte(`,"effect":"helped"}`)...)
	if _, err := DecodeStrict[MemoryUsageReceipt](injected); err == nil {
		t.Fatal("unknown attribution field must be rejected")
	}
}

func TestMemoryUsageReceiptErrorsAreRedacted(t *testing.T) {
	_, _, receipt, ref := usageCaptureFixture(t)
	secret := "/Users/private/.ssh/id_rsa; password=hunter2"
	receipt.Usages = []MemoryUsageSelection{{MemoryRef: ref, UsageStage: secret}}
	err := receipt.Validate()
	if ErrorCode(err) != CodeUsageCaptureInvalid {
		t.Fatalf("invalid stage must use stable code: %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "/Users/")) {
		t.Fatalf("capture error leaked untrusted input: %v", err)
	}
}
