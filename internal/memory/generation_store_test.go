package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openGenerationProject(t *testing.T, root string) GenerationStore {
	t.Helper()
	s := openProject(t, root, Options{})
	return NewGenerationStore(s)
}

func openGenerationGlobal(t *testing.T, root string) GenerationStore {
	t.Helper()
	s := mustOpenStore(t, root, StoreScopeGlobal)
	return NewGenerationStore(s)
}

func beginReq(key string, base *string) BeginGenerationRequest {
	return BeginGenerationRequest{
		Scope:                   ScopeProject,
		BaseGeneration:          base,
		CompilerVersion:         "mnemosyne-compiler/1",
		CanonicalizationVersion: 1,
		SchemaVersion:           1,
		IdempotencyKey:          key,
	}
}

// manifestFor builds the exact Manifest the transaction layer will accept:
// the output hash is the deterministic hash of the predicted generation.json
// and the base generation always matches the transaction's base.
func manifestFor(tx *GenerationTx, inputs []ManifestInput) GenerationInputManifest {
	gen := generationDoc{
		SchemaVersion:           SchemaVersion,
		GenerationID:            tx.GenerationID,
		Scope:                   tx.Scope,
		CompilerVersion:         tx.CompilerVersion,
		CanonicalizationVersion: tx.CanonicalizationVersion,
		TransactionID:           tx.TransactionID,
	}
	if tx.BaseGeneration != nil {
		gen.BaseGeneration = *tx.BaseGeneration
	}
	outHash, err := gen.outputHash()
	if err != nil {
		panic(err)
	}
	m := GenerationInputManifest{
		SchemaVersion:           SchemaVersion,
		GenerationID:            tx.GenerationID,
		Scope:                   tx.Scope,
		BaseGeneration:          tx.BaseGeneration,
		CompilerVersion:         tx.CompilerVersion,
		CanonicalizationVersion: tx.CanonicalizationVersion,
		Inputs:                  inputs,
		OutputSHA256:            outHash,
		TransactionID:           tx.TransactionID,
		CreatedAt:               "2026-08-10T00:00:00Z",
	}
	h, err := m.ContentHash()
	if err != nil {
		panic(err)
	}
	m.InputManifestSHA256 = h
	return m
}

// inputFor derives the canonical manifest input for a fact exactly the way
// the transaction layer derives it from the fact itself.
func inputFor(f Fact) ManifestInput {
	ft, fid, err := factIdentity(f)
	if err != nil {
		panic(err)
	}
	h, err := f.ContentHash()
	if err != nil {
		panic(err)
	}
	return ManifestInput{
		FactType:          ft,
		FactID:            fid,
		FactSchemaVersion: factSchemaVersion(f),
		ContentSHA256:     h,
	}
}

// commitOne runs a full single-generation commit with one prepared fact and
// returns the transaction.
func commitOne(t *testing.T, gs GenerationStore, key string, base *string) *GenerationTx {
	t.Helper()
	tx, err := gs.Begin(context.Background(), beginReq(key, base))
	if err != nil {
		t.Fatal(err)
	}
	gov := validGovernanceEvent()
	if err := gs.PrepareFact(context.Background(), tx, gov); err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, []ManifestInput{inputFor(gov)})); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestGenerationFirstCommit(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	tx, err := gs.Begin(context.Background(), beginReq("gen_first", nil))
	if err != nil {
		t.Fatal(err)
	}
	if tx.TransactionID == "" || tx.GenerationID == "" {
		t.Fatal("begin must assign transaction and generation ids")
	}
	gov := validGovernanceEvent()
	if err := gs.PrepareFact(context.Background(), tx, gov); err != nil {
		t.Fatal(err)
	}
	mf := manifestFor(tx, []ManifestInput{inputFor(gov)})
	if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
		t.Fatal(err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	res, err := gs.Commit(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CommitCommitted {
		t.Fatalf("first commit must be committed, got %v", res.Status)
	}
	if res.GenerationID != tx.GenerationID {
		t.Error("commit must return the transaction generation")
	}
	// CURRENT points at the new generation; the generation is published.
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != tx.GenerationID {
		t.Fatalf("CURRENT must point at the committed generation: %v %v", cur, err)
	}
	if _, err := os.Stat(filepath.Join(root, "generations", tx.GenerationID, "generation.json")); err != nil {
		t.Errorf("published generation.json missing: %v", err)
	}
	// The manifest is a permanent fact.
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
		t.Errorf("manifest fact missing: %v", err)
	}
	// The prepared fact is an audit record inside the transaction.
	if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "commit.json")); err != nil {
		t.Errorf("commit record missing: %v", err)
	}
}

func readCurrentForTest(root string) (*currentPointer, error) {
	data, err := os.ReadFile(filepath.Join(root, "CURRENT"))
	if err != nil {
		return nil, err
	}
	var cur currentPointer
	if err := json.Unmarshal(data, &cur); err != nil {
		return nil, err
	}
	return &cur, nil
}

func TestGenerationChainCommit(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	base := (*string)(nil)
	var firstGen string
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("gen_chain_%d", i)
		tx, err := gs.Begin(context.Background(), beginReq(key, base))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		next := tx.GenerationID
		base = &next
		if i == 0 {
			firstGen = tx.GenerationID
		}
	}
	cur, err := readCurrentForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	if cur.GenerationID == firstGen {
		t.Error("CURRENT must advance to the second generation")
	}
}

func TestGenerationManifestDeterminism(t *testing.T) {
	// Input order and duplicates must not change the manifest hash; a
	// conflicting duplicate (same id, different hash) must be rejected.
	gov := validGovernanceEvent()
	in := inputFor(gov)
	inputs := []ManifestInput{in, inputFor(validGovernanceEvent())}
	inputs[1].FactType = "memory_revision"
	inputs[1].FactID = "mem_abc@2"
	inputs[1].ContentSHA256 = testHash
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	tx, err := gs.Begin(context.Background(), beginReq("gen_manifest_det", nil))
	if err != nil {
		t.Fatal(err)
	}

	shuffled := []ManifestInput{inputs[1], inputs[0], inputs[0]} // reversed + duplicate
	m1 := manifestFor(tx, inputs)
	m2 := manifestFor(tx, shuffled)
	h1, _ := m1.ContentHash()
	h2, _ := m2.ContentHash()
	if h1 != h2 {
		t.Error("manifest hash must be order/duplicate insensitive")
	}
	// PrepareManifest only persists the manifest; input/fact matching is
	// verified at Commit time, so a syntactically valid manifest is stored.
	if err := gs.PrepareManifest(context.Background(), tx, m2); err != nil {
		t.Fatalf("deduplicated manifest must be accepted: %v", err)
	}
	if err := gs.Release(context.Background(), tx); err != nil {
		t.Fatal(err)
	}

	// Conflicting duplicate: same id, different content hash is rejected by
	// the manifest model itself (the hash cannot even be computed).
	conflict := []ManifestInput{
		{FactType: in.FactType, FactID: in.FactID, FactSchemaVersion: 1, ContentSHA256: testHash},
		{FactType: in.FactType, FactID: in.FactID, FactSchemaVersion: 1, ContentSHA256: "sha256_" + strings.Repeat("e", 64)},
	}
	m3 := manifestFor(tx, nil)
	m3.Inputs = conflict
	if _, err := m3.ContentHash(); err == nil {
		t.Error("conflicting duplicate manifest inputs must be rejected by the model")
	}
}

func TestGenerationIdempotentReplay(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	key := "gen_replay"
	tx1, err := gs.Begin(context.Background(), beginReq(key, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx1, manifestFor(tx1, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx1); err != nil {
		t.Fatal(err)
	}

	// Same key, same request: replay returns the existing committed result.
	tx2, err := gs.Begin(context.Background(), beginReq(key, nil))
	if err != nil {
		t.Fatal(err)
	}
	if tx2.TransactionID != tx1.TransactionID {
		t.Errorf("replay must reuse the same transaction, got %s != %s", tx2.TransactionID, tx1.TransactionID)
	}
	res, err := gs.Commit(context.Background(), tx2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != CommitAlreadyCommitted {
		t.Errorf("replayed commit must be already_committed, got %v", res.Status)
	}
	// Only one generation was ever published.
	entries, err := os.ReadDir(filepath.Join(root, "generations"))
	if err != nil {
		t.Fatal(err)
	}
	published := 0
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".staging") {
			published++
		}
	}
	if published != 1 {
		t.Errorf("replay must not create a second generation, found %d", published)
	}
}

func TestGenerationIdempotentConflict(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	key := "gen_conflict"
	tx1, err := gs.Begin(context.Background(), beginReq(key, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Release(context.Background(), tx1); err != nil {
		t.Fatal(err)
	}
	// Same key with a different request (different base) must fail closed
	// with zero side effects.
	base := "gen_other"
	_, err = gs.Begin(context.Background(), beginReq(key, &base))
	if ErrorCode(err) != CodeGenerationIdempotency {
		t.Fatalf("same key different request: want idempotency_conflict, got %v", err)
	}
	// No transaction directory was created for the rejected attempt.
	txDir := filepath.Join(root, "transactions")
	if entries, err := os.ReadDir(txDir); err != nil || len(entries) != 1 {
		t.Errorf("failed claim must not create transactions, found %d (%v)", len(entries), err)
	}
}

func TestGenerationIdempotentRace(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	key := "gen_race"
	var mu sync.Mutex
	txIDs := map[string]bool{}
	fails := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := gs.Begin(context.Background(), beginReq(key, nil))
			if err != nil {
				// The claim winner materializes its transaction record
				// before taking the lock, so every concurrent claimant must
				// succeed (resume or win); a dangling-claim misdiagnosis
				// would fail here.
				mu.Lock()
				fails++
				mu.Unlock()
				return
			}
			mu.Lock()
			txIDs[tx.TransactionID] = true
			mu.Unlock()
			// The tx holds the scope lock until released; drop it so the
			// remaining claimants can serialize and observe the same claim.
			_ = gs.Release(context.Background(), tx)
		}()
	}
	wg.Wait()
	if fails != 0 {
		t.Errorf("concurrent claims must all succeed, %d failed", fails)
	}
	if len(txIDs) != 1 {
		t.Errorf("concurrent claims must agree on one transaction, got %d", len(txIDs))
	}
}

func TestGenerationCurrentCasConflict(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	// Tx A commits the first generation.
	txA, err := gs.Begin(context.Background(), beginReq("gen_cas_a", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), txA, manifestFor(txA, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), txA); err != nil {
		t.Fatal(err)
	}
	// Tx B expects the same base (nil): its CAS must fail and leave CURRENT
	// unchanged.
	txB, err := gs.Begin(context.Background(), beginReq("gen_cas_b", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), txB, manifestFor(txB, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), txB); ErrorCode(err) != CodeGenerationCurrentCAS {
		t.Fatalf("stale base must fail with current_cas_conflict, got %v", err)
	}
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != txA.GenerationID {
		t.Errorf("CURRENT must stay on tx A's generation, got %v %v", cur, err)
	}
	// tx B published its Manifest and Generation before the CAS check, so
	// they remain as isolated orphans for diagnosis (never auto-adopted).
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", txB.GenerationID+".json")); err != nil {
		t.Error("failed CAS must keep the orphan manifest for diagnosis")
	}
	if _, err := os.Stat(filepath.Join(root, "generations", txB.GenerationID, "generation.json")); err != nil {
		t.Error("failed CAS must keep the orphan generation for diagnosis")
	}
}

func TestGenerationStagingMismatchKeepsOld(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	// Wrong predicted output hash: staging validation must fail and nothing
	// may change.
	tx, err := gs.Begin(context.Background(), beginReq("gen_bad_out", nil))
	if err != nil {
		t.Fatal(err)
	}
	bad := manifestFor(tx, nil)
	bad.OutputSHA256 = "sha256_" + strings.Repeat("a", 64)
	bad = fillManifestHash(bad)
	if err := gs.PrepareManifest(context.Background(), tx, bad); err != nil {
		t.Fatalf("prepare manifest should accept a syntactically valid manifest: %v", err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("output mismatch: want staging_invalid, got %v", err)
	}
	if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("commit with bad output hash must fail: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
		t.Error("failed staging must not create CURRENT")
	}
	// The manifest was only staged inside the transaction; no permanent
	// manifest may exist.
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); !os.IsNotExist(err) {
		t.Error("failed staging must not leave a permanent manifest")
	}
}

func TestManifestPublishFailureKeepsCurrent(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	tx, err := gs.Begin(context.Background(), beginReq("gen_manifest_fail", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	// Make the permanent manifest directory unwritable so the Manifest
	// publish (which runs before the Generation publish and the CAS, per the
	// frozen order) fails with an ordinary error.
	mfDir := filepath.Join(root, "facts", "generation-input-manifests")
	if err := os.Chmod(mfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeInsecurePermissions {
		t.Fatalf("manifest publish failure must be an ordinary error, got %v", err)
	}
	// CURRENT untouched, no Generation published, no pending-recovery signal.
	if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
		t.Error("manifest publish failure must not switch CURRENT")
	}
	if _, err := os.Stat(filepath.Join(root, "generations", tx.GenerationID, "generation.json")); !os.IsNotExist(err) {
		t.Error("manifest publish failure must not publish a generation")
	}
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); !os.IsNotExist(err) {
		t.Error("failed manifest publish must not leave a permanent manifest")
	}

	// Retry through a fresh Begin on the same key completes the commit.
	if err := os.Chmod(mfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tx2, err := gs.Begin(context.Background(), beginReq("gen_manifest_fail", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx2); err != nil {
		t.Fatalf("retried commit must complete: %v", err)
	}
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != tx.GenerationID {
		t.Fatalf("CURRENT must point at the committed generation: %v %v", cur, err)
	}
}

func TestGenerationPreparedFactIsolated(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	gs := NewGenerationStore(s)

	tx, err := gs.Begin(context.Background(), beginReq("gen_isolated", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareFact(context.Background(), tx, validGovernanceEvent()); err != nil {
		t.Fatal(err)
	}
	// The prepared fact must not be visible through the FactStore.
	key := factKeyOf(validGovernanceEvent())
	if _, err := s.Get(context.Background(), FactKindGovernanceEvent, key); ErrorCode(err) != CodeNotFound {
		t.Errorf("prepared fact must be invisible to normal reads, got %v", err)
	}
}

func factKeyOf(f Fact) string {
	_, key, err := factKey(f)
	if err != nil {
		panic(err)
	}
	return key
}

func TestGenerationAbort(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	tx, err := gs.Begin(context.Background(), beginReq("gen_abort", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	if err := gs.Abort(context.Background(), tx, "operator decision"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "abort.json")); err != nil {
		t.Errorf("abort record missing: %v", err)
	}
	if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationTxConflict {
		t.Errorf("committing an aborted transaction must fail, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
		t.Error("abort must not create CURRENT")
	}
	// Abort must leave no permanent manifest behind.
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); !os.IsNotExist(err) {
		t.Error("abort must not leave a permanent manifest")
	}
}

func TestGenerationCompilerUnavailable(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	req := beginReq("gen_compiler", nil)
	req.CompilerVersion = "nonexistent-compiler/9"
	tx, err := gs.Begin(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); ErrorCode(err) != CodeGenerationCompilerUnavailable {
		t.Fatalf("unavailable compiler must block manifest preparation, got %v", err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); ErrorCode(err) != CodeGenerationCompilerUnavailable {
		t.Fatalf("unavailable compiler must block staging, got %v", err)
	}
}

func TestGenerationScopeIsolation(t *testing.T) {
	proj := openGenerationProject(t, tempRoot(t))
	glob := openGenerationGlobal(t, tempRoot(t))

	// Global store must reject a project-scope request.
	if _, err := glob.Begin(context.Background(), beginReq("gen_scope", nil)); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("global store must reject project scope, got %v", err)
	}
	// Project transaction succeeds and its CURRENT is isolated.
	tx, err := proj.Begin(context.Background(), beginReq("gen_scope", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := proj.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	// The global store has no CURRENT.
	if _, err := os.Stat(filepath.Join(globalRoot(glob), "CURRENT")); !os.IsNotExist(err) {
		t.Error("global store must not see the project CURRENT")
	}
}

func globalRoot(gs GenerationStore) string {
	return gs.(*generationStore).store.root
}

func TestGenerationContextCancellation(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gs.Begin(ctx, beginReq("gen_cancel", nil)); err == nil {
		t.Error("cancelled context must fail begin")
	}
	tx, err := gs.Begin(context.Background(), beginReq("gen_cancel2", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(ctx, tx); err == nil {
		t.Error("cancelled context must fail commit")
	}
	if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
		t.Error("cancelled commit must not switch CURRENT")
	}
}

func TestGenerationPublishedImmutable(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	tx, err := gs.Begin(context.Background(), beginReq("gen_immutable", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	// Replaying the same committed transaction via Begin is a no-op, not a
	// rewrite.
	tx2, err := gs.Begin(context.Background(), beginReq("gen_immutable", nil))
	if err != nil {
		t.Fatal(err)
	}
	res, err := gs.Commit(context.Background(), tx2)
	if err != nil || res.Status != CommitAlreadyCommitted {
		t.Errorf("re-commit must be already_committed: %v %v", res, err)
	}
	// A tampered published generation is detected by its own hash.
	genPath := filepath.Join(root, "generations", tx.GenerationID, "generation.json")
	orig, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(genPath, []byte(strings.Replace(string(orig), tx.GenerationID, "gen_tampered", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readJSONFile[generationDoc](genPath)
	if err != nil {
		t.Fatal(err)
	}
	computed, err := got.outputHash()
	if err != nil {
		t.Fatal(err)
	}
	if computed == got.OutputGenerationSHA256 {
		t.Error("tampered generation must fail its own output hash check")
	}
}

func TestGenerationKeepsOldOnFailure(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	txA, err := gs.Begin(context.Background(), beginReq("gen_keep_a", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), txA, manifestFor(txA, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), txA); err != nil {
		t.Fatal(err)
	}
	// A failing transaction (CAS conflict) must leave the old generation,
	// its manifest and CURRENT untouched, and only add isolated leftovers.
	txB, err := gs.Begin(context.Background(), beginReq("gen_keep_b", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), txB, manifestFor(txB, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), txB); ErrorCode(err) != CodeGenerationCurrentCAS {
		t.Fatalf("want cas conflict, got %v", err)
	}
	// Old manifest still present; failed tx's manifest and generation are
	// isolated orphans kept for diagnosis (never auto-adopted).
	for _, gen := range []string{txA.GenerationID, txB.GenerationID} {
		if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", gen+".json")); err != nil {
			t.Errorf("manifest for %s must be preserved: %v", gen, err)
		}
	}
	// Old generation still published; failed tx's generation is an orphan
	// that stays for diagnosis.
	if _, err := os.Stat(filepath.Join(root, "generations", txA.GenerationID, "generation.json")); err != nil {
		t.Errorf("old generation must be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "generations", txB.GenerationID, "generation.json")); err != nil {
		t.Error("failed tx's orphan generation must be preserved for diagnosis")
	}
}

// ---- CTO review regressions ----

func TestManifestInvisibleUntilCommit(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	gs := NewGenerationStore(s)

	tx, err := gs.Begin(context.Background(), beginReq("gen_invisible", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	// Before commit the manifest lives only in the transaction area and is
	// invisible to normal FactStore reads.
	if _, err := s.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID); ErrorCode(err) != CodeNotFound {
		t.Fatalf("uncommitted manifest must be invisible, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); !os.IsNotExist(err) {
		t.Fatal("uncommitted manifest must not exist under facts/")
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID); err != nil {
		t.Fatalf("committed manifest must be readable: %v", err)
	}
}

func TestManifestInputValidation(t *testing.T) {
	committedGov := validGovernanceEvent()
	newGov := validGovernanceEvent()
	newGov.EventID = "gov_input_extra"

	t.Run("input matches prepared fact", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		tx, err := gs.Begin(context.Background(), beginReq("in_ok", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, newGov); err != nil {
			t.Fatal(err)
		}
		mf := manifestFor(tx, []ManifestInput{inputFor(newGov)})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatalf("matched prepared input must commit: %v", err)
		}
	})

	t.Run("input matches committed fact", func(t *testing.T) {
		root := tempRoot(t)
		s := openProject(t, root, Options{})
		gs := NewGenerationStore(s)
		if _, err := s.Put(context.Background(), committedGov); err != nil {
			t.Fatal(err)
		}
		tx, err := gs.Begin(context.Background(), beginReq("in_committed", nil))
		if err != nil {
			t.Fatal(err)
		}
		mf := manifestFor(tx, []ManifestInput{inputFor(committedGov)})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatalf("matched committed input must commit: %v", err)
		}
	})

	t.Run("missing input fact fails closed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		tx, err := gs.Begin(context.Background(), beginReq("in_missing", nil))
		if err != nil {
			t.Fatal(err)
		}
		ghost := validGovernanceEvent()
		ghost.EventID = "gov_ghost"
		mf := manifestFor(tx, []ManifestInput{inputFor(ghost)})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("missing input must fail with manifest_mismatch, got %v", err)
		}
	})

	t.Run("content hash mismatch fails closed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		tx, err := gs.Begin(context.Background(), beginReq("in_hash", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, newGov); err != nil {
			t.Fatal(err)
		}
		bad := inputFor(newGov)
		bad.ContentSHA256 = "sha256_" + strings.Repeat("f", 64)
		mf := manifestFor(tx, []ManifestInput{bad})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("hash mismatch must fail closed, got %v", err)
		}
	})

	t.Run("schema version mismatch fails closed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		tx, err := gs.Begin(context.Background(), beginReq("in_schema", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, newGov); err != nil {
			t.Fatal(err)
		}
		bad := inputFor(newGov)
		bad.FactSchemaVersion = 99
		mf := manifestFor(tx, []ManifestInput{bad})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("schema mismatch must fail closed, got %v", err)
		}
	})

	t.Run("prepared fact scope mismatch fails closed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("in_scope", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, newGov); err != nil {
			t.Fatal(err)
		}
		// Corrupt the prepared record so the stored fact scope disagrees
		// with the transaction scope; Commit must fail closed.
		dir, err := impl.txDir(context.Background(), tx.TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
		if err != nil {
			t.Fatal(err)
		}
		rec.PreparedFacts[0].Scope = ScopeGlobal
		if err := writeJSONFile(impl, filepath.Join(dir, "prepared.json"), rec); err != nil {
			t.Fatal(err)
		}
		mf := manifestFor(tx, []ManifestInput{inputFor(newGov)})
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("scope mismatch must fail closed, got %v", err)
		}
	})

	t.Run("unreferenced prepared fact fails closed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		tx, err := gs.Begin(context.Background(), beginReq("in_extra", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, newGov); err != nil {
			t.Fatal(err)
		}
		// The manifest references nothing; the prepared fact is extra.
		mf := manifestFor(tx, nil)
		if err := gs.PrepareManifest(context.Background(), tx, mf); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("unreferenced prepared fact must fail closed, got %v", err)
		}
	})

	t.Run("conflicting prepared duplicates fail at prepare time", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		// The same identity may only be prepared with the exact same content;
		// a different second fact for the same identity would silently bypass
		// Manifest verification, so it is rejected when staged.
		first := validGovernanceEvent()
		first.EventID = "gov_dup"
		second := validGovernanceEvent()
		second.EventID = "gov_dup"
		second.Reason = "a different governance reason"
		tx, err := gs.Begin(context.Background(), beginReq("in_dup", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, first); err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareFact(context.Background(), tx, second); ErrorCode(err) != CodeGenerationTxConflict {
			t.Fatalf("conflicting prepared duplicate must fail closed, got %v", err)
		}
		// Preparing the exact same fact again is idempotent.
		if err := gs.PrepareFact(context.Background(), tx, first); err != nil {
			t.Fatalf("identical re-prepare must be idempotent: %v", err)
		}
	})
}

func TestBaseGenerationNilNonNull(t *testing.T) {
	t.Run("manifest base must strictly match transaction base", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		// Transaction expects nil base; manifest claims a base: reject.
		tx, err := gs.Begin(context.Background(), beginReq("base_strict1", nil))
		if err != nil {
			t.Fatal(err)
		}
		other := "gen_somewhere"
		bad := manifestFor(tx, nil)
		bad.BaseGeneration = &other
		bad = fillManifestHash(bad)
		if err := gs.PrepareManifest(context.Background(), tx, bad); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("manifest with non-nil base for nil-base tx must be rejected, got %v", err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Transaction expects base; manifest claims nil: reject.
		base := "gen_base"
		tx2, err := gs.Begin(context.Background(), beginReq("base_strict2", &base))
		if err != nil {
			t.Fatal(err)
		}
		bad2 := manifestFor(tx2, nil)
		bad2.BaseGeneration = nil
		bad2 = fillManifestHash(bad2)
		if err := gs.PrepareManifest(context.Background(), tx2, bad2); ErrorCode(err) != CodeGenerationManifestMismatch {
			t.Fatalf("manifest with nil base for based tx must be rejected, got %v", err)
		}
		if err := gs.Release(context.Background(), tx2); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("nil base requires empty CURRENT", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		commitOne(t, gs, "base_nil_a", nil)
		// Now CURRENT exists; a nil-base transaction must fail the CAS.
		tx, err := gs.Begin(context.Background(), beginReq("base_nil_b", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); ErrorCode(err) != CodeGenerationCurrentCAS {
			t.Fatalf("nil base against existing CURRENT must fail CAS, got %v", err)
		}
	})

	t.Run("non-nil base requires matching CURRENT", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		txA := commitOne(t, gs, "base_nn_a", nil)
		// Matching base commits.
		base := txA.GenerationID
		txB := commitOne(t, gs, "base_nn_b", &base)
		if txB.BaseGeneration == nil || *txB.BaseGeneration != txA.GenerationID {
			t.Errorf("second generation must chain from first, got %v", txB.BaseGeneration)
		}
		// Mismatched base fails.
		wrong := "gen_wrong"
		txC, err := gs.Begin(context.Background(), beginReq("base_nn_c", &wrong))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), txC, manifestFor(txC, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), txC); ErrorCode(err) != CodeGenerationCurrentCAS {
			t.Fatalf("wrong base must fail CAS, got %v", err)
		}
	})
}

func TestAbortClaimUpdateFailure(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	impl := gs.(*generationStore)

	tx, err := gs.Begin(context.Background(), beginReq("gen_abort_claim", nil))
	if err != nil {
		t.Fatal(err)
	}
	// Remove the claim file: Abort records the abort but cannot update the
	// claim, so it must return the stable abort_failed error (never silently
	// ignore the failure).
	claimPath, err := impl.claimPath(context.Background(), "gen_abort_claim")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(claimPath); err != nil {
		t.Fatal(err)
	}
	if err := gs.Abort(context.Background(), tx, "operator decision"); ErrorCode(err) != CodeGenerationAbortFailed {
		t.Fatalf("abort with unwritable claim must return abort_failed, got %v", err)
	}
	// The abort record itself is durable.
	if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "abort.json")); err != nil {
		t.Fatalf("abort record must still be written: %v", err)
	}
	// Recovery must deterministically diagnose the aborted transaction even
	// though its claim is gone (orphan transaction report).
	actions, err := impl.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if a.TransactionID == tx.TransactionID && a.Kind == RecoveryReport {
			found = true
		}
	}
	if !found {
		t.Errorf("recovery must diagnose the orphan aborted transaction, got %v", actions)
	}
}

func TestCommitAuditFailurePendingRecovery(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)

	tx, err := gs.Begin(context.Background(), beginReq("gen_pending_recovery", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
		t.Fatal(err)
	}
	// Make the transaction directory read-only so the commit audit write
	// fails after CURRENT has switched.
	txDir := filepath.Join(root, "transactions", tx.TransactionID)
	if err := os.Chmod(txDir, 0o500); err != nil {
		t.Fatal(err)
	}
	res, err := gs.Commit(context.Background(), tx)
	if err == nil {
		t.Fatal("commit must fail when the audit cannot be written")
	}
	if res.Status != CommitPendingRecovery {
		t.Fatalf("CURRENT-switched audit failure must report committed_recovery_pending, got %v", res.Status)
	}
	if ErrorCode(err) != CodeGenerationRecoveryPending {
		t.Fatalf("want recovery_pending code, got %v", err)
	}
	// CURRENT really is effective; the caller must not believe otherwise.
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != tx.GenerationID {
		t.Fatalf("CURRENT must have switched despite the audit failure: %v %v", cur, err)
	}
	// The permanent manifest was published (before the Generation publish
	// and CURRENT switch, per the frozen order) and survives the audit
	// failure.
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
		t.Fatalf("manifest must be permanent before audit: %v", err)
	}
	// An effective transaction must not be abortable (that would leave a
	// commit/abort contradiction that recovery would resolve).
	if err := gs.Abort(context.Background(), tx, "too late"); ErrorCode(err) != CodeGenerationRecoveryPending {
		t.Fatalf("abort after effective CURRENT must be rejected, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "abort.json")); !os.IsNotExist(err) {
		t.Fatal("rejected abort must not write an abort record")
	}

	// Retry: once the directory is writable again, Commit completes the
	// remaining steps and returns committed.
	if err := os.Chmod(txDir, 0o700); err != nil {
		t.Fatal(err)
	}
	res2, err := gs.Commit(context.Background(), tx)
	if err != nil {
		t.Fatalf("retried commit must complete: %v", err)
	}
	if res2.Status != CommitCommitted {
		t.Fatalf("retried commit must report committed, got %v", res2.Status)
	}
	if _, err := os.Stat(filepath.Join(txDir, "commit.json")); err != nil {
		t.Fatalf("commit audit must exist after retry: %v", err)
	}
}

// ---- MEM-01E OKF compiler integration ----

func TestOKFGenerationCommitFlow(t *testing.T) {
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
	tx, err := gs.Begin(context.Background(), beginReq("okf_commit", nil))
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
	// The compiled pages are part of the published generation.
	for rel, want := range res.Outputs {
		got, err := os.ReadFile(filepath.Join(root, "generations", tx.GenerationID, rel))
		if err != nil {
			t.Fatalf("published output %q missing: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Errorf("published output %q differs from compiled bytes", rel)
		}
	}
	// generation.json pins the compiled output hash.
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", tx.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CompiledOutputSHA256 != res.CompiledSHA256 {
		t.Errorf("generation must pin the compiled output hash: %s != %s", doc.CompiledOutputSHA256, res.CompiledSHA256)
	}
	// The manifest is permanent and lists the revision + evidence inputs.
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
		t.Errorf("manifest fact missing: %v", err)
	}
	// CURRENT points at the compiled generation.
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != tx.GenerationID {
		t.Fatalf("CURRENT must point at the compiled generation: %v %v", cur, err)
	}
}

// TestOKFVerifyPublishedIntegrity: CTO review — verifyPublished must
// recompute the compiled output hash over the published wiki/ and state/
// directories and require it to match generation.json. Tampering with a page,
// deleting a page or replacing compiled_output_sha256 all fail closed on a
// retried commit.
func TestOKFVerifyPublishedIntegrity(t *testing.T) {
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

	// publishAndRelease simulates a crash after the Generation publish but
	// before the CURRENT switch / commit audit: staging is renamed to the
	// final directory by hand and the lock is released. The retried Commit
	// on the same key then hits verifyPublished.
	publishAndRelease := func(key string) string {
		tx, err := gs.Begin(context.Background(), beginReq(key, nil))
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
		if err := os.Rename(
			filepath.Join(root, "generations", tx.GenerationID+".staging"),
			filepath.Join(root, "generations", tx.GenerationID),
		); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		return tx.GenerationID
	}
	replay := func(key string) error {
		tx2, err := gs.Begin(context.Background(), beginReq(key, nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx2, manifestFor(tx2, res.Inputs)); err != nil {
			t.Fatal(err)
		}
		if err := gs.WriteCompiledOutput(context.Background(), tx2, res.Outputs); err != nil {
			t.Fatal(err)
		}
		if err := gs.ValidateStaging(context.Background(), tx2); err != nil {
			t.Fatal(err)
		}
		_, err = gs.Commit(context.Background(), tx2)
		return err
	}

	// 1. Tampered page content must fail closed.
	genB := publishAndRelease("okf_integrity_tamper")
	page := filepath.Join(root, "generations", genB, "wiki/strategies/verify-before-upgrade-retry.md")
	if _, err := os.Stat(page); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(orig, []byte("\n# tampered\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replay("okf_integrity_tamper"); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("tampered page must fail verifyPublished, got %v", err)
	}

	// 2. Deleted page must fail closed.
	genC := publishAndRelease("okf_integrity_delete")
	page = filepath.Join(root, "generations", genC, "wiki/strategies/verify-before-upgrade-retry.md")
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	if err := replay("okf_integrity_delete"); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("deleted page must fail verifyPublished, got %v", err)
	}

	// 3. Replaced compiled_output_sha256 must fail closed.
	genD := publishAndRelease("okf_integrity_replace")
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", genD, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc.CompiledOutputSHA256 = "sha256_" + strings.Repeat("0", 64)
	replacement, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generations", genD, "generation.json"), replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replay("okf_integrity_replace"); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("replaced compiled_output_sha256 must fail verifyPublished, got %v", err)
	}

	// 4. A clean replay passes and completes the commit.
	genA := publishAndRelease("okf_integrity_clean")
	if err := replay("okf_integrity_clean"); err != nil {
		t.Fatalf("clean replay must pass verifyPublished: %v", err)
	}
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID != genA {
		t.Fatalf("CURRENT must point at the completed generation: %v %v", cur, err)
	}
}

func TestOKFDeleteAndRebuild(t *testing.T) {
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
	tx, err := gs.Begin(context.Background(), beginReq("okf_rebuild", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, res.Inputs)); err != nil {
		t.Fatal(err)
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, res.Outputs); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	// Delete every derived view; a fresh compile from the same canonical
	// facts must reproduce identical bytes and the same pinned hash.
	if err := os.RemoveAll(filepath.Join(root, "generations", tx.GenerationID, "wiki")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "generations", tx.GenerationID, "state")); err != nil {
		t.Fatal(err)
	}
	res2, err := CompileOKF(context.Background(), s, okfRequest(rev, ev))
	if err != nil {
		t.Fatal(err)
	}
	if res2.CompiledSHA256 != res.CompiledSHA256 {
		t.Error("rebuilt output hash must match the pinned generation hash")
	}
	for rel, want := range res.Outputs {
		if string(res2.Outputs[rel]) != string(want) {
			t.Errorf("rebuilt output %q differs", rel)
		}
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", tx.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CompiledOutputSHA256 != res2.CompiledSHA256 {
		t.Errorf("rebuilt hash must match generation.json: %s != %s", res2.CompiledSHA256, doc.CompiledOutputSHA256)
	}
}

func TestOKFWriteCompiledOutputRejectsUnsafePaths(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	gs := NewGenerationStore(s)
	tx, err := gs.Begin(context.Background(), beginReq("okf_unsafe", nil))
	if err != nil {
		t.Fatal(err)
	}
	bad := []string{
		"../wiki/escape.md",
		"wiki/../state/x.md",
		"/etc/passwd",
		"wiki/../../outside",
		"other/not-a-view.md",
		"",
		"wiki/",
	}
	for _, rel := range bad {
		if err := gs.WriteCompiledOutput(context.Background(), tx, map[string][]byte{rel: []byte("x")}); err == nil {
			t.Errorf("unsafe relpath %q must be rejected", rel)
		}
	}
	if err := gs.Release(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
}

func TestOKFMultiProcessConcurrent(t *testing.T) {
	root := tempRoot(t)
	openProject(t, root, Options{})

	run := func(key string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(),
			"MEM_OKF_HELPER=1",
			"MEM_GEN_ROOT="+root,
			"MEM_GEN_KEY="+key,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "status=launch-error"
		}
		return strings.TrimSpace(string(out))
	}
	var wg sync.WaitGroup
	outs := make([]string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i] = run(fmt.Sprintf("k%d", i))
		}(i)
	}
	wg.Wait()
	committed, conflicted := 0, 0
	for _, o := range outs {
		switch {
		case strings.Contains(o, "status=committed"):
			committed++
		case strings.Contains(o, "current_cas_conflict"):
			conflicted++
		default:
			t.Errorf("unexpected child status: %q", o)
		}
	}
	if committed != 1 || conflicted != 1 {
		t.Errorf("want 1 committed + 1 cas conflict, got %d + %d", committed, conflicted)
	}
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID == "" {
		t.Fatalf("CURRENT must point at one generation: %v %v", cur, err)
	}
	// Both processes compiled the same input: the committed generation's
	// compiled pages must be identical to a fresh local compile.
	s2, err := OpenProject(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	ev := validEvidenceGeneration()
	if _, err := s2.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Put(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	res, err := CompileOKF(context.Background(), s2, okfRequest(rev, ev))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", cur.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.CompiledOutputSHA256 != res.CompiledSHA256 {
		t.Errorf("multi-process generation hash mismatch: %s != %s", doc.CompiledOutputSHA256, res.CompiledSHA256)
	}
}

func TestGenerationPathSafety(t *testing.T) {
	t.Run("CURRENT symlink rejected", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		cur, err := impl.currentPath()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tempRoot(t), cur); err != nil {
			t.Fatal(err)
		}
		if _, err := impl.readCurrent(context.Background()); ErrorCode(err) != CodeSymlinkRejected {
			t.Errorf("CURRENT symlink: want symlink_rejected, got %v", err)
		}
	})

	t.Run("generations directory insecure permissions", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		if err := os.MkdirAll(filepath.Join(root, "generations"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(root, "generations"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := impl.stagingDir(context.Background(), "gen_x"); ErrorCode(err) != CodeInsecurePermissions {
			t.Errorf("0755 generations dir: want insecure_permissions, got %v", err)
		}
	})

	t.Run("transactions symlink rejected", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		// Create a transaction, then replace its directory with a symlink.
		tx, err := gs.Begin(context.Background(), beginReq("gen_path_tx", nil))
		if err != nil {
			t.Fatal(err)
		}
		txDir := filepath.Join(root, "transactions", tx.TransactionID)
		if err := os.RemoveAll(txDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tempRoot(t), txDir); err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); ErrorCode(err) != CodeSymlinkRejected {
			t.Errorf("symlinked tx dir: want symlink_rejected, got %v", err)
		}
	})
}

func TestGenerationErrorsRedacted(t *testing.T) {
	root := tempRoot(t)
	gs := openGenerationProject(t, root)
	// A path-like idempotency key must fail without echoing the raw value.
	req := beginReq("../etc/passwd", nil)
	_, err := gs.Begin(context.Background(), req)
	if err == nil {
		t.Fatal("path-like idempotency key must be rejected")
	}
	for _, secret := range []string{"/etc/passwd", "passwd"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error must not leak the attempted key: %q", err.Error())
		}
	}
	// Abort reason with control characters is rejected.
	tx, err := gs.Begin(context.Background(), beginReq("gen_redact_abort", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Abort(context.Background(), tx, "bad\x00reason"); err == nil {
		t.Error("abort reason with control characters must be rejected")
	}
}

func TestGenerationMultiProcessConcurrent(t *testing.T) {
	root := tempRoot(t)
	openProject(t, root, Options{})

	run := func(key string) string {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(),
			"MEM_GEN_HELPER=1",
			"MEM_GEN_ROOT="+root,
			"MEM_GEN_KEY="+key,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "status=launch-error"
		}
		return strings.TrimSpace(string(out))
	}
	var wg sync.WaitGroup
	outs := make([]string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i] = run(fmt.Sprintf("k%d", i))
		}(i)
	}
	wg.Wait()
	// Both processes race for the same base (nil): exactly one commits,
	// the other fails the CURRENT CAS. Never two CURRENTs, never two
	// successes.
	committed, conflicted := 0, 0
	for _, o := range outs {
		switch {
		case strings.Contains(o, "status=committed"):
			committed++
		case strings.Contains(o, "current_cas_conflict"):
			conflicted++
		default:
			t.Errorf("unexpected child status: %q", o)
		}
	}
	if committed != 1 || conflicted != 1 {
		t.Errorf("want 1 committed + 1 cas conflict, got %d + %d", committed, conflicted)
	}
	cur, err := readCurrentForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	if cur.GenerationID == "" {
		t.Error("CURRENT must point at exactly one generation")
	}
	// Cross-process invariants: the committed transaction's Manifest is
	// permanent, and the failed CAS transaction's Manifest and Generation
	// are preserved as isolated orphans (published before the CAS check).
	manifests, err := os.ReadDir(filepath.Join(root, "facts", "generation-input-manifests"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) < 1 {
		t.Fatalf("at least the committed manifest must exist, found %d", len(manifests))
	}
	if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", cur.GenerationID+".json")); err != nil {
		t.Errorf("CURRENT generation's manifest must be permanent: %v", err)
	}
	if len(manifests) == 2 {
		// The conflicted process also reached the publish step: its orphan
		// manifest must remain, and its orphan generation must exist too.
		gens, gerr := os.ReadDir(filepath.Join(root, "generations"))
		if gerr != nil {
			t.Fatal(gerr)
		}
		published := 0
		for _, e := range gens {
			if e.IsDir() && !strings.HasSuffix(e.Name(), ".staging") {
				published++
			}
		}
		if published != 2 {
			t.Errorf("orphan generation must be preserved alongside the committed one, found %d", published)
		}
	}
}

// TestOKFCompiledOutputRejectsSymlinks: CTO security fix — compiledOutputHash
// (and therefore verifyPublished / Recover / completeCommitAudit) must reject
// symbolic links under wiki/ and state/ instead of following them. A file
// replaced by a symlink to an external target, or a directory replaced by a
// symlink, must fail the integrity check; the external target must never be
// read into a hash, written, or leaked into diagnostics.
func TestOKFCompiledOutputRejectsSymlinks(t *testing.T) {
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

	// publishAndRelease simulates a crash after the Generation publish but
	// before the CURRENT switch / commit audit; the retried Commit on the
	// same key then hits verifyPublished -> compiledOutputHash.
	publishAndRelease := func(key string) string {
		tx, err := gs.Begin(context.Background(), beginReq(key, nil))
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
		if err := os.Rename(
			filepath.Join(root, "generations", tx.GenerationID+".staging"),
			filepath.Join(root, "generations", tx.GenerationID),
		); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		return tx.GenerationID
	}
	replay := func(key string) error {
		tx2, err := gs.Begin(context.Background(), beginReq(key, nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx2, manifestFor(tx2, res.Inputs)); err != nil {
			t.Fatal(err)
		}
		if err := gs.WriteCompiledOutput(context.Background(), tx2, res.Outputs); err != nil {
			t.Fatal(err)
		}
		if err := gs.ValidateStaging(context.Background(), tx2); err != nil {
			t.Fatal(err)
		}
		_, err = gs.Commit(context.Background(), tx2)
		return err
	}

	// External target outside the store. Its content is a byte-for-byte copy
	// of the published page, so following the symlink would produce the same
	// hash and pass the integrity check: only an explicit symlink rejection
	// can fail closed. It must also never be written or overwritten.
	extDir := t.TempDir()

	// 1. A published output file replaced by a symlink to the external file.
	genFile := publishAndRelease("okf_symlink_file")
	page := filepath.Join(root, "generations", genFile, "wiki/strategies/verify-before-upgrade-retry.md")
	origPage, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	extFile := filepath.Join(extDir, "same-content.md")
	if err := os.WriteFile(extFile, origPage, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(page); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extFile, page); err != nil {
		t.Fatal(err)
	}
	if err := replay("okf_symlink_file"); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("file symlink must fail the integrity check, got %v", err)
	}

	// 2. A directory under wiki/ replaced by a symlink.
	genDir := publishAndRelease("okf_symlink_dir")
	strategies := filepath.Join(root, "generations", genDir, "wiki/strategies")
	if err := os.RemoveAll(strategies); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extDir, strategies); err != nil {
		t.Fatal(err)
	}
	if err := replay("okf_symlink_dir"); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("directory symlink must fail the integrity check, got %v", err)
	}

	// The external target must not have been written or overwritten.
	got, err := os.ReadFile(extFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(origPage) {
		t.Errorf("external target must not be written or overwritten, got %d bytes", len(got))
	}

	// 3. Recover must reject the same symlink and must not leak the external
	// content into diagnostics.
	genRec := publishAndRelease("okf_symlink_recover")
	if err := replay("okf_symlink_recover"); err != nil {
		t.Fatalf("clean commit before symlink swap: %v", err)
	}
	pageRec := filepath.Join(root, "generations", genRec, "wiki/strategies/verify-before-upgrade-retry.md")
	if err := os.Remove(pageRec); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(extFile, pageRec); err != nil {
		t.Fatal(err)
	}
	impl := gs.(*generationStore)
	actions, err := impl.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if a.GenerationID == genRec && a.Kind == RecoveryBlock {
			found = true
			if strings.Contains(a.Detail, string(origPage)) {
				t.Errorf("diagnostic output must not leak external content: %q", a.Detail)
			}
		}
	}
	if !found {
		t.Errorf("symlinked CURRENT generation must be reported as blocked, got %v", actions)
	}
}
