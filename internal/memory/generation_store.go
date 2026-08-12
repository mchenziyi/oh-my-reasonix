package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// GenerationStore implements the MEM-01D generation transaction skeleton:
// prepared isolation, idempotent claim, scope single-writer lock held for the
// whole transaction lifetime, Generation Input Manifest permanence (published
// only by a successful Commit, never before CURRENT) and the CURRENT CAS
// commit point. All storage reuses the MEM-01B FactStore (secure paths,
// permissions, symlink checks, atomic writes, locking); no second store, hash
// or path logic exists here.
type GenerationStore interface {
	Begin(ctx context.Context, req BeginGenerationRequest) (*GenerationTx, error)
	PrepareFact(ctx context.Context, tx *GenerationTx, fact Fact) error
	PrepareManifest(ctx context.Context, tx *GenerationTx, manifest GenerationInputManifest) error
	ValidateStaging(ctx context.Context, tx *GenerationTx) error
	// WriteCompiledOutput stages the OKF compiler's view set into the
	// transaction's generation staging directory (MEM-01E).
	WriteCompiledOutput(ctx context.Context, tx *GenerationTx, outputs map[string][]byte) error
	Commit(ctx context.Context, tx *GenerationTx) (CommitResult, error)
	Abort(ctx context.Context, tx *GenerationTx, reason string) error
	// Release drops the transaction handle without reaching a terminal
	// state, releasing the scope write lock. It is idempotent.
	Release(ctx context.Context, tx *GenerationTx) error
	Recover(ctx context.Context) ([]RecoveryAction, error)
}

// BeginGenerationRequest describes one generation transaction. The request
// hash binds the idempotency key to the exact request; a replayed key with a
// different request fails closed.
type BeginGenerationRequest struct {
	Scope                   Scope
	BaseGeneration          *string
	CompilerVersion         string
	CanonicalizationVersion int
	SchemaVersion           int
	IdempotencyKey          string
	RequestSHA256           string // optional; verified against the computed request hash when set
}

// GenerationTx is the live handle of one prepared transaction. It owns the
// scope write lock from Begin until Commit/Abort/Release, so every prepare
// step is serialized against other writers and CURRENT is fixed under the
// lock at Begin time.
type GenerationTx struct {
	TransactionID           string
	GenerationID            string
	Scope                   Scope
	BaseGeneration          *string
	CompilerVersion         string
	CanonicalizationVersion int
	RequestSHA256           string

	// internal state: the owning store and the lock release function.
	gs       *generationStore
	unlock   func()
	mu       sync.Mutex
	released bool
}

// release hands the scope write lock back exactly once.
func (tx *GenerationTx) release() {
	if tx == nil {
		return
	}
	tx.mu.Lock()
	if tx.released || tx.unlock == nil {
		tx.mu.Unlock()
		return
	}
	tx.released = true
	u := tx.unlock
	tx.mu.Unlock()
	u()
}

// checkOpen fails closed on a released or foreign transaction handle.
func (gs *generationStore) checkOpen(tx *GenerationTx) error {
	if tx == nil {
		return storeError(CodeGenerationTxConflict, "transaction handle is closed")
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.released || tx.gs != gs {
		return storeError(CodeGenerationTxConflict, "transaction handle is closed")
	}
	return nil
}

// CommitResult reports the outcome of a Commit call.
type CommitResult struct {
	TransactionID          string
	GenerationID           string
	Status                 CommitStatus
	BaseGeneration         string
	OutputGenerationSHA256 string
}

// CommitStatus distinguishes a fresh commit, an idempotent replay of an
// already-committed transaction, and a commit whose CURRENT already switched
// but whose audit/manifest persistence is still pending recovery.
type CommitStatus int

const (
	CommitCommitted CommitStatus = iota
	CommitAlreadyCommitted
	CommitPendingRecovery
)

func (s CommitStatus) String() string {
	switch s {
	case CommitCommitted:
		return "committed"
	case CommitAlreadyCommitted:
		return "already_committed"
	case CommitPendingRecovery:
		return "committed_recovery_pending"
	default:
		return "unknown"
	}
}

// generationStore is the concrete GenerationStore bound to one scope store.
type generationStore struct {
	store *FactStore
}

// NewGenerationStore wraps an already-open FactStore. The returned store
// inherits its scope, permissions, lock, symlink and redaction behavior.
func NewGenerationStore(store *FactStore) GenerationStore {
	return &generationStore{store: store}
}

// ---- path helpers (all through secureJoin; never raw filepath.Join) ----

func (gs *generationStore) txDir(ctx context.Context, txID string) (string, error) {
	if err := validateID(txID, "transaction_id"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid transaction id")
	}
	return secureJoin(gs.store.root, []string{"transactions", txID}, false, false)
}

func (gs *generationStore) txDirCreate(ctx context.Context, txID string) (string, error) {
	if err := validateID(txID, "transaction_id"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid transaction id")
	}
	return secureJoin(gs.store.root, []string{"transactions", txID}, true, false)
}

func (gs *generationStore) claimPath(ctx context.Context, key string) (string, error) {
	if err := validateID(key, "idempotency_key"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid idempotency key")
	}
	return secureJoin(gs.store.root, []string{"idempotency", key + ".json"}, false, true)
}

func (gs *generationStore) claimPathCreate(ctx context.Context, key string) (string, error) {
	if err := validateID(key, "idempotency_key"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid idempotency key")
	}
	return secureJoin(gs.store.root, []string{"idempotency", key + ".json"}, true, true)
}

func (gs *generationStore) currentPath() (string, error) {
	// creating=true so a missing CURRENT on the first generation resolves to
	// its path instead of failing; the file itself is never created here.
	return secureJoin(gs.store.root, []string{"CURRENT"}, true, true)
}

func (gs *generationStore) stagingDir(ctx context.Context, genID string) (string, error) {
	if err := validateID(genID, "generation_id"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid generation id")
	}
	return secureJoin(gs.store.root, []string{"generations", genID + ".staging"}, true, false)
}

func (gs *generationStore) publishedGenDir(ctx context.Context, genID string) (string, error) {
	if err := validateID(genID, "generation_id"); err != nil {
		return "", storeError(CodePathUnsafe, "invalid generation id")
	}
	return secureJoin(gs.store.root, []string{"generations", genID}, false, false)
}

// ---- atomic replace (overwrite for the tx's own files and CURRENT) ----

// atomicReplace writes data to path atomically (same-directory temp file,
// fsync, rename). Unlike atomicWriteFile it overwrites the target; it is only
// used for files owned by this transaction or the CURRENT pointer, never for
// immutable facts.
func (gs *generationStore) atomicReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".omr-gentx-*.tmp")
	if err != nil {
		return storeError(CodePermissionDenied, "cannot create temporary file")
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot set file permissions")
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot write transaction file")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot sync transaction file")
	}
	if err := tmp.Close(); err != nil {
		return storeError(CodePermissionDenied, "cannot close transaction file")
	}
	if err := os.Rename(tmpName, path); err != nil {
		return storeError(CodePermissionDenied, "cannot publish transaction file")
	}
	syncDir(dir)
	return nil
}

func writeJSONFile(gs *generationStore, path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return storeError(CodeSchemaInvalid, "cannot encode transaction record")
	}
	return gs.atomicReplace(path, data)
}

func readJSONFile[T any](path string) (T, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, storeError(CodePermissionDenied, "cannot read transaction record")
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, storeError(CodeCorruptFile, "transaction record is not valid JSON")
	}
	return v, nil
}

// ---- Begin ----

// Begin runs the fixed claim-then-lock sequence: the idempotency claim is
// created atomically (no-overwrite) before any business side effect, the
// claim's transaction record is materialized immediately (claim-exclusive,
// so no lock is needed), then the scope write lock is acquired and held by
// the returned transaction until Commit/Abort/Release. The base generation
// is fixed by the request; CURRENT is validated under the lock at Commit
// time (CAS), never outside it.
func (gs *generationStore) Begin(ctx context.Context, req BeginGenerationRequest) (*GenerationTx, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "transaction cancelled")
	}
	if !gs.store.scopeMatches(req.Scope) {
		return nil, storeError(CodeScopeMismatch, "transaction scope does not match store scope")
	}
	if req.SchemaVersion != 0 && req.SchemaVersion != SchemaVersion {
		return nil, storeError(CodeSchemaInvalid, "unsupported schema version")
	}
	if req.BaseGeneration != nil {
		if err := validateID(*req.BaseGeneration, "base_generation"); err != nil {
			return nil, storeError(CodePathUnsafe, "invalid base generation")
		}
	}
	if err := validateVersionID(req.CompilerVersion, "compiler_version"); err != nil {
		return nil, storeError(CodeSchemaInvalid, "invalid compiler version")
	}
	if req.CanonicalizationVersion < 1 {
		return nil, storeError(CodeSchemaInvalid, "canonicalization version must be >= 1")
	}
	rHash, err := requestHash(req.Scope, req.BaseGeneration, req.CompilerVersion, req.CanonicalizationVersion)
	if err != nil {
		return nil, storeError(CodeSchemaInvalid, "request hash cannot be computed")
	}
	if req.RequestSHA256 != "" && req.RequestSHA256 != rHash {
		return nil, storeError(CodeSchemaInvalid, "request hash mismatch")
	}

	// Step 1: idempotent claim, before any side effect.
	claim, created, err := gs.claim(ctx, req, rHash)
	if err != nil {
		return nil, err
	}

	// Step 2: materialize the transaction record immediately after winning
	// the claim. The record is claim-exclusive (only its owner writes it),
	// so this is safe outside the lock, and it closes the race where a later
	// claimant reaches the lock before the winner: it always finds the
	// record, and only a genuinely dangling claim (crash between claim and
	// record) is ever reported as recovery-blocked.
	if created {
		if err := gs.createTxRecord(ctx, claim, req, rHash); err != nil {
			return nil, err
		}
	}

	// Step 3: scope write lock, held by the transaction from here on.
	unlock, err := gs.store.acquireWriteLock(ctx)
	if err != nil {
		return nil, err
	}
	tx := &GenerationTx{
		Scope:                   req.Scope,
		BaseGeneration:          req.BaseGeneration,
		CompilerVersion:         req.CompilerVersion,
		CanonicalizationVersion: req.CanonicalizationVersion,
		RequestSHA256:           rHash,
		gs:                      gs,
		unlock:                  unlock,
	}

	// Step 4: under the lock, bind the claim to its transaction record.
	if err := gs.finalizeBegin(ctx, tx, req, claim, created); err != nil {
		tx.release()
		return nil, err
	}
	return tx, nil
}

// claim atomically creates the idempotency claim or returns the existing
// one. created reports whether this caller won the claim.
func (gs *generationStore) claim(ctx context.Context, req BeginGenerationRequest, rHash string) (idempotencyClaim, bool, error) {
	claimFile, err := gs.claimPathCreate(ctx, req.IdempotencyKey)
	if err != nil {
		return idempotencyClaim{}, false, err
	}
	// Try to read an existing claim first (it may have been created by a
	// concurrent writer between our lookup and now).
	if existing, readErr := readJSONFile[idempotencyClaim](claimFile); readErr == nil {
		return existing, false, nil
	}
	txID, err := newRandomID("tx")
	if err != nil {
		return idempotencyClaim{}, false, storeError(CodePermissionDenied, "cannot generate transaction id")
	}
	genID, err := newRandomID("gen")
	if err != nil {
		return idempotencyClaim{}, false, storeError(CodePermissionDenied, "cannot generate generation id")
	}
	claim := idempotencyClaim{
		SchemaVersion:  SchemaVersion,
		IdempotencyKey: req.IdempotencyKey,
		RequestSHA256:  rHash,
		TransactionID:  txID,
		GenerationID:   genID,
		Status:         txPending,
		CreatedAt:      nowRFC3339(),
	}
	claimData, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return idempotencyClaim{}, false, storeError(CodeSchemaInvalid, "cannot encode idempotency claim")
	}
	if err := gs.store.atomicWriteFile(claimFile, claimData); err != nil {
		if err == errTargetExists {
			// Lost the race: another process claimed the key first.
			existing, rerr := readJSONFile[idempotencyClaim](claimFile)
			if rerr != nil {
				return idempotencyClaim{}, false, storeError(CodeGenerationTxConflict, "idempotency claim is not readable")
			}
			return existing, false, nil
		}
		return idempotencyClaim{}, false, err
	}
	return claim, true, nil
}

// createTxRecord writes the claim's transaction record (transactions/<txID>/prepared.json).
// It runs outside the scope lock because the record is claim-exclusive: only
// the claim winner writes it, so there is no shared-write race.
func (gs *generationStore) createTxRecord(ctx context.Context, claim idempotencyClaim, req BeginGenerationRequest, rHash string) error {
	dir, err := gs.txDirCreate(ctx, claim.TransactionID)
	if err != nil {
		return err
	}
	rec := txRecord{
		SchemaVersion:           SchemaVersion,
		TransactionID:           claim.TransactionID,
		IdempotencyKey:          req.IdempotencyKey,
		GenerationID:            claim.GenerationID,
		Scope:                   req.Scope,
		BaseGeneration:          req.BaseGeneration,
		CompilerVersion:         req.CompilerVersion,
		CanonicalizationVersion: req.CanonicalizationVersion,
		RequestSHA256:           rHash,
		PreparedFacts:           []preparedFact{},
		CreatedAt:               nowRFC3339(),
	}
	return writeJSONFile(gs, filepath.Join(dir, "prepared.json"), rec)
}

// finalizeBegin runs under the scope lock: it verifies the request hash and
// binds the claim to a live transaction record. A dangling claim (crash
// between claim and record) must never be reused and never be silently
// overwritten; recovery reports it.
func (gs *generationStore) finalizeBegin(ctx context.Context, tx *GenerationTx, req BeginGenerationRequest, claim idempotencyClaim, created bool) error {
	if claim.RequestSHA256 != tx.RequestSHA256 {
		return storeError(CodeGenerationIdempotency, "idempotency key already used with a different request")
	}
	if !created {
		// The claim must reference a live transaction record. When we won
		// the claim we created the record before taking the lock, so a
		// record that appears shortly after the claim is the winner still
		// materializing it (it needs no lock for that); only a record that
		// never appears is a genuine dangling claim.
		if err := gs.waitTxRecord(ctx, claim); err != nil {
			return err
		}
	}
	tx.TransactionID = claim.TransactionID
	tx.GenerationID = claim.GenerationID
	return nil
}

// txRecordWaitTries bounds how long a claimant waits, under the lock, for the
// claim winner to materialize the transaction record before treating the
// claim as dangling.
const txRecordWaitTries = 50

// waitTxRecord polls for the claim's transaction record. The winner creates
// it outside the lock immediately after winning the claim, so a brief wait
// closes the begin race; a record that never appears is a dangling claim and
// fails closed.
func (gs *generationStore) waitTxRecord(ctx context.Context, claim idempotencyClaim) error {
	for i := 0; ; i++ {
		if ctx.Err() != nil {
			return storeError(CodeLockTimeout, "transaction cancelled")
		}
		dir, err := gs.txDir(ctx, claim.TransactionID)
		if err != nil {
			if ErrorCode(err) == CodePathUnsafe {
				return storeError(CodeGenerationRecoveryBlocked, "idempotency claim references an unsafe transaction id")
			}
			if ErrorCode(err) == CodeNotFound {
				// not materialized yet: keep waiting
			} else {
				// A real filesystem problem is not a transient begin race;
				// fail closed instead of polling on it.
				return err
			}
		} else if _, serr := os.Stat(filepath.Join(dir, "prepared.json")); serr == nil {
			return nil
		}
		if i >= txRecordWaitTries {
			return storeError(CodeGenerationRecoveryBlocked, "idempotency claim references a missing transaction")
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			return storeError(CodeLockTimeout, "transaction cancelled")
		}
	}
}

// ---- PrepareFact ----

// PrepareFact validates a fact and stages it into the transaction's prepared
// area with its full structured identity (fact type/id, schema version,
// scope, content hash). Prepared facts are never visible to normal reads;
// they become the exact reference set the Manifest inputs are verified
// against at Commit time.
func (gs *generationStore) PrepareFact(ctx context.Context, tx *GenerationTx, fact Fact) error {
	if err := ctx.Err(); err != nil {
		return storeError(CodeLockTimeout, "transaction cancelled")
	}
	if err := gs.checkOpen(tx); err != nil {
		return err
	}
	if err := fact.Validate(); err != nil {
		return classifyValidateError(err)
	}
	dir, err := gs.txDir(ctx, tx.TransactionID)
	if err != nil {
		return err
	}
	if err := gs.checkTxTerminal(dir); err != nil {
		return err
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return err
	}
	if rec.TransactionID != tx.TransactionID {
		return storeError(CodeGenerationTxConflict, "transaction record mismatch")
	}
	if sc, ok := factScope(fact); ok && sc != rec.Scope {
		return storeError(CodeScopeMismatch, "fact scope does not match transaction scope")
	}
	ft, fid, err := factIdentity(fact)
	if err != nil {
		return storeError(CodeSchemaInvalid, "fact type is not storable")
	}
	h, err := fact.ContentHash()
	if err != nil {
		return storeError(CodeSchemaInvalid, "fact hash cannot be computed")
	}
	canon, err := fact.EncodeCanonical()
	if err != nil {
		return storeError(CodeSchemaInvalid, "fact cannot be canonicalized")
	}
	if len(rec.PreparedFacts) >= maxPreparedFacts {
		return storeError(CodeGenerationTxConflict, "too many prepared facts")
	}
	sc, _ := factScope(fact)
	// The same identity may be staged more than once only with the exact
	// same content (idempotent); a second, different fact for the same
	// identity would silently bypass the Manifest input verification, so it
	// fails closed here.
	for _, pf := range rec.PreparedFacts {
		if pf.FactType == ft && pf.FactID == fid {
			if pf.ContentSHA256 != h {
				return storeError(CodeGenerationTxConflict, "conflicting prepared facts for the same identity")
			}
			return nil
		}
	}
	rec.PreparedFacts = append(rec.PreparedFacts, preparedFact{
		FactType:          ft,
		FactID:            fid,
		FactSchemaVersion: factSchemaVersion(fact),
		Scope:             sc,
		ContentSHA256:     h,
		Canonical:         canon,
	})
	return writeJSONFile(gs, filepath.Join(dir, "prepared.json"), rec)
}

// ---- PrepareManifest ----

// PrepareManifest validates the Generation Input Manifest against the
// transaction record and stages it as transactions/<txID>/prepared-manifest.json.
// It is never written into facts/ before CURRENT switches: only Commit, after
// staging validation succeeds and while holding the lock, publishes the
// permanent no-overwrite manifest. Abort, CAS conflicts and staging failures
// therefore never leave a Manifest readable through the FactStore.
func (gs *generationStore) PrepareManifest(ctx context.Context, tx *GenerationTx, manifest GenerationInputManifest) error {
	if err := ctx.Err(); err != nil {
		return storeError(CodeLockTimeout, "transaction cancelled")
	}
	if err := gs.checkOpen(tx); err != nil {
		return err
	}
	dir, err := gs.txDir(ctx, tx.TransactionID)
	if err != nil {
		return err
	}
	if err := gs.checkTxTerminal(dir); err != nil {
		return err
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return err
	}
	if manifest.TransactionID != tx.TransactionID {
		return storeError(CodeGenerationManifestMismatch, "manifest transaction does not match")
	}
	if manifest.GenerationID != rec.GenerationID {
		return storeError(CodeGenerationManifestMismatch, "manifest generation does not match transaction")
	}
	if manifest.Scope != rec.Scope {
		return storeError(CodeGenerationManifestMismatch, "manifest scope does not match transaction")
	}
	if manifest.CompilerVersion != rec.CompilerVersion || manifest.CanonicalizationVersion != rec.CanonicalizationVersion {
		return storeError(CodeGenerationManifestMismatch, "manifest compiler does not match transaction")
	}
	if !generationCompilerAvailable(manifest.CompilerVersion, manifest.CanonicalizationVersion) {
		return storeError(CodeGenerationCompilerUnavailable, "generation compiler is not available")
	}
	if !sameBase(manifest.BaseGeneration, rec.BaseGeneration) {
		return storeError(CodeGenerationManifestMismatch, "manifest base generation does not strictly match transaction")
	}
	if err := manifest.Validate(); err != nil {
		return storeError(CodeGenerationManifestMismatch, "manifest violates the schema")
	}
	return writeJSONFile(gs, filepath.Join(dir, "prepared-manifest.json"), manifest)
}

// readPreparedManifest reads the staged (transaction-local) manifest.
func (gs *generationStore) readPreparedManifest(ctx context.Context, txID string) (GenerationInputManifest, error) {
	dir, err := gs.txDir(ctx, txID)
	if err != nil {
		return GenerationInputManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "prepared-manifest.json"))
	if err != nil {
		return GenerationInputManifest{}, storeError(CodeGenerationManifestMismatch, "generation manifest is missing or unreadable")
	}
	return DecodeStrict[GenerationInputManifest](data)
}

// ---- ValidateStaging ----

// ValidateStaging builds the staging Generation (if absent) and verifies its
// integrity: compiler availability, output hash agreement with the Manifest,
// and a complete published input set.
func (gs *generationStore) ValidateStaging(ctx context.Context, tx *GenerationTx) error {
	if err := ctx.Err(); err != nil {
		return storeError(CodeLockTimeout, "transaction cancelled")
	}
	if err := gs.checkOpen(tx); err != nil {
		return err
	}
	_, err := gs.buildStaging(ctx, tx, false)
	return err
}

// ---- compiled output staging ----

// WriteCompiledOutput stages the OKF compiler's view set (wiki/ and state/)
// into the transaction's generation staging directory. It never bypasses the
// transaction: the staged files become part of the published Generation only
// through Commit, and their deterministic hash is pinned by generation.json.
// Paths are strictly validated (no traversal, no absolute paths, no symlink
// escape, no control characters) and every file is written atomically with
// store permissions.
func (gs *generationStore) WriteCompiledOutput(ctx context.Context, tx *GenerationTx, outputs map[string][]byte) error {
	if err := gs.checkOpen(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return storeError(CodeLockTimeout, "write cancelled")
	}
	if len(outputs) == 0 {
		return nil
	}
	staging, err := gs.stagingDir(ctx, tx.GenerationID)
	if err != nil {
		return err
	}
	for rel, data := range outputs {
		comps, err := validateCompiledRelPath(rel)
		if err != nil {
			return err
		}
		if int64(len(data)) > maxCompiledOutputBytes {
			return storeError(CodeOKFCompileError, "compiled output exceeds size limit")
		}
		if !utf8.Valid(data) {
			return storeError(CodeOKFCompileError, "compiled output is not valid UTF-8")
		}
		path, err := secureJoin(staging, comps, true, true)
		if err != nil {
			return err
		}
		if err := gs.atomicReplace(path, data); err != nil {
			return err
		}
	}
	return nil
}

// validateCompiledRelPath checks a generation-relative compiled view path:
// it must live under wiki/ or state/, contain no traversal or absolute
// components, and every component must be a safe identifier.
func validateCompiledRelPath(rel string) ([]string, error) {
	if rel == "" || len(rel) > 4*MaxFactKeyBytes {
		return nil, storeError(CodeOKFCompileError, "invalid compiled output path")
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, "..") || strings.Contains(rel, "\\") ||
		strings.ContainsAny(rel, "\x00\r\n") {
		return nil, storeError(CodeOKFCompileError, "invalid compiled output path")
	}
	if !strings.HasPrefix(rel, "wiki/") && !strings.HasPrefix(rel, "state/") {
		return nil, storeError(CodeOKFCompileError, "compiled output must live under wiki/ or state/")
	}
	comps := strings.Split(rel, "/")
	for _, c := range comps {
		if c == "" || strings.HasPrefix(c, ".") || !reIdentifier.MatchString(c) {
			return nil, storeError(CodeOKFCompileError, "invalid compiled output path")
		}
	}
	return comps, nil
}

// ---- Commit ----

// Commit executes the frozen transaction order while holding the scope write
// lock: staging validation, then permanent Manifest publish, then Generation
// publish, then the CURRENT CAS, then the commit audit and claim update.
// Manifest publish failure is an ordinary error (CURRENT untouched, no
// Generation published); only failures after CURRENT has switched (commit
// audit, claim update) return the committed-recovery-pending result.
func (gs *generationStore) Commit(ctx context.Context, tx *GenerationTx) (res CommitResult, err error) {
	if err := gs.checkOpen(tx); err != nil {
		return CommitResult{}, err
	}
	// The transaction normally ends here, so the scope lock is handed back
	// on every path except committed-but-recovery-pending: after CURRENT
	// switched with a failed audit the lock stays with the transaction so a
	// retried Commit can deterministically complete the remaining steps.
	defer func() {
		if res.Status != CommitPendingRecovery {
			tx.release()
		}
	}()
	if err := ctx.Err(); err != nil {
		return CommitResult{}, storeError(CodeLockTimeout, "transaction cancelled")
	}

	dir, err := gs.txDir(ctx, tx.TransactionID)
	if err != nil {
		return CommitResult{}, err
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return CommitResult{}, err
	}
	// Already committed: idempotent replay returns the existing result. The
	// claim status is synced best-effort (the commit record is the fact; a
	// drifted claim is completed deterministically by Recover).
	if _, err := os.Stat(filepath.Join(dir, "commit.json")); err == nil {
		commitRec, rerr := readJSONFile[txCommitRecord](filepath.Join(dir, "commit.json"))
		if rerr != nil {
			return CommitResult{}, storeError(CodeGenerationAlreadyCommitted, "commit record is unreadable")
		}
		_ = gs.updateClaimStatus(ctx, rec, txCommitted)
		return CommitResult{
			TransactionID:          commitRec.TransactionID,
			GenerationID:           commitRec.GenerationID,
			Status:                 CommitAlreadyCommitted,
			BaseGeneration:         commitRec.BaseGeneration,
			OutputGenerationSHA256: commitRec.OutputGenerationSHA256,
		}, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "abort.json")); err == nil {
		return CommitResult{}, storeError(CodeGenerationTxConflict, "transaction is aborted")
	}

	// The manifest must be staged before staging can be validated (it pins
	// the exact output hash); input/fact cross-verification happens here.
	mf, err := gs.readPreparedManifest(ctx, tx.TransactionID)
	if err != nil {
		return CommitResult{}, storeError(CodeGenerationManifestMismatch, "generation manifest is missing or unreadable")
	}
	if err := gs.verifyManifestInputs(ctx, rec, mf); err != nil {
		return CommitResult{}, err
	}

	gen, err := gs.buildStaging(ctx, tx, false)
	if err != nil {
		return CommitResult{}, err
	}

	// Publish the permanent Generation Input Manifest (no-overwrite, so a
	// concurrent or recovering writer can never replace it) from the
	// prepared record. This happens under the lock after staging validation
	// and before the Generation publish / CURRENT switch, per the frozen
	// architecture. A failure here is an ordinary error: CURRENT is
	// untouched, no Generation has been published and the Manifest is not an
	// effective fact — nothing returns committed-recovery-pending yet.
	if _, err := gs.store.putLocked(ctx, mf); err != nil {
		return CommitResult{}, err
	}

	// Publish the immutable Generation directory.
	staging, err := gs.stagingDir(ctx, rec.GenerationID)
	if err != nil {
		return CommitResult{}, err
	}
	final := filepath.Join(filepath.Dir(staging), rec.GenerationID)
	if _, err := os.Stat(final); err == nil {
		// Already published (retry of a crash between publish and CURRENT):
		// verify it matches, then continue to CAS.
		if err := gs.verifyPublished(ctx, rec, gen, final); err != nil {
			return CommitResult{}, err
		}
	} else if !os.IsNotExist(err) {
		return CommitResult{}, storeError(CodePermissionDenied, "cannot inspect generation directory")
	} else if err := os.Rename(staging, final); err != nil {
		return CommitResult{}, storeError(CodePermissionDenied, "cannot publish generation")
	}
	syncDir(filepath.Dir(final))

	// CAS under the lock: CURRENT is re-read here (the lock is held, so the
	// value is stable) rather than trusting a Begin-time baseline, so a
	// retried Commit after a recovery-pending outcome still sees the CURRENT
	// it switched. If CURRENT already points at this transaction's
	// generation, a previous Commit switched it and crashed before the
	// audit; resume the completion path. On a CAS conflict the already
	// published orphan Generation and its Manifest stay in place for
	// diagnosis and CURRENT keeps its value.
	cur, err := gs.readCurrent(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	selfEffective := cur != nil && cur.GenerationID == rec.GenerationID
	if !selfEffective {
		if err := gs.checkBase(rec, cur); err != nil {
			return CommitResult{}, err
		}
		// CAS: atomically update CURRENT (the unique commit point).
		curDoc := currentPointer{
			SchemaVersion:          SchemaVersion,
			GenerationID:           gen.GenerationID,
			OutputGenerationSHA256: gen.OutputGenerationSHA256,
			TransactionID:          txIDOf(tx),
			CreatedAt:              nowRFC3339(),
		}
		curPath, err := gs.currentPath()
		if err != nil {
			return CommitResult{}, err
		}
		curData, err := json.MarshalIndent(curDoc, "", "  ")
		if err != nil {
			return CommitResult{}, storeError(CodeSchemaInvalid, "cannot encode CURRENT")
		}
		if err := gs.atomicReplace(curPath, curData); err != nil {
			return CommitResult{}, err
		}
	}

	// CURRENT is effective from here on; any failure below returns the
	// committed-recovery-pending result instead of an ordinary error.
	commitRec := txCommitRecord{
		SchemaVersion:            SchemaVersion,
		TransactionID:            txIDOf(tx),
		GenerationID:             rec.GenerationID,
		BaseGeneration:           baseOrEmpty(rec.BaseGeneration),
		OutputGenerationSHA256:   gen.OutputGenerationSHA256,
		GenerationManifestSHA256: mf.InputManifestSHA256,
		CommittedAt:              nowRFC3339(),
	}
	if err := writeJSONFile(gs, filepath.Join(dir, "commit.json"), commitRec); err != nil {
		return gs.recoveryPending(tx, gen, err)
	}
	if err := gs.updateClaimStatus(ctx, rec, txCommitted); err != nil {
		return gs.recoveryPending(tx, gen, err)
	}
	return CommitResult{
		TransactionID:          txIDOf(tx),
		GenerationID:           rec.GenerationID,
		Status:                 CommitCommitted,
		BaseGeneration:         baseOrEmpty(rec.BaseGeneration),
		OutputGenerationSHA256: gen.OutputGenerationSHA256,
	}, nil
}

func txIDOf(tx *GenerationTx) string {
	if tx == nil {
		return ""
	}
	return tx.TransactionID
}

// recoveryPending builds the stable committed-but-recovery-pending result:
// CURRENT has switched (the Manifest and Generation were already published
// before the switch), so the caller must not believe nothing happened; the
// remaining audit/claim work is deterministic and Recover completes it. The
// result still carries the base and output hash of the effective generation.
func (gs *generationStore) recoveryPending(tx *GenerationTx, gen generationDoc, cause error) (CommitResult, error) {
	res := CommitResult{
		TransactionID:          txIDOf(tx),
		GenerationID:           tx.GenerationID,
		Status:                 CommitPendingRecovery,
		BaseGeneration:         baseOrEmpty(tx.BaseGeneration),
		OutputGenerationSHA256: gen.OutputGenerationSHA256,
	}
	return res, storeError(CodeGenerationRecoveryPending, "commit is effective but recovery is pending")
}

func baseOrEmpty(b *string) string {
	if b == nil {
		return ""
	}
	return *b
}

// ---- manifest input verification ----

// verifyManifestInputs enforces the strict input contract at Commit time:
// every manifest input must match a prepared fact or a committed fact on
// fact type, fact id, schema version and content hash; the fact scope must
// match the transaction scope; missing, unreferenced (extra) or inconsistent
// entries all fail closed.
func (gs *generationStore) verifyManifestInputs(ctx context.Context, rec txRecord, mf GenerationInputManifest) error {
	prepared := map[string]preparedFact{}
	for _, pf := range rec.PreparedFacts {
		key := pf.FactType + "\x00" + pf.FactID
		if _, exists := prepared[key]; !exists {
			prepared[key] = pf
		}
	}
	referenced := map[string]bool{}
	for _, in := range mf.Inputs {
		key := in.FactType + "\x00" + in.FactID
		referenced[key] = true
		if pf, ok := prepared[key]; ok {
			if err := matchManifestInput(in, pf.FactSchemaVersion, pf.Scope, pf.ContentSHA256, rec.Scope); err != nil {
				return storeError(CodeGenerationManifestMismatch, "manifest input does not match the prepared fact")
			}
			continue
		}
		// Not prepared: it must be an already-committed fact read through
		// the full verification chain.
		kind, fkey, err := resolveManifestInput(in.FactType, in.FactID)
		if err != nil {
			return storeError(CodeGenerationManifestMismatch, "manifest input does not resolve to a fact")
		}
		data, err := gs.store.Get(ctx, kind, fkey)
		if err != nil {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact is missing or unreadable")
		}
		fact, derr := decodeKind(kind, data)
		if derr != nil {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact is not decodable")
		}
		ft, fid, err := factIdentity(fact)
		if err != nil || ft != in.FactType || fid != in.FactID {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact identity mismatch")
		}
		h, herr := fact.ContentHash()
		if herr != nil || h != in.ContentSHA256 {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact hash mismatch")
		}
		if factSchemaVersion(fact) != in.FactSchemaVersion {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact schema mismatch")
		}
		if sc, ok := factScope(fact); ok && sc != rec.Scope {
			return storeError(CodeGenerationManifestMismatch, "manifest input fact scope mismatch")
		}
	}
	// Every prepared fact must be referenced by the manifest: an unreferenced
	// staged fact means the input set is incomplete.
	for _, pf := range rec.PreparedFacts {
		key := pf.FactType + "\x00" + pf.FactID
		if !referenced[key] {
			return storeError(CodeGenerationManifestMismatch, "prepared fact is not referenced by the manifest")
		}
	}
	return nil
}

// matchManifestInput compares one manifest input against a prepared fact's
// pinned identity. Scope is only compared when the fact carries one.
func matchManifestInput(in ManifestInput, schemaVersion int, scope Scope, hash string, txScope Scope) error {
	if in.FactSchemaVersion != schemaVersion {
		return storeError(CodeGenerationManifestMismatch, "manifest input schema mismatch")
	}
	if in.ContentSHA256 != hash {
		return storeError(CodeGenerationManifestMismatch, "manifest input hash mismatch")
	}
	if scope != "" && scope != txScope {
		return storeError(CodeGenerationManifestMismatch, "manifest input scope mismatch")
	}
	return nil
}

// ---- staging build ----

func (gs *generationStore) buildStaging(ctx context.Context, tx *GenerationTx, rebuild bool) (generationDoc, error) {
	dir, err := gs.txDir(ctx, tx.TransactionID)
	if err != nil {
		return generationDoc{}, err
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return generationDoc{}, err
	}
	if !generationCompilerAvailable(rec.CompilerVersion, rec.CanonicalizationVersion) {
		return generationDoc{}, storeError(CodeGenerationCompilerUnavailable, "generation compiler is not available")
	}
	mf, err := gs.readPreparedManifest(ctx, tx.TransactionID)
	if err != nil {
		return generationDoc{}, storeError(CodeGenerationManifestMismatch, "generation manifest is missing or unreadable")
	}
	staging, err := gs.stagingDir(ctx, rec.GenerationID)
	if err != nil {
		return generationDoc{}, err
	}
	// The compiled OKF views (wiki/, state/) already written into staging
	// determine the compiled output hash pinned by this document; an empty
	// view set pins the empty string.
	compiledHash, err := gs.compiledOutputHash(ctx, staging)
	if err != nil {
		return generationDoc{}, err
	}
	gen := generationDoc{
		SchemaVersion:           SchemaVersion,
		GenerationID:            rec.GenerationID,
		Scope:                   rec.Scope,
		CompilerVersion:         rec.CompilerVersion,
		CanonicalizationVersion: rec.CanonicalizationVersion,
		TransactionID:           tx.TransactionID,
		CompiledOutputSHA256:    compiledHash,
	}
	if rec.BaseGeneration != nil {
		gen.BaseGeneration = *rec.BaseGeneration
	}
	outHash, err := gen.outputHash()
	if err != nil {
		return generationDoc{}, storeError(CodeSchemaInvalid, "cannot compute generation output hash")
	}
	gen.OutputGenerationSHA256 = outHash

	// The Manifest's output_sha256 must predict the staging output hash.
	if mf.OutputSHA256 != outHash {
		return generationDoc{}, storeError(CodeGenerationStagingInvalid, "staging output hash does not match manifest")
	}

	genPath := filepath.Join(staging, "generation.json")
	if _, err := os.Stat(genPath); err == nil && !rebuild {
		// Staging already built: verify it matches what we would build,
		// including the pinned compiled output hash.
		existing, rerr := readJSONFile[generationDoc](genPath)
		if rerr != nil {
			return generationDoc{}, storeError(CodeGenerationStagingInvalid, "staging generation is unreadable")
		}
		if existing.OutputGenerationSHA256 != outHash || existing.GenerationID != gen.GenerationID ||
			existing.CompiledOutputSHA256 != compiledHash {
			return generationDoc{}, storeError(CodeGenerationStagingInvalid, "staging generation is inconsistent")
		}
		return existing, nil
	}
	if err := writeJSONFile(gs, genPath, gen); err != nil {
		return generationDoc{}, err
	}
	return gen, nil
}

// compiledOutputHash deterministically hashes the compiled OKF views already
// written into the staging directory (wiki/ and state/ subtrees, excluding
// generation.json and any transaction temp files). An empty view set hashes
// to the empty string. Symbolic links anywhere under the view subtrees are
// rejected instead of followed, so a tampered generation can never pull
// content from outside the store into the hash.
func (gs *generationStore) compiledOutputHash(ctx context.Context, staging string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", storeError(CodeLockTimeout, "staging hash cancelled")
	}
	var entries []string
	for _, sub := range []string{"wiki", "state"} {
		root := filepath.Join(staging, sub)
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return storeError(CodeOKFCompileError, "compiled output contains a symbolic link")
			}
			if info.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(staging, p)
			if rerr != nil {
				return rerr
			}
			if info.Size() > maxCompiledOutputBytes {
				return storeError(CodeOKFCompileError, "compiled output exceeds size limit")
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			entries = append(entries, filepath.ToSlash(rel)+"\n"+hashOf(data))
			return nil
		})
		if err != nil {
			return "", storeError(CodeOKFCompileError, "compiled output is not readable")
		}
	}
	if len(entries) == 0 {
		return "", nil
	}
	sort.Strings(entries)
	return hashOf([]byte(strings.Join(entries, "\n"))), nil
}

// ---- CURRENT ----

func (gs *generationStore) readCurrent(ctx context.Context) (*currentPointer, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "transaction cancelled")
	}
	path, err := gs.currentPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, storeError(CodePermissionDenied, "cannot read CURRENT")
	}
	var cur currentPointer
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cur); err != nil {
		return nil, storeError(CodeCorruptFile, "CURRENT is not valid")
	}
	if err := validateID(cur.GenerationID, "generation_id"); err != nil {
		return nil, storeError(CodeCorruptFile, "CURRENT is not valid")
	}
	return &cur, nil
}

// checkBase fails closed unless CURRENT still matches the transaction's
// expected base generation (CAS). nil and non-nil bases are strict: nil
// requires an empty CURRENT, a non-nil base requires CURRENT to carry that
// exact generation id.
func (gs *generationStore) checkBase(rec txRecord, cur *currentPointer) error {
	if rec.BaseGeneration == nil {
		if cur != nil {
			return storeError(CodeGenerationCurrentCAS, "CURRENT has advanced past the expected base")
		}
		return nil
	}
	if cur == nil || cur.GenerationID != *rec.BaseGeneration {
		return storeError(CodeGenerationCurrentCAS, "CURRENT does not match the expected base generation")
	}
	return nil
}

// ---- helpers ----

// maxPreparedFacts bounds the number of facts one transaction may stage; it
// is a safety bound, not a Schema limit.
const maxPreparedFacts = 64

// checkTxTerminal rejects operations on a transaction that already reached a
// terminal state.
func (gs *generationStore) checkTxTerminal(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "commit.json")); err == nil {
		return storeError(CodeGenerationTxConflict, "transaction is already committed")
	}
	if _, err := os.Stat(filepath.Join(dir, "abort.json")); err == nil {
		return storeError(CodeGenerationTxConflict, "transaction is aborted")
	}
	return nil
}

func (gs *generationStore) updateClaimStatus(ctx context.Context, rec txRecord, status txStatus) error {
	path, err := gs.claimPath(ctx, rec.IdempotencyKey)
	if err != nil {
		return err
	}
	claims, err := readJSONFile[idempotencyClaim](path)
	if err != nil {
		return err
	}
	claims.Status = status
	return writeJSONFile(gs, path, claims)
}

// verifyPublished re-verifies an already-published generation during a
// retried commit: the published generation document must carry the exact
// identity and hashes this transaction staged, and the compiled OKF views
// must still hash to the pinned compiled_output_sha256. Tampering with page
// content, deleting pages or replacing compiled_output_sha256 all fail
// closed.
func (gs *generationStore) verifyPublished(ctx context.Context, rec txRecord, gen generationDoc, final string) error {
	got, err := readJSONFile[generationDoc](filepath.Join(final, "generation.json"))
	if err != nil {
		return storeError(CodeGenerationStagingInvalid, "published generation is unreadable")
	}
	if got.GenerationID != gen.GenerationID || got.OutputGenerationSHA256 != gen.OutputGenerationSHA256 ||
		got.CompiledOutputSHA256 != gen.CompiledOutputSHA256 {
		return storeError(CodeGenerationStagingInvalid, "published generation does not match transaction")
	}
	return gs.verifyCompiledOutputIntegrity(ctx, final, got)
}

// verifyCompiledOutputIntegrity recomputes the compiled output hash over a
// generation directory's wiki/ and state/ subtrees and requires it to match
// the generation document's compiled_output_sha256. An empty hash is only
// consistent with an empty view set: an OKF generation always emits views, so
// an empty hash on one is tampering even if the views were deleted too, and a
// non-OKF generation (e.g. MEM-01D) with compiled views on disk is equally
// inconsistent. Callers should hold the scope lock when a decision depends on
// the result (the check itself only reads).
func (gs *generationStore) verifyCompiledOutputIntegrity(ctx context.Context, dir string, doc generationDoc) error {
	if doc.CompiledOutputSHA256 == "" {
		if doc.CompilerVersion == OKFCompilerVersion || doc.CompilerVersion == OKFCompilerVersionV1 {
			return storeError(CodeGenerationStagingInvalid, "compiled output hash is missing on an OKF generation")
		}
		emptyHash, err := gs.compiledOutputHash(ctx, dir)
		if err != nil {
			return storeError(CodeGenerationStagingInvalid, "published compiled output is not verifiable")
		}
		if emptyHash != "" {
			return storeError(CodeGenerationStagingInvalid, "compiled output hash is empty but compiled views exist")
		}
		return nil
	}
	hash, err := gs.compiledOutputHash(ctx, dir)
	if err != nil {
		return storeError(CodeGenerationStagingInvalid, "published compiled output is not verifiable")
	}
	if hash != doc.CompiledOutputSHA256 {
		return storeError(CodeGenerationStagingInvalid, "published compiled output does not match the generation document")
	}
	return nil
}

// Abort marks the transaction aborted without deleting any history. A
// rollback is a new transaction; old CURRENT, Generations, facts and
// Manifests stay untouched. The abort record and the claim update both happen
// under the scope write lock; a claim update failure is never ignored and
// returns a stable error so Recovery can diagnose the pending claim.
func (gs *generationStore) Abort(ctx context.Context, tx *GenerationTx, reason string) (err error) {
	if err := gs.checkOpen(tx); err != nil {
		return err
	}
	// The transaction normally ends here, so the scope lock is handed back
	// on every terminal outcome. The one exception: rejecting an abort of an
	// already-effective transaction keeps the lock with the transaction so a
	// retried Commit can deterministically complete the pending recovery.
	defer func() {
		if err != nil && ErrorCode(err) == CodeGenerationRecoveryPending {
			return
		}
		tx.release()
	}()
	if err := ctx.Err(); err != nil {
		return storeError(CodeLockTimeout, "transaction cancelled")
	}

	if len(reason) > maxAbortReasonLen || hasControl(reason) {
		return storeError(CodeSchemaInvalid, "invalid abort reason")
	}
	dir, err := gs.txDir(ctx, tx.TransactionID)
	if err != nil {
		return err
	}
	rec, err := readJSONFile[txRecord](filepath.Join(dir, "prepared.json"))
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "commit.json")); err == nil {
		return storeError(CodeGenerationTxConflict, "transaction is already committed")
	}
	// If CURRENT already points at this transaction's generation, a previous
	// Commit switched it and crashed before the audit: the transaction is
	// effective and must not be aborted (Recover completes it instead).
	if gs.currentIs(rec.GenerationID) {
		return storeError(CodeGenerationRecoveryPending, "transaction is already effective; abort is not allowed")
	}
	if _, err := os.Stat(filepath.Join(dir, "abort.json")); os.IsNotExist(err) {
		abortRec := txAbortRecord{
			SchemaVersion: SchemaVersion,
			TransactionID: tx.TransactionID,
			GenerationID:  rec.GenerationID,
			Reason:        reason,
			AbortedAt:     nowRFC3339(),
		}
		if err := writeJSONFile(gs, filepath.Join(dir, "abort.json"), abortRec); err != nil {
			return err
		}
	}
	// The claim must reflect the abort; a failure here is a real error, not
	// something to ignore, so Recovery can deterministically complete it.
	path, perr := gs.claimPath(ctx, rec.IdempotencyKey)
	if perr != nil {
		return storeError(CodeGenerationAbortFailed, "abort recorded but claim update failed")
	}
	claims, cerr := readJSONFile[idempotencyClaim](path)
	if cerr != nil {
		return storeError(CodeGenerationAbortFailed, "abort recorded but claim update failed")
	}
	claims.Status = txAborted
	if werr := writeJSONFile(gs, path, claims); werr != nil {
		return storeError(CodeGenerationAbortFailed, "abort recorded but claim update failed")
	}
	return nil
}

// Release hands the scope write lock back without reaching a terminal state.
// It is idempotent and safe to call after Commit/Abort.
func (gs *generationStore) Release(ctx context.Context, tx *GenerationTx) error {
	if tx == nil {
		return storeError(CodeGenerationTxConflict, "transaction handle is closed")
	}
	tx.release()
	return nil
}

const maxAbortReasonLen = 256

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' {
			return true
		}
	}
	return false
}
