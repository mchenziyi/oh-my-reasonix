package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("MEM_STORE_HELPER") == "1" {
		runStoreHelper()
		os.Exit(0)
	}
	if os.Getenv("MEM_GEN_HELPER") == "1" {
		runGenerationHelper()
		os.Exit(0)
	}
	if os.Getenv("MEM_OKF_HELPER") == "1" {
		runOKFHelper()
		os.Exit(0)
	}
	if os.Getenv("MEM_DERIVE_HELPER") == "1" {
		runDeriveHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runDeriveHelper derives the scope from a fixed root and prints the
// deterministic serialization, so a parent process can compare bytes across
// process boundaries (multi-process isolation and determinism).
func runDeriveHelper() {
	root := os.Getenv("MEM_DERIVE_ROOT")
	s, err := OpenProject(root, Options{})
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	res, err := DeriveState(context.Background(), s, DerivedStateRequest{Scope: ScopeProject, Now: deriveNow})
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	b, err := json.Marshal(res)
	if err != nil {
		fmt.Println("status=error:encode")
		return
	}
	fmt.Printf("status=ok\n%s\n", b)
}

// runOKFHelper compiles a fixed input set and commits it inside a child
// process (multi-process OKF compilation tests).
func runOKFHelper() {
	root := os.Getenv("MEM_GEN_ROOT")
	s, err := OpenProject(root, Options{})
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	gs := NewGenerationStore(s)
	rev := validRevision()
	ev := validEvidenceGeneration()
	if _, err := s.Put(context.Background(), rev); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if _, err := s.Put(context.Background(), ev); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if _, err := s.Put(context.Background(), policyOf(PolicyTypeIndex)); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	res, err := CompileOKF(context.Background(), s, okfRequest(rev, ev))
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	tx, err := gs.Begin(context.Background(), beginReq("mp_"+os.Getenv("MEM_GEN_KEY"), nil))
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, res.Inputs)); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, res.Outputs); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	cres, err := gs.Commit(context.Background(), tx)
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if cres.Status == CommitCommitted {
		fmt.Println("status=committed")
	} else {
		fmt.Println("status=already_committed")
	}
}

// runGenerationHelper executes one generation commit inside a child process
// (multi-process concurrency tests) and prints a machine-readable status.
func runGenerationHelper() {
	root := os.Getenv("MEM_GEN_ROOT")
	base := os.Getenv("MEM_GEN_BASE")
	s, err := OpenProject(root, Options{})
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	gs := NewGenerationStore(s)
	var basePtr *string
	if base != "" {
		basePtr = &base
	}
	tx, err := gs.Begin(context.Background(), beginReq("mp_"+os.Getenv("MEM_GEN_KEY"), basePtr))
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	gov := validGovernanceEvent()
	if err := gs.PrepareFact(context.Background(), tx, gov); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, []ManifestInput{inputFor(gov)})); err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	res, err := gs.Commit(context.Background(), tx)
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if res.Status == CommitCommitted {
		fmt.Println("status=committed")
	} else {
		fmt.Println("status=already_committed")
	}
}

// runStoreHelper is executed inside child processes for multi-process
// concurrency tests. It performs one Put of a fixed revision (or a
// conflicting variant) and prints a machine-readable status.
func runStoreHelper() {
	root := os.Getenv("MEM_STORE_ROOT")
	variant := os.Getenv("MEM_STORE_VARIANT")
	s, err := OpenProject(root, Options{})
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	r := validRevision()
	if variant == "conflict" {
		// Fresh identity (no file yet): the helper processes race to create
		// it, each with a different content (per-PID title), so exactly one
		// created and the rest conflict.
		r.MemoryID = "mem_helper_conflict"
		r.Title = fmt.Sprintf("Helper Variant %d", os.Getpid())
		r = fillRevisionHash(r)
	}
	res, err := s.Put(context.Background(), r)
	if err != nil {
		fmt.Println("status=error:" + string(ErrorCode(err)))
		return
	}
	if res.Status == WriteCreated {
		fmt.Println("status=created")
	} else {
		fmt.Println("status=noop")
	}
}

func openProject(t *testing.T, root string, opts Options) *FactStore {
	t.Helper()
	s, err := OpenProject(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// tempRoot returns an isolated, 0700 store root. Store directories must be
// owner-exclusive; t.TempDir() is not guaranteed 0700 in every environment
// (e.g. session temp trees may carry group/other bits), so tests that place
// a Store at the temp dir explicitly enforce 0700.
func tempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScopeIsolation(t *testing.T) {
	project := openProject(t, tempRoot(t), Options{})
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Cross-scope facts must be rejected by the store's own scope.
	globalRev := validRevision()
	globalRev.Scope = ScopeGlobal
	globalRev = fillRevisionHash(globalRev)
	if _, err := project.Put(context.Background(), globalRev); ErrorCode(err) != CodeScopeMismatch {
		t.Errorf("project store must reject global fact: %v", err)
	}
	if _, err := global.Put(context.Background(), validRevision()); ErrorCode(err) != CodeScopeMismatch {
		t.Errorf("global store must reject project fact: %v", err)
	}

	// Portable is schema-only in this phase: no store may accept it.
	portableRev := validRevision()
	portableRev.Scope = ScopePortable
	portableRev = fillRevisionHash(portableRev)
	if _, err := project.Put(context.Background(), portableRev); ErrorCode(err) != CodeScopeMismatch {
		t.Errorf("portable fact must not map into any store: %v", err)
	}

	// Scoped facts succeed in their matching store.
	if _, err := global.Put(context.Background(), globalRev); err != nil {
		t.Errorf("global store must accept global fact: %v", err)
	}
	if _, err := project.Put(context.Background(), validRevision()); err != nil {
		t.Errorf("project store must accept project fact: %v", err)
	}

	// Facts without a scope field are bound by the store they land in.
	ev := validEvidenceGeneration()
	if _, err := project.Put(context.Background(), ev); err != nil {
		t.Errorf("evidence generation has no scope field and must follow the store: %v", err)
	}
	if _, err := global.Put(context.Background(), ev); err != nil {
		t.Errorf("evidence generation must be writable to any store: %v", err)
	}
}

func validEvidenceGeneration() MemoryEvidenceGeneration {
	ev := MemoryEvidenceGeneration{
		SchemaVersion:      1,
		MemoryID:           "mem_01K7A9X2",
		Revision:           2,
		EvidenceGeneration: 3,
		EvidenceRefs:       []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}},
		TransactionID:      "tx_01K",
		CreatedAt:          "2026-08-07T00:00:00Z",
	}
	h, err := ev.ContentHash()
	if err != nil {
		panic(err)
	}
	ev.EvidenceSetSHA256 = h
	return ev
}

func TestPutGetRoundTripAndRouting(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	rev := validRevision()
	if _, err := s.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	ev := validEvidenceGeneration()
	if _, err := s.Put(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	judgment := validConfirmationJudgment()
	if _, err := s.Put(context.Background(), judgment); err != nil {
		t.Fatal(err)
	}
	policy := validPolicy()
	if _, err := s.Put(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	event := validGovernanceEvent()
	if _, err := s.Put(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	// Route checks: each fact lands in its kind directory.
	rel := map[string]string{
		"rev": "facts/memory-revisions/mem_01K7A9X2/2.json",
		"ev":  "facts/memory-evidence-generations/mem_01K7A9X2/2/3.json",
		"jud": "facts/judgments/judgment_01K.json",
		"pol": "facts/policies/freshness_policy_v1/1.json",
		"gov": "facts/governance-events/governance_01K.json",
	}
	for name, r := range rel {
		if _, err := os.Stat(filepath.Join(s.root, r)); err != nil {
			t.Errorf("%s: expected file at %s: %v", name, r, err)
		}
	}

	// Round trip: Get returns the exact canonical bytes.
	got, err := s.Get(context.Background(), FactKindMemoryRevision, "mem_01K7A9X2/2")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := rev.EncodeCanonical()
	if string(got) != string(want) {
		t.Error("Get must return the exact stored bytes")
	}
	decoded, err := DecodeStrict[MemoryRevision](got)
	if err != nil {
		t.Fatalf("stored bytes must decode strictly: %v", err)
	}
	if decoded.ContentSHA256 != rev.ContentSHA256 {
		t.Error("round-trip hash mismatch")
	}

	if ok, err := s.Exists(context.Background(), FactKindMemoryRevision, "mem_01K7A9X2/2"); err != nil || !ok {
		t.Errorf("Exists should be true: ok=%v err=%v", ok, err)
	}
	if ok, err := s.Exists(context.Background(), FactKindMemoryRevision, "mem_missing/1"); err != nil || ok {
		t.Errorf("Exists should be false for missing: ok=%v err=%v", ok, err)
	}
	if _, err := s.Get(context.Background(), FactKindMemoryRevision, "mem_missing/1"); ErrorCode(err) != CodeNotFound {
		t.Errorf("missing get: want not_found, got %v", err)
	}
}

func validGovernanceEvent() GovernanceEvent {
	return GovernanceEvent{
		SchemaVersion: 1,
		EventID:       "governance_01K",
		Scope:         ScopeProject,
		MemoryID:      "mem_01K7A9X2",
		Revision:      2,
		Operation:     "pin",
		Reason:        "user requested priority",
		Source:        "user",
		BasisRefs:     []BasisRef{},
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
}

func TestPutIdempotentNoop(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	rev := validRevision()
	res1, err := s.Put(context.Background(), rev)
	if err != nil || res1.Status != WriteCreated {
		t.Fatalf("first put: %v %v", res1, err)
	}
	firstBytes, _ := os.ReadFile(filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"))

	res2, err := s.Put(context.Background(), rev)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Status != WriteNoop {
		t.Errorf("second put must be noop, got %v", res2.Status)
	}
	secondBytes, _ := os.ReadFile(filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"))
	if string(firstBytes) != string(secondBytes) {
		t.Error("noop put must not rewrite the immutable fact")
	}
}

func TestPutIdentityConflictZeroWrite(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	rev := validRevision()
	if _, err := s.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"))

	conflict := validRevision()
	conflict.Title = "Different Content"
	conflict = fillRevisionHash(conflict)
	if _, err := s.Put(context.Background(), conflict); ErrorCode(err) != CodeIdentityConflict {
		t.Errorf("want identity_conflict, got %v", err)
	}
	afterBytes, _ := os.ReadFile(filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"))
	if string(firstBytes) != string(afterBytes) {
		t.Error("conflict must leave the immutable fact untouched")
	}
}

func TestPutRejectsInvalidFacts(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// Hash mismatch: field edited after hashing.
	rev := validRevision()
	rev.ContentSHA256 = "sha256_" + strings.Repeat("f", 64)
	if _, err := s.Put(context.Background(), rev); ErrorCode(err) != CodeHashMismatch {
		t.Errorf("want hash_mismatch, got %v", err)
	}

	// Schema invalid: empty title.
	rev2 := validRevision()
	rev2.Title = ""
	rev2 = fillRevisionHash(rev2)
	if _, err := s.Put(context.Background(), rev2); ErrorCode(err) != CodeSchemaInvalid {
		t.Errorf("want schema_invalid, got %v", err)
	}

	// Unsupported fact type must not be persisted.
	if _, err := s.Put(context.Background(), ApplicabilityCondition{}); err == nil {
		t.Error("conditions are not store facts and must be rejected")
	}
}

func TestPermissions(t *testing.T) {
	// Use a nested root so the store itself creates it (t.TempDir parents
	// are 0755 and must not be blamed on the store).
	root := filepath.Join(tempRoot(t), "store")
	s := openProject(t, root, Options{})
	if _, err := s.Put(context.Background(), validRevision()); err != nil {
		t.Fatal(err)
	}
	checkMode(t, filepath.Join(s.root), 0o700)
	checkMode(t, filepath.Join(s.root, "facts"), 0o700)
	checkMode(t, filepath.Join(s.root, "facts", "memory-revisions"), 0o700)
	checkMode(t, filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2"), 0o700)
	checkMode(t, filepath.Join(s.root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"), 0o600)
	checkMode(t, filepath.Join(s.root, "locks", "store.lock"), 0o600)
	checkMode(t, filepath.Join(s.root, "locks"), 0o700)
}

func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != want {
		t.Errorf("%s: mode %v, want %v", path, fi.Mode().Perm(), want)
	}
}

func TestAtomicFailureLeavesNoTempFiles(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	judgmentsDir := filepath.Join(s.root, "facts", "judgments")
	if err := os.Chmod(judgmentsDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(judgmentsDir, 0o700)

	judgment := validConfirmationJudgment()
	if _, err := s.Put(context.Background(), judgment); err == nil {
		t.Error("put into read-only directory must fail")
	}
	entries, err := os.ReadDir(judgmentsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestConcurrentPutsSameProcess(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	const n = 16
	var wg sync.WaitGroup
	statuses := make([]WriteStatus, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := s.Put(context.Background(), validRevision())
			statuses[i] = res.Status
			errs[i] = err
		}(i)
	}
	wg.Wait()
	created := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("put %d failed: %v", i, errs[i])
		}
		if statuses[i] == WriteCreated {
			created++
		}
	}
	if created != 1 {
		t.Errorf("exactly one concurrent put should create, got %d", created)
	}

	// Conflicting concurrent puts on a fresh identity: exactly one created,
	// rest conflicts.
	conflictErrs := make([]error, n)
	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			r := validRevision()
			// Same identity (same key), different content: exactly one
			// may win, every other put must conflict.
			r.MemoryID = "mem_conflict_same"
			r.Title = fmt.Sprintf("Variant %d", i)
			r = fillRevisionHash(r)
			_, err := s.Put(context.Background(), r)
			conflictErrs[i] = err
		}(i)
	}
	wg2.Wait()
	created2, conflicts := 0, 0
	for i := 0; i < n; i++ {
		switch ErrorCode(conflictErrs[i]) {
		case "":
			created2++
		case CodeIdentityConflict:
			conflicts++
		default:
			t.Fatalf("unexpected error %v", conflictErrs[i])
		}
	}
	if created2 != 1 || conflicts != n-1 {
		t.Errorf("want 1 created + %d conflicts, got %d created + %d conflicts", n-1, created2, conflicts)
	}
}

func TestMultiProcessConcurrentPut(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	_ = s

	run := func(variant string, n int) []string {
		outs := make([]string, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				cmd := exec.Command(os.Args[0], "-test.run=^$")
				cmd.Env = append(os.Environ(),
					"MEM_STORE_HELPER=1",
					"MEM_STORE_ROOT="+root,
					"MEM_STORE_VARIANT="+variant,
				)
				out, err := cmd.CombinedOutput()
				if err != nil {
					outs[i] = "status=launch-error"
					return
				}
				for _, line := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(line, "status=") {
						outs[i] = line
					}
				}
			}(i)
		}
		wg.Wait()
		return outs
	}

	// Identical puts across processes: exactly one created.
	outs := run("", 8)
	created, noop := 0, 0
	for _, o := range outs {
		switch {
		case o == "status=created":
			created++
		case o == "status=noop":
			noop++
		default:
			t.Fatalf("unexpected helper output %q", o)
		}
	}
	if created != 1 || noop != 7 {
		t.Errorf("want 1 created + 7 noop across processes, got %d + %d", created, noop)
	}

	// Conflicting puts across processes: exactly one created, rest conflict.
	outs = run("conflict", 6)
	created, conflicts := 0, 0
	for _, o := range outs {
		switch {
		case o == "status=created":
			created++
		case o == "status=error:memory_store_identity_conflict":
			conflicts++
		default:
			t.Fatalf("unexpected helper output %q", o)
		}
	}
	if created != 1 || conflicts != 5 {
		t.Errorf("want 1 created + 5 conflicts across processes, got %d + %d", created, conflicts)
	}

	// The surviving files must be valid and readable after all processes.
	for _, id := range []string{"mem_01K7A9X2", "mem_helper_conflict"} {
		got, err := s.Get(context.Background(), FactKindMemoryRevision, id+"/2")
		if err != nil {
			t.Fatalf("final file %s unreadable: %v", id, err)
		}
		if _, err := DecodeStrict[MemoryRevision](got); err != nil {
			t.Fatalf("final file %s corrupt: %v", id, err)
		}
	}
}

func TestGetCorruptionDiagnostics(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	path := filepath.Join(s.root, "facts", "judgments", "judgment_01K.json")

	put := func(data string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Invalid JSON.
	put("{not json")
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeInvalidJSON {
		t.Errorf("invalid json: want invalid_json, got %v", err)
	}
	// Unknown field.
	put(`{"schema_version":1,"judgment_id":"judgment_01K","judgment_type":"confirmation","scope":"project","subject":{"subject_type":"memory_revision","memory_ref":{"scope":"project","memory_type":"preference","memory_id":"mem_pref_01K","revision":1,"content_sha256":"` + testHash + `"}},"source":{"source_type":"user","source_id":"local_user"},"confirmation":{"status":"confirmed","declared_scope":"project"},"basis_refs":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z","evil":1}`)
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeUnknownField {
		t.Errorf("unknown field: want unknown_field, got %v", err)
	}
	// Truncated JSON.
	put(`{"schema_version":1,"judgment_id":"judgment_01`)
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeInvalidJSON {
		t.Errorf("truncated json: want invalid_json, got %v", err)
	}
	// Schema invalid (bad status).
	put(`{"schema_version":1,"judgment_id":"judgment_01K","judgment_type":"confirmation","scope":"project","subject":{"subject_type":"memory_revision","memory_ref":{"scope":"project","memory_type":"preference","memory_id":"mem_pref_01K","revision":1,"content_sha256":"` + testHash + `"}},"source":{"source_type":"user","source_id":"local_user"},"confirmation":{"status":"maybe","declared_scope":"project"},"basis_refs":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`)
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeSchemaInvalid {
		t.Errorf("schema invalid: want schema_invalid, got %v", err)
	}
	// Size limit.
	small := openProject(t, tempRoot(t), Options{MaxFactBytes: 64})
	bigPath := filepath.Join(small.root, "facts", "policies", "big.json")
	os.MkdirAll(filepath.Dir(bigPath), 0o700)
	os.WriteFile(bigPath, []byte(`{"padding":"`+strings.Repeat("x", 256)+`"}`), 0o600)
	if _, err := small.Get(context.Background(), FactKindPolicy, "big"); ErrorCode(err) != CodeCorruptFile {
		t.Errorf("oversize: want corrupt_file, got %v", err)
	}
}

func TestGetRejectsUnsafeLayout(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	judgment := validConfirmationJudgment()
	if _, err := s.Put(context.Background(), judgment); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(s.root, "facts", "judgments", "judgment_01K.json")

	// Replace the stored file with a symlink: read must refuse.
	os.Remove(filePath)
	os.Symlink(tempRoot(t), filePath)
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("symlink target: want symlink_rejected, got %v", err)
	}
	os.Remove(filePath)

	// Replace the kind directory with a symlink: read and write must refuse.
	os.RemoveAll(filepath.Join(s.root, "facts", "judgments"))
	os.Symlink(tempRoot(t), filepath.Join(s.root, "facts", "judgments"))
	if _, err := s.Get(context.Background(), FactKindJudgment, "judgment_01K"); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("symlink kind dir: want symlink_rejected, got %v", err)
	}
	if _, err := s.Put(context.Background(), judgment); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("put into symlink kind dir: want symlink_rejected, got %v", err)
	}
}

func TestGetRejectsDirectoryAsFile(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	dirAsFile := filepath.Join(s.root, "facts", "policies", "p1.json")
	if err := os.MkdirAll(dirAsFile, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), FactKindPolicy, "p1"); ErrorCode(err) != CodePathUnsafe {
		t.Errorf("directory as file: want path_unsafe, got %v", err)
	}
}

func TestErrorMessagesRedacted(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	// Scope mismatch.
	globalRev := validRevision()
	globalRev.Scope = ScopeGlobal
	globalRev = fillRevisionHash(globalRev)
	_, err := s.Put(context.Background(), globalRev)
	if err == nil {
		t.Fatal("expected scope error")
	}
	assertRedacted(t, root, err)

	// Path traversal key.
	if _, err := s.Get(context.Background(), FactKindJudgment, "../escape"); err == nil {
		t.Fatal("expected path error")
	} else {
		assertRedacted(t, root, err)
	}

	// NotFound on a legal key.
	_, err = s.Get(context.Background(), FactKindJudgment, "judgment_missing")
	assertRedacted(t, root, err)

	// Symlink rejection.
	os.Symlink(tempRoot(t), filepath.Join(s.root, "facts", "policies"))
	_, err = s.Put(context.Background(), validPolicy())
	assertRedacted(t, root, err)
}

func assertRedacted(t *testing.T, secret string, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("error leaks absolute path: %q", err.Error())
	}
}

func TestGovernanceEventDriftDetected(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	event := validGovernanceEvent()
	if _, err := s.Put(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, "facts", "governance-events", "governance_01K.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Governance events carry no hash field, so only non-canonical byte
	// drift is detectable: compacting the stored document decodes fine but
	// must fail the canonical round-trip comparison.
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, compact.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), FactKindGovernanceEvent, "governance_01K"); ErrorCode(err) != CodeHashMismatch {
		t.Errorf("non-canonical byte drift on governance event: want hash_mismatch, got %v", err)
	}
}

func TestPutSizeLimit(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{MaxFactBytes: 256})
	big := validRevision()
	big.Summary = strings.Repeat("x", 512)
	big = fillRevisionHash(big)
	if _, err := s.Put(context.Background(), big); ErrorCode(err) != CodeSchemaInvalid {
		t.Errorf("oversized fact: want schema_invalid, got %v", err)
	}
	// The file must not exist afterwards.
	if ok, err := s.Exists(context.Background(), FactKindMemoryRevision, "mem_01K7A9X2/2"); err != nil || ok {
		t.Errorf("oversized put must leave no file: ok=%v err=%v", ok, err)
	}
}

func TestPutToCorruptExistingFileFailsClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	target := filepath.Join(s.root, "facts", "policies", "p1", "1.json")
	if err := writeFileForTest(root, "facts/policies/p1/1.json", []byte("{corrupt")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(target)

	policy := policyOf(PolicyTypeFreshness)
	policy.PolicyID = "p1"
	policy.PolicyVersion = 1
	policy = fillPolicyHash(policy)
	if _, err := s.Put(context.Background(), policy); err == nil {
		t.Error("put onto corrupt existing fact must fail closed")
	}
	after, _ := os.ReadFile(target)
	if string(before) != string(after) {
		t.Error("corrupt existing fact must not be overwritten or deleted")
	}
}

func fillPolicyHash(p PolicyFact) PolicyFact {
	h, err := p.ContentHash()
	if err != nil {
		panic(err)
	}
	p.ContentSHA256 = h
	return p
}

func TestNoOverwriteCommitUnderExternalCreate(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	target := filepath.Join(s.root, "facts", "judgments", "judgment_01K.json")

	// An external creator writes a different, schema-valid fact into the
	// target while a Put is waiting for the store lock. After the lock is
	// released the Put must see the existing fact, never overwrite it.
	lockPath := filepath.Join(s.root, "locks", "store.lock")
	heldF, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(heldF.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	other := validConfirmationJudgment()
	other.JudgmentID = "judgment_01K"
	other.Subject = JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_ext"}
	other = fillJudgmentHash(other)
	externalBytes, err := other.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, externalBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	judgment := validConfirmationJudgment()
	judgment.JudgmentID = "judgment_01K"
	judgment = fillJudgmentHash(judgment)
	go func() {
		time.Sleep(200 * time.Millisecond)
		syscall.Flock(int(heldF.Fd()), syscall.LOCK_UN)
		syscall.Close(int(heldF.Fd()))
	}()
	_, err = s.Put(context.Background(), judgment)
	if ErrorCode(err) != CodeIdentityConflict {
		t.Errorf("put onto externally created different fact: want identity_conflict, got %v", err)
	}
	after, _ := os.ReadFile(target)
	if string(after) != string(externalBytes) {
		t.Error("external fact must survive byte-for-byte")
	}
}

func TestAtomicWriteFileNeverOverwritesExistingTarget(t *testing.T) {
	// Directly exercises the link-commit race branch: when the target
	// exists, atomicWriteFile must fail with errTargetExists without
	// touching the target and without leaving a temp file.
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	dir := filepath.Join(s.root, "facts", "policies")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "p1.json")
	existing := []byte(`{"existing":true}`)
	if err := os.WriteFile(target, existing, 0o600); err != nil {
		t.Fatal(err)
	}
	err := s.atomicWriteFile(target, []byte(`{"new":true}`))
	if err != errTargetExists {
		t.Fatalf("want errTargetExists, got %v", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(existing) {
		t.Error("existing target must not be overwritten")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), tempPrefix) {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
	// And the commit path succeeds when the target is absent.
	target2 := filepath.Join(dir, "p2.json")
	if err := s.atomicWriteFile(target2, []byte(`{"new":true}`)); err != nil {
		t.Fatalf("commit to absent target should succeed: %v", err)
	}
	got, err := os.ReadFile(target2)
	if err != nil || string(got) != `{"new":true}` {
		t.Errorf("committed content mismatch: %q %v", got, err)
	}
}

func TestOpenProjectRejectsInsecureDirPermissions(t *testing.T) {
	// Pre-existing Store directories must be 0700: any group/other bits are
	// rejected with a stable permission error, never silently chmod'ed.
	cases := []struct {
		name string
		rel  string // directory relative to the store root
		perm os.FileMode
	}{
		{"root 0755", ".", 0o755},
		{"root 0775", ".", 0o775},
		{"facts 0755", "facts", 0o755},
		{"facts 0775", "facts", 0o775},
		{"locks 0755", "locks", 0o755},
		{"locks 0775", "locks", 0o775},
		{"diagnostics 0755", "diagnostics", 0o755},
		{"judgments 0755", filepath.Join("facts", "judgments"), 0o755},
		{"judgments 0775", filepath.Join("facts", "judgments"), 0o775},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tempRoot(t)
			if _, err := OpenProject(root, Options{}); err != nil {
				t.Fatalf("initial open should succeed: %v", err)
			}
			if err := os.Chmod(filepath.Join(root, tc.rel), tc.perm); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenProject(root, Options{}); ErrorCode(err) != CodeInsecurePermissions {
				t.Errorf("reopen with %s %v: want insecure_permissions, got %v", tc.rel, tc.perm, err)
			}
			fi, err := os.Stat(filepath.Join(root, tc.rel))
			if err != nil || fi.Mode().Perm() != tc.perm {
				t.Error("directory must not be silently chmod'ed back")
			}
		})
	}
}

func TestRestartPersistence(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	if _, err := s.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	// Reopen the store from the same root.
	s2 := openProject(t, root, Options{})
	got, err := s2.Get(context.Background(), FactKindMemoryRevision, "mem_01K7A9X2/2")
	if err != nil {
		t.Fatalf("fact must survive store restart: %v", err)
	}
	decoded, err := DecodeStrict[MemoryRevision](got)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := decoded.ContentHash()
	if h != rev.ContentSHA256 {
		t.Error("restart hash mismatch")
	}
}

func TestWriteFailureLeavesNoHalfFile(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// Read-only kind directory: the atomic temp-file creation must fail
	// mid-flight without leaving a visible half file.
	policiesDir := filepath.Join(s.root, "facts", "policies")
	if err := os.Chmod(policiesDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(policiesDir, 0o700)
	if _, err := s.Put(context.Background(), validPolicy()); err == nil {
		t.Error("put must fail on read-only kind directory")
	}
	// No residual temp files anywhere under facts.
	filepath.Walk(filepath.Join(s.root, "facts"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.Contains(info.Name(), ".tmp") {
			t.Errorf("residual temp file: %s", p)
		}
		return nil
	})
}

// ---- FactStore.List (derived-state enumeration) ----

func TestFactStoreList(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})

	rev := validRevision()
	put(t, s, rev)
	ev := validEvidenceGeneration()
	put(t, s, ev)
	u := validUsage()
	put(t, s, u)
	o := validOutcome()
	put(t, s, o)

	revs, err := s.List(context.Background(), FactKindMemoryRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0] != "mem_01K7A9X2/2" {
		t.Errorf("revision list mismatch: %v", revs)
	}
	evs, err := s.List(context.Background(), FactKindMemoryEvidenceGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0] != "mem_01K7A9X2/2/3" {
		t.Errorf("evidence list mismatch: %v", evs)
	}
	us, err := s.List(context.Background(), FactKindMemoryUsage)
	if err != nil {
		t.Fatal(err)
	}
	if len(us) != 1 || us[0] != "usage_01K" {
		t.Errorf("usage list mismatch: %v", us)
	}
	os, err := s.List(context.Background(), FactKindOutcome)
	if err != nil {
		t.Fatal(err)
	}
	if len(os) != 1 || os[0] != "outcome_01K" {
		t.Errorf("outcome list mismatch: %v", os)
	}
	// Empty kinds list cleanly.
	empty, err := s.List(context.Background(), FactKindJudgment)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("empty kind must list nothing, got %v", empty)
	}
}

func TestFactStoreListRejectsUnsafeEntries(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// A symlink inside the kind directory must be rejected, never followed.
	kindDir := filepath.Join(root, "facts", "judgments")
	if err := os.MkdirAll(kindDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(kindDir, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(context.Background(), FactKindJudgment); ErrorCode(err) != CodeSymlinkRejected {
		t.Fatalf("symlink in kind dir must fail closed, got %v", err)
	}
}

func put(t *testing.T, s *FactStore, f Fact) {
	t.Helper()
	if _, err := s.Put(context.Background(), f); err != nil {
		t.Fatalf("put: %v", err)
	}
}
