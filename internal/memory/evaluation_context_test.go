package memory

// MEM-02-01 failure-first tests: EvaluationContext / ProjectGenerationRef /
// GlobalGenerationRef and the build/rebuild paths. Written before the
// implementation existed; the package did not compile until
// evaluation_context.go landed.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// ---- refs ----

func TestProjectGenerationRefValidation(t *testing.T) {
	valid := ProjectGenerationRef{
		SchemaVersion:       1,
		Scope:               ScopeProject,
		GenerationID:        "gen_project_000001",
		InputManifestID:     "gen_project_000001",
		InputManifestSHA256: testHash,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid project generation ref rejected: %v", err)
	}
	// Wrong scope for the ref type must be rejected.
	badScope := valid
	badScope.Scope = ScopeGlobal
	if err := badScope.Validate(); err == nil {
		t.Error("project generation ref with global scope must be rejected")
	}
	// Missing / malformed fields must be rejected.
	cases := []ProjectGenerationRef{
		{SchemaVersion: 0, Scope: ScopeProject, GenerationID: "gen_1", InputManifestID: "gen_1", InputManifestSHA256: testHash},
		{SchemaVersion: 1, Scope: ScopeProject, GenerationID: "", InputManifestID: "gen_1", InputManifestSHA256: testHash},
		{SchemaVersion: 1, Scope: ScopeProject, GenerationID: "gen_1", InputManifestID: "../evil", InputManifestSHA256: testHash},
		{SchemaVersion: 1, Scope: ScopeProject, GenerationID: "gen_1", InputManifestID: "gen_1", InputManifestSHA256: "sha256_bad"},
		{SchemaVersion: 1, Scope: ScopePortable, GenerationID: "gen_1", InputManifestID: "gen_1", InputManifestSHA256: testHash},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d must be rejected", i)
		}
	}
}

func TestGlobalGenerationRefValidation(t *testing.T) {
	valid := GlobalGenerationRef{
		SchemaVersion:       1,
		Scope:               ScopeGlobal,
		GenerationID:        "gen_global_000001",
		InputManifestID:     "gen_global_000001",
		InputManifestSHA256: testHash,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid global generation ref rejected: %v", err)
	}
	bad := valid
	bad.Scope = ScopeProject
	if err := bad.Validate(); err == nil {
		t.Error("global generation ref with project scope must be rejected")
	}
}

// ---- EvaluationContext schema ----

func validEvaluationContext() EvaluationContext {
	now := "2026-08-11T00:00:00Z"
	ec := EvaluationContext{
		SchemaVersion: 1,
		ContextID:     "eval_ctx_001",
		Scope:         ScopeProject,
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion:       1,
			Scope:               ScopeProject,
			GenerationID:        "gen_project_000001",
			InputManifestID:     "gen_project_000001",
			InputManifestSHA256: testHash,
		},
		CompilerVersion:         OKFCompilerVersion,
		CanonicalizationVersion: OKFCanonicalizationVersion,
		ContextSignatureVersion: 1,
		CreatedAt:               now,
	}
	h, err := ec.ContentHash()
	if err != nil {
		panic(err)
	}
	ec.ContextSignature = h
	return ec
}

func TestEvaluationContextSchemaAndSignature(t *testing.T) {
	ec := validEvaluationContext()
	if err := ec.Validate(); err != nil {
		t.Fatalf("valid evaluation context rejected: %v", err)
	}
	b1, err := ec.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := ec.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Error("canonical bytes must be deterministic")
	}
	// Changing a pinned field changes the signature requirement.
	drifting := ec
	drifting.ContextSignature = testHash
	if err := drifting.Validate(); err == nil {
		t.Error("signature drift must be rejected")
	}
	// Cross-scope ref must be rejected (evaluation scope must match ref scope).
	cross := ec
	cross.Scope = ScopeGlobal
	if err := cross.Validate(); err == nil {
		t.Error("project ref under a global evaluation context must be rejected")
	}
	// A global context must carry a global ref and no project ref.
	g := ec
	g.Scope = ScopeGlobal
	g.ProjectGenerationRef = nil
	g.GlobalGenerationRef = &GlobalGenerationRef{
		SchemaVersion: 1, Scope: ScopeGlobal, GenerationID: "gen_global_1",
		InputManifestID: "gen_global_1", InputManifestSHA256: testHash,
	}
	h, err := g.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	g.ContextSignature = h
	if err := g.Validate(); err != nil {
		t.Fatalf("valid global evaluation context rejected: %v", err)
	}
	// Neither ref present must be rejected.
	none := ec
	none.ProjectGenerationRef = nil
	none.GlobalGenerationRef = nil
	h2, err := none.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	none.ContextSignature = h2
	if err := none.Validate(); err == nil {
		t.Error("evaluation context with no generation ref must be rejected")
	}
	// Future references are rejected at build time (with the request's Now),
	// not at schema time: the schema must stay pure and deterministic.
}

func TestEvaluationContextRejectsUnknownFields(t *testing.T) {
	ec := validEvaluationContext()
	b, err := json.Marshal(ec)
	if err != nil {
		t.Fatal(err)
	}
	// Inject an unknown field.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["context_id"] = "hacked"
	m["unknown_field"] = true
	injected, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStrict[EvaluationContext](injected); err == nil {
		t.Error("unknown field must be rejected")
	}
}

// ---- build ----

// commitOKFGeneration commits one full OKF generation and returns the tx and
// stores.
func commitOKFGeneration(t *testing.T, root, key string, base *string) (*GenerationTx, *FactStore, GenerationStore) {
	t.Helper()
	s := openProject(t, root, Options{})
	gs := NewGenerationStore(s)
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)
	compileReq := okfRequest(rev, ev)
	compileReq.EvaluationTime = time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	res, err := CompileOKF(context.Background(), s, compileReq)
	if err != nil {
		t.Fatal(err)
	}
	req := beginReq(key, base)
	req.CompilerVersion = OKFCompilerVersion
	req.CanonicalizationVersion = OKFCanonicalizationVersion
	tx, err := gs.Begin(context.Background(), req)
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
	return tx, s, gs
}

func TestBuildEvaluationContextPinsCurrent(t *testing.T) {
	root := tempRoot(t)
	tx, s, _ := commitOKFGeneration(t, root, "okf_eval_ctx", nil)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	ec, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_build", Now: now,
	})
	if err != nil {
		t.Fatalf("build evaluation context: %v", err)
	}
	if ec.ProjectGenerationRef == nil {
		t.Fatal("project generation ref must be pinned")
	}
	if ec.ProjectGenerationRef.GenerationID != tx.GenerationID {
		t.Errorf("pinned generation = %q, want %q", ec.ProjectGenerationRef.GenerationID, tx.GenerationID)
	}
	if ec.ProjectGenerationRef.InputManifestSHA256 == "" || ec.ProjectGenerationRef.InputManifestID != tx.GenerationID {
		t.Errorf("manifest identity not pinned: %+v", ec.ProjectGenerationRef)
	}
	if ec.CompilerVersion != OKFCompilerVersion {
		t.Errorf("compiler version = %q", ec.CompilerVersion)
	}
	if ec.ContextSignature == "" || ec.ContextSignature != ec.ContentHashOrZero(t) {
		t.Errorf("context signature must equal the content hash")
	}
	if ec.GlobalGenerationRef != nil {
		t.Error("project evaluation context must not carry a global ref")
	}

	// Repeat build must produce the same signature (byte-stable context).
	ec2, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_build", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ec2.ContextSignature != ec.ContextSignature {
		t.Errorf("signature not stable: %s != %s", ec2.ContextSignature, ec.ContextSignature)
	}
}

// ContentHashOrZero returns the context content hash or fails the test; used
// to assert signature == content hash without recomputing logic twice.
func (ec *EvaluationContext) ContentHashOrZero(t *testing.T) string {
	t.Helper()
	h, err := ec.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestBuildEvaluationContextRejectsFutureGeneration(t *testing.T) {
	root := tempRoot(t)
	_, s, _ := commitOKFGeneration(t, root, "okf_eval_future", nil)

	// The committed manifest pins created_at=2026-08-10; asking for a context
	// anchored before that date is a future-generation reference and must
	// fail closed.
	early := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	_, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_future", Now: early,
	})
	if ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("future generation must fail closed, got %v", err)
	}
}

func TestBuildEvaluationContextRejectsTamperedGeneration(t *testing.T) {
	root := tempRoot(t)
	tx, s, _ := commitOKFGeneration(t, root, "okf_eval_tamper", nil)

	// Tamper with the published generation.json output hash.
	genPath := filepath.Join(root, "generations", tx.GenerationID, "generation.json")
	doc, err := readJSONFile[generationDoc](genPath)
	if err != nil {
		t.Fatal(err)
	}
	doc.OutputGenerationSHA256 = testHash
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(genPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_tamper",
		Now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("tampered generation must fail closed with hash mismatch, got %v", err)
	}
}

func TestEvaluationContextStableAcrossCURRENTSwitch(t *testing.T) {
	root := tempRoot(t)
	tx1, s, _ := commitOKFGeneration(t, root, "okf_eval_gen1", nil)
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	ec1, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_stable", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ec1.ProjectGenerationRef.GenerationID != tx1.GenerationID {
		t.Fatalf("context must pin the first generation")
	}

	// Commit a second generation; CURRENT switches.
	base := tx1.GenerationID
	if _, _, _ = commitOKFGeneration(t, root, "okf_eval_gen2", &base); err != nil {
		t.Fatal(err)
	}
	cur, err := readCurrentForTest(root)
	if err != nil || cur.GenerationID == tx1.GenerationID {
		t.Fatalf("CURRENT must have switched away from gen1: %v %v", cur, err)
	}
	// Rebuild the historical context: it must still resolve, byte-stable,
	// even though CURRENT moved on.
	reb, err := RebuildEvaluationContext(context.Background(), s, ec1)
	if err != nil {
		t.Fatalf("historical context must rebuild after CURRENT switch: %v", err)
	}
	if reb.Status != EvaluationRebuildAvailable {
		t.Fatalf("historical generation must stay available, got %s", reb.Status)
	}
	if reb.ContextSignature != ec1.ContextSignature {
		t.Errorf("rebuild signature drifted: %s != %s", reb.ContextSignature, ec1.ContextSignature)
	}
}

func TestRebuildEvaluationContextFromManifest(t *testing.T) {
	root := tempRoot(t)
	tx, s, _ := commitOKFGeneration(t, root, "okf_eval_rebuild", nil)
	now := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	ec, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_rebuild", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	pinned := ec.ProjectGenerationRef

	// In-place: available without recompiling.
	reb, err := RebuildEvaluationContext(context.Background(), s, ec)
	if err != nil {
		t.Fatal(err)
	}
	if reb.Status != EvaluationRebuildAvailable {
		t.Fatalf("in-place generation must be available, got %s", reb.Status)
	}

	// Delete the published generation; the permanent manifest must allow an
	// exact deterministic rebuild.
	if err := os.RemoveAll(filepath.Join(root, "generations", tx.GenerationID)); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json"),
		filepath.Join(root, "facts", "memory-evidence-generations", "mem_01K7A9X2", "2", "3.json"),
		filepath.Join(root, "facts", "generation-input-manifests", tx.GenerationID+".json"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Logf("missing fact file: %s", p)
		}
	}
	reb, err = RebuildEvaluationContext(context.Background(), s, ec)
	if err != nil {
		t.Fatalf("rebuild from manifest: %v", err)
	}
	if reb.Status != EvaluationRebuildAvailable {
		t.Fatalf("manifest rebuild must be available, got %s", reb.Status)
	}
	if reb.ContextSignature != ec.ContextSignature {
		t.Errorf("rebuild signature must match the pinned context")
	}
	if reb.CompiledSHA256 == "" {
		t.Error("rebuild must produce a compiled output hash")
	}
	if len(reb.Outputs) == 0 {
		t.Error("rebuild must reproduce the compiled views")
	}
	// The rebuild must be byte-stable.
	reb2, err := RebuildEvaluationContext(context.Background(), s, ec)
	if err != nil {
		t.Fatal(err)
	}
	if reb2.CompiledSHA256 != reb.CompiledSHA256 {
		t.Errorf("rebuild not deterministic: %s != %s", reb2.CompiledSHA256, reb.CompiledSHA256)
	}

	// Now drop one input fact: rebuild must become unavailable, not guess.
	revPath := filepath.Join(root, "facts", "memory-revisions", "mem_01K7A9X2", "2.json")
	if err := os.Remove(revPath); err != nil {
		t.Fatal(err)
	}
	reb, err = RebuildEvaluationContext(context.Background(), s, ec)
	if err != nil {
		t.Fatalf("rebuild with missing input should report unavailable, got error %v", err)
	}
	if reb.Status != EvaluationRebuildUnavailable {
		t.Fatalf("missing input fact must yield unavailable, got %s", reb.Status)
	}

	// Restore the input; a tampered manifest hash must fail closed.
	rev := validRevision()
	putRevisionEvidence(t, s, rev, validEvidenceGeneration())
	manifestPath := filepath.Join(root, "facts", "generation-input-manifests", pinned.GenerationID+".json")
	mf, err := readJSONFile[GenerationInputManifest](manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	mf.InputManifestSHA256 = testHash
	raw, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RebuildEvaluationContext(context.Background(), s, ec); ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("tampered manifest must fail closed, got %v", err)
	}
}

func TestBuildEvaluationContextScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	_, s, _ := commitOKFGeneration(t, root, "okf_eval_scope", nil)
	// A project store must reject a global evaluation context.
	_, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeGlobal, ContextID: "eval_ctx_scope",
		Now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope build must fail closed, got %v", err)
	}
}

// TestBuildEvaluationContextRequiresExplicitNow — 确定性回归：零值 Now 必须
// 稳定失败（CodeDerivedInvalidInput），绝不回退到墙上时间。
func TestBuildEvaluationContextRequiresExplicitNow(t *testing.T) {
	root := tempRoot(t)
	_, s, _ := commitOKFGeneration(t, root, "okf_eval_explicit_now", nil)

	req := EvaluationContextRequest{Scope: ScopeProject, ContextID: "eval_ctx_explicit_now"}
	_, err := BuildEvaluationContext(context.Background(), s, req)
	if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now must fail with derived_invalid_input, got %v", err)
	}
	// 错误消息固定、脱敏：两次调用必须完全一致（无时间戳/路径/随机量）。
	_, err2 := BuildEvaluationContext(context.Background(), s, req)
	if err2 == nil || err2.Error() != err.Error() {
		t.Errorf("error must be fixed and stable: %q vs %q", err, err2)
	}
	// 绝不读取墙上时间：消息中不得出现数字时间戳。
	for _, r := range err.Error() {
		if r >= '0' && r <= '9' {
			t.Errorf("error must not embed a wall-clock timestamp: %q", err.Error())
		}
	}
}

// TestBuildEvaluationContextByteStableRepeat — 相同 Now 重复构建必须字节一致。
func TestBuildEvaluationContextByteStableRepeat(t *testing.T) {
	root := tempRoot(t)
	_, s, _ := commitOKFGeneration(t, root, "okf_eval_byte_stable", nil)

	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	req := EvaluationContextRequest{Scope: ScopeProject, ContextID: "eval_ctx_byte_stable", Now: now}
	ec1, err := BuildEvaluationContext(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := BuildEvaluationContext(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := ec1.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := ec2.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("repeat build must be byte-identical:\n%s\n%s", b1, b2)
	}
	if ec1.ContextSignature != ec2.ContextSignature {
		t.Errorf("signature must be stable: %s != %s", ec1.ContextSignature, ec2.ContextSignature)
	}
}

// TestBuildEvaluationContextNowOnlyChangesCreatedAt — 不同显式 Now 只在显式
// 输入变化时改变结果：除 CreatedAt（及其派生的 signature）外全部字段不变。
func TestBuildEvaluationContextNowOnlyChangesCreatedAt(t *testing.T) {
	root := tempRoot(t)
	_, s, _ := commitOKFGeneration(t, root, "okf_eval_now_only", nil)

	now1 := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	now2 := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	ec1, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_now_only", Now: now1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := BuildEvaluationContext(context.Background(), s, EvaluationContextRequest{
		Scope: ScopeProject, ContextID: "eval_ctx_now_only", Now: now2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ec1.CreatedAt == ec2.CreatedAt {
		t.Error("different explicit Now must change CreatedAt")
	}
	if !reflect.DeepEqual(ec1.ProjectGenerationRef, ec2.ProjectGenerationRef) {
		t.Errorf("generation ref must not depend on Now: %+v vs %+v", ec1.ProjectGenerationRef, ec2.ProjectGenerationRef)
	}
	if ec1.CompilerVersion != ec2.CompilerVersion || ec1.CanonicalizationVersion != ec2.CanonicalizationVersion {
		t.Error("compiler identity must not depend on Now")
	}
}
