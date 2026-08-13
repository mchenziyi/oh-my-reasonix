package memory

import (
	"context"
	"testing"
)

func TestCheckMigrationReadinessIsReadOnlyAndDeterministic(t *testing.T) {
	source := openProject(t, tempRoot(t), Options{})
	target := openProject(t, tempRoot(t), Options{})
	tx := commitOne(t, NewGenerationStore(source), "migration_doctor", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := target.List(context.Background(), FactKindGenerationInputManifest)
	if err != nil {
		t.Fatal(err)
	}
	report, err := CheckMigrationReadiness(context.Background(), source, target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy || report.SourceFactCount != 2 || report.TargetMissingCount != 2 || report.TargetExistingCount != 0 {
		t.Fatalf("unexpected readiness report: %+v", report)
	}
	a, _ := report.EncodeCanonical()
	replay, err := CheckMigrationReadiness(context.Background(), source, target, plan)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := replay.EncodeCanonical()
	if string(a) != string(b) {
		t.Fatal("readiness report must be deterministic")
	}
	after, _ := target.List(context.Background(), FactKindGenerationInputManifest)
	if len(before) != len(after) {
		t.Fatal("doctor must not write target facts")
	}
}

func TestCheckMigrationReadinessReportsTargetConflict(t *testing.T) {
	source := openProject(t, tempRoot(t), Options{})
	target := openProject(t, tempRoot(t), Options{})
	tx := commitOne(t, NewGenerationStore(source), "migration_doctor_conflict", nil)
	plan, err := BuildMigrationPlanFromStores(context.Background(), source, target, tx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, manifest, facts, err := loadMigrationSource(context.Background(), source, plan)
	if err != nil || len(facts) == 0 {
		t.Fatal(err)
	}
	if _, err := target.Put(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	// The target already has the manifest, but the input fact is not present;
	// this is a partial, still safe readiness state rather than an auto-copy.
	report, err := CheckMigrationReadiness(context.Background(), source, target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy || report.TargetExistingCount != 1 || report.TargetMissingCount != 1 {
		t.Fatalf("unexpected partial readiness report: %+v", report)
	}
	_ = facts
}
