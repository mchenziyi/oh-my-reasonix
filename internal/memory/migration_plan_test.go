package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildMigrationPlanFromStoresIsVerifiedAndReadOnly(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := openProject(t, targetRoot, Options{})
	tx := commitOne(t, NewGenerationStore(source), "migration_verified", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, tx.GenerationID)
	if err != nil || !plan.Eligible || plan.FactCount != 1 || plan.InputManifestSHA256 == "" {
		t.Fatalf("unexpected verified migration plan: %+v %v", plan, err)
	}
	if _, err := target.Get(context.Background(), FactKindGenerationInputManifest, tx.GenerationID); ErrorCode(err) != CodeNotFound {
		t.Fatalf("preview must not write target: %v", err)
	}
}

func TestBuildMigrationPlanFromStoresRejectsCrossScope(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := mustOpenStore(t, targetRoot, StoreScopeGlobal)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, "gen_01")
	if err != nil || plan.Eligible || plan.BlockedReason == "" {
		t.Fatalf("cross scope migration must be blocked: %+v %v", plan, err)
	}
}

func TestApplyMigrationCopyIsAtomicAndIdempotent(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := openProject(t, targetRoot, Options{})
	tx := commitOne(t, NewGenerationStore(source), "migration_copy", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ApplyMigrationCopy(context.Background(), source, target, MigrationCopyRequest{Plan: plan})
	if err != nil || first.Created != 2 || first.Noop != 0 {
		t.Fatalf("unexpected copy result: %+v %v", first, err)
	}
	second, err := ApplyMigrationCopy(context.Background(), source, target, MigrationCopyRequest{Plan: plan})
	if err != nil || second.Created != 0 || second.Noop != 2 {
		t.Fatalf("copy replay must be idempotent: %+v %v", second, err)
	}
}

func TestApplyMigrationPublishesNewTargetGeneration(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := openProject(t, targetRoot, Options{})
	sourceTx := commitOne(t, NewGenerationStore(source), "migration_apply_source", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, sourceTx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ApplyMigration(context.Background(), source, target, MigrationApplyRequest{Plan: plan, IdempotencyKey: "migration_apply_key"})
	if err != nil || res.Commit.Status != CommitCommitted || res.GenerationID == sourceTx.GenerationID {
		t.Fatalf("migration apply failed: %+v %v", res, err)
	}
	targetCur, err := NewGenerationStore(target).(*generationStore).readCurrent(context.Background())
	if err != nil || targetCur.GenerationID != res.GenerationID {
		t.Fatalf("target CURRENT mismatch: %+v %v", targetCur, err)
	}
	sourceCur, _ := NewGenerationStore(source).(*generationStore).readCurrent(context.Background())
	if sourceCur.GenerationID != sourceTx.GenerationID {
		t.Fatal("migration must not change source CURRENT")
	}
}

func TestApplyMigrationRejectsCrossScopeAndTamperedSource(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := mustOpenStore(t, targetRoot, StoreScopeGlobal)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, "gen_01")
	if err != nil || plan.Eligible {
		t.Fatalf("cross scope preview must be blocked: %+v %v", plan, err)
	}
	projectTarget := openProject(t, tempRoot(t), Options{})
	tx := commitOne(t, NewGenerationStore(source), "migration_tamper", nil)
	plan, err = BuildMigrationPlanFromStores(context.Background(), source, projectTarget, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "generations", tx.GenerationID, "generation.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyMigration(context.Background(), source, projectTarget, MigrationApplyRequest{Plan: plan, IdempotencyKey: "migration_tamper_key"}); ErrorCode(err) != CodeGenerationStagingInvalid {
		t.Fatalf("tampered source must fail closed, got %v", err)
	}
}

func TestBuildMigrationPlanScopeAndRefGate(t *testing.T) {
	req := MigrationRequest{SourceScope: ScopeProject, TargetScope: ScopeProject, ProjectGenerationRef: &ProjectGenerationRef{SchemaVersion: SchemaVersion, Scope: ScopeProject, GenerationID: "gen_migrate", InputManifestID: "manifest_migrate", InputManifestSHA256: "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, FactCount: 4, InputManifestSHA256: "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	p, err := BuildMigrationPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Eligible || p.GenerationID != "gen_migrate" || len(p.Steps) != 6 {
		t.Fatalf("unexpected migration plan: %+v", p)
	}
	bad := req
	bad.TargetScope = ScopeGlobal
	p, err = BuildMigrationPlan(bad)
	if err != nil || p.Eligible || p.BlockedReason == "" {
		t.Fatalf("project to global must be blocked: %+v %v", p, err)
	}
	bad = req
	bad.ProjectGenerationRef = nil
	if _, err := BuildMigrationPlan(bad); err == nil {
		t.Fatal("missing generation ref must fail closed")
	}
}

func TestBuildMigrationPlanIsDeterministic(t *testing.T) {
	req := MigrationRequest{SourceScope: ScopeGlobal, TargetScope: ScopeGlobal, GlobalGenerationRef: &GlobalGenerationRef{SchemaVersion: SchemaVersion, Scope: ScopeGlobal, GenerationID: "gen_global", InputManifestID: "manifest_global", InputManifestSHA256: "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, InputManifestSHA256: "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	a, err := BuildMigrationPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildMigrationPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	ba, _ := a.CanonicalBytes()
	bb, _ := b.CanonicalBytes()
	if string(ba) != string(bb) || a.PlanHash() != b.PlanHash() {
		t.Fatal("migration plans must be byte stable")
	}
}
