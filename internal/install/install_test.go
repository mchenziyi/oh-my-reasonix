package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/commenthook"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func testAssets() Assets {
	return Assets{
		Root:          "test-assets",
		BasePrompt:    []byte("base\n"),
		Orchestrator:  []byte("orchestrator\n"),
		Explore:       []byte("skill\n"),
		Research:      []byte("research\n"),
		Debug:         []byte("debug\n"),
		Planner:       []byte("planner\n"),
		Frontend:      []byte("frontend\n"),
		ReviewBrief:   []byte("review\n"),
		GrillMe:       []byte("grill-me\n"),
		GrillWithDocs: []byte("grill-with-docs\n"),
	}
}

func newProject(t *testing.T, config string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUpgradeMigratesHookAndRefreshesManifestRecord(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	oldOmr := filepath.Join(root, "old-omr")
	if err := os.WriteFile(oldOmr, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: oldOmr,
	}); err != nil {
		t.Fatal(err)
	}
	newOmr := filepath.Join(root, "new-omr")
	if err := os.WriteFile(newOmr, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(Options{ProjectDir: root, Upgrade: true, HookCommand: newOmr, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	got, err := manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hook == nil || !got.Hook.Enabled || got.Hook.EntrySHA256 == "" {
		t.Fatalf("hook record was not refreshed: %#v", got.Hook)
	}
	settings, err := os.ReadFile(commenthook.SettingsPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), commenthook.BuildHookCommand(newOmr)) ||
		strings.Contains(string(settings), commenthook.BuildHookCommand(oldOmr)) {
		t.Fatalf("Hook command was not migrated: %s", settings)
	}
}

func TestInitIsIdempotentAndUninstallRestoresConfig(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	first, err := Init(Options{ProjectDir: root, Assets: assets})
	if err != nil {
		t.Fatalf("first init: %v %#v", err, first)
	}
	if !first.Written {
		t.Fatal("first init did not write")
	}
	second, err := Init(Options{ProjectDir: root, Assets: assets})
	if err != nil {
		t.Fatalf("second init: %v %#v", err, second)
	}
	if !second.NoOp {
		t.Fatalf("second init was not a no-op: %#v", second)
	}
	manifestData, err := manifest.Load(ManifestPath(root))
	if err != nil || manifestData.Prompt.FinalSHA256 == "" {
		t.Fatalf("manifest invalid: %v %#v", err, manifestData)
	}
	profiles := manifestData.NormalizedProfiles()
	if len(profiles) != 7 || profiles[0].ID != "omr-explore" || profiles[0].Path != ExploreProfileRel || profiles[1].ID != "omr-research" || profiles[1].Path != ResearchProfileRel || profiles[2].ID != "omr-debug" || profiles[2].Path != DebugProfileRel || profiles[3].ID != "omr-planner" || profiles[3].Path != PlannerProfileRel || profiles[4].ID != "omr-frontend" || profiles[4].Path != FrontendProfileRel || profiles[5].ID != "omr-grill-me" || profiles[5].Path != GrillMeProfileRel || profiles[6].ID != "omr-grill-with-docs" || profiles[6].Path != GrillWithDocsProfileRel {
		t.Fatalf("manifest profiles invalid: %#v", profiles)
	}
	if _, err := Uninstall(Options{ProjectDir: root}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(root, "reasonix.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(config) != "[agent]\nmodel = \"test\"\n" {
		t.Fatalf("config not restored: %q", config)
	}
	if _, err := os.Stat(ManifestPath(root)); !os.IsNotExist(err) {
		t.Fatalf("manifest still exists: %v", err)
	}
}

func TestUninstallRemovesOMRHookAndPreservesUserHook(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	settingsPath := commenthook.SettingsPath(root)
	if err := os.WriteFile(settingsPath, []byte(`{"hooks":{"PreToolUse":[{"match":"read","command":"user-hook","description":"user hook"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	omrPath := filepath.Join(root, "stable-omr")
	if err := os.WriteFile(omrPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: omrPath,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := Uninstall(Options{ProjectDir: root, HookCommand: omrPath}); err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settings), commenthook.OMRDescription) {
		t.Fatalf("OMR Hook remained after uninstall: %s", settings)
	}
	if !strings.Contains(string(settings), "user-hook") {
		t.Fatalf("user Hook was removed during uninstall: %s", settings)
	}
	if _, err := os.Stat(ManifestPath(root)); !os.IsNotExist(err) {
		t.Fatalf("Manifest should be removed: %v", err)
	}
}

func TestUpgradeBlocksModifiedHookWithoutWriting(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	omrPath := filepath.Join(root, "stable-omr")
	if err := os.WriteFile(omrPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: omrPath,
	}); err != nil {
		t.Fatal(err)
	}
	settingsPath := commenthook.SettingsPath(root)
	beforeManifest, err := os.ReadFile(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(
		string(mustReadFile(t, settingsPath)),
		`"timeout": 10000`,
		`"timeout": 9999`,
		1,
	)
	if err := os.WriteFile(settingsPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	upgraded := assets
	upgraded.Orchestrator = []byte("changed orchestrator\n")
	if _, err := Init(Options{
		ProjectDir:  root,
		Upgrade:     true,
		HookCommand: omrPath,
		Assets:      upgraded,
	}); err == nil {
		t.Fatal("expected modified Hook to block upgrade")
	}
	afterManifest, err := os.ReadFile(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterManifest) != string(beforeManifest) {
		t.Fatal("Manifest changed after blocked Hook upgrade")
	}
	if string(mustReadFile(t, settingsPath)) != tampered {
		t.Fatal("settings changed after blocked Hook upgrade")
	}
}

func TestUpgradeDryRunPlansHookMigrationWithoutWriting(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	oldOmr := filepath.Join(root, "old-omr")
	if err := os.WriteFile(oldOmr, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: oldOmr,
	}); err != nil {
		t.Fatal(err)
	}
	newOmr := filepath.Join(root, "new-omr")
	if err := os.WriteFile(newOmr, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	settingsPath := commenthook.SettingsPath(root)
	beforeSettings := mustReadFile(t, settingsPath)
	beforeManifest := mustReadFile(t, ManifestPath(root))
	report, err := Init(Options{
		ProjectDir:  root,
		Upgrade:     true,
		DryRun:      true,
		HookCommand: newOmr,
		Assets:      assets,
	})
	if err != nil {
		t.Fatalf("dry-run Hook migration: %v %#v", err, report)
	}
	if report.Written || report.NoOp {
		t.Fatalf("dry-run migration should only report a plan: %#v", report)
	}
	if string(mustReadFile(t, settingsPath)) != string(beforeSettings) {
		t.Fatal("dry-run migration changed Hook settings")
	}
	if string(mustReadFile(t, ManifestPath(root))) != string(beforeManifest) {
		t.Fatal("dry-run migration changed Manifest")
	}
}

func TestUninstallDryRunPreservesHookAndManifest(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	omrPath := filepath.Join(root, "stable-omr")
	if err := os.WriteFile(omrPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: omrPath,
	}); err != nil {
		t.Fatal(err)
	}

	settingsPath := commenthook.SettingsPath(root)
	beforeSettings := mustReadFile(t, settingsPath)
	beforeManifest := mustReadFile(t, ManifestPath(root))
	report, err := Uninstall(Options{
		ProjectDir:  root,
		DryRun:      true,
		HookCommand: omrPath,
	})
	if err != nil {
		t.Fatalf("dry-run uninstall: %v %#v", err, report)
	}
	if report.Written {
		t.Fatalf("dry-run uninstall reported a write: %#v", report)
	}
	if string(mustReadFile(t, settingsPath)) != string(beforeSettings) {
		t.Fatal("dry-run uninstall changed Hook settings")
	}
	if string(mustReadFile(t, ManifestPath(root))) != string(beforeManifest) {
		t.Fatal("dry-run uninstall changed Manifest")
	}
}

func TestUninstallBlocksModifiedHookWithoutWriting(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	omrPath := filepath.Join(root, "stable-omr")
	if err := os.WriteFile(omrPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := commenthook.EnableHook(commenthook.TransactionOptions{
		ProjectDir: root,
		OmrCommand: omrPath,
	}); err != nil {
		t.Fatal(err)
	}

	settingsPath := commenthook.SettingsPath(root)
	tampered := strings.Replace(
		string(mustReadFile(t, settingsPath)),
		`"timeout": 10000`,
		`"timeout": 9999`,
		1,
	)
	if err := os.WriteFile(settingsPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeManifest := mustReadFile(t, ManifestPath(root))
	beforePrompt := mustReadFile(t, GeneratedPromptPath(root))

	if _, err := Uninstall(Options{ProjectDir: root, HookCommand: omrPath}); err == nil {
		t.Fatal("expected modified Hook to block uninstall")
	}
	if string(mustReadFile(t, settingsPath)) != tampered {
		t.Fatal("blocked uninstall changed Hook settings")
	}
	if string(mustReadFile(t, ManifestPath(root))) != string(beforeManifest) {
		t.Fatal("blocked uninstall changed Manifest")
	}
	if string(mustReadFile(t, GeneratedPromptPath(root))) != string(beforePrompt) {
		t.Fatal("blocked uninstall changed generated Prompt")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestComposeRequiresPersistenceConfirmation(t *testing.T) {
	root := newProject(t, "[agent]\nsystem_prompt = \"user prompt\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, ComposePrompt: true, Assets: assets}); err == nil {
		t.Fatal("expected persistence confirmation conflict")
	}
	if _, err := Init(Options{ProjectDir: root, ComposePrompt: true, AllowPersistUserPrompt: true, Assets: assets}); err != nil {
		t.Fatalf("confirmed compose failed: %v", err)
	}
	generated, err := os.ReadFile(GeneratedPromptPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "user prompt") {
		t.Fatalf("user segment missing: %q", generated)
	}
}

func TestUpgradeComposesCategoryRouting(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}
	configDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[routing]\nfrontend = \"omr-frontend\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(Options{ProjectDir: root, Upgrade: true, Assets: assets}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	generated, err := os.ReadFile(GeneratedPromptPath(root))
	if err != nil || !strings.Contains(string(generated), "`frontend` → `omr-frontend`") {
		t.Fatalf("category routing missing: err=%v prompt=%q", err, generated)
	}
}

func TestUpgradeComposesEnabledMCPMetadataWithoutSecrets(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatalf("init: %v", err)
	}
	configDir := filepath.Join(root, ".reasonix", "omr")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "[mcp.docs]\ntransport = \"http\"\nurl = \"https://secret.example/mcp\"\ncapabilities = [\"docs\"]\nenabled = true\nenv = [\"DOCS_API_KEY\"]\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(Options{ProjectDir: root, Upgrade: true, Assets: assets}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	generated, err := os.ReadFile(GeneratedPromptPath(root))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(generated)
	if !strings.Contains(prompt, "Optional Project MCP") || !strings.Contains(prompt, "`docs`") {
		t.Fatalf("MCP guidance missing: %q", prompt)
	}
	if strings.Contains(prompt, "secret.example") || strings.Contains(prompt, "DOCS_API_KEY") {
		t.Fatalf("MCP prompt leaked endpoint or env name: %q", prompt)
	}
	installedManifest, err := manifest.Load(ManifestPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if drift := PromptSourceDrift(root, installedManifest, assets); len(drift) != 0 {
		t.Fatalf("dynamic MCP guidance changed static source hashes: %v", drift)
	}
}

func TestProfileCollisionDoesNotOverwrite(t *testing.T) {
	root := newProject(t, "[agent]\n")
	profilePath := ExploreProfilePath(root)
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, []byte("user-owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Init(Options{ProjectDir: root, Assets: testAssets()})
	if err == nil {
		t.Fatal("expected profile collision")
	}
	data, readErr := os.ReadFile(profilePath)
	if readErr != nil || string(data) != "user-owned\n" {
		t.Fatalf("profile overwritten: %q %v", data, readErr)
	}
}

func TestUninstallPreservesUserConfigChange(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	if _, err := Init(Options{ProjectDir: root, Assets: testAssets()}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "reasonix.toml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(config), "system_prompt_file = \".reasonix/omr/generated/system-prompt.md\"", "system_prompt_file = \"user/other.md\"", 1)
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(Options{ProjectDir: root}); err == nil {
		t.Fatal("expected conflict for modified owned key")
	}
	if actual, _ := fileutil.SHA256File(GeneratedPromptPath(root)); actual == "" {
		t.Fatal("generated file unexpectedly missing after blocked uninstall")
	}
}

func TestUpgradeRequiresManifestAndPreservesBaseline(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()
	if _, err := Init(Options{ProjectDir: root, Assets: assets}); err != nil {
		t.Fatal(err)
	}
	upgraded := assets
	upgraded.Orchestrator = []byte("orchestrator v2\n")
	report, err := Init(Options{ProjectDir: root, Assets: upgraded, Upgrade: true})
	if err != nil {
		t.Fatalf("upgrade: %v %#v", err, report)
	}
	if !report.Written || report.Manifest.Prompt.FinalSHA256 == "" {
		t.Fatalf("upgrade did not write a new manifest: %#v", report)
	}
	if _, err := Uninstall(Options{ProjectDir: root}); err != nil {
		t.Fatal(err)
	}
	rootWithoutManifest := newProject(t, "[agent]\n")
	if _, err := Init(Options{ProjectDir: rootWithoutManifest, Assets: assets, Upgrade: true}); err == nil {
		t.Fatal("expected upgrade without manifest to fail")
	}
}

func TestInitDryRunDoesNotWriteFiles(t *testing.T) {
	root := newProject(t, "[agent]\n")
	report, err := Init(Options{ProjectDir: root, Assets: testAssets(), DryRun: true})
	if err != nil {
		t.Fatalf("dry-run init: %v %#v", err, report)
	}
	if report.Written || report.NoOp {
		t.Fatalf("dry-run should not report a write or no-op: %#v", report)
	}
	if _, err := os.Stat(ManifestPath(root)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote manifest: %v", err)
	}
	if _, err := os.Stat(GeneratedPromptPath(root)); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote generated Prompt: %v", err)
	}
}

func TestInitDetectsOrphanedEventFile(t *testing.T) {
	root := newProject(t, "[agent]\nmodel = \"test\"\n")
	assets := testAssets()

	// First init creates the .reasonix/omr/ structure
	first, err := Init(Options{ProjectDir: root, Assets: assets})
	if err != nil {
		t.Fatalf("first init: %v %#v", err, first)
	}

	// Create orphan event file in the sessions directory
	sessionDir := filepath.Join(root, ".reasonix", "omr", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "orphan.event-index.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restore original config (first init set system_prompt_file)
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\nmodel = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Clean up other state to avoid prompt/profile conflicts
	os.Remove(GeneratedPromptPath(root))
	os.Remove(ManifestPath(root))
	os.RemoveAll(filepath.Join(root, ".reasonix", "skills"))

	// Second init should detect orphan event file
	report, err := Init(Options{ProjectDir: root, Assets: assets})
	if err == nil {
		t.Fatal("expected conflict from orphan event file")
	}
	foundOrphan := false
	for _, c := range report.Conflicts {
		if strings.Contains(c, "event-index") {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatalf("expected orphan event-index conflict, got: %v", report.Conflicts)
	}
}
