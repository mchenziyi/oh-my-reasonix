package memory

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// PolicyStore is the MEM-01C policy API. It is a thin, scope-bound layer
// over the MEM-01B FactStore: no second store, hash or path-safety logic
// exists here. Versioned policy facts live at facts/policies/<policy_id>/
// <policy_version>.json, so updating a policy creates a new immutable Fact
// and never overwrites older versions. Lifecycle, Health, Usage, Index,
// Generation, Web View and evaluation results are derived state and are
// never stored as policy facts.
type PolicyStore interface {
	PutPolicy(ctx context.Context, policy PolicyFact) (WriteResult, error)
	GetPolicy(ctx context.Context, ref PolicyRef) (PolicyFact, error)
	GetPolicyVersion(ctx context.Context, policyID string, version int) (PolicyFact, error)
}

type policyStore struct {
	store *FactStore
}

// NewPolicyStore wraps an already-open FactStore. The returned PolicyStore
// inherits its scope (project/global), permissions, lock and symlink
// behavior. The caller must not share the underlying store's write path
// concurrently with other writers; the store lock serializes writes.
func NewPolicyStore(store *FactStore) PolicyStore {
	return &policyStore{store: store}
}

// PutPolicy validates the policy strictly and writes it as an immutable
// fact. Identical identity + hash is a NOOP; identical identity with a
// different hash fails closed. A new policy_version is a new Fact and never
// overwrites an older one.
func (ps *policyStore) PutPolicy(ctx context.Context, policy PolicyFact) (WriteResult, error) {
	return ps.store.Put(ctx, policy)
}

// GetPolicyVersion loads exactly the version of the policy identified by
// policy_id + version. A missing, corrupt or drifted version returns a
// stable error; it is never substituted by a "current" policy.
func (ps *policyStore) GetPolicyVersion(ctx context.Context, policyID string, version int) (PolicyFact, error) {
	if err := ctx.Err(); err != nil {
		return PolicyFact{}, storeError(CodeLockTimeout, "read cancelled")
	}
	if err := validateID(policyID, "policy_id"); err != nil {
		return PolicyFact{}, storeError(CodePathUnsafe, "invalid policy id")
	}
	if version < 1 {
		return PolicyFact{}, storeError(CodeSchemaInvalid, "policy version must be >= 1")
	}
	key := fmt.Sprintf("%s/%d", policyID, version)
	data, err := ps.store.Get(ctx, FactKindPolicy, key)
	if err != nil {
		return PolicyFact{}, err
	}
	fact, err := DecodeStrict[PolicyFact](data)
	if err != nil {
		// Map model errors onto stable store codes (hash drift, corrupt
		// JSON, unknown fields) so callers never see raw validator text.
		return PolicyFact{}, classifyDecodeError(err)
	}
	return fact, nil
}

// GetPolicy loads the exact historical fact pinned by the PolicyRef
// (policy_id + policy_type + content_sha256). It never picks the newest
// version: the ref must resolve to one unique stored fact or the call fails
// with a stable not-found error.
func (ps *policyStore) GetPolicy(ctx context.Context, ref PolicyRef) (PolicyFact, error) {
	if err := ctx.Err(); err != nil {
		return PolicyFact{}, storeError(CodeLockTimeout, "read cancelled")
	}
	if err := ref.Validate(); err != nil {
		return PolicyFact{}, storeError(CodeSchemaInvalid, "invalid policy ref")
	}
	versions, err := ps.listVersions(ctx, ref.PolicyID)
	if err != nil {
		return PolicyFact{}, err
	}
	for _, v := range versions {
		fact, err := ps.GetPolicyVersion(ctx, ref.PolicyID, v)
		if err != nil {
			// Corrupt / drifted history fails closed instead of being
			// silently skipped or substituted.
			return PolicyFact{}, err
		}
		if fact.PolicyType == ref.PolicyType && fact.ContentSHA256 == ref.ContentSHA256 {
			return fact, nil
		}
	}
	return PolicyFact{}, storeError(CodeNotFound, "no policy fact matches the ref")
}

// listVersions enumerates the stored version numbers of a policy id. It only
// reads directory entry names (never file contents) — every candidate is
// later read through the full FactStore verification chain.
func (ps *policyStore) listVersions(ctx context.Context, policyID string) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, storeError(CodeLockTimeout, "read cancelled")
	}
	if err := validateID(policyID, "policy_id"); err != nil {
		return nil, storeError(CodePathUnsafe, "invalid policy id")
	}
	// Resolve the policy directory through the same secure chain used for
	// every store path: facts/policies/<policy_id> must be a real, 0700
	// directory with no symlink in any component, staying inside the store
	// root. A missing directory means "no versions" (the caller keeps the
	// stable not-found semantics for empty histories).
	dir, err := secureJoin(ps.store.root, []string{"facts", "policies", policyID}, false, false)
	if err != nil {
		if ErrorCode(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot inspect policy directory")
	}
	var versions []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
		if err != nil || n < 1 {
			continue // not a version file; never trusted
		}
		versions = append(versions, n)
	}
	sort.Ints(versions)
	return versions, nil
}
