package memory

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RecoveryActionKind classifies one deterministic recovery decision.
type RecoveryActionKind string

const (
	// RecoveryPreserve keeps a record for diagnosis without touching it.
	RecoveryPreserve RecoveryActionKind = "preserve"
	// RecoveryReport reports a consistent state that needs no action.
	RecoveryReport RecoveryActionKind = "report"
	// RecoveryBlock stops any rebuild because input requirements are unmet.
	RecoveryBlock RecoveryActionKind = "recovery_blocked"
	// RecoveryComplete fills in a missing commit audit record or a pending
	// abort claim.
	RecoveryComplete RecoveryActionKind = "complete_audit"
	// RecoveryConflict marks a CURRENT/base mismatch.
	RecoveryConflict RecoveryActionKind = "cas_conflict"
)

// RecoveryAction is one deterministic recovery outcome. Recovery never
// deletes old Generations, Manifests, facts or user data and never guesses.
type RecoveryAction struct {
	Kind          RecoveryActionKind `json:"kind"`
	TransactionID string             `json:"transaction_id,omitempty"`
	GenerationID  string             `json:"generation_id,omitempty"`
	Detail        string             `json:"detail"`
}

// pendingCompletion is a deterministic write that Recover performs under the
// scope lock: completing a commit audit (and restoring the permanent Manifest
// from the prepared record when needed) or synchronizing a pending abort
// claim. All decisions are derived from durable records; nothing is
// fabricated.
type pendingCompletion struct {
	commit    *completionTarget
	abort     *idempotencyClaim
	claimSync *idempotencyClaim
}

type completionTarget struct {
	cur  *currentPointer
	txID string
}

// Recover inspects the transaction records, idempotency claims, Manifests,
// Generations and CURRENT, and returns the deterministic recovery matrix
// outcome for every observed state. Only audit completion (a commit record
// missing after CURRENT switched, a permanent Manifest missing after CURRENT
// switched, or an abort claim left pending) is written, and always under the
// scope lock; everything else is preserved or reported.
func (gs *generationStore) Recover(ctx context.Context) ([]RecoveryAction, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "recovery cancelled")
	}
	var actions []RecoveryAction
	var pendings []pendingCompletion

	cur, curErr := gs.readCurrent(ctx)
	if curErr != nil {
		return nil, curErr
	}

	// 1. Every idempotency claim must have a consistent transaction record.
	claims, err := gs.listClaims(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range claims {
		dir, derr := gs.txDir(ctx, c.TransactionID)
		if derr != nil {
			if ErrorCode(derr) == CodeNotFound {
				actions = append(actions, RecoveryAction{
					Kind: RecoveryPreserve, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
					Detail: "claim exists but no transaction record",
				})
				continue
			}
			actions = append(actions, RecoveryAction{
				Kind: RecoveryBlock, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
				Detail: "claim references an unsafe transaction id",
			})
			continue
		}
		if _, serr := os.Stat(filepath.Join(dir, "prepared.json")); os.IsNotExist(serr) {
			actions = append(actions, RecoveryAction{
				Kind: RecoveryPreserve, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
				Detail: "claim exists but no transaction record",
			})
			continue
		}
		status, serr := gs.readTxStatus(ctx, c.TransactionID)
		if serr != nil {
			actions = append(actions, RecoveryAction{
				Kind: RecoveryPreserve, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
				Detail: "transaction record is not readable",
			})
			continue
		}
		switch status {
		case txCommitted:
			if c.Status != txCommitted {
				pendings = append(pendings, pendingCompletion{claimSync: &c})
			} else {
				actions = append(actions, RecoveryAction{
					Kind: RecoveryReport, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
					Detail: "transaction committed",
				})
			}
		case txAborted:
			if c.Status != txAborted {
				pendings = append(pendings, pendingCompletion{abort: &c})
			} else {
				actions = append(actions, RecoveryAction{
					Kind: RecoveryReport, TransactionID: c.TransactionID, GenerationID: c.GenerationID,
					Detail: "transaction aborted",
				})
			}
		default: // pending
			act := gs.diagnosePending(ctx, c.TransactionID, c.GenerationID)
			actions = append(actions, act...)
		}
	}

	// 2. Orphan staging directories (no claim references them) are reported.
	staging, err := gs.listStagingDirs(ctx)
	if err != nil {
		return nil, err
	}
	for _, genID := range staging {
		if !gs.stagingReferenced(claims, genID) {
			actions = append(actions, RecoveryAction{
				Kind: RecoveryReport, GenerationID: genID,
				Detail: "orphan staging directory",
			})
		}
	}

	// 3. CURRENT points at a Generation whose commit audit is missing:
	// complete the audit (restoring the permanent Manifest from the prepared
	// record if the crash hit before its publish) under the scope lock. The
	// compiled OKF views of the CURRENT generation are verified the same way
	// verifyPublished verifies them; a tampered or deleted page blocks the
	// recovery instead of being silently accepted.
	if cur != nil {
		genDir, gerr := gs.publishedGenDir(ctx, cur.GenerationID)
		if gerr != nil {
			actions = append(actions, RecoveryAction{
				Kind: RecoveryBlock, GenerationID: cur.GenerationID,
				Detail: "CURRENT generation directory is not readable",
			})
		} else {
			doc, derr := readJSONFile[generationDoc](filepath.Join(genDir, "generation.json"))
			if derr != nil {
				actions = append(actions, RecoveryAction{
					Kind: RecoveryBlock, GenerationID: cur.GenerationID,
					Detail: "CURRENT generation is unreadable",
				})
			} else if verr := gs.verifyCompiledOutputIntegrity(ctx, genDir, doc); verr != nil {
				actions = append(actions, RecoveryAction{
					Kind: RecoveryBlock, GenerationID: cur.GenerationID,
					Detail: "CURRENT compiled output failed the integrity check",
				})
			} else if !gs.txCommittedFor(ctx, cur.GenerationID) {
				txID, terr := gs.transactionOf(ctx, cur.GenerationID)
				if terr != nil {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryBlock, GenerationID: cur.GenerationID,
						Detail: "CURRENT generation has no transaction record",
					})
				} else {
					pendings = append(pendings, pendingCompletion{
						commit: &completionTarget{cur: cur, txID: txID},
					})
				}
			}
		}
	}

	// 4. Orphan transactions (no claim references them) are diagnosed so an
	// Abort whose claim update failed is still deterministically visible.
	orphans, err := gs.listOrphanTransactions(ctx, claims)
	if err != nil {
		return nil, err
	}
	for _, o := range orphans {
		status, serr := gs.readTxStatus(ctx, o)
		if serr != nil {
			actions = append(actions, RecoveryAction{
				Kind: RecoveryPreserve, TransactionID: o,
				Detail: "orphan transaction record is not readable",
			})
			continue
		}
		switch status {
		case txCommitted:
			actions = append(actions, RecoveryAction{
				Kind: RecoveryReport, TransactionID: o,
				Detail: "committed transaction without claim",
			})
		case txAborted:
			actions = append(actions, RecoveryAction{
				Kind: RecoveryReport, TransactionID: o,
				Detail: "aborted transaction without claim",
			})
		default:
			actions = append(actions, RecoveryAction{
				Kind: RecoveryPreserve, TransactionID: o,
				Detail: "pending transaction without claim; kept isolated",
			})
		}
	}

	// 5. Deterministic completions under the scope write lock.
	if len(pendings) > 0 {
		unlock, err := gs.store.acquireWriteLock(ctx)
		if err != nil {
			return nil, err
		}
		defer unlock()
		for _, p := range pendings {
			if p.commit != nil {
				if cerr := gs.completeCommitAudit(ctx, p.commit.cur, p.commit.txID); cerr == nil {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryComplete, TransactionID: p.commit.txID, GenerationID: p.commit.cur.GenerationID,
						Detail: "commit audit completed from CURRENT and manifest",
					})
				} else {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryBlock, TransactionID: p.commit.txID, GenerationID: p.commit.cur.GenerationID,
						Detail: "commit audit completion is blocked",
					})
				}
			}
			if p.abort != nil {
				if aerr := gs.completeAbortClaim(ctx, *p.abort); aerr == nil {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryComplete, TransactionID: p.abort.TransactionID, GenerationID: p.abort.GenerationID,
						Detail: "abort claim completed from abort record",
					})
				} else {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryBlock, TransactionID: p.abort.TransactionID, GenerationID: p.abort.GenerationID,
						Detail: "abort claim completion is blocked",
					})
				}
			}
			if p.claimSync != nil {
				if serr := gs.completeClaimStatus(ctx, *p.claimSync, txCommitted); serr == nil {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryComplete, TransactionID: p.claimSync.TransactionID, GenerationID: p.claimSync.GenerationID,
						Detail: "commit claim synchronized with commit record",
					})
				} else {
					actions = append(actions, RecoveryAction{
						Kind: RecoveryBlock, TransactionID: p.claimSync.TransactionID, GenerationID: p.claimSync.GenerationID,
						Detail: "commit claim synchronization is blocked",
					})
				}
			}
		}
	}

	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Kind != actions[j].Kind {
			return actions[i].Kind < actions[j].Kind
		}
		return actions[i].TransactionID < actions[j].TransactionID
	})
	return actions, nil
}

// diagnosePending maps the crash matrix for a pending transaction.
func (gs *generationStore) diagnosePending(ctx context.Context, txID, genID string) []RecoveryAction {
	dir, err := gs.txDir(ctx, txID)
	if err != nil {
		return []RecoveryAction{{Kind: RecoveryBlock, TransactionID: txID, GenerationID: genID, Detail: "transaction path is unsafe"}}
	}
	// Staging built?
	stagingOK := false
	if staging, serr := gs.stagingDir(ctx, genID); serr == nil {
		if _, oerr := os.Stat(filepath.Join(staging, "generation.json")); oerr == nil {
			stagingOK = true
		}
	}
	// Manifest staged in the transaction area?
	manifestOK := false
	if _, merr := os.Stat(filepath.Join(dir, "prepared-manifest.json")); merr == nil {
		manifestOK = true
	}
	// Manifest published as a permanent fact?
	permanentManifestOK := false
	if _, perr := gs.store.Get(ctx, FactKindGenerationInputManifest, genID); perr == nil {
		permanentManifestOK = true
	}
	// Generation published?
	published := false
	if _, perr := os.Stat(filepath.Join(gs.store.root, "generations", genID, "generation.json")); perr == nil {
		published = true
	}
	switch {
	case !manifestOK:
		return []RecoveryAction{{
			Kind: RecoveryPreserve, TransactionID: txID, GenerationID: genID,
			Detail: "prepared facts present but manifest missing; keep isolated",
		}}
	case stagingOK && !permanentManifestOK:
		// Staging is verified but the permanent Manifest was not published
		// before the crash; CURRENT must never switch, the transaction can
		// be retried under the lock (which republishes the Manifest).
		return []RecoveryAction{{
			Kind: RecoveryReport, TransactionID: txID, GenerationID: genID,
			Detail: "staging verified but manifest not permanent; may retry commit",
		}}
	case !stagingOK && !published:
		return []RecoveryAction{{
			Kind: RecoveryReport, TransactionID: txID, GenerationID: genID,
			Detail: "manifest stored but staging incomplete; may rebuild or abort",
		}}
	case published && !gs.currentIs(genID):
		return []RecoveryAction{{
			Kind: RecoveryReport, TransactionID: txID, GenerationID: genID,
			Detail: "generation published but CURRENT not switched; kept isolated",
		}}
	default:
		return []RecoveryAction{{
			Kind: RecoveryReport, TransactionID: txID, GenerationID: genID,
			Detail: "staging complete; may retry commit under the scope lock or abort",
		}}
	}
}

func (gs *generationStore) currentIs(genID string) bool {
	cur, err := gs.readCurrent(context.Background())
	if err != nil || cur == nil {
		return false
	}
	return cur.GenerationID == genID
}

func (gs *generationStore) listClaims(ctx context.Context) ([]idempotencyClaim, error) {
	dir, err := secureJoin(gs.store.root, []string{"idempotency"}, false, false)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot inspect idempotency directory")
	}
	var out []idempotencyClaim
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		claim, rerr := readJSONFile[idempotencyClaim](filepath.Join(dir, e.Name()))
		if rerr != nil {
			// An unreadable claim is a deterministic diagnostic, not
			// something to skip silently.
			out = append(out, idempotencyClaim{
				SchemaVersion:  0,
				IdempotencyKey: strings.TrimSuffix(e.Name(), ".json"),
				TransactionID:  "<unreadable>",
				GenerationID:   "",
				Status:         txPending,
			})
			continue
		}
		out = append(out, claim)
	}
	return out, nil
}

// listOrphanTransactions returns transaction ids that no claim references.
func (gs *generationStore) listOrphanTransactions(ctx context.Context, claims []idempotencyClaim) ([]string, error) {
	dir, err := secureJoin(gs.store.root, []string{"transactions"}, false, false)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot inspect transactions directory")
	}
	referenced := map[string]bool{}
	for _, c := range claims {
		referenced[c.TransactionID] = true
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || referenced[e.Name()] {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

func (gs *generationStore) listStagingDirs(ctx context.Context) ([]string, error) {
	dir, err := secureJoin(gs.store.root, []string{"generations"}, false, false)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot inspect generations directory")
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".staging") {
			out = append(out, strings.TrimSuffix(e.Name(), ".staging"))
		}
	}
	return out, nil
}

func (gs *generationStore) stagingReferenced(claims []idempotencyClaim, genID string) bool {
	for _, c := range claims {
		if c.GenerationID == genID {
			return true
		}
	}
	return false
}

func (gs *generationStore) txCommittedFor(ctx context.Context, genID string) bool {
	dir, err := secureJoin(gs.store.root, []string{"generations", genID}, false, false)
	if err != nil {
		return false
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil {
		return false
	}
	txDir, err := gs.txDir(ctx, doc.TransactionID)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(txDir, "commit.json"))
	return err == nil
}

func (gs *generationStore) transactionOf(ctx context.Context, genID string) (string, error) {
	dir, err := secureJoin(gs.store.root, []string{"generations", genID}, false, false)
	if err != nil {
		return "", err
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil {
		return "", err
	}
	return doc.TransactionID, nil
}

// completeCommitAudit completes the audit for a Generation that CURRENT
// already points at, deriving everything from the stored Generation and the
// prepared Manifest. If the permanent Manifest is missing (crash between
// CURRENT switch and manifest publish), it is restored no-overwrite from the
// prepared record. It never fabricates a missing prepared record. Callers
// hold the scope lock.
func (gs *generationStore) completeCommitAudit(ctx context.Context, cur *currentPointer, txID string) error {
	dir, err := gs.txDirCreate(ctx, txID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "prepared.json")); os.IsNotExist(err) {
		return storeError(CodeGenerationRecoveryBlocked, "cannot fabricate missing transaction record")
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return err
	}
	// Consistency: the CURRENT output hash must match the stored Generation
	// document before the audit can be trusted.
	genDir, gerr := gs.publishedGenDir(ctx, cur.GenerationID)
	if gerr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "published generation is not readable")
	}
	doc, derr := readJSONFile[generationDoc](filepath.Join(genDir, "generation.json"))
	if derr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "published generation is not readable")
	}
	computed, herr := doc.outputHash()
	if herr != nil || computed != cur.OutputGenerationSHA256 {
		return storeError(CodeGenerationRecoveryBlocked, "CURRENT hash does not match the published generation")
	}
	// The compiled OKF views must still hash to the pinned
	// compiled_output_sha256; tampering blocks the audit completion.
	if verr := gs.verifyCompiledOutputIntegrity(ctx, genDir, doc); verr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "published compiled output failed the integrity check")
	}
	// The manifest must be recoverable from the prepared record and present
	// as a permanent fact before the audit references it.
	mf, merr := gs.readPreparedManifest(ctx, txID)
	if merr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "manifest missing for audit completion")
	}
	if _, gerr := gs.store.Get(ctx, FactKindGenerationInputManifest, cur.GenerationID); gerr != nil {
		if ErrorCode(gerr) != CodeNotFound {
			return storeError(CodeGenerationRecoveryBlocked, "permanent manifest cannot be verified")
		}
		if _, perr := gs.store.putLocked(ctx, mf); perr != nil {
			return storeError(CodeGenerationRecoveryBlocked, "permanent manifest cannot be restored")
		}
	}
	commitRec := txCommitRecord{
		SchemaVersion:            SchemaVersion,
		TransactionID:            txID,
		GenerationID:             cur.GenerationID,
		BaseGeneration:           baseOrEmpty(rec.BaseGeneration),
		OutputGenerationSHA256:   cur.OutputGenerationSHA256,
		GenerationManifestSHA256: mf.InputManifestSHA256,
		CommittedAt:              nowRFC3339(),
	}
	if err := writeJSONFile(gs, filepath.Join(dir, "commit.json"), commitRec); err != nil {
		return err
	}
	// Keep the claim in step with the completed audit so a single recovery
	// pass converges.
	_ = gs.updateClaimStatus(ctx, rec, txCommitted)
	return nil
}

// completeAbortClaim synchronizes a pending claim with an existing abort
// record. Callers hold the scope lock.
func (gs *generationStore) completeAbortClaim(ctx context.Context, claim idempotencyClaim) error {
	return gs.completeClaimStatus(ctx, claim, txAborted)
}

// completeClaimStatus sets a claim's status from the transaction's terminal
// record. Callers hold the scope lock.
func (gs *generationStore) completeClaimStatus(ctx context.Context, claim idempotencyClaim, status txStatus) error {
	dir, err := gs.txDir(ctx, claim.TransactionID)
	if err != nil {
		return storeError(CodeGenerationRecoveryBlocked, "claim cannot be completed")
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return storeError(CodeGenerationRecoveryBlocked, "claim cannot be completed")
	}
	path, perr := gs.claimPath(ctx, rec.IdempotencyKey)
	if perr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "claim cannot be completed")
	}
	claims, cerr := readJSONFile[idempotencyClaim](path)
	if cerr != nil {
		return storeError(CodeGenerationRecoveryBlocked, "claim cannot be completed")
	}
	claims.Status = status
	return writeJSONFile(gs, path, claims)
}
