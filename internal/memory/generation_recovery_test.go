package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerationRecoveryMatrix drives the deterministic crash-state matrix:
// claim without transaction record, pending without manifest, published but
// CURRENT not switched, and CURRENT switched without commit audit.
func TestGenerationRecoveryMatrix(t *testing.T) {
	t.Run("clean store", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 0 {
			t.Errorf("clean store must have no recovery actions, got %v", actions)
		}
	})

	t.Run("claim without transaction record", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		// Manually create a claim that references a transaction that never
		// materialized (the idempotency directory is created via the secure
		// path helper).
		claim := idempotencyClaim{
			SchemaVersion:  SchemaVersion,
			IdempotencyKey: "recovery_orphan",
			RequestSHA256:  "sha256_" + strings.Repeat("b", 64),
			TransactionID:  "tx_recovery_missing",
			GenerationID:   "gen_recovery_missing",
			Status:         txPending,
			CreatedAt:      nowRFC3339(),
		}
		path, err := impl.claimPathCreate(context.Background(), "recovery_orphan")
		if err != nil {
			t.Fatal(err)
		}
		if err := impl.store.atomicWriteFile(path, mustJSON(claim)); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 1 || actions[0].Kind != RecoveryPreserve {
			t.Errorf("want preserve for claim without transaction, got %v", actions)
		}
	})

	t.Run("pending transaction with manifest but no staging", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_pending", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
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
			t.Errorf("pending transaction must be reported for retry/abort, got %v", actions)
		}
	})

	t.Run("published but CURRENT not switched stays isolated", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_published", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if err := gs.ValidateStaging(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Simulate a crash after publish but before CURRENT: rename staging
		// to the published directory by hand.
		if err := os.Rename(
			filepath.Join(root, "generations", tx.GenerationID+".staging"),
			filepath.Join(root, "generations", tx.GenerationID),
		); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && strings.Contains(a.Detail, "CURRENT not switched") {
				found = true
			}
		}
		if !found {
			t.Errorf("published-but-not-current must be reported as isolated, got %v", actions)
		}
		// CURRENT must not exist (nothing was switched).
		if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
			t.Error("CURRENT must not be created by recovery")
		}
	})

	t.Run("CURRENT switched without commit audit is completed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_no_audit", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		// Commit fully, then delete the commit audit to simulate the crash
		// window between CURRENT switch and audit write.
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "transactions", tx.TransactionID, "commit.json")); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.Kind == RecoveryComplete && a.GenerationID == tx.GenerationID {
				found = true
			}
		}
		if !found {
			t.Errorf("missing commit audit must be completed, got %v", actions)
		}
		if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "commit.json")); err != nil {
			t.Errorf("commit audit must be written by recovery: %v", err)
		}
	})

	t.Run("CURRENT switched with manifest and audit missing is completed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_no_manifest", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Simulate the crash window between CURRENT switch and manifest +
		// audit persistence: remove both, then let recovery rebuild the
		// manifest from the prepared record and complete the audit.
		if err := os.Remove(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, "transactions", tx.TransactionID, "commit.json")); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.Kind == RecoveryComplete && a.GenerationID == tx.GenerationID {
				found = true
			}
		}
		if !found {
			t.Errorf("recovery must complete manifest + audit, got %v", actions)
		}
		if _, err := os.Stat(filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json")); err != nil {
			t.Errorf("manifest must be restored by recovery: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "transactions", tx.TransactionID, "commit.json")); err != nil {
			t.Errorf("commit audit must be restored by recovery: %v", err)
		}
	})

	t.Run("committed claim not updated is synchronized", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_commit_claim", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if _, err := gs.Commit(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Drift the claim back to pending, simulating a failed claim update
		// after a successful commit; Recover must synchronize it.
		claims, err := impl.listClaims(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range claims {
			if c.TransactionID != tx.TransactionID {
				continue
			}
			path, perr := impl.claimPath(context.Background(), c.IdempotencyKey)
			if perr != nil {
				t.Fatal(perr)
			}
			c.Status = txPending
			if err := writeJSONFile(impl, path, c); err != nil {
				t.Fatal(err)
			}
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && a.Kind == RecoveryComplete {
				found = true
			}
		}
		if !found {
			t.Errorf("drifted committed claim must be synchronized, got %v", actions)
		}
	})

	t.Run("staging verified but manifest not permanent is reported", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_staging_no_perm", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if err := gs.ValidateStaging(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// State: prepared manifest + verified staging exist, but the
		// permanent Manifest was never published; recovery must report it
		// and CURRENT must never switch.
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && a.Kind == RecoveryReport && strings.Contains(a.Detail, "manifest not permanent") {
				found = true
			}
		}
		if !found {
			t.Errorf("staging-verified-without-permanent-manifest must be reported, got %v", actions)
		}
		if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
			t.Error("recovery must not switch CURRENT")
		}
	})

	t.Run("pending transaction without staged manifest is preserved", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_no_staged_manifest", nil))
		if err != nil {
			t.Fatal(err)
		}
		// The transaction has a prepared record but never staged a manifest;
		// recovery must preserve it, never switch CURRENT.
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && a.Kind == RecoveryPreserve {
				found = true
			}
		}
		if !found {
			t.Errorf("manifest-less pending transaction must be preserved, got %v", actions)
		}
		if _, err := os.Stat(filepath.Join(root, "CURRENT")); !os.IsNotExist(err) {
			t.Error("recovery must not switch CURRENT")
		}
	})

	t.Run("staging and permanent manifest ready but unpublished is reported", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_ready_unpublished", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if err := gs.ValidateStaging(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Simulate the crash window after the permanent Manifest publish but
		// before the Generation publish: release the transaction, then
		// publish the Manifest by hand.
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		mf, err := impl.readPreparedManifest(context.Background(), tx.TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := impl.store.Put(context.Background(), mf); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && a.Kind == RecoveryReport && strings.Contains(a.Detail, "may retry commit") {
				found = true
			}
		}
		if !found {
			t.Errorf("ready-but-unpublished transaction must be reported for retry, got %v", actions)
		}
	})

	t.Run("abort claim not updated is completed", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		tx, err := gs.Begin(context.Background(), beginReq("recovery_abort_claim", nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, nil)); err != nil {
			t.Fatal(err)
		}
		if err := gs.Release(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		// Write abort.json directly, leaving the claim pending (simulating
		// the crash window between abort record and claim update).
		dir, err := impl.txDir(context.Background(), tx.TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
		if err != nil {
			t.Fatal(err)
		}
		abortRec := txAbortRecord{
			SchemaVersion: SchemaVersion,
			TransactionID: tx.TransactionID,
			GenerationID:  rec.GenerationID,
			Reason:        "simulated crash",
			AbortedAt:     nowRFC3339(),
		}
		if err := writeJSONFile(impl, filepath.Join(dir, "abort.json"), abortRec); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.TransactionID == tx.TransactionID && a.Kind == RecoveryComplete {
				found = true
			}
		}
		if !found {
			t.Errorf("pending claim for aborted tx must be completed, got %v", actions)
		}
		// The claim now reads aborted.
		claims, err := impl.listClaims(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range claims {
			if c.TransactionID == tx.TransactionID && c.Status != txAborted {
				t.Errorf("claim must be aborted after recovery, got %v", c.Status)
			}
		}
	})

	t.Run("dangling claim blocks begin", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		req := beginReq("recovery_dangling", nil)
		rHash, err := requestHash(req.Scope, req.BaseGeneration, req.CompilerVersion, req.CanonicalizationVersion)
		if err != nil {
			t.Fatal(err)
		}
		claim := idempotencyClaim{
			SchemaVersion:  SchemaVersion,
			IdempotencyKey: "recovery_dangling",
			RequestSHA256:  rHash,
			TransactionID:  "tx_dangling",
			GenerationID:   "gen_dangling",
			Status:         txPending,
			CreatedAt:      nowRFC3339(),
		}
		path, err := impl.claimPathCreate(context.Background(), "recovery_dangling")
		if err != nil {
			t.Fatal(err)
		}
		if err := impl.store.atomicWriteFile(path, mustJSON(claim)); err != nil {
			t.Fatal(err)
		}
		// Any Begin on the dangling key must fail closed instead of reusing
		// or overwriting it.
		if _, err := gs.Begin(context.Background(), req); ErrorCode(err) != CodeGenerationRecoveryBlocked {
			t.Errorf("dangling claim must block begin, got %v", err)
		}
	})

	t.Run("orphan staging is reported", func(t *testing.T) {
		root := tempRoot(t)
		gs := openGenerationProject(t, root)
		impl := gs.(*generationStore)
		dir, err := impl.stagingDir(context.Background(), "gen_orphan")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSONFile(impl, filepath.Join(dir, "generation.json"), map[string]any{"schema_version": 1}); err != nil {
			t.Fatal(err)
		}
		actions, err := impl.Recover(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, a := range actions {
			if a.GenerationID == "gen_orphan" && a.Kind == RecoveryReport {
				found = true
			}
		}
		if !found {
			t.Errorf("orphan staging must be reported, got %v", actions)
		}
	})
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return b
}

// TestRecoveryRejectsTamperedCompiledOutput: CTO review — Recover checks the
// CURRENT generation's compiled output the same way verifyPublished does; a
// tampered page makes the CURRENT generation blocked instead of silently
// accepting it.
func TestRecoveryRejectsTamperedCompiledOutput(t *testing.T) {
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
	tx, err := gs.Begin(context.Background(), beginReq("okf_recovery_tamper", nil))
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

	// Tamper with a published page after the commit.
	page := filepath.Join(root, "generations", tx.GenerationID, "wiki/strategies/verify-before-upgrade-retry.md")
	orig, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, append(orig, []byte("\n# tampered\n")...), 0o600); err != nil {
		t.Fatal(err)
	}

	impl := gs.(*generationStore)
	actions, err := impl.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if a.GenerationID == tx.GenerationID && a.Kind == RecoveryBlock {
			found = true
		}
	}
	if !found {
		t.Errorf("tampered CURRENT generation must be reported as blocked, got %v", actions)
	}

	// Restore the page, then wipe compiled_output_sha256: an OKF generation
	// always emits compiled views, so an empty hash is tampering and must
	// block recovery even though the content was restored.
	if err := os.WriteFile(page, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(root, "generations", tx.GenerationID, "generation.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc.CompiledOutputSHA256 = ""
	replacement, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generations", tx.GenerationID, "generation.json"), replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	actions, err = impl.Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, a := range actions {
		if a.GenerationID == tx.GenerationID && a.Kind == RecoveryBlock {
			found = true
		}
	}
	if !found {
		t.Errorf("wiped compiled_output_sha256 on an OKF generation must be reported as blocked, got %v", actions)
	}
}
