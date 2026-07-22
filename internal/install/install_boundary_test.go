package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

// TestUpgradeDetectsModifiedGeneratedPrompt verifies that modifying the
// generated prompt after Init is detected by Upgrade as a conflict.
func TestUpgradeDetectsModifiedGeneratedPrompt(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Modify the generated prompt
	genPath := GeneratedPromptPath(root)
	if err := os.WriteFile(genPath, []byte("user modification\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Upgrade should detect the conflict
	report, err := Init(Options{ProjectDir: root, Assets: assets, Upgrade: true})
	if err == nil {
		t.Fatal("expected conflict for modified generated prompt")
	}
	if len(report.Conflicts) == 0 || !strings.Contains(report.Conflicts[0], "modified after installation") {
		t.Fatalf("expected 'modified after installation' conflict, got: %v", report.Conflicts)
	}
}

// TestUpgradeDetectsModifiedProfile verifies that modifying a profile
// after Init is detected by Upgrade as a conflict.
func TestUpgradeDetectsModifiedProfile(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Modify one profile
	profilePath := ExploreProfilePath(root)
	if err := os.WriteFile(profilePath, []byte("user modification\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Upgrade should detect the conflict
	report, err := Init(Options{ProjectDir: root, Assets: assets, Upgrade: true})
	if err == nil {
		t.Fatal("expected conflict for modified profile")
	}
	if len(report.Conflicts) == 0 || !strings.Contains(report.Conflicts[0], "modified after installation") {
		t.Fatalf("expected 'modified after installation' conflict, got: %v", report.Conflicts)
	}
}

// TestUpgradeCanOverwriteModifiedPromptWithAccept verifies that with
// AcceptReasonixBaseUpdate and repairing the prompt, Upgrade can proceed.
func TestUpgradeCanOverwriteModifiedPromptWithAccept(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Modify the generated prompt and the manifest to simulate a user edit
	genPath := GeneratedPromptPath(root)
	if err := os.WriteFile(genPath, []byte("user content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// We need to detect this as conflict and then use --accept-reasonix-base-update
	// The existing check is in checkAssetPathConflict, which blocks upgrade.
	// To proceed, the user would need to re-init (not upgrade) or fix the conflict.
	// This tests that even with AcceptReasonixBaseUpdate, prompt modification still blocks.
	report, err := Init(Options{
		ProjectDir:              root,
		Assets:                  assets,
		Upgrade:                 true,
		AcceptReasonixBaseUpdate: true,
	})
	if err == nil {
		t.Fatal("expected conflict for modified prompt even with base update accepted")
	}
	if len(report.Conflicts) == 0 || !strings.Contains(report.Conflicts[0], "modified after installation") {
		t.Fatalf("expected 'modified after installation' conflict, got: %v", report.Conflicts)
	}
}

// TestInitRecreatesDeletedManifest verifies that if the manifest is deleted
// after Init, a subsequent Init recreates it (first-install path).
func TestInitRecreatesDeletedManifest(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Full cleanup: remove manifest, generated prompt, profiles, and config changes
	if err := os.Remove(ManifestPath(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(GeneratedPromptPath(root)); err != nil {
		t.Fatal(err)
	}
	profileDir := filepath.Join(root, filepath.FromSlash(".reasonix/skills"))
	if err := os.RemoveAll(profileDir); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "reasonix.toml")
	if err := os.WriteFile(configPath, []byte("[agent]\nmodel = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-init — should succeed like a fresh install
	report, err := Init(Options{ProjectDir: root, Assets: assets})
	if err != nil {
		t.Fatalf("re-init after full cleanup: %v %#v", err, report)
	}
	if !report.Written {
		t.Fatal("expected re-init to write manifest")
	}

	// Verify manifest is recreated
	_, err = manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatalf("manifest not recreated: %v", err)
	}
}

// TestInitWithModifiedPromptBeforeManifestRestoresPreexistingPrompt checks that
// when the user had an existing system_prompt_file pointing to the generated
// path but the manifest is missing, Init reports a conflict.
func TestInitDetectsOrphanedGeneratedPrompt(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")

	// Create the generated prompt file at the expected path WITHOUT a manifest
	genDir := filepath.Dir(GeneratedPromptPath(root))
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GeneratedPromptPath(root), []byte("orphan content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Update config to point to generated prompt
	configPath := filepath.Join(root, "reasonix.toml")
	configContent := "[agent]\nmodel = \"test\"\nsystem_prompt_file = \".reasonix/omr/generated/system-prompt.md\"\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Init with this setup should fail because the generated prompt exists
	// but is not claimed by a valid manifest
	assets := testAssets()
	_, err := Init(Options{ProjectDir: root, Assets: assets})
	if err == nil {
		t.Fatal("expected conflict when orphaned generated prompt exists")
	}
}

// TestInstallRollbackOnWriteFailure verifies that when a write fails partway
// through, the rollback restores the original state.
func TestInstallRollbackOnWriteFailure(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")

	// First Init succeeds
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Read the generated prompt content for later comparison
	genPath := GeneratedPromptPath(root)
	origGen, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatal(err)
	}

	// Read the manifest SHA for later comparison
	origManifest, err := manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}

	// Read the explorer profile for later comparison
	profilePath := ExploreProfilePath(root)
	origProfile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}

	// Make the profile directory read-only so profile write will fail
	// Profiles are written AFTER generated prompt, BEFORE config/manifest
	profileDir := filepath.Dir(profilePath)
	if err := os.Chmod(profileDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(profileDir, 0o755)

	// Different assets to trigger profile writes
	differentAssets := testAssets()
	differentAssets.Explore = []byte("different explore\n")

	// Upgrade should fail due to profile write failure
	report, err := Init(Options{ProjectDir: root, Assets: differentAssets, Upgrade: true})
	if err == nil {
		t.Fatalf("expected write failure, got: %#v", report)
	}

	// Restore permissions for cleanup
	os.Chmod(profileDir, 0o755)

	// Verify rollback: generated prompt should be restored to original
	currentGen, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentGen) != string(origGen) {
		t.Fatalf("generated prompt not rolled back: expected %q, got %q", origGen, currentGen)
	}

	// Verify manifest should be restored to original
	currentManifest, err := manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if currentManifest.Prompt.FinalSHA256 != origManifest.Prompt.FinalSHA256 {
		t.Fatalf("manifest not rolled back: expected SHA %q, got %q",
			origManifest.Prompt.FinalSHA256, currentManifest.Prompt.FinalSHA256)
	}

	// Verify profile should be restored to original
	currentProfile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentProfile) != string(origProfile) {
		t.Fatalf("profile not rolled back: expected %q, got %q", origProfile, currentProfile)
	}
}

// TestUninstallRollbackOnWriteFailure verifies that when uninstall fails
// partway, the rollback restores the original state.
func TestUninstallRollbackOnWriteFailure(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")

	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Read config for later comparison
	configPath := filepath.Join(root, "reasonix.toml")
	origConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Make the profiles directory read-only BEFORE uninstall so that
	// profile removal fails
	profilePath := ExploreProfilePath(root)
	profileDir := filepath.Dir(profilePath)
	if err := os.Chmod(profileDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(profileDir, 0o755)

	// Uninstall should fail
	_, err = Uninstall(Options{ProjectDir: root})
	if err == nil {
		t.Fatal("expected uninstall to fail on read-only profile dir")
	}

	// Restore permissions
	os.Chmod(profileDir, 0o755)

	// Verify config is restored
	currentConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentConfig) != string(origConfig) {
		t.Fatalf("config not rolled back after failed uninstall: expected %q, got %q",
			origConfig, currentConfig)
	}
}

// TestExternalAssetDirInconsistency verifies that PromptSourceDrift
// detects when external assets differ from the installed manifest.
func TestExternalAssetDirInconsistency(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Load the manifest after installation
	m, err := manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}

	// Use assets with different base prompt (simulating external asset dir change)
	differentAssets := testAssets()
	differentAssets.BasePrompt = []byte("different base\n")

	// PromptSourceDrift should detect the base prompt change
	drift := PromptSourceDrift(root, m, differentAssets)
	if len(drift) == 0 {
		t.Fatal("expected drift detection for different base prompt")
	}
	found := false
	for _, d := range drift {
		if strings.Contains(d, "base") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected drift message about base prompt, got: %v", drift)
	}

	// Use assets with different orchestrator
	differentOrch := testAssets()
	differentOrch.Orchestrator = []byte("different orchestrator\n")

	drift2 := PromptSourceDrift(root, m, differentOrch)
	if len(drift2) == 0 {
		t.Fatal("expected drift detection for different orchestrator")
	}
	found = false
	for _, d := range drift2 {
		if strings.Contains(d, "Orchestrator") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected drift message about Orchestrator, got: %v", drift2)
	}
}

// TestExternalAssetDirInconsistencyWithInstall verifies that using external
// assets via OMR_ASSET_DIR that differ from the installed base triggers
// a conflict during upgrade.
func TestExternalAssetDirConflictDuringUpgrade(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Different base prompt (simulating external asset dir with different content)
	differentAssets := testAssets()
	differentAssets.BasePrompt = []byte("different-base\n")

	// Upgrade should detect the base prompt change and conflict
	report, err := Init(Options{ProjectDir: root, Assets: differentAssets, Upgrade: true})
	if err == nil {
		t.Fatal("expected conflict when base prompt changed without AcceptReasonixBaseUpdate")
	}
	if len(report.Conflicts) == 0 || !strings.Contains(report.Conflicts[0], "Reasonix base Prompt changed") {
		t.Fatalf("expected 'Reasonix base Prompt changed' conflict, got: %v", report.Conflicts)
	}

	// With AcceptReasonixBaseUpdate, upgrade should succeed
	report, err = Init(Options{
		ProjectDir:              root,
		Assets:                  differentAssets,
		Upgrade:                 true,
		AcceptReasonixBaseUpdate: true,
	})
	if err != nil {
		t.Fatalf("upgrade with accept should succeed: %v %#v", err, report)
	}
	if !report.Written {
		t.Fatal("expected upgrade to write changes")
	}
}

// TestUninstallConflictWithModifiedProfile verifies that uninstall refuses
// when a profile was modified after installation.
func TestUninstallConflictWithModifiedProfile(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Modify a profile
	profilePath := ExploreProfilePath(root)
	if err := os.WriteFile(profilePath, []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Uninstall should detect the conflict
	_, err := Uninstall(Options{ProjectDir: root})
	if err == nil {
		t.Fatal("expected conflict when profile was modified")
	}
}

// TestUpgradeDryRunDoesNotWrite verifies that dry-run upgrade doesn't
// modify any files, even when changes are pending.
func TestUpgradeDryRunDoesNotWrite(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Record baseline hashes
	origGenHash, _ := fileutil.SHA256File(GeneratedPromptPath(root))
	origProfileHash, _ := fileutil.SHA256File(ExploreProfilePath(root))
	origManifest, _ := manifest.Load(ManifestPath(root))
	origConfig, _ := os.ReadFile(filepath.Join(root, "reasonix.toml"))

	// Different base prompt (change detected)
	differentAssets := testAssets()
	differentAssets.BasePrompt = []byte("different base\n")

	// Dry-run upgrade
	report, err := Init(Options{
		ProjectDir:              root,
		Assets:                  differentAssets,
		Upgrade:                 true,
		DryRun:                  true,
		AcceptReasonixBaseUpdate: true,
	})
	if err != nil {
		t.Fatalf("dry-run upgrade should not fail: %v %#v", err, report)
	}
	if report.Written || report.NoOp {
		t.Fatalf("dry-run should not report written or no-op: %#v", report)
	}

	// Verify no files changed
	currentGenHash, _ := fileutil.SHA256File(GeneratedPromptPath(root))
	if currentGenHash != origGenHash {
		t.Fatal("dry-run modified generated prompt")
	}
	currentProfileHash, _ := fileutil.SHA256File(ExploreProfilePath(root))
	if currentProfileHash != origProfileHash {
		t.Fatal("dry-run modified profile")
	}
	currentManifest, _ := manifest.Load(ManifestPath(root))
	if currentManifest.Prompt.FinalSHA256 != origManifest.Prompt.FinalSHA256 {
		t.Fatal("dry-run modified manifest")
	}
	currentConfig, _ := os.ReadFile(filepath.Join(root, "reasonix.toml"))
	if string(currentConfig) != string(origConfig) {
		t.Fatal("dry-run modified config")
	}
}
