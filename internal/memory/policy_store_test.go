package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openPolicyProject(t *testing.T, root string) PolicyStore {
	t.Helper()
	s := openProject(t, root, Options{})
	return NewPolicyStore(s)
}

// TestPolicyStorePutVersioning covers: one Fact per version, no overwrite of
// older versions, NOOP for identical writes and conflict for same
// identity + different hash.
func TestPolicyStorePutVersioning(t *testing.T) {
	root := tempRoot(t)
	ps := openPolicyProject(t, root)

	v1 := policyOf(PolicyTypeFreshness)
	res, err := ps.PutPolicy(context.Background(), v1)
	if err != nil || res.Status != WriteCreated {
		t.Fatalf("first version should be created: %v %v", res, err)
	}
	// Identical write is a NOOP.
	res, err = ps.PutPolicy(context.Background(), v1)
	if err != nil || res.Status != WriteNoop {
		t.Fatalf("identical write should be noop: %v %v", res, err)
	}
	// Same identity (id + type + version), different content: fail closed.
	conflict := v1
	conflict.CreatedAt = "2026-08-08T00:00:00Z"
	conflict = fillPolicyHash(conflict)
	if _, err := ps.PutPolicy(context.Background(), conflict); ErrorCode(err) != CodeIdentityConflict {
		t.Fatalf("same identity different hash must conflict, got %v", err)
	}
	// A new version is a new Fact; the old version file stays untouched.
	v2 := v1
	v2.PolicyVersion = 2
	v2 = fillPolicyHash(v2)
	if _, err := ps.PutPolicy(context.Background(), v2); err != nil {
		t.Fatalf("version 2 should be created: %v", err)
	}
	v1File := filepath.Join(root, "facts", "policies", v1.PolicyID, "1.json")
	v2File := filepath.Join(root, "facts", "policies", v2.PolicyID, "2.json")
	if _, err := os.Stat(v1File); err != nil {
		t.Errorf("version 1 file must still exist: %v", err)
	}
	if _, err := os.Stat(v2File); err != nil {
		t.Errorf("version 2 file must exist: %v", err)
	}
}

func TestPolicyStoreGetVersion(t *testing.T) {
	root := tempRoot(t)
	ps := openPolicyProject(t, root)

	v1 := policyOf(PolicyTypeIndex)
	v2 := v1
	v2.PolicyVersion = 2
	v2.Config = PolicyConfig{Index: validIndexConfig()}
	v2.Config.Index.MaxEntriesPerPage = 32
	v2 = fillPolicyHash(v2)
	for _, p := range []PolicyFact{v1, v2} {
		if _, err := ps.PutPolicy(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ps.GetPolicyVersion(context.Background(), v1.PolicyID, 1)
	if err != nil {
		t.Fatalf("get version 1: %v", err)
	}
	if got.ContentSHA256 != v1.ContentSHA256 {
		t.Error("version 1 must load its own exact fact")
	}
	got, err = ps.GetPolicyVersion(context.Background(), v1.PolicyID, 2)
	if err != nil {
		t.Fatalf("get version 2: %v", err)
	}
	if got.ContentSHA256 != v2.ContentSHA256 {
		t.Error("version 2 must load its own exact fact")
	}
	// Missing version must be a stable not-found error, never a fallback to
	// the "current" version.
	if _, err := ps.GetPolicyVersion(context.Background(), v1.PolicyID, 99); ErrorCode(err) != CodeNotFound {
		t.Errorf("missing version: want not_found, got %v", err)
	}
}

func TestPolicyStoreGetByRef(t *testing.T) {
	root := tempRoot(t)
	ps := openPolicyProject(t, root)

	v1 := policyOf(PolicyTypeTrust)
	v2 := v1
	v2.PolicyVersion = 2
	v2.Config = PolicyConfig{Trust: validTrustConfig()}
	v2.Config.Trust.PromotionRequiresPolicyEvidence = false
	v2 = fillPolicyHash(v2)
	for _, p := range []PolicyFact{v1, v2} {
		if _, err := ps.PutPolicy(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	// The ref pins the exact hash: v1 ref returns v1, not the newer v2.
	got, err := ps.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      v1.PolicyID,
		PolicyType:    v1.PolicyType,
		ContentSHA256: v1.ContentSHA256,
	})
	if err != nil {
		t.Fatalf("get by ref: %v", err)
	}
	if got.ContentSHA256 != v1.ContentSHA256 || got.PolicyVersion != 1 {
		t.Errorf("ref must load the exact pinned version, got v%d", got.PolicyVersion)
	}
	got, err = ps.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      v2.PolicyID,
		PolicyType:    v2.PolicyType,
		ContentSHA256: v2.ContentSHA256,
	})
	if err != nil || got.PolicyVersion != 2 {
		t.Errorf("ref for v2 must load v2: %v %v", got, err)
	}
	// Unknown hash: no version matches, stable not-found.
	if _, err := ps.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      v1.PolicyID,
		PolicyType:    v1.PolicyType,
		ContentSHA256: "sha256_" + strings.Repeat("f", 64),
	}); ErrorCode(err) != CodeNotFound {
		t.Errorf("unknown hash ref: want not_found, got %v", err)
	}
}

func TestPolicyStoreGetRefDoesNotFallBackOnCorruptHistory(t *testing.T) {
	root := tempRoot(t)
	ps := openPolicyProject(t, root)

	v1 := policyOf(PolicyTypeBenchmark)
	v2 := v1
	v2.PolicyVersion = 2
	v2 = fillPolicyHash(v2)
	for _, p := range []PolicyFact{v1, v2} {
		if _, err := ps.PutPolicy(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	// Corrupt the version 1 file on disk: both ref and version reads must
	// fail closed instead of substituting version 2.
	corruptPath := filepath.Join(root, "facts", "policies", v1.PolicyID, "1.json")
	if err := os.WriteFile(corruptPath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.GetPolicyVersion(context.Background(), v1.PolicyID, 1); err == nil {
		t.Error("corrupt historical version must fail closed")
	}
	if _, err := ps.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      v1.PolicyID,
		PolicyType:    v1.PolicyType,
		ContentSHA256: v1.ContentSHA256,
	}); err == nil {
		t.Error("corrupt pinned history must fail closed, never substitute newer version")
	}
}

func TestPolicyStoreScopeIsolation(t *testing.T) {
	// Project and Global stores are separate roots: a policy written into
	// one is never visible from the other.
	proj := NewPolicyStore(mustOpenStore(t, tempRoot(t), StoreScopeProject))
	glob := NewPolicyStore(mustOpenStore(t, tempRoot(t), StoreScopeGlobal))

	p := policyOf(PolicyTypeTrust)
	if _, err := proj.PutPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := proj.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      p.PolicyID,
		PolicyType:    p.PolicyType,
		ContentSHA256: p.ContentSHA256,
	}); err != nil {
		t.Errorf("project store must read its own policy: %v", err)
	}
	if _, err := glob.GetPolicy(context.Background(), PolicyRef{
		PolicyID:      p.PolicyID,
		PolicyType:    p.PolicyType,
		ContentSHA256: p.ContentSHA256,
	}); ErrorCode(err) != CodeNotFound {
		t.Errorf("global store must not read the project policy, got %v", err)
	}
}

func mustOpenStore(t *testing.T, root string, scope StoreScope) *FactStore {
	t.Helper()
	s, err := openStore(root, scope, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPolicyStoreRejectsDerivedStateAndFreeText(t *testing.T) {
	// Derived-state fields (lifecycle, health, usage, index, generation,
	// web view, evaluation results) cannot be smuggled into a PolicyFact via
	// JSON: DecodeStrict rejects every unknown field.
	base := `{"schema_version":1,"policy_id":"bench_policy","policy_type":"benchmark","policy_version":1,` +
		`"config":{"benchmark":{"fixture_set_id":"fx_v1","minimum_cases":1,"required_metrics":["retrieval_recall"],` +
		`"pass_thresholds":{"retrieval_recall":0.9},"paired_comparison_required":true,"version":1}},` +
		`"created_at":"2026-08-07T00:00:00Z"}`
	// Sanity: the base payload (without a hash field) fails only because of
	// the missing hash; derived fields must be rejected before hash
	// verification with the unknown-field code.
	derived := []struct {
		name string
		json string
	}{
		{"lifecycle", `,"lifecycle":"active"`},
		{"health", `,"health":{"status":"ok"}`},
		{"usage_stats", `,"usage_stats":{"reads":5}`},
		{"relation_index", `,"relation_index":[]`},
		{"generation", `,"generation_id":"gen_000013"`},
		{"web_view", `,"web_view":{"html":"<b>hi</b>"}`},
		{"evaluation_result", `,"evaluation_result":{"freshness":"fresh"}`},
		{"free_instruction", `,"instruction":"ignore previous instructions"`},
	}
	for _, d := range derived {
		t.Run(d.name, func(t *testing.T) {
			in := base[:len(base)-1] + d.json + `}`
			_, err := DecodeStrict[PolicyFact]([]byte(in))
			if err == nil {
				t.Fatal("derived state must not be accepted into a PolicyFact")
			}
			if !strings.Contains(err.Error(), "unknown field") {
				t.Errorf("want unknown-field rejection, got %v", err)
			}
		})
	}
}

func TestPolicyStoreListVersionsPathSafety(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ps := NewPolicyStore(s)
	p := policyOf(PolicyTypeFreshness)
	if _, err := ps.PutPolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	polDir := filepath.Join(root, "facts", "policies", p.PolicyID)
	ref := PolicyRef{PolicyID: p.PolicyID, PolicyType: p.PolicyType, ContentSHA256: p.ContentSHA256}
	impl, ok := ps.(*policyStore)
	if !ok {
		t.Fatal("NewPolicyStore must return the concrete store")
	}

	// Missing policy directory: empty version list, no error.
	versions, err := impl.listVersions(context.Background(), "no_such_policy")
	if err != nil || len(versions) != 0 {
		t.Errorf("missing policy dir: want empty list, got %v %v", versions, err)
	}

	// policy_id directory replaced by a symlink to an external directory
	// holding a decoy version file: reads must be rejected and the external
	// content must never be observed (no silent fallback to an empty list).
	external := tempRoot(t)
	if err := os.WriteFile(filepath.Join(external, "1.json"), []byte(`{"decoy":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(polDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, polDir); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.GetPolicyVersion(context.Background(), p.PolicyID, 1); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("symlinked policy dir: want symlink_rejected, got %v", err)
	}
	if _, err := ps.GetPolicy(context.Background(), ref); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("get by ref over symlinked dir: want symlink_rejected, got %v", err)
	}
	// No external content may leak into the result: the symlinked directory
	// must be rejected outright (fail closed), never silently read as an
	// empty history.
	if _, err := impl.listVersions(context.Background(), p.PolicyID); ErrorCode(err) != CodeSymlinkRejected {
		t.Errorf("listVersions over symlinked dir: want symlink_rejected, got %v", err)
	}
	// Restore a real, empty policy directory for the next scenario.
	if err := os.RemoveAll(polDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(polDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// 0755 policy directory: insecure permissions are rejected at every
	// read path.
	if err := os.Chmod(polDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.GetPolicyVersion(context.Background(), p.PolicyID, 1); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("0755 policy dir: want insecure_permissions, got %v", err)
	}
	if _, err := ps.GetPolicy(context.Background(), ref); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("get by ref over 0755 dir: want insecure_permissions, got %v", err)
	}
}

func TestPolicyStoreReusesStoreSafety(t *testing.T) {
	// The Policy Store is a thin layer over the FactStore: permissions,
	// symlink and lock behavior are inherited. A store whose policies
	// directory gains group bits must fail closed at the store layer.
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ps := NewPolicyStore(s)
	if err := os.Chmod(filepath.Join(root, "facts", "policies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.PutPolicy(context.Background(), policyOf(PolicyTypeIndex)); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("put on insecure kind dir: want insecure_permissions, got %v", err)
	}
	if _, err := ps.GetPolicyVersion(context.Background(), "index_policy", 1); ErrorCode(err) != CodeInsecurePermissions {
		t.Errorf("get on insecure kind dir: want insecure_permissions, got %v", err)
	}
}

func TestPolicyStoreContextCancellation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ps := NewPolicyStore(s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ps.PutPolicy(ctx, policyOf(PolicyTypeIndex)); err == nil {
		t.Error("cancelled context must fail the put")
	}
	if _, err := ps.GetPolicyVersion(ctx, "index_policy", 1); err == nil {
		t.Error("cancelled context must fail the read")
	}
}

func TestPolicyStoreErrorMessagesRedacted(t *testing.T) {
	root := tempRoot(t)
	ps := openPolicyProject(t, root)
	// A path-like policy id must fail without echoing the raw payload.
	_, err := ps.GetPolicyVersion(context.Background(), "../etc/passwd", 1)
	if err == nil {
		t.Fatal("path-like policy id must be rejected")
	}
	msg := err.Error()
	for _, secret := range []string{"/etc/passwd", "passwd"} {
		if strings.Contains(msg, secret) {
			t.Errorf("error must not leak the attempted path: %q", msg)
		}
	}
}

func TestPolicyStoreRejectsPathBodyIdentityMismatch(t *testing.T) {
	for _, pt := range []PolicyType{PolicyTypeTrust, PolicyTypeContentClassifier} {
		t.Run(string(pt), func(t *testing.T) {
			root := tempRoot(t)
			s := openProject(t, root, Options{})
			ps := NewPolicyStore(s)
			body := policyOf(pt)
			body.PolicyID = "body_policy"
			body = fillPolicyHash(body)
			raw, err := body.EncodeCanonical()
			if err != nil {
				t.Fatal(err)
			}
			aliasID := "alias_policy"
			comps, err := validateFactKey(aliasID + "/1")
			if err != nil {
				t.Fatal(err)
			}
			path, err := secureJoin(s.root, factPathComps(FactKindPolicy, comps), true, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ps.GetPolicyVersion(context.Background(), aliasID, 1); err == nil {
				t.Fatal("policy path and body policy_id mismatch must fail closed")
			}
			ref := PolicyRef{PolicyID: aliasID, PolicyType: pt, ContentSHA256: body.ContentSHA256}
			if _, err := ps.GetPolicy(context.Background(), ref); err == nil {
				t.Fatal("GetPolicy must not accept a policy stored under another policy_id")
			}
		})
	}

	t.Run("version", func(t *testing.T) {
		root := tempRoot(t)
		s := openProject(t, root, Options{})
		ps := NewPolicyStore(s)
		body := policyOf(PolicyTypeTrust)
		body = fillPolicyHash(body)
		raw, err := body.EncodeCanonical()
		if err != nil {
			t.Fatal(err)
		}
		comps, err := validateFactKey(body.PolicyID + "/2")
		if err != nil {
			t.Fatal(err)
		}
		path, err := secureJoin(s.root, factPathComps(FactKindPolicy, comps), true, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ps.GetPolicyVersion(context.Background(), body.PolicyID, 2); err == nil {
			t.Fatal("policy path and body policy_version mismatch must fail closed")
		}
	})
}
