package memory

import "testing"

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
