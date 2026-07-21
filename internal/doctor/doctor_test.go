package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
)

func doctorAssets() install.Assets {
	return install.Assets{
		Root:         "test-assets",
		BasePrompt:   []byte("base\n"),
		Orchestrator: []byte("orchestrator\n"),
		Explore:      []byte("skill\n"),
		ReviewBrief:  []byte("review\n"),
	}
}

func doctorProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte("[agent]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := install.Init(install.Options{ProjectDir: root, Assets: doctorAssets()}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunPassesWithManifestAndWarnsWithoutReasonixPath(t *testing.T) {
	root := doctorProject(t)
	result, err := Run(root, doctorAssets())
	if err != nil {
		t.Fatalf("doctor: %v %#v", err, result)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected doctor errors: %#v", result.Errors)
	}
}

func TestRunRejectsGeneratedPromptDrift(t *testing.T) {
	root := doctorProject(t)
	path := install.GeneratedPromptPathForDoctor(root)
	if err := os.WriteFile(path, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 {
		t.Fatalf("expected drift error: %#v %v", result, err)
	}
}
