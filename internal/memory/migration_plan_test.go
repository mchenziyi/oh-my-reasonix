package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestBuildMigrationPlanBindsTargetBaseGeneration(t *testing.T) {
	source := openProject(t, tempRoot(t), Options{})
	target := openProject(t, tempRoot(t), Options{})
	sourceTx := commitOne(t, NewGenerationStore(source), "migration_plan_base_source", nil)
	targetTx := commitOne(t, NewGenerationStore(target), "migration_plan_base_target", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, sourceTx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetBaseGenerationID == nil || *plan.TargetBaseGenerationID != targetTx.GenerationID {
		t.Fatalf("plan must bind the target CURRENT generation: %+v", plan)
	}
	planHash := plan.PlanHash()
	changed := plan
	otherBase := "gen_01K7A9X2TARGET"
	changed.TargetBaseGenerationID = &otherBase
	if changed.PlanHash() == planHash {
		t.Fatal("target base generation must participate in the plan hash")
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
	encoded, _ := json.Marshal(res)
	if string(encoded) == "" || !strings.Contains(string(encoded), `"status":"committed"`) {
		t.Fatalf("migration result must have stable JSON status: %s", encoded)
	}
	targetCur, err := NewGenerationStore(target).(*generationStore).readCurrent(context.Background())
	if err != nil || targetCur.GenerationID != res.GenerationID {
		t.Fatalf("target CURRENT mismatch: %+v %v", targetCur, err)
	}
	sourceCur, _ := NewGenerationStore(source).(*generationStore).readCurrent(context.Background())
	if sourceCur.GenerationID != sourceTx.GenerationID {
		t.Fatal("migration must not change source CURRENT")
	}
	changed := plan
	changed.FactCount++
	if _, err := ApplyMigration(context.Background(), source, target, MigrationApplyRequest{Plan: changed, IdempotencyKey: "migration_apply_key"}); ErrorCode(err) != CodeGenerationIdempotency {
		t.Fatalf("changed migration plan with same key must fail closed before reuse, got %v", err)
	}
}

func TestApplyMigrationPersistsDeterministicSnapshotBeforePublish(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := openProject(t, targetRoot, Options{})
	sourceTx := commitOne(t, NewGenerationStore(source), "migration_snapshot_source", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, sourceTx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ApplyMigration(context.Background(), source, target, MigrationApplyRequest{Plan: plan, IdempotencyKey: "migration_snapshot_key"})
	if err != nil || res.SnapshotID == "" {
		t.Fatalf("migration must publish a snapshot: %+v %v", res, err)
	}
	replay, err := ApplyMigration(context.Background(), source, target, MigrationApplyRequest{Plan: plan, IdempotencyKey: "migration_snapshot_key"})
	if err != nil || replay.Commit.Status != CommitAlreadyCommitted || replay.GenerationID != res.GenerationID || replay.SnapshotID != res.SnapshotID {
		t.Fatalf("migration replay must return the durable commit and snapshot: %+v %v", replay, err)
	}
	path := filepath.Join(target.root, "migration-snapshots", res.SnapshotID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("migration snapshot missing: %v", err)
	}
	var snapshot MigrationSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.PlanHash != plan.PlanHash() || snapshot.SourceGenerationID != plan.GenerationID || snapshot.SourceManifestSHA256 != plan.InputManifestSHA256 {
		t.Fatalf("snapshot does not bind source plan: %+v", snapshot)
	}
	before := string(data)
	if _, err := persistMigrationSnapshot(target, plan, nil); err != nil {
		t.Fatalf("same snapshot must be idempotent: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != before {
		t.Fatalf("snapshot replay must not rewrite immutable bytes")
	}
}

func TestMigrationFactRollbackRemovesOnlyFactsCreatedByApply(t *testing.T) {
	sourceRoot := tempRoot(t)
	targetRoot := tempRoot(t)
	source := openProject(t, sourceRoot, Options{})
	target := openProject(t, targetRoot, Options{})
	sourceTx := commitOne(t, NewGenerationStore(source), "migration_fact_rollback", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, sourceTx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, manifest, facts, err := loadMigrationSource(context.Background(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := target.acquireWriteLock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, rollback, err := copyMigrationFactsLockedWithRollback(context.Background(), target, facts, manifest)
	if err != nil || result.Created != 2 {
		unlock()
		t.Fatalf("expected migration facts to be created before downstream failure: %+v %v", result, err)
	}
	rollback()
	unlock()
	for _, fact := range append(append([]Fact{}, facts...), manifest) {
		kind, key, keyErr := factKey(fact)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		if _, getErr := target.Get(context.Background(), kind, key); ErrorCode(getErr) != CodeNotFound {
			t.Fatalf("downstream failure rollback left fact %s: %v", key, getErr)
		}
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
