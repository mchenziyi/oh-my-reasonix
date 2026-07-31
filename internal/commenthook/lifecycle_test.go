package commenthook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func lifecycleExecutable(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "stable-omr")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeLifecycleSettings(t *testing.T, root, command string, userHook bool) []byte {
	t.Helper()
	user := ""
	if userHook {
		user = `{"match":"read","command":"user-hook","description":"user hook"},`
	}
	data := []byte(`{"custom":"value","hooks":{"PreToolUse":[` + user +
		`{"match":"bash","command":"` + command + `","description":"` + OMRDescription + `","timeout":10000}]}}`)
	path := SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPlanUpgradeLifecycleRepairsManifestForCurrentCommand(t *testing.T) {
	root := t.TempDir()
	path := lifecycleExecutable(t, root)
	writeLifecycleSettings(t, root, BuildHookCommand(path), false)

	plan, err := PlanUpgradeLifecycle(root, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SettingsChanged || !plan.RecordChanged || plan.Record == nil || !plan.Record.Enabled {
		t.Fatalf("unexpected lifecycle plan: %#v", plan)
	}
	if plan.Record.EntrySHA256 == "" || plan.Record.InstalledFileSHA256 == "" {
		t.Fatalf("missing manifest evidence: %#v", plan.Record)
	}
}

func TestPlanUpgradeLifecycleMigratesLegacyCommand(t *testing.T) {
	root := t.TempDir()
	path := lifecycleExecutable(t, root)
	writeLifecycleSettings(t, root, OMRCommandLegacy, false)

	plan, err := PlanUpgradeLifecycle(root, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SettingsChanged || plan.Action != "migrate" {
		t.Fatalf("expected migration plan, got %#v", plan)
	}
	if !strings.Contains(string(plan.After), BuildHookCommand(path)) {
		t.Fatalf("migrated command missing: %s", plan.After)
	}
}

func TestPlanUpgradeLifecycleMigratesManifestOwnedOldAbsolutePath(t *testing.T) {
	root := t.TempDir()
	current := lifecycleExecutable(t, root)
	old := filepath.Join(root, "old-omr")
	raw := writeLifecycleSettings(t, root, BuildHookCommand(old), false)
	parsed, err := ParseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := &manifest.HookRecord{
		Enabled:             true,
		SettingsPath:        HookSettingsRel,
		Event:               "PreToolUse",
		Description:         OMRDescription,
		EntrySHA256:         sha256Hex(parsed.Hooks["PreToolUse"][0].Raw),
		InstalledFileSHA256: sha256Hex(raw),
	}

	plan, err := PlanUpgradeLifecycle(root, current, record)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SettingsChanged || !strings.Contains(string(plan.After), BuildHookCommand(current)) {
		t.Fatalf("expected old absolute path migration, got %#v", plan)
	}
}

func TestPlanUpgradeLifecycleRejectsDriftedOldAbsolutePath(t *testing.T) {
	root := t.TempDir()
	current := lifecycleExecutable(t, root)
	writeLifecycleSettings(t, root, BuildHookCommand(filepath.Join(root, "old-omr")), false)
	record := &manifest.HookRecord{
		Enabled:             true,
		SettingsPath:        HookSettingsRel,
		Event:               "PreToolUse",
		Description:         OMRDescription,
		EntrySHA256:         strings.Repeat("a", 64),
		InstalledFileSHA256: strings.Repeat("b", 64),
	}
	if _, err := PlanUpgradeLifecycle(root, current, record); err == nil {
		t.Fatal("expected drifted old absolute command to block upgrade")
	}
}

func TestPlanUpgradeLifecycleRejectsDisabledRecordWithEntry(t *testing.T) {
	root := t.TempDir()
	path := lifecycleExecutable(t, root)
	writeLifecycleSettings(t, root, BuildHookCommand(path), false)
	record := &manifest.HookRecord{
		Enabled:             false,
		SettingsPath:        HookSettingsRel,
		Event:               "PreToolUse",
		Description:         OMRDescription,
		EntrySHA256:         strings.Repeat("a", 64),
		BaseFileSHA256:      "",
		InstalledFileSHA256: "",
	}
	if _, err := PlanUpgradeLifecycle(root, path, record); err == nil {
		t.Fatal("expected disabled record with settings entry to block upgrade")
	}
}

func TestPlanUninstallLifecycleRemovesOnlyOMREntry(t *testing.T) {
	root := t.TempDir()
	path := lifecycleExecutable(t, root)
	writeLifecycleSettings(t, root, BuildHookCommand(path), true)

	plan, err := PlanUninstallLifecycle(root, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SettingsChanged || strings.Contains(string(plan.After), OMRDescription) {
		t.Fatalf("OMR entry was not removed: %#v", plan)
	}
	if !strings.Contains(string(plan.After), "user-hook") || !strings.Contains(string(plan.After), `"custom": "value"`) {
		t.Fatalf("user settings were not preserved: %s", plan.After)
	}
}

func TestPlanLifecycleRejectsMultipleMarkers(t *testing.T) {
	root := t.TempDir()
	path := lifecycleExecutable(t, root)
	data := []byte(`{"hooks":{"PreToolUse":[` +
		`{"match":"bash","command":"` + BuildHookCommand(path) + `","description":"` + OMRDescription + `","timeout":10000},` +
		`{"match":"bash","command":"` + OMRCommandLegacy + `","description":"` + OMRDescription + `","timeout":10000}]}}`)
	settingsPath := SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanUpgradeLifecycle(root, path, nil); err == nil {
		t.Fatal("expected multiple markers to block upgrade")
	}
	if _, err := PlanUninstallLifecycle(root, path, nil); err == nil {
		t.Fatal("expected multiple markers to block uninstall")
	}
}
