package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/commenthook"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
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

func TestDoctorRunBlocksHookEntryHashDrift(t *testing.T) {
	root := doctorProject(t)
	settings := []byte(`{"hooks":{"PreToolUse":[{"match":"bash","command":"` + commenthook.OMRCommandLegacy + `","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`)
	settingsPath := commenthook.SettingsPath(root)
	if err := os.WriteFile(settingsPath, settings, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(install.ManifestPathForDoctor(root))
	if err != nil {
		t.Fatal(err)
	}
	m.Hook = hookRecordForTest(t, settings, true)
	m.Hook.EntrySHA256 = strings.Repeat("f", 64)
	if err := manifest.Write(install.ManifestPathForDoctor(root), m); err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, doctorAssets())
	if err == nil || !result.Blocking() {
		t.Fatalf("expected blocking doctor result, got err=%v result=%#v", err, result)
	}
	found := false
	for _, detail := range result.Errors {
		found = found || strings.Contains(detail, "comment-hook")
	}
	if !found {
		t.Fatalf("comment Hook drift was not promoted to result.Errors: %#v", result.Errors)
	}
}

func TestDoctorRejectsSettingsFileSymlink(t *testing.T) {
	root := doctorProject(t)
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsPath := commenthook.SettingsPath(root)
	if err := os.Symlink(outside, settingsPath); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || !result.Blocking() {
		t.Fatalf("expected settings symlink to block doctor, got err=%v result=%#v", err, result)
	}
}

func TestDoctorRejectsManifestSymlink(t *testing.T) {
	root := doctorProject(t)
	manifestPath := install.ManifestPathForDoctor(root)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "manifest.lock.yaml")
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, manifestPath); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	result, err := Run(root, doctorAssets())
	if err == nil || !result.Blocking() {
		t.Fatalf("expected manifest symlink to block doctor, got err=%v result=%#v", err, result)
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

func TestRunReportsUnavailableMCPWithoutBlocking(t *testing.T) {
	root := doctorProject(t)
	path := filepath.Join(root, ".reasonix", "omr", "config.toml")
	data := "[mcp.docs]\ncommand = \"definitely-missing-omr-mcp\"\nenabled = true\nenv = [\"OMR_MISSING_DOCS_KEY\"]\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	result, err := Run(root, doctorAssets())
	if err != nil {
		t.Fatalf("optional MCP should not block doctor: %v %#v", err, result)
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "omr.config.mcp.docs" && check.Status == "WARN" &&
			strings.Contains(check.Detail, "command_not_in_path") &&
			strings.Contains(check.Detail, "MISSING_DOCS_KEY") {
			found = true
		}
	}
	if !found || len(result.Warnings) == 0 {
		t.Fatalf("missing MCP warning: %#v", result)
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

func TestResolveReasonixBinaryFromEnvironment(t *testing.T) {
	t.Setenv("OMR_REASONIX_BIN", "/bin/sh")
	got, err := resolveReasonixBinary()
	if err != nil || got != "/bin/sh" {
		t.Fatalf("unexpected configured Reasonix binary: %q, err=%v", got, err)
	}
}

func TestResolveReasonixBinaryRejectsMissingConfiguredPath(t *testing.T) {
	t.Setenv("OMR_REASONIX_BIN", filepath.Join(t.TempDir(), "missing-reasonix"))
	if _, err := resolveReasonixBinary(); err == nil {
		t.Fatal("expected missing configured Reasonix binary error")
	}
}

func TestResolveReasonixBinaryRejectsNonExecutableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reasonix")
	if err := os.WriteFile(path, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMR_REASONIX_BIN", path)
	if _, err := resolveReasonixBinary(); err == nil {
		t.Fatal("expected non-executable configured Reasonix binary error")
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

// TestHookCheckUnsupportedNotBlocking verifies that the reasonix.hooks
// UNSUPPORTED check does not produce errors or block the doctor.
func TestHookCheckUnsupportedNotBlocking(t *testing.T) {
	result := Result{
		Checks: []Check{
			{Name: "reasonix.hooks", Status: "UNSUPPORTED", Detail: "Reasonix 尚无 Hook 查询接口"},
		},
	}
	if result.Blocking() {
		t.Fatal("UNSUPPORTED hook check should not block the doctor")
	}
	if len(result.Errors) > 0 {
		t.Fatal("UNSUPPORTED hook check should not produce errors")
	}
}

// TestHookCheckWarnNotBlocking verifies that the reasonix.hooks WARN
// check does not produce errors or block the doctor.
func TestHookCheckWarnNotBlocking(t *testing.T) {
	result := Result{
		Checks: []Check{
			{Name: "reasonix.hooks", Status: "WARN", Detail: "存在 hooks.yaml 但宿主不支持"},
		},
	}
	if result.Blocking() {
		t.Fatal("WARN hook check should not block the doctor")
	}
	if len(result.Errors) > 0 {
		t.Fatal("WARN hook check should not produce errors")
	}
}

func TestCommentHookDiagnosticWarnsForLegacyCommand(t *testing.T) {
	root := t.TempDir()
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"hooks":{"PreToolUse":[{"match":"bash","command":"` + commenthook.OMRCommandLegacy + `","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	check := commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "/opt/homebrew/bin/omr", nil, hookRecordForTest(t, []byte(data), true))
	if check.Status != "WARN" || !strings.Contains(check.Detail, "PATH") {
		t.Fatalf("expected legacy migration warning, got %#v", check)
	}
}

func TestCommentHookDiagnosticDetectsExecutablePathDrift(t *testing.T) {
	root := t.TempDir()
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"hooks":{"PreToolUse":[{"match":"bash","command":"/old/path/omr hook comment-check guard --project-dir .","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	check := commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "/new/path/omr", nil, nil)
	if check.Status != "ERROR" {
		t.Fatalf("expected path drift error, got %#v", check)
	}
}

func TestCommentHookDiagnosticReportsUnresolvableExecutableBeforeDrift(t *testing.T) {
	root := t.TempDir()
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// An absolute-path entry whose current omr binary cannot be resolved must
	// be reported as an unresolvable executable, not as a drifted entry.
	data := `{"hooks":{"PreToolUse":[{"match":"bash","command":"/old/path/omr hook comment-check guard --project-dir .","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	check := commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "", errors.New("omr binary moved"), nil)
	if check.Status != "ERROR" || !strings.Contains(check.Detail, "无法解析") {
		t.Fatalf("expected unresolvable executable error, got %#v", check)
	}
	if strings.Contains(check.Detail, "被修改") {
		t.Fatalf("must not misreport as drift when the executable is unresolvable: %#v", check)
	}
}

func TestCommentHookDiagnosticRequiresManifestOwnership(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"hooks":{"PreToolUse":[{"match":"bash","command":"/opt/homebrew/bin/omr hook comment-check guard --project-dir .","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`)
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	check := commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "/opt/homebrew/bin/omr", nil, nil)
	if check.Status != "ERROR" || !strings.Contains(check.Detail, "Manifest") {
		t.Fatalf("expected missing Manifest ownership error, got %#v", check)
	}

	record := hookRecordForTest(t, data, false)
	check = commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "/opt/homebrew/bin/omr", nil, record)
	if check.Status != "ERROR" || !strings.Contains(check.Detail, "禁用") {
		t.Fatalf("expected disabled Manifest ownership error, got %#v", check)
	}
}

func TestCommentHookDiagnosticChecksManifestHashes(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"hooks":{"PreToolUse":[{"match":"bash","command":"/opt/homebrew/bin/omr hook comment-check guard --project-dir .","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`)
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	record := hookRecordForTest(t, data, true)
	record.EntrySHA256 = strings.Repeat("f", 64)
	check := commentHookDiagnosticWithExecutable(root, true, reasonix.HookListOutput{}, "/opt/homebrew/bin/omr", nil, record)
	if check.Status != "ERROR" || !strings.Contains(check.Detail, "条目哈希") {
		t.Fatalf("expected entry hash error, got %#v", check)
	}
}

func TestCommentHookDiagnosticPassesWithMatchingManifestAndProjectHook(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"hooks":{"PreToolUse":[{"match":"bash","command":"/opt/homebrew/bin/omr hook comment-check guard --project-dir .","description":"` + commenthook.OMRDescription + `","timeout":10000}]}}`)
	path := commenthook.SettingsPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	hooks := reasonix.HookListOutput{Hooks: []reasonix.HookInfo{{
		Event: "PreToolUse", Match: "bash", Scope: "project", Status: "active",
	}}}
	check := commentHookDiagnosticWithExecutable(root, true, hooks, "/opt/homebrew/bin/omr", nil, hookRecordForTest(t, data, true))
	if check.Status != "PASS" {
		t.Fatalf("expected PASS, got %#v", check)
	}
}

func hookRecordForTest(t *testing.T, settings []byte, enabled bool) *manifest.HookRecord {
	t.Helper()
	parsed, err := commenthook.ParseSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	entries := parsed.Hooks["PreToolUse"]
	if len(entries) != 1 {
		t.Fatalf("expected one Hook entry, got %d", len(entries))
	}
	return &manifest.HookRecord{
		Enabled:             enabled,
		SettingsPath:        commenthook.HookSettingsRel,
		Event:               "PreToolUse",
		Description:         commenthook.OMRDescription,
		EntrySHA256:         fileutil.SHA256(entries[0].Raw),
		InstalledFileSHA256: fileutil.SHA256(settings),
	}
}

func TestRunReportsMissingProfileMetadata(t *testing.T) {
	root := doctorProject(t)
	result, err := Run(root, doctorAssets())
	if err != nil {
		t.Fatalf("doctor: %v %#v", err, result)
	}
	// Doctor should produce warnings (not errors) for minimal profiles
	found := false
	for _, w := range result.Warnings {
		if len(w) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected warnings for minimal profile metadata")
	}
}
