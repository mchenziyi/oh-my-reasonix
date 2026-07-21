package install

import (
	"testing"
)

func TestLoadAssetsFallsBackToEmbeddedReleaseAssets(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	if assets.Root != "embedded" {
		t.Fatalf("expected embedded asset source, got %q", assets.Root)
	}
	for name, data := range map[string][]byte{
		"base prompt":  assets.BasePrompt,
		"orchestrator": assets.Orchestrator,
		"explore":      assets.Explore,
		"review brief": assets.ReviewBrief,
	} {
		if len(data) == 0 {
			t.Errorf("embedded %s is empty", name)
		}
	}
}

func TestLoadAssetsInvalidConfiguredDirectoryDoesNotFallback(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", t.TempDir()+"/missing")
	if _, err := LoadAssets(t.TempDir()); err == nil {
		t.Fatal("expected invalid OMR_ASSET_DIR to fail")
	}
}
