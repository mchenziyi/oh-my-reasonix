package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/install"
)

func doctorAssets() install.Assets {
	return install.Assets{
		Root:         "test-assets",
		BasePrompt:   []byte("base\n"),
		Orchestrator: []byte("orchestrator\n"),
		Explore:      []byte("skill\n"),
		Research:     []byte("research\n"),
		Debug:        []byte("debug\n"),
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

func TestRunRejectsInvalidOMRConfig(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[quality]\nmin_qualified_rate = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 {
		t.Fatalf("expected config error: %#v %v", result, err)
	}
}

func TestRunReportsValidOMRConfig(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[quality]\nmin_qualified_rate = 1\nmax_cost = 1\n[runtime]\nconcurrency = 2\n[routing]\nexplore = \"omr-explore\"\n[profiles]\ndisabled = \"omr-debug\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err != nil {
		t.Fatalf("doctor: %v %#v", err, result)
	}
	found, routing, concurrency, cost, disabled := false, false, false, false, false
	for _, check := range result.Checks {
		if check.Name == "omr.config" && check.Status == "PASS" {
			found = true
		}
		routing = routing || check.Name == "omr.config.routing"
		concurrency = concurrency || check.Name == "omr.config.concurrency"
		cost = cost || check.Name == "omr.config.max_cost"
		disabled = disabled || check.Name == "omr.config.disabled"
	}
	if !found || !routing || !concurrency || !cost || !disabled {
		t.Fatalf("valid config check missing: %#v", result.Checks)
	}
}

func TestRunRejectsCategoryForUninstalledProfile(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[routing]\nfrontend = \"missing-profile\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "category") {
		t.Fatalf("expected category installation error: %#v, err=%v", result.Errors, err)
	}
}

func TestRunRejectsCategoryForDisabledProfile(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[routing]\nexplore = \"omr-explore\"\n[profiles]\ndisabled = \"omr-explore\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "disabled Profile") {
		t.Fatalf("expected disabled routing error: %#v, err=%v", result.Errors, err)
	}
}

func TestRunSortsProfileConfigErrors(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	data := "[agent.z-profile]\nmodel = \"deepseek\"\n[agent.a-profile]\nmodel = \"deepseek\"\n[routing]\nz = \"z-profile\"\na = \"a-profile\"\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) < 4 {
		t.Fatalf("expected sorted profile errors: %#v, err=%v", result.Errors, err)
	}
	if !strings.Contains(result.Errors[0], "a-profile") || !strings.Contains(result.Errors[1], "z-profile") {
		t.Fatalf("profile errors are not sorted: %#v", result.Errors)
	}
}

func TestRunSortsDisabledProfileErrors(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[profiles]\ndisabled = \"z-profile, a-profile\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) < 2 {
		t.Fatalf("expected disabled profile errors: %#v, err=%v", result.Errors, err)
	}
	if !strings.Contains(result.Errors[0], "a-profile") || !strings.Contains(result.Errors[1], "z-profile") {
		t.Fatalf("disabled profile errors are not sorted: %#v", result.Errors)
	}
}

func TestSourceDriftMessageIncludesRemediation(t *testing.T) {
	if got := sourceDriftMessage("Reasonix base Prompt source hash changed"); !strings.Contains(got, "accept-reasonix-base-update") {
		t.Fatalf("missing base update remediation: %q", got)
	}
	if got := sourceDriftMessage("OMR Orchestrator Prompt source hash changed"); !strings.Contains(got, "omr upgrade") {
		t.Fatalf("missing orchestrator remediation: %q", got)
	}
}

func TestRunRejectsConfigForUninstalledProfile(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-missing]\nmodel = \"deepseek\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 {
		t.Fatalf("expected missing Profile error: %#v %v", result, err)
	}
	found := false
	for _, issue := range result.Errors {
		if issue == `OMR config references uninstalled Profile "omr-missing"` {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing Profile error not reported: %#v", result.Errors)
	}
}

func TestRunRejectsMissingAgentPromptFile(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	if err := os.WriteFile(path, []byte("[agent.omr-explore]\nprompt_file = \"prompts/missing.md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || len(result.Errors) == 0 {
		t.Fatalf("expected missing prompt file error: %#v %v", result, err)
	}
	found := false
	for _, issue := range result.Errors {
		if strings.HasPrefix(issue, "Prompt file for Profile \"omr-explore\"") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing prompt file error not reported: %#v", result.Errors)
	}
}

func TestProfileFrontmatterReadOnly(t *testing.T) {
	if !profileFrontmatterReadOnly("---\nread-only: true\n---\n") {
		t.Fatal("expected read-only frontmatter to be recognized")
	}
	if profileFrontmatterReadOnly("---\nread-only: false\n---\n") {
		t.Fatal("did not expect read-only false to be recognized")
	}
}
