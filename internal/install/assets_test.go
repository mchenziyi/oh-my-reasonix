package install

import (
	"os"
	"path/filepath"
	"strings"
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
		"research":     assets.Research,
		"debug":        assets.Debug,
		"review brief": assets.ReviewBrief,
	} {
		if len(data) == 0 {
			t.Errorf("embedded %s is empty", name)
		}
	}
}

func TestLoadAssetsFromExternalDirectory(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"prompts/reasonix-base-464d494.md",
		"prompts/orchestrator.zh.md",
		"prompts/review-task-protocol.zh.md",
		"skills/omr-explore/SKILL.md",
		"skills/omr-research/SKILL.md",
		"skills/omr-debug/SKILL.md",
		"skills/omr-planner/SKILL.md",
		"skills/omr-frontend/SKILL.md",
		"skills/omr-git/SKILL.md",
		"skills/omr-lsp/SKILL.md",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("OMR_ASSET_DIR", root)
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	if assets.Root != root || string(assets.Frontend) != "skills/omr-frontend/SKILL.md" || string(assets.ReviewBrief) != "prompts/review-task-protocol.zh.md" || string(assets.Git) != "skills/omr-git/SKILL.md" || string(assets.LSP) != "skills/omr-lsp/SKILL.md" {
		t.Fatalf("unexpected external assets: %#v", assets)
	}
}

func TestEmbeddedOrchestratorInjectsProjectRules(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	orchestrator := string(assets.Orchestrator)
	for _, required := range []string{"AGENTS.md", "README.md", ".reasonix/rules", ".claude/rules"} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator does not mention %s", required)
		}
	}
}

func TestEmbeddedOrchestratorRoutesReadOnlyProfiles(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	orchestrator := string(assets.Orchestrator)
	for _, required := range []string{"omr-explore", "omr-research", "omr-debug", "omr-planner", "omr-frontend", "任务类别路由", "delivery", "complete_step", "review", "verification.command"} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator does not route %s", required)
		}
	}
}

func TestEmbeddedOrchestratorConstrainsToolOutputAndContext(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatalf("LoadAssets: %v", err)
	}
	orchestrator := string(assets.Orchestrator)
	for _, required := range []string{"超大 grep", "上下文窗口", "最后一次验证命令"} {
		if !strings.Contains(orchestrator, required) {
			t.Fatalf("orchestrator does not include context discipline %q", required)
		}
	}
}

func TestEmbeddedReviewProtocolUsesReviewEvidence(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	protocol := string(assets.ReviewBrief)
	for _, required := range []string{"complete_step", "review", "verification.command", "task(profile=\"review\")"} {
		if !strings.Contains(protocol, required) {
			t.Fatalf("review protocol does not mention %s", required)
		}
	}
}

func TestLoadAssetsInvalidConfiguredDirectoryDoesNotFallback(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", t.TempDir()+"/missing")
	if _, err := LoadAssets(t.TempDir()); err == nil {
		t.Fatal("expected invalid OMR_ASSET_DIR to fail")
	}
}
