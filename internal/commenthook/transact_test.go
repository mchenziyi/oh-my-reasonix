package commenthook

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

func TestSettingsPath(t *testing.T) {
	root := "/tmp/test"
	got := SettingsPath(root)
	if !strings.HasSuffix(got, ".reasonix/settings.json") {
		t.Fatalf("expected .reasonix/settings.json suffix, got %s", got)
	}
}

func testTransactionOptions(t *testing.T, projectDir string) TransactionOptions {
	t.Helper()
	path := filepath.Join(projectDir, "test-omr")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return TransactionOptions{ProjectDir: projectDir, OmrCommand: path}
}

func TestResolveOmrPathUsesStableCurrentExecutableWithoutPATH(t *testing.T) {
	path := filepath.Join(t.TempDir(), "omr")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveOmrPath(
		func() (string, error) { return path, nil },
		func(string) (string, error) { return "", errors.New("not found") },
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %q, want %q", got, path)
	}
}

func TestResolveOmrPathRejectsGoRunTempAndMissingPATH(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go-build123", "exe", "omr")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := resolveOmrPath(
		func() (string, error) { return path, nil },
		func(string) (string, error) { return "", errors.New("not found") },
	)
	if err == nil {
		t.Fatal("expected resolution failure")
	}
}

func TestValidateShellExecutablePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		goos string
		ok   bool
	}{
		{name: "posix ordinary", path: "/opt/homebrew/bin/omr", goos: "darwin", ok: true},
		{name: "posix newline", path: "/tmp/omr\nnext", goos: "darwin"},
		{name: "windows ordinary", path: `C:\Program Files\OMR\omr.exe`, goos: "windows", ok: true},
		{name: "windows percent expansion", path: `C:\Users\%USERNAME%\omr.exe`, goos: "windows"},
		{name: "windows delayed expansion", path: `C:\Tools\!OMR!\omr.exe`, goos: "windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShellExecutablePath(tt.path, tt.goos)
			if tt.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected unsafe shell path to be rejected")
			}
		})
	}
}

func TestBuildHookCommandExecutesPathWithSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell quoting test")
	}
	dir := filepath.Join(t.TempDir(), "OMR's (Test)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "omr")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("sh", "-c", BuildHookCommand(path)).CombinedOutput(); err != nil {
		t.Fatalf("quoted hook command failed: %v: %s", err, output)
	}
}

func TestBuildHookCommandQuotesDollarAndBacktick(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell quoting test")
	}
	dir := filepath.Join(t.TempDir(), "$HOME `boom` dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "omr")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Single-quoting must keep $HOME and backticks literal: the command
	// must resolve the real path, not expand shell metacharacters.
	if output, err := exec.Command("sh", "-c", BuildHookCommand(path)).CombinedOutput(); err != nil {
		t.Fatalf("quoted hook command failed: %v: %s", err, output)
	}
}

func TestEnableHook_DryRun(t *testing.T) {
	dir := t.TempDir()
	opts := testTransactionOptions(t, dir)
	opts.DryRun = true
	report, err := EnableHook(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Blocking() {
		t.Fatalf("unexpected blocking: %#v", report.Conflicts)
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run on fresh project should show changes")
	}
	if len(report.Changes) == 0 {
		t.Fatal("dry-run should list changes")
	}
	// Verify no files were written.
	settingsPath := SettingsPath(dir)
	if _, err := os.Stat(settingsPath); err == nil {
		t.Fatal("dry-run should not create settings.json")
	}
}

func TestEnableHook_FreshProject(t *testing.T) {
	dir := t.TempDir()
	report, err := EnableHook(testTransactionOptions(t, dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Blocking() {
		t.Fatalf("unexpected blocking: %#v", report.Conflicts)
	}
	if !report.Written {
		t.Fatal("expected written")
	}
	// Verify settings.json was created.
	settingsPath := SettingsPath(dir)
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json should exist: %v", err)
	}
	if !strings.Contains(string(data), OMRDescription) {
		t.Fatal("settings.json should contain OMR description")
	}
}

func TestEnableHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	opts := testTransactionOptions(t, dir)
	report1, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !report1.Written {
		t.Fatal("first enable should write")
	}

	report2, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !report2.NoOp {
		t.Fatal("second enable should be NOOP")
	}
}

func TestEnableHookRepairsMissingManifestRecord(t *testing.T) {
	dir := t.TempDir()
	opts := testTransactionOptions(t, dir)
	if _, err := EnableHook(opts); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, dir, nil)

	report, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.NoOp || !report.Written {
		t.Fatalf("expected manifest repair, got %#v", report)
	}
	got, err := manifest.Load(manifestPathFor(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hook == nil || !got.Hook.Enabled || got.Hook.EntrySHA256 == "" {
		t.Fatalf("manifest Hook record was not repaired: %#v", got.Hook)
	}

	report, err = EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.NoOp {
		t.Fatalf("expected fully consistent state to be NOOP, got %#v", report)
	}
}

func TestEnableHook_PreservesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	customField := map[string]any{
		"custom_top_field": "value",
		"hooks": map[string]any{
			"PostToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user post hook",
				},
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(settingsPath, customField); err != nil {
		t.Fatal(err)
	}

	report, err := EnableHook(testTransactionOptions(t, dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Blocking() {
		t.Fatalf("unexpected blocking: %#v", report)
	}

	// Verify custom field is preserved.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "custom_top_field") {
		t.Fatal("custom field should be preserved")
	}
	if !strings.Contains(string(data), "PostToolUse") {
		t.Fatal("PostToolUse hooks should be preserved")
	}
	if !strings.Contains(string(data), OMRDescription) {
		t.Fatal("OMR entry should be added")
	}
}

func TestEnableHookDoesNotOverwriteExistingBackup(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\n  \"custom\": \"value\"\n}\n")
	if err := os.WriteFile(settingsPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	backupDir := HookBackupPath(dir)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(backupDir, "settings.json."+sha256Hex(raw)[:12])
	if err := os.WriteFile(backupFile, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnableHook(testTransactionOptions(t, dir)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("existing backup was overwritten: %q", got)
	}
}

func TestEnableHookRejectsMismatchedExistingBackup(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\n  \"custom\": \"value\"\n}\n")
	if err := os.WriteFile(settingsPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	backupDir := HookBackupPath(dir)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(backupDir, "settings.json."+sha256Hex(raw)[:12])
	if err := os.WriteFile(backupFile, []byte("different-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := EnableHook(testTransactionOptions(t, dir))
	if err == nil {
		t.Fatal("expected mismatched backup to block enable")
	}
	if !report.Blocking() {
		t.Fatalf("expected blocking report, got %#v", report)
	}
	got, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(raw) {
		t.Fatalf("settings changed after backup conflict: %q", got)
	}
}

func TestEnableHook_OmrNotAvailableBlocked(t *testing.T) {
	dir := t.TempDir()
	report, err := EnableHook(TransactionOptions{
		ProjectDir:      dir,
		OmrNotAvailable: true,
	})
	if err == nil {
		t.Fatal("expected error when omr not available")
	}
	if !report.Blocking() {
		t.Fatal("expected blocking")
	}
}

func TestEnableHookRejectsMissingExecutable(t *testing.T) {
	dir := t.TempDir()
	// OmrCommand points to a non-existent file — enable must fail explicitly.
	missing := filepath.Join(dir, "missing-omr")
	report, err := EnableHook(TransactionOptions{
		ProjectDir: dir,
		OmrCommand: missing,
	})
	if err == nil {
		t.Fatal("expected error when executable path does not exist")
	}
	if !report.Blocking() {
		t.Fatal("expected blocking")
	}
	// No settings.json must be written.
	settingsPath := SettingsPath(dir)
	if _, statErr := os.Stat(settingsPath); statErr == nil {
		t.Fatal("enable must not write settings when executable is missing")
	}
}

func TestDisableHook_DryRun(t *testing.T) {
	dir := t.TempDir()
	// First enable.
	opts := testTransactionOptions(t, dir)
	_, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}

	opts.DryRun = true
	report, err := DisableHook(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Written {
		t.Fatal("dry-run should not write")
	}
	if report.NoOp {
		t.Fatal("dry-run on enabled hook should show changes")
	}

	// Verify settings still has OMR entry.
	settingsPath := SettingsPath(dir)
	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), OMRDescription) {
		t.Fatal("dry-run should not remove OMR entry")
	}
}

func TestDisableHook_FreshProjectNoop(t *testing.T) {
	dir := t.TempDir()
	report, err := DisableHook(TransactionOptions{ProjectDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.NoOp {
		t.Fatal("disable on fresh project should be NOOP")
	}
}

func TestDisableHook_DisablesEnabledHook(t *testing.T) {
	dir := t.TempDir()
	// Enable.
	opts := testTransactionOptions(t, dir)
	_, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Disable.
	report, err := DisableHook(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp {
		t.Fatal("disable on enabled hook should change")
	}
	if !report.Written {
		t.Fatal("expected written")
	}

	// Verify OMR entry is removed.
	settingsPath := SettingsPath(dir)
	data, _ := os.ReadFile(settingsPath)
	if strings.Contains(string(data), OMRDescription) {
		t.Fatal("OMR entry should be removed after disable")
	}
}

func TestDisableHookRemovesAbsoluteEntryWhenBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	// Standard OMR installs always have a manifest; create a valid one so
	// EnableHook records the ownership evidence (EntrySHA256).
	writeValidManifest(t, dir)

	// Enable with an absolute executable path, writing a manifest record.
	opts := testTransactionOptions(t, dir)
	if _, err := EnableHook(opts); err != nil {
		t.Fatal(err)
	}

	// Simulate the omr binary becoming unresolvable (moved/removed): the CLI
	// passes OmrCommand="" on resolve failure. The manifest EntrySHA256 must
	// still allow removal of the OMR-installed absolute-path entry.
	report, err := DisableHook(TransactionOptions{ProjectDir: dir})
	if err != nil {
		t.Fatalf("disable must succeed via manifest ownership even when binary is missing: %v", err)
	}
	if report.NoOp {
		t.Fatal("disable on enabled hook should change")
	}
	if !report.Written {
		t.Fatal("expected written")
	}
	data, _ := os.ReadFile(SettingsPath(dir))
	if strings.Contains(string(data), OMRDescription) {
		t.Fatal("OMR entry should be removed after disable")
	}
}

func TestDisableHookRejectsTamperedEntryWhenBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	writeValidManifest(t, dir)

	// Enable with an absolute executable path.
	opts := testTransactionOptions(t, dir)
	if _, err := EnableHook(opts); err != nil {
		t.Fatal(err)
	}

	// Tamper with the installed OMR entry so its hash no longer matches the
	// manifest record (e.g. the command was rewritten by the user).
	settingsPath := SettingsPath(dir)
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), BuildHookCommand(opts.OmrCommand), "/tmp/evil/omr"+OMRCommandSuffix, 1)
	if tampered == string(data) {
		t.Fatal("tampering did not change the command")
	}
	if err := os.WriteFile(settingsPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	// With the binary missing (OmrCommand="") and the hash mismatched, disable
	// must fail closed and leave the tampered entry in place.
	report, err := DisableHook(TransactionOptions{ProjectDir: dir})
	if err == nil {
		t.Fatal("expected conflict for tampered entry with missing binary")
	}
	if !report.Blocking() {
		t.Fatal("expected blocking")
	}
	after, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(after), "/tmp/evil/omr") {
		t.Fatal("tampered entry must not be removed")
	}
}

// writeValidManifest creates a minimal manifest that passes Validate, so
// EnableHook can record the Hook ownership evidence like a standard install.
func writeValidManifest(t *testing.T, root string) {
	t.Helper()
	m := manifest.New()
	m.Prompt = manifest.Prompt{
		GeneratedPath: ".reasonix/omr/generated/system-prompt.md",
		FinalSHA256:   strings.Repeat("a", 64),
	}
	m.ProfilePath = ".reasonix/skills/omr-explore/SKILL.md"
	m.ProfileSHA256 = strings.Repeat("b", 64)
	if err := manifest.Write(manifestPathFor(root), m); err != nil {
		t.Fatal(err)
	}
}

func TestDisableHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// Disable on fresh project.
	report1, err := DisableHook(TransactionOptions{ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !report1.NoOp {
		t.Fatal("first disable on fresh should be NOOP")
	}
}

func TestDisableHookRepairsStaleManifestRecord(t *testing.T) {
	dir := t.TempDir()
	writeTestManifest(t, dir, &manifest.HookRecord{
		Enabled:             true,
		SettingsPath:        HookSettingsRel,
		Event:               "PreToolUse",
		Description:         OMRDescription,
		EntrySHA256:         strings.Repeat("a", 64),
		BaseFileSHA256:      strings.Repeat("b", 64),
		InstalledFileSHA256: strings.Repeat("c", 64),
	})

	report, err := DisableHook(TransactionOptions{ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if report.NoOp || !report.Written {
		t.Fatalf("expected manifest repair, got %#v", report)
	}
	got, err := manifest.Load(manifestPathFor(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hook == nil || got.Hook.Enabled {
		t.Fatalf("manifest Hook record was not disabled: %#v", got.Hook)
	}
	if got.Hook.EntrySHA256 != strings.Repeat("a", 64) {
		t.Fatalf("entry evidence should be preserved: %#v", got.Hook)
	}
	if got.Hook.BaseFileSHA256 != "" || got.Hook.InstalledFileSHA256 != "" {
		t.Fatalf("enabled-state file hashes should be cleared: %#v", got.Hook)
	}

	report, err = DisableHook(TransactionOptions{ProjectDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !report.NoOp {
		t.Fatalf("expected fully consistent disabled state to be NOOP, got %#v", report)
	}
}

func TestDisableHook_PreservesUserHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	initial := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user pre hook",
				},
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(settingsPath, initial); err != nil {
		t.Fatal(err)
	}

	// Enable then disable.
	opts := testTransactionOptions(t, dir)
	_, err := EnableHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	report, err := DisableHook(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp {
		t.Fatal("disable should change")
	}

	// Verify user hook is preserved.
	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "user-hook") {
		t.Fatal("user hook should be preserved")
	}
	if strings.Contains(string(data), OMRDescription) {
		t.Fatal("OMR entry should be removed")
	}
}

func TestCheckPathSafety_InsideProject(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	err := checkPathSafety(settingsPath, dir)
	if err != nil {
		t.Fatalf("expected inside path to be safe: %v", err)
	}
}

func TestCheckPathSafety_OutsideProject(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "..", "outside.json")
	err := checkPathSafety(outside, dir)
	if err == nil {
		t.Fatal("expected error for outside path")
	}
}

func TestCheckPathSafety_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()

	// Create a symlink inside the project that points outside.
	linkDir := filepath.Join(dir, "ext")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	// Check a path through the symlink.
	settingsPath := filepath.Join(linkDir, "settings.json")
	// The symlinked directory is outside the project root.
	// Resolve what EvalSymlinks will do: on macOS /tmp may resolve to /private/tmp
	// so we need to check if the link dir resolves outside.
	realDir, err := filepath.EvalSymlinks(linkDir)
	if err == nil {
		// Check if realDir is inside dir.
		rel, _ := filepath.Rel(dir, realDir)
		if strings.HasPrefix(rel, "..") {
			err := checkPathSafety(settingsPath, dir)
			if err == nil {
				t.Fatal("expected error for symlink escape")
			}
		} else {
			t.Log("symlink does not escape project root, skipping")
		}
	}
}

func TestCheckPathSafety_FileSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsPath := SettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, settingsPath); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	if err := checkPathSafety(settingsPath, dir); err == nil {
		t.Fatal("expected settings file symlink escape to be rejected")
	}
}

func TestEnableHookRejectsManifestDirectorySymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	omrDir := filepath.Join(dir, ".reasonix", "omr")
	if err := os.MkdirAll(filepath.Dir(omrDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, omrDir); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	report, err := EnableHook(testTransactionOptions(t, dir))
	if err == nil {
		t.Fatal("expected manifest path symlink escape to be rejected")
	}
	if !report.Blocking() {
		t.Fatalf("expected blocking report, got %#v", report)
	}
}

func TestEnableHookRejectsBackupDirectorySymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	settingsPath := SettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	backupDir := HookBackupPath(dir)
	if err := os.MkdirAll(filepath.Dir(backupDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, backupDir); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	report, err := EnableHook(testTransactionOptions(t, dir))
	if err == nil {
		t.Fatal("expected backup path symlink escape to be rejected")
	}
	if !report.Blocking() {
		t.Fatalf("expected blocking report, got %#v", report)
	}
}

func TestCheckPathSafety_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	// Create the .reasonix directory.
	reasonixDir := filepath.Join(dir, ".reasonix")
	if err := os.MkdirAll(reasonixDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := SettingsPath(dir)
	err := checkPathSafety(settingsPath, dir)
	if err != nil {
		t.Fatalf("expected existing dir to be safe: %v", err)
	}
}

// writeJSONFile writes a JSON object to a file.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTestManifest(t *testing.T, root string, hook *manifest.HookRecord) {
	t.Helper()
	m := manifest.New()
	m.Prompt.GeneratedPath = ".reasonix/omr/generated/system-prompt.md"
	m.Prompt.FinalSHA256 = "prompt-hash"
	m.Profiles = []manifest.Profile{{
		ID:            "omr-explore",
		Path:          ".reasonix/skills/omr-explore/SKILL.md",
		ContentSHA256: "profile-hash",
	}}
	m.Assets = []manifest.Asset{{
		ID:            "owned",
		LicenseStatus: "project-owned",
	}}
	m.Hook = hook
	if err := manifest.Write(manifestPathFor(root), m); err != nil {
		t.Fatal(err)
	}
}
