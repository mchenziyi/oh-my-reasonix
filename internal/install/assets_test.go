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
		"base prompt":     assets.BasePrompt,
		"orchestrator":    assets.Orchestrator,
		"explore":         assets.Explore,
		"research":        assets.Research,
		"debug":           assets.Debug,
		"review brief":    assets.ReviewBrief,
		"grill-me":        assets.GrillMe,
		"grill-with-docs": assets.GrillWithDocs,
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
		"skills/omr-grill-me/SKILL.md",
		"skills/omr-grill-with-docs/SKILL.md",
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
	if assets.Root != root || string(assets.Frontend) != "skills/omr-frontend/SKILL.md" || string(assets.ReviewBrief) != "prompts/review-task-protocol.zh.md" || string(assets.Git) != "skills/omr-git/SKILL.md" || string(assets.LSP) != "skills/omr-lsp/SKILL.md" || string(assets.GrillMe) != "skills/omr-grill-me/SKILL.md" || string(assets.GrillWithDocs) != "skills/omr-grill-with-docs/SKILL.md" {
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

// TestGrillMePromptContract_StopsWhenSufficient is a Prompt contract test:
// it verifies that the SKILL.md document declares the "information sufficient"
// stop condition — no more questions needed. It does NOT verify real Agent behavior.
func TestGrillMePromptContract_StopsWhenSufficient(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	skill := string(assets.GrillMe)
	if !strings.Contains(skill, "信息已充分，无需更多问题") {
		t.Fatal("Grill Me SKILL must declare stop condition: info sufficient")
	}
}

// TestGrillMePromptContract_StopsAtMaxRounds is a Prompt contract test:
// it verifies the SKILL.md declares a maximum-rounds stop condition (6 questions).
func TestGrillMePromptContract_StopsAtMaxRounds(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	skill := string(assets.GrillMe)
	if !strings.Contains(skill, "已问满 6 个问题") {
		t.Fatal("Grill Me SKILL must declare stop condition: max rounds (6)")
	}
}

// TestGrillMePromptContract_StopsOnUserRequest is a Prompt contract test:
// it verifies the SKILL.md declares the user-initiated stop condition.
func TestGrillMePromptContract_StopsOnUserRequest(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	skill := string(assets.GrillMe)
	if !strings.Contains(skill, "父任务指示停止") {
		t.Fatal("Grill Me SKILL must declare stop condition: user request")
	}
}

// TestGrillMePromptContract_NeverConfirmsUnverifiedAssumptions is a Prompt
// contract test: it verifies the SKILL.md hard-constraints section forbids
// placing unconfirmed assumptions in the output. It does NOT prove the Agent
// will obey at runtime.
func TestGrillMePromptContract_NeverConfirmsUnverifiedAssumptions(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	skill := string(assets.GrillMe)
	// The SKILL.md hard-constraints section forbids this behavior
	if !strings.Contains(skill, "未确认的假设写入") && !strings.Contains(skill, "assumptions_confirmed") {
		t.Fatal("Grill Me SKILL must forbid unverified assumptions in output")
	}
}

// TestGrillMePromptContract_NeverModifiesFiles is a Prompt contract test:
// it verifies the SKILL.md declares read-only constraints (no file modifications).
func TestGrillMePromptContract_NeverModifiesFiles(t *testing.T) {
	t.Setenv("OMR_ASSET_DIR", "")
	assets, err := LoadAssets(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	skill := string(assets.GrillMe)
	for _, required := range []string{"不修改文件", "不创建提交", "不运行写入命令"} {
		if !strings.Contains(skill, required) {
			t.Fatalf("Grill Me SKILL must forbid file modifications (missing: %s)", required)
		}
	}
	// Also verify frontmatter declares read-only
	if !strings.Contains(skill, "read-only: true") {
		t.Fatal("Grill Me SKILL frontmatter must declare read-only: true")
	}
}
