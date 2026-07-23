package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateRoundTrip(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := strings.TrimSpace(`
[quality]
fixtures = "benchmarks/fixtures"
min_qualified_rate = 0.9
max_cost = 1.5

[runtime]
metrics_dir = ".reasonix/omr/metrics"
model = "deepseek-v4-flash"
max_steps = 20
concurrency = 4
timeout = "2m"

[agent.omr-research]
model = "deepseek-v4-flash"
prompt_file = "prompts/research.md"
read_only = true

[agent.omr-debug]
model = "gpt-4o"
read_only = true

[routing]
frontend = "omr-frontend"
explore = "omr-explore"

[profiles]
disabled = "omr-debug, omr-research"
`) + "\n"
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	err := ExecuteMigration(tomlPath, jsoncPath, false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify both parse to the same Config
	srcCfg, err := loadTOML(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := configDiff(srcCfg, dstCfg); diff != "" {
		t.Fatalf("config mismatch: %s", diff)
	}
}

func TestMigratePreservesEnvVars(t *testing.T) {
	t.Setenv("MODEL_VAR", "deepseek-v4-flash")
	t.Setenv("PROMPT_VAR", "prompts/research.md")

	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[runtime]
model = "${MODEL_VAR}"

[agent.omr-research]
prompt_file = "${PROMPT_VAR}"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	// Read JSONC and check ${VAR} is preserved
	data, err := os.ReadFile(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	jsoncContent := string(data)
	if !strings.Contains(jsoncContent, "${MODEL_VAR}") {
		t.Fatalf("expected ${MODEL_VAR} preserved in JSONC, got: %s", jsoncContent)
	}
	if !strings.Contains(jsoncContent, "${PROMPT_VAR}") {
		t.Fatalf("expected ${PROMPT_VAR} preserved in JSONC, got: %s", jsoncContent)
	}
}

func TestMigratePreservesAgents(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[agent.omr-research]
model = "deepseek-v4-flash"
prompt_file = "prompts/research.md"
read_only = true

[agent.omr-debug]
model = "gpt-4o"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(dstCfg.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(dstCfg.Agents))
	}
	if dstCfg.Agents["omr-research"].Model != "deepseek-v4-flash" {
		t.Fatalf("unexpected omr-research model: %q", dstCfg.Agents["omr-research"].Model)
	}
	if dstCfg.Agents["omr-research"].ReadOnly == nil || !*dstCfg.Agents["omr-research"].ReadOnly {
		t.Fatalf("expected omr-research read_only=true")
	}
	if dstCfg.Agents["omr-debug"].Model != "gpt-4o" {
		t.Fatalf("unexpected omr-debug model: %q", dstCfg.Agents["omr-debug"].Model)
	}
}

func TestMigratePreservesRouting(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[routing]
frontend = "omr-frontend"
explore = "omr-explore"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if dstCfg.Categories["frontend"] != "omr-frontend" || dstCfg.Categories["explore"] != "omr-explore" {
		t.Fatalf("unexpected categories: %#v", dstCfg.Categories)
	}
}

func TestMigratePreservesDisabledProfiles(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[profiles]
disabled = "omr-debug, omr-research"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(dstCfg.DisabledProfiles) != 2 {
		t.Fatalf("expected 2 disabled profiles, got %v", dstCfg.DisabledProfiles)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "test"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	// First migration
	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	// Second migration — should be idempotent (no error)
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatalf("expected idempotent migration to succeed, got: %v", err)
	}
}

func TestMigrateConflictDetected(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "original"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	// Write a different JSONC
	differentJSONC := `{"quality": {"fixtures": "different"}}`
	if err := os.WriteFile(jsoncPath, []byte(differentJSONC), 0o600); err != nil {
		t.Fatal(err)
	}

	// Try migration without force
	err := ExecuteMigration(tomlPath, jsoncPath, false)
	if err == nil {
		t.Fatal("expected conflict error without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected conflict error, got: %v", err)
	}

	// With --force should succeed
	if err := ExecuteMigration(tomlPath, jsoncPath, true); err != nil {
		t.Fatalf("expected migration with --force to succeed, got: %v", err)
	}

	// Verify content
	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if dstCfg.Fixtures != "original" {
		t.Fatalf("expected fixtures=original, got %q", dstCfg.Fixtures)
	}
}

func TestMigrateCreatesBackup(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "test"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	// Check .bak file exists
	bakPath := tomlPath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Fatalf("backup file not found: %v", err)
	}

	// Verify backup content matches original
	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(bakData) != tomlData {
		t.Fatalf("backup content mismatch: expected %q, got %q", tomlData, string(bakData))
	}
}

func TestMigrateNoSourceFile(t *testing.T) {
	root := t.TempDir()
	jsoncPath := filepath.Join(root, "config.jsonc")
	tomlPath := filepath.Join(root, "nonexistent.toml")

	err := ExecuteMigration(tomlPath, jsoncPath, false)
	if err == nil {
		t.Fatal("expected error when source does not exist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestMigrateDryRunNoFiles(t *testing.T) {
	root := t.TempDir()
	source, dest := DefaultConfigPaths(root)

	plan := PlanMigration(source, dest)
	if plan.SourceExists {
		t.Fatal("expected source to not exist")
	}
	if plan.CanMigrate {
		t.Fatal("expected plan to not allow migration")
	}

	// Verify no files were created
	if _, err := os.Stat(source); err == nil {
		t.Fatal("dry run should not create source file")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("dry run should not create dest file")
	}
}

func TestMigrateWithEnvVarsRoundTrip(t *testing.T) {
	t.Setenv("OMR_MIGRATE_MODEL", "deepseek-v4-flash")
	t.Setenv("OMR_MIGRATE_FIXTURES", "benchmarks/fixtures")
	t.Setenv("OMR_MIGRATE_PROMPT", "prompts/research.md")

	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "${OMR_MIGRATE_FIXTURES}"

[runtime]
model = "$OMR_MIGRATE_MODEL"

[agent.omr-research]
prompt_file = "${OMR_MIGRATE_PROMPT}"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	// Both should parse to the same config
	srcCfg, err := loadTOML(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	dstCfg, err := loadJSONC(jsoncPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := configDiff(srcCfg, dstCfg); diff != "" {
		t.Fatalf("config mismatch with env vars: %s", diff)
	}
}

func TestMigrateBackupDoesNotOverwriteExisting(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "test"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	// First migration creates .bak
	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	bakPath := tomlPath + ".bak"
	bakModTime1, err := os.Stat(bakPath)
	if err != nil {
		t.Fatal(err)
	}

	// Idempotent second migration should not overwrite backup
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	bakModTime2, err := os.Stat(bakPath)
	if err != nil {
		t.Fatal(err)
	}

	if !bakModTime1.ModTime().Equal(bakModTime2.ModTime()) {
		t.Fatal("backup was unexpectedly overwritten on idempotent migration")
	}
}

func TestMigrateConfigDiff(t *testing.T) {
	tests := []struct {
		name string
		a    Config
		b    Config
		want string
	}{
		{
			name: "identical",
			a:    Config{Fixtures: "a", MaxSteps: 10},
			b:    Config{Fixtures: "a", MaxSteps: 10},
			want: "",
		},
		{
			name: "fixtures differ",
			a:    Config{Fixtures: "a"},
			b:    Config{Fixtures: "b"},
			want: "Fixtures",
		},
		{
			name: "agents differ",
			a:    Config{Agents: map[string]AgentConfig{"r": {Model: "a"}}},
			b:    Config{Agents: map[string]AgentConfig{"r": {Model: "b"}}},
			want: "model",
		},
		{
			name: "categories differ",
			a:    Config{Categories: map[string]string{"f": "omr-f"}},
			b:    Config{Categories: map[string]string{"f": "omr-x"}},
			want: "routing",
		},
		{
			name: "disabled profiles differ",
			a:    Config{DisabledProfiles: []string{"a"}},
			b:    Config{DisabledProfiles: []string{"b"}},
			want: "disabled",
		},
		{
			name: "timeout differs",
			a:    Config{Timeout: time.Second, TimeoutSet: true},
			b:    Config{Timeout: time.Minute, TimeoutSet: true},
			want: "Timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configDiff(tt.a, tt.b)
			if tt.want == "" && got != "" {
				t.Fatalf("expected no diff, got: %s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("expected diff containing %q, got: %s", tt.want, got)
			}
		})
	}
}

// ─── FIX-04: migration failure recoverability tests ───

func TestMigrateRejectsInvalidTOML(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	// Duplicate key in TOML
	tomlData := `[runtime]
model = "a"
model = "b"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	// loadTOML should reject duplicate keys
	_, err := loadTOML(tomlPath)
	if err == nil {
		t.Fatal("expected duplicate TOML key to be rejected")
	}
	// ExecuteMigration should also fail
	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err == nil {
		t.Fatal("expected migration to reject duplicate TOML key")
	}
}

func TestMigrateBackupFailureNoDestWritten(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "test"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	// Make the backup path unwritable by making the parent dir read-only
	// Backup goes to sourcePath + ".bak" in the same directory
	// Make the directory read-only BEFORE migration so backup fails
	backupPath := tomlPath + ".bak"
	_ = backupPath // backup path is in same dir
	// We can't easily make the single backup fail without also blocking the source read
	// Instead, verify that if backup can't be created, no dest is written
	jsoncPath := filepath.Join(root, "config.jsonc")

	// Make the source directory read-only so backup write will fail
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o755)

	err := ExecuteMigration(tomlPath, jsoncPath, false)
	if err == nil {
		t.Fatal("expected migration to fail when backup cannot be written")
	}

	os.Chmod(root, 0o755)

	// Verify dest file was NOT written
	if _, err := os.Stat(jsoncPath); err == nil {
		t.Fatal("dest file should not exist after failed migration")
	}
}

func TestMigrateConfigDiffDoesNotMutate(t *testing.T) {
	a := Config{Fixtures: "original", MaxSteps: 10}
	b := Config{Fixtures: "original", MaxSteps: 20}

	aCopy := a
	bCopy := b

	_ = configDiff(a, b)

	// Verify inputs were not modified
	if a.Fixtures != aCopy.Fixtures || a.MaxSteps != aCopy.MaxSteps {
		t.Fatal("configDiff modified input config 'a'")
	}
	if b.Fixtures != bCopy.Fixtures || b.MaxSteps != bCopy.MaxSteps {
		t.Fatal("configDiff modified input config 'b'")
	}
}

func TestMigrateRollbackRestoresFilePermissions(t *testing.T) {
	root := t.TempDir()
	tomlPath := filepath.Join(root, "config.toml")
	tomlData := `[quality]
fixtures = "test"
`
	if err := os.WriteFile(tomlPath, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}

	jsoncPath := filepath.Join(root, "config.jsonc")
	if err := ExecuteMigration(tomlPath, jsoncPath, false); err != nil {
		t.Fatal(err)
	}

	// Read original file permissions
	origInfo, err := os.Stat(tomlPath)
	if err != nil {
		t.Fatal(err)
	}

	// Make dest file read-only, then force-overwrite — the rollback should restore
	// the original source (which already has a .bak)
	os.Chmod(jsoncPath, 0o444)
	defer os.Chmod(jsoncPath, 0o644)

	// Change source content
	if err := os.WriteFile(tomlPath, []byte("[quality]\nfixtures = \"changed\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Force migration with read-only dest — should fail on write
	err = ExecuteMigration(tomlPath, jsoncPath, true)
	if err == nil {
		t.Fatal("expected migration to fail when dest is read-only")
	}

	// Verify original file permissions are restored (not tightened by failed migration)
	afterInfo, err := os.Stat(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	if afterInfo.Mode().Perm() != origInfo.Mode().Perm() {
		t.Fatalf("file permission changed after rollback: was %o, got %o",
			origInfo.Mode().Perm(), afterInfo.Mode().Perm())
	}
}
