package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// FactKind routes a fact type to its facts/ subdirectory.
type FactKind string

const (
	FactKindMemoryRevision           FactKind = "memory-revisions"
	FactKindMemoryEvidenceGeneration FactKind = "memory-evidence-generations"
	FactKindJudgment                 FactKind = "judgments"
	FactKindPolicy                   FactKind = "policies"
	FactKindGovernanceEvent          FactKind = "governance-events"
	FactKindGenerationInputManifest  FactKind = "generation-input-manifests"
	FactKindMemoryUsage              FactKind = "memory-usages"
	FactKindOutcome                  FactKind = "outcomes"
)

var allKinds = []FactKind{
	FactKindMemoryRevision,
	FactKindMemoryEvidenceGeneration,
	FactKindJudgment,
	FactKindPolicy,
	FactKindGovernanceEvent,
	FactKindGenerationInputManifest,
	FactKindMemoryUsage,
	FactKindOutcome,
}

func (k FactKind) valid() bool {
	for _, kk := range allKinds {
		if k == kk {
			return true
		}
	}
	return false
}

// StoreScope distinguishes the two concrete stores. Portable exists only as
// a Schema value and is never mapped onto a store.
type StoreScope int

const (
	StoreScopeProject StoreScope = iota
	StoreScopeGlobal
)

func (ss StoreScope) String() string {
	switch ss {
	case StoreScopeProject:
		return "project"
	case StoreScopeGlobal:
		return "global"
	default:
		return "unknown"
	}
}

// Options configures a FactStore. Zero values select the documented
// defaults; the limits are safety bounds, not Schema semantics.
type Options struct {
	// MaxFactBytes limits one fact JSON file (default DefaultMaxFactBytes).
	MaxFactBytes int64
	// LockTimeout bounds the write-lock wait (default DefaultLockTimeout).
	LockTimeout time.Duration
}

// WriteStatus reports what Put did.
type WriteStatus int

const (
	// WriteCreated: the immutable fact was atomically created.
	WriteCreated WriteStatus = iota
	// WriteNoop: an identical fact already exists; nothing was written.
	WriteNoop
)

func (st WriteStatus) String() string {
	switch st {
	case WriteCreated:
		return "created"
	case WriteNoop:
		return "noop"
	default:
		return "unknown"
	}
}

// WriteResult is the outcome of a Put.
type WriteResult struct {
	Status WriteStatus
	Key    string // redacted identity key
	Hash   string // content hash of the stored fact
}

// FactStore is a scope-bound, single-writer immutable fact store.
type FactStore struct {
	storeScope  StoreScope
	root        string // normalized absolute root
	factsDir    string
	locksDir    string
	lockPath    string
	diagDir     string
	maxBytes    int64
	lockTimeout time.Duration
	local       chan struct{} // in-process write guard
}

// OpenProject opens (or initializes) a project-scope store at root.
func OpenProject(root string, opts Options) (*FactStore, error) {
	return openStore(root, StoreScopeProject, opts)
}

// OpenGlobal opens (or initializes) a global-scope store at root. Global
// and Project stores never share files; there is no auto-discovery of a
// global path in this phase.
func OpenGlobal(root string, opts Options) (*FactStore, error) {
	return openStore(root, StoreScopeGlobal, opts)
}

func openStore(root string, scope StoreScope, opts Options) (*FactStore, error) {
	normRoot, err := secureRoot(root)
	if err != nil {
		return nil, err
	}
	s := &FactStore{
		storeScope:  scope,
		root:        normRoot,
		maxBytes:    opts.MaxFactBytes,
		lockTimeout: opts.LockTimeout,
		local:       make(chan struct{}, 1),
	}
	if s.maxBytes <= 0 {
		s.maxBytes = DefaultMaxFactBytes
	}
	if s.lockTimeout <= 0 {
		s.lockTimeout = DefaultLockTimeout
	}
	s.factsDir = filepath.Join(s.root, "facts")
	s.locksDir = filepath.Join(s.root, "locks")
	s.lockPath = filepath.Join(s.locksDir, "store.lock")
	s.diagDir = filepath.Join(s.root, "diagnostics")

	// Fail closed on pre-existing directories with group/other bits before
	// creating anything; missing directories are created 0700 below.
	if err := s.verifyDirPermissions(); err != nil {
		return nil, err
	}
	for _, dir := range [][]string{{"facts"}, {"locks"}, {"diagnostics"}} {
		if err := s.ensureDir(dir); err != nil {
			return nil, err
		}
	}
	for _, kind := range allKinds {
		if err := s.ensureDir([]string{"facts", string(kind)}); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// verifyDirPermissions checks every already-existing top-level Store
// directory — root, facts, locks, diagnostics and each facts/<kind>
// directory — for the 0700 owner-exclusive permission. Any group/other bit
// fails closed with a stable insecure_permissions error; directories are
// never silently chmod'ed. (secureRoot enforces the same rule on the root,
// and secureJoin re-checks every directory component on each operation, so
// deeper facts/<kind>/<dirs>/ directories are verified when they are
// touched.)
func (s *FactStore) verifyDirPermissions() error {
	check := func(dir string) error {
		fi, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return storeError(CodePermissionDenied, "cannot inspect store directory")
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return storeError(CodeSymlinkRejected, "store directory is a symlink")
		}
		if !fi.IsDir() {
			return storeError(CodePathUnsafe, "store directory is not a directory")
		}
		if fi.Mode().Perm()&0o077 != 0 {
			return storeError(CodeInsecurePermissions, "store directory permissions too permissive")
		}
		return nil
	}
	for _, d := range []string{s.root, s.factsDir, s.locksDir, s.diagDir} {
		if err := check(d); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(s.factsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return storeError(CodePermissionDenied, "cannot inspect facts directory")
	}
	for _, e := range entries {
		if err := check(filepath.Join(s.factsDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s *FactStore) ensureDir(rel []string) error {
	_, err := secureJoin(s.root, rel, true, false)
	return err
}

// Put validates the fact strictly (type, scope, identity, content hash),
// canonicalizes it, takes the store write lock and atomically creates the
// immutable fact file. Identical identity+hash is a NOOP; identical identity
// with a different hash fails closed with zero writes.
func (s *FactStore) Put(ctx context.Context, fact Fact) (WriteResult, error) {
	unlock, err := s.acquireWriteLock(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	defer unlock()
	return s.putLocked(ctx, fact)
}

// putLocked is Put with the assumption that the caller already holds the
// store write lock. It is used by the generation transaction layer, which
// holds the scope lock for the whole transaction lifetime and must never
// re-acquire it (that would deadlock on the in-process guard).
func (s *FactStore) putLocked(ctx context.Context, fact Fact) (WriteResult, error) {
	if err := ctx.Err(); err != nil {
		return WriteResult{}, storeError(CodeLockTimeout, "write cancelled")
	}
	if err := fact.Validate(); err != nil {
		return WriteResult{}, classifyValidateError(err)
	}
	kind, key, err := factKey(fact)
	if err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "fact type is not storable")
	}
	comps, err := validateFactKey(key)
	if err != nil {
		return WriteResult{}, err
	}
	if sc, ok := factScope(fact); ok {
		if !s.scopeMatches(sc) {
			return WriteResult{}, storeError(CodeScopeMismatch, "fact scope does not match store scope")
		}
	}
	contentHash, err := fact.ContentHash()
	if err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "fact hash cannot be computed")
	}
	canon, err := fact.EncodeCanonical()
	if err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "fact cannot be canonicalized")
	}
	if int64(len(canon)) > s.maxBytes {
		// The fact itself violates the store's trusted-size boundary; it is
		// refused, not truncated.
		return WriteResult{}, storeError(CodeSchemaInvalid, "fact exceeds size limit")
	}

	path, err := secureJoin(s.root, factPathComps(kind, comps), true, true)
	if err != nil {
		return WriteResult{}, err
	}
	if _, err := os.Lstat(path); err == nil {
		return s.handleExistingTarget(ctx, kind, comps, key, contentHash)
	} else if !os.IsNotExist(err) {
		return WriteResult{}, storeError(CodePermissionDenied, "cannot inspect fact path")
	}

	// No-overwrite atomic commit: the fact is fully written and fsynced to a
	// same-directory temp file, then published with hard-link(2). link fails
	// with EEXIST (atomically, without touching the target) if another
	// process created the target after our Lstat check, so an existing
	// immutable fact can never be overwritten. The temp file is removed on
	// every path; residuals are reported by Diagnose.
	if err := s.atomicWriteFile(path, canon); err != nil {
		if err == errTargetExists {
			// Lost the race: a target appeared between our Lstat check and
			// the commit. Read and strictly verify the existing fact; zero
			// writes happened and the target is untouched.
			return s.handleExistingTarget(ctx, kind, comps, key, contentHash)
		}
		return WriteResult{}, err
	}
	// Post-commit verification: a same-uid attacker could swap an
	// intermediate directory for a symlink between our component checks and
	// the commit, redirecting the write outside the store. Re-running the
	// full component walk and re-verifying the committed file makes such a
	// redirect fail loudly. (Fully closing this window requires openat-style
	// descriptor anchoring, which the standard library does not expose on
	// darwin; see the delivery report.)
	if err := s.verifyCommitted(kind, comps, canon); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Status: WriteCreated, Key: key, Hash: contentHash}, nil
}

// handleExistingTarget verifies a fact that already occupies the identity:
// identical content is a NOOP, different content is a stable conflict, and
// a corrupt existing file fails closed. The existing file is never
// overwritten, truncated or deleted.
func (s *FactStore) handleExistingTarget(ctx context.Context, kind FactKind, comps []string, key, contentHash string) (WriteResult, error) {
	_, existing, err := s.readVerified(ctx, kind, comps)
	if err != nil {
		return WriteResult{}, err
	}
	existingHash, err := existing.ContentHash()
	if err != nil {
		return WriteResult{}, storeError(CodeSchemaInvalid, "existing fact hash cannot be computed")
	}
	if existingHash == contentHash {
		return WriteResult{Status: WriteNoop, Key: key, Hash: existingHash}, nil
	}
	return WriteResult{}, storeError(CodeIdentityConflict, "same identity with different content hash")
}

// verifyCommitted re-checks the write path and the committed file after an
// atomic write, so a race-time symlink redirect is detected instead of
// silently accepted.
func (s *FactStore) verifyCommitted(kind FactKind, comps []string, want []byte) error {
	verified, err := secureJoin(s.root, factPathComps(kind, comps), false, true)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(verified, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return storeError(CodePermissionDenied, "cannot verify committed fact")
	}
	defer f.Close()
	got, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		return storeError(CodePermissionDenied, "cannot verify committed fact")
	}
	if string(got) != string(want) {
		return storeError(CodeHashMismatch, "committed fact does not match written content")
	}
	return nil
}

// Get reads one immutable fact through the full verification chain: safe
// path resolution, type and permission checks, size limit, strict JSON
// decode, Validate and a byte-level canonical round-trip (which re-verifies
// the content hash). The returned bytes are the exact stored document.
func (s *FactStore) Get(ctx context.Context, kind FactKind, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "read cancelled")
	}
	if !kind.valid() {
		return nil, storeError(CodePathUnsafe, "unknown fact kind")
	}
	comps, err := validateFactKey(key)
	if err != nil {
		return nil, err
	}
	data, _, err := s.readVerified(ctx, kind, comps)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Exists reports whether an immutable fact occupies the identity, applying
// the same path-safety checks as Get (but not the content verification).
func (s *FactStore) Exists(ctx context.Context, kind FactKind, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, storeError(CodeLockTimeout, "read cancelled")
	}
	if !kind.valid() {
		return false, storeError(CodePathUnsafe, "unknown fact kind")
	}
	comps, err := validateFactKey(key)
	if err != nil {
		return false, err
	}
	_, err = secureJoin(s.root, factPathComps(kind, comps), false, true)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// List enumerates the stable identity keys of one fact kind, for derived
// layers that must reduce over the whole fact set (e.g. MEM-01F). The store
// is scope-bound, so a Project store only ever lists project facts. The walk
// rejects symlinks anywhere under the kind directory (never follows them),
// and every listed key is re-validated with the same identifier rules used
// by Put/Get, so a tampered directory can only surface as an error, never as
// a path escape or a plausible key. A missing kind directory is treated as
// an empty list (no facts of that kind exist yet). Keys are sorted for
// determinism.
func (s *FactStore) List(ctx context.Context, kind FactKind) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "read cancelled")
	}
	if !kind.valid() {
		return nil, storeError(CodePathUnsafe, "unknown fact kind")
	}
	base, err := secureJoin(s.root, []string{"facts", string(kind)}, false, false)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	err = filepath.Walk(base, func(p string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return storeError(CodeSymlinkRejected, "fact kind directory contains a symbolic link")
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".json") {
			// Non-fact files (temp files, stray documents) are ignored; they
			// never become plausible fact identities.
			return nil
		}
		rel, rerr := filepath.Rel(base, p)
		if rerr != nil {
			return rerr
		}
		key := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		if _, kerr := validateFactKey(key); kerr != nil {
			return storeError(CodePathUnsafe, "fact kind directory contains an unsafe entry")
		}
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// readVerified implements the fixed read chain. It returns the raw stored
// bytes and the decoded fact.
func (s *FactStore) readVerified(ctx context.Context, kind FactKind, comps []string) ([]byte, Fact, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, storeError(CodeLockTimeout, "read cancelled")
	}
	path, err := secureJoin(s.root, factPathComps(kind, comps), false, true)
	if err != nil {
		return nil, nil, err
	}
	// O_NOFOLLOW defends the read against a symlink planted after the
	// component checks.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, storeError(CodeNotFound, "fact not found")
		}
		return nil, nil, storeError(CodePermissionDenied, "cannot open fact")
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, storeError(CodePermissionDenied, "cannot stat fact")
	}
	if fi.Size() > s.maxBytes {
		return nil, nil, storeError(CodeCorruptFile, "fact exceeds size limit")
	}
	if fi.Size() == 0 {
		return nil, nil, storeError(CodeInvalidJSON, "fact file is empty")
	}
	data, err := io.ReadAll(io.LimitReader(f, s.maxBytes+1))
	if err != nil {
		return nil, nil, storeError(CodePermissionDenied, "cannot read fact")
	}
	if int64(len(data)) > s.maxBytes {
		return nil, nil, storeError(CodeCorruptFile, "fact exceeds size limit")
	}
	fact, err := decodeKind(kind, data)
	if err != nil {
		return nil, nil, classifyDecodeError(err)
	}
	canon, err := fact.EncodeCanonical()
	if err != nil {
		return nil, nil, storeError(CodeSchemaInvalid, "fact cannot be canonicalized")
	}
	// Byte-level drift check. For hash-carrying facts DecodeStrict already
	// re-verified the content hash; this comparison also covers facts
	// without a stored hash field (governance events).
	if string(canon) != string(data) {
		return nil, nil, storeError(CodeHashMismatch, "fact content drift detected")
	}
	return data, fact, nil
}

// errTargetExists signals that the no-overwrite commit lost the race and the
// target was created concurrently; the caller must verify the existing fact.
var errTargetExists = errors.New("target already exists")

// atomicWriteFile performs a no-overwrite atomic commit: the data is fully
// written and fsynced to a same-directory temp file (0600), then published
// with hard-link(2). link(2) atomically fails with EEXIST when the target
// exists, without touching it, so an existing immutable fact can never be
// overwritten. The temp file is removed on every path; a crash between link
// and unlink leaves a residual that Diagnose reports.
func (s *FactStore) atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tempPrefix+"*.tmp")
	if err != nil {
		return storeError(CodePermissionDenied, "cannot create temporary file")
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot set file permissions")
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot write fact")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return storeError(CodePermissionDenied, "cannot sync fact")
	}
	if err := tmp.Close(); err != nil {
		return storeError(CodePermissionDenied, "cannot close fact")
	}
	if err := os.Link(tmpName, path); err != nil {
		if os.IsExist(err) {
			return errTargetExists
		}
		return storeError(CodePermissionDenied, "cannot commit fact")
	}
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}

// factPathComps renders the on-disk path components: facts/<kind>/<dirs...>/<file>.json.
func factPathComps(kind FactKind, comps []string) []string {
	dirs := append([]string{"facts", string(kind)}, comps[:len(comps)-1]...)
	return append(dirs, comps[len(comps)-1]+".json")
}

// factKey derives the stable identity key of a fact. Policies are keyed by
// policy_id + policy_version (MEM-01C): a policy update creates a new
// immutable version fact instead of overwriting the previous one.
func factKey(f Fact) (FactKind, string, error) {
	switch v := f.(type) {
	case MemoryRevision:
		return FactKindMemoryRevision, fmt.Sprintf("%s/%d", v.MemoryID, v.Revision), nil
	case MemoryEvidenceGeneration:
		return FactKindMemoryEvidenceGeneration, fmt.Sprintf("%s/%d/%d", v.MemoryID, v.Revision, v.EvidenceGeneration), nil
	case JudgmentFact:
		return FactKindJudgment, v.JudgmentID, nil
	case PolicyFact:
		return FactKindPolicy, fmt.Sprintf("%s/%d", v.PolicyID, v.PolicyVersion), nil
	case GovernanceEvent:
		return FactKindGovernanceEvent, v.EventID, nil
	case GenerationInputManifest:
		return FactKindGenerationInputManifest, v.GenerationID, nil
	case MemoryUsage:
		return FactKindMemoryUsage, v.UsageID, nil
	case Outcome:
		return FactKindOutcome, v.OutcomeID, nil
	default:
		return "", "", fmt.Errorf("unsupported fact type %T", f)
	}
}

// factScope returns the schema scope of a fact. Facts without a scope field
// (evidence generations, policies) are bound by the store they land in.
func factScope(f Fact) (Scope, bool) {
	switch v := f.(type) {
	case MemoryRevision:
		return v.Scope, true
	case JudgmentFact:
		return v.Scope, true
	case GovernanceEvent:
		return v.Scope, true
	case GenerationInputManifest:
		return v.Scope, true
	case MemoryUsage:
		return v.Scope, true
	case Outcome:
		return v.Scope, true
	default:
		return "", false
	}
}

func (s *FactStore) scopeMatches(sc Scope) bool {
	switch s.storeScope {
	case StoreScopeProject:
		return sc == ScopeProject
	case StoreScopeGlobal:
		return sc == ScopeGlobal
	default:
		return false
	}
}

func classifyValidateError(err error) error {
	if err == nil {
		return nil
	}
	if isHashMismatchMsg(err.Error()) {
		return storeError(CodeHashMismatch, "fact content hash mismatch")
	}
	return storeError(CodeSchemaInvalid, "fact violates the schema")
}

func decodeKind(kind FactKind, data []byte) (Fact, error) {
	switch kind {
	case FactKindMemoryRevision:
		return DecodeStrict[MemoryRevision](data)
	case FactKindMemoryEvidenceGeneration:
		return DecodeStrict[MemoryEvidenceGeneration](data)
	case FactKindJudgment:
		return DecodeStrict[JudgmentFact](data)
	case FactKindPolicy:
		return DecodeStrict[PolicyFact](data)
	case FactKindGovernanceEvent:
		return DecodeStrict[GovernanceEvent](data)
	case FactKindGenerationInputManifest:
		return DecodeStrict[GenerationInputManifest](data)
	case FactKindMemoryUsage:
		return DecodeStrict[MemoryUsage](data)
	case FactKindOutcome:
		return DecodeStrict[Outcome](data)
	default:
		return nil, storeError(CodePathUnsafe, "unknown fact kind")
	}
}

// Diagnostic describes a store state a maintainer should look at. The store
// never repairs anything; Doctor and Repair belong to later phases.
type Diagnostic struct {
	Code   Code   `json:"code"`
	Detail string `json:"detail"` // redacted
}

// Diagnose reports residual temp files and the live lock state without
// changing anything. The lock file itself normally persists after use, so
// only a currently held lock (flock probe) is reported; stale lock files are
// for Doctor to judge, not the store.
func (s *FactStore) Diagnose(ctx context.Context) ([]Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "diagnose cancelled")
	}
	var diags []Diagnostic
	seen := map[string]bool{}
	err := filepath.Walk(s.factsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // unreadable subtrees are not repair targets here
		}
		if !info.IsDir() && strings.Contains(info.Name(), tempPrefix) {
			rel, _ := filepath.Rel(s.factsDir, p)
			key := "facts/" + filepath.ToSlash(rel)
			if !seen[key] {
				seen[key] = true
				diags = append(diags, Diagnostic{Code: CodeCorruptFile, Detail: "residual temporary file: " + key})
			}
		}
		return nil
	})
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot walk facts directory")
	}
	if info, err := s.LockInfo(ctx); err == nil && info.Locked {
		diags = append(diags, Diagnostic{Code: CodeLockTimeout, Detail: "write lock is currently held"})
	}
	return diags, nil
}
