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
