package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func testAssets() Assets {
	return Assets{
		Root:         "test-assets",
		BasePrompt:   []byte("base\n"),
		Orchestrator: []byte("orchestrator\n"),
		Explore:      []byte("skill\n"),
		Research:     []byte("research\n"),
		Debug:        []byte("debug\n"),
		Planner:      []byte("planner\n"),
		Frontend:     []byte("frontend\n"),
		ReviewBrief:  []byte("review\n"),
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
	if len(profiles) != 5 || profiles[0].ID != "omr-explore" || profiles[0].Path != ExploreProfileRel || profiles[1].ID != "omr-research" || profiles[1].Path != ResearchProfileRel || profiles[2].ID != "omr-debug" || profiles[2].Path != DebugProfileRel || profiles[3].ID != "omr-planner" || profiles[3].Path != PlannerProfileRel || profiles[4].ID != "omr-frontend" || profiles[4].Path != FrontendProfileRel {
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
