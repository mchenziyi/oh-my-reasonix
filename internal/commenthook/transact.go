package commenthook

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

// HookSettingsRel is the relative path to the project-level settings.json.
const HookSettingsRel = ".reasonix/settings.json"

// SettingsPath returns the absolute path to settings.json for the project.
func SettingsPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(HookSettingsRel))
}

// HookBackupRel returns the relative backup directory for hook state.
const HookBackupRel = ".reasonix/omr/backups/hook"

// HookBackupPath returns the absolute backup directory.
func HookBackupPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(HookBackupRel))
}

// HookReport describes the result of an enable/disable operation.
type HookReport struct {
	Root       string   `json:"root"`
	Changes    []string `json:"changes"`
	Warnings   []string `json:"warnings"`
	Conflicts  []string `json:"conflicts"`
	Errors     []string `json:"errors"`
	NoOp       bool     `json:"noop"`
	Written    bool     `json:"written,omitempty"`
	ResultJSON string   `json:"result_json,omitempty"`
}

func (r *HookReport) Blocking() bool {
	return len(r.Conflicts) > 0 || len(r.Errors) > 0
}

// TransactionOptions controls the enable/disable transaction behaviour.
type TransactionOptions struct {
	ProjectDir      string
	DryRun          bool
	OmrNotAvailable bool   // no stable omr executable path could be resolved
	OmrCommand      string // resolved absolute path to omr binary; empty = use legacy
}

// EnableHook performs a transactional enable of the Comment Checker Hook.
// It follows the full write pipeline: preflight → backup → atomic write → manifest update → rollback on failure.
func EnableHook(opts TransactionOptions) (HookReport, error) {
	report := HookReport{Root: opts.ProjectDir}

	// Resolve project root.
	root, err := resolveProjectRoot(opts.ProjectDir)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	report.Root = root

	settingsPath := SettingsPath(root)
	manifestPath := manifestPathFor(root)

	// Preflight: check symlink safety for the settings path.
	if err := checkPathSafety(settingsPath, root); err != nil {
		report.Errors = append(report.Errors, "settings path safety check failed: "+err.Error())
		return report, fmt.Errorf("settings path safety: %w", err)
	}
	if err := checkPathSafety(manifestPath, root); err != nil {
		report.Errors = append(report.Errors, "manifest path safety check failed: "+err.Error())
		return report, fmt.Errorf("manifest path safety: %w", err)
	}

	// Preflight: check omr availability.
	if opts.OmrNotAvailable || opts.OmrCommand == "" {
		report.Conflicts = append(report.Conflicts, "stable omr executable path cannot be resolved; cannot enable Hook")
		return report, fmt.Errorf("omr not available")
	}
	omrPath, err := stableExecutablePath(opts.OmrCommand)
	if err != nil {
		report.Conflicts = append(report.Conflicts, "invalid omr executable path: "+err.Error())
		return report, fmt.Errorf("invalid omr executable path: %w", err)
	}
	opts.OmrCommand = omrPath

	// Read existing settings with proper error handling.
	// os.IsNotExist is treated as empty (not yet configured).
	// Other errors (permission, is-directory, I/O) must fail closed.
	raw, readErr := readSettingsFile(settingsPath)
	if readErr != nil {
		report.Errors = append(report.Errors, "read settings: "+readErr.Error())
		return report, fmt.Errorf("read settings: %w", readErr)
	}

	// Compute merge.
	enableOpts := &EnableOptions{
		OmrNotAvailable: opts.OmrNotAvailable,
		OmrCommand:      opts.OmrCommand,
	}
	mergeReport, err := EnableMerge(raw, enableOpts)
	if err != nil {
		var me *MergeError
		if asMerge(err, &me) && me.Conflict {
			report.Conflicts = append(report.Conflicts, me.Detail)
			return report, err
		}
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	if mergeReport.NoOp {
		m, hasManifest, err := loadManifest(manifestPath)
		if err != nil {
			report.Errors = append(report.Errors, "load manifest: "+err.Error())
			return report, err
		}
		if hasManifest {
			entryRaw := findOMREntryRaw(raw, opts.OmrCommand)
			if len(entryRaw) == 0 {
				err := fmt.Errorf("installed OMR Hook entry cannot be verified")
				report.Errors = append(report.Errors, err.Error())
				return report, err
			}
			hook := manifest.HookRecord{
				Enabled:             true,
				SettingsPath:        HookSettingsRel,
				Event:               "PreToolUse",
				Description:         OMRDescription,
				EntrySHA256:         sha256Hex(entryRaw),
				InstalledFileSHA256: sha256Hex(raw),
			}
			if m.Hook != nil {
				hook.BaseFileSHA256 = m.Hook.BaseFileSHA256
			}
			if m.Hook == nil || *m.Hook != hook {
				report.Changes = append(report.Changes, "UPDATE: manifest (repair hook record)")
				if opts.DryRun {
					return report, nil
				}
				m.Hook = &hook
				if err := manifest.Write(manifestPath, m); err != nil {
					report.Errors = append(report.Errors, "write manifest: "+err.Error())
					return report, err
				}
				report.Written = true
				return report, nil
			}
		}
		report.NoOp = true
		report.Changes = append(report.Changes, "NOOP: Hook already enabled")
		return report, nil
	}

	report.ResultJSON = mergeReport.ResultJSON
	report.Changes = append(report.Changes, "UPDATE: "+HookSettingsRel)

	// Compute SHA256 values for the installed content and immutable backup.
	newHash := sha256Hex([]byte(mergeReport.ResultJSON))
	baseHash := sha256Hex(raw)

	// Backup path.
	backupDir := HookBackupPath(root)
	backupFile := ""
	if len(raw) > 0 {
		backupFile = filepath.Join(backupDir, "settings.json."+baseHash[:12])
		if err := checkPathSafety(backupFile, root); err != nil {
			report.Errors = append(report.Errors, "backup path safety check failed: "+err.Error())
			return report, fmt.Errorf("backup path safety: %w", err)
		}
		backupRel := filepath.ToSlash(filepath.Join(HookBackupRel, "settings.json."+baseHash[:12]))
		report.Changes = append(report.Changes, "BACKUP: "+backupRel)
	}

	// Optionally update manifest if it exists.
	if _, err := os.Stat(manifestPath); err == nil {
		report.Changes = append(report.Changes, "UPDATE: manifest (hook record)")
	}

	if opts.DryRun {
		return report, nil
	}

	// Execute transaction.
	// Step 1: Create backup if old content exists.
	backupCreated := false
	if len(raw) > 0 {
		backupCreated, err = writeBackupExclusive(backupDir, backupFile, raw)
		if err != nil {
			report.Errors = append(report.Errors, "write backup: "+err.Error())
			return report, err
		}
	}

	// Step 2: Atomically write new settings.
	if err := fileutil.AtomicWrite(settingsPath, []byte(mergeReport.ResultJSON), 0o644); err != nil {
		rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
		report.Errors = append(report.Errors, "write settings: "+err.Error())
		return report, err
	}

	// Step 3: Optionally update manifest.
	m, hasManifest, err := loadManifest(manifestPath)
	if err != nil {
		rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
		report.Errors = append(report.Errors, "load manifest: "+err.Error())
		return report, err
	}

	if hasManifest {
		entryRaw := findOMREntryRaw([]byte(mergeReport.ResultJSON), opts.OmrCommand)
		if len(entryRaw) == 0 {
			rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
			err := fmt.Errorf("installed OMR Hook entry cannot be verified")
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}
		m.Hook = &manifest.HookRecord{
			Enabled:             true,
			SettingsPath:        HookSettingsRel,
			Event:               "PreToolUse",
			Description:         OMRDescription,
			EntrySHA256:         sha256Hex(entryRaw),
			BaseFileSHA256:      baseHash,
			InstalledFileSHA256: newHash,
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			// Rollback.
			rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
			report.Errors = append(report.Errors, "write manifest: "+err.Error())
			return report, err
		}
	}

	report.Written = true
	return report, nil
}

// DisableHook performs a transactional disable of the Comment Checker Hook.
func DisableHook(opts TransactionOptions) (HookReport, error) {
	report := HookReport{Root: opts.ProjectDir}

	root, err := resolveProjectRoot(opts.ProjectDir)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}
	report.Root = root

	settingsPath := SettingsPath(root)
	manifestPath := manifestPathFor(root)

	// Check symlink safety.
	if err := checkPathSafety(settingsPath, root); err != nil {
		report.Errors = append(report.Errors, "settings path safety check failed: "+err.Error())
		return report, fmt.Errorf("settings path safety: %w", err)
	}
	if err := checkPathSafety(manifestPath, root); err != nil {
		report.Errors = append(report.Errors, "manifest path safety check failed: "+err.Error())
		return report, fmt.Errorf("manifest path safety: %w", err)
	}

	raw, readErr := readSettingsFile(settingsPath)
	if readErr != nil {
		report.Errors = append(report.Errors, "read settings: "+readErr.Error())
		return report, fmt.Errorf("read settings: %w", readErr)
	}

	// Capture the description-marked entry's raw bytes before any removal so
	// the manifest can record the exact hash even when the current omr binary
	// is unresolvable (the removal itself is then authorized by the manifest).
	markerRaw := findMarkerRaw(raw)

	// Load the manifest first so a disabled binary still allows removal of
	// an entry whose hash matches the OMR ownership record. A corrupted
	// manifest must fail explicitly rather than producing a misleading
	// "entry modified" conflict.
	expectedHash := ""
	if m, hasManifest, loadErr := loadManifest(manifestPath); loadErr != nil {
		report.Errors = append(report.Errors, "load manifest: "+loadErr.Error())
		return report, fmt.Errorf("load manifest: %w", loadErr)
	} else if hasManifest && m.Hook != nil {
		expectedHash = m.Hook.EntrySHA256
	}

	mergeReport, err := DisableMerge(raw, &EnableOptions{
		OmrCommand:          opts.OmrCommand,
		ExpectedEntrySHA256: expectedHash,
	})
	if err != nil {
		var me *MergeError
		if asMerge(err, &me) && me.Conflict {
			report.Conflicts = append(report.Conflicts, me.Detail)
			return report, err
		}
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	if mergeReport.NoOp {
		m, hasManifest, err := loadManifest(manifestPath)
		if err != nil {
			report.Errors = append(report.Errors, "load manifest: "+err.Error())
			return report, err
		}
		if hasManifest && m.Hook != nil && m.Hook.Enabled {
			hook := *m.Hook
			hook.Enabled = false
			hook.SettingsPath = HookSettingsRel
			hook.Event = "PreToolUse"
			hook.Description = OMRDescription
			hook.BaseFileSHA256 = ""
			hook.InstalledFileSHA256 = ""
			report.Changes = append(report.Changes, "UPDATE: manifest (repair disabled hook record)")
			if opts.DryRun {
				return report, nil
			}
			m.Hook = &hook
			if err := manifest.Write(manifestPath, m); err != nil {
				report.Errors = append(report.Errors, "write manifest: "+err.Error())
				return report, err
			}
			report.Written = true
			return report, nil
		}
		report.NoOp = true
		report.Changes = append(report.Changes, "NOOP: Hook not enabled")
		return report, nil
	}

	report.ResultJSON = mergeReport.ResultJSON

	// Backup naming.
	baseHash := sha256Hex(raw)
	backupDir := HookBackupPath(root)
	backupFile := filepath.Join(backupDir, "settings.json."+baseHash[:12])
	if err := checkPathSafety(backupFile, root); err != nil {
		report.Errors = append(report.Errors, "backup path safety check failed: "+err.Error())
		return report, fmt.Errorf("backup path safety: %w", err)
	}
	backupRel := filepath.ToSlash(filepath.Join(HookBackupRel, "settings.json."+baseHash[:12]))

	report.Changes = append(report.Changes, "UPDATE: "+HookSettingsRel+" (remove OMR Hook)")
	report.Changes = append(report.Changes, "BACKUP: "+backupRel)

	// Optionally update manifest if it exists.
	if _, err := os.Stat(manifestPath); err == nil {
		report.Changes = append(report.Changes, "UPDATE: manifest (hook disabled)")
	}

	if opts.DryRun {
		return report, nil
	}

	// Execute transaction.
	backupCreated := false
	if len(raw) > 0 && mergeReport.ResultJSON != string(raw) {
		backupCreated, err = writeBackupExclusive(backupDir, backupFile, raw)
		if err != nil {
			report.Errors = append(report.Errors, "write backup: "+err.Error())
			return report, err
		}
	}

	if mergeReport.ResultJSON != "" {
		if err := fileutil.AtomicWrite(settingsPath, []byte(mergeReport.ResultJSON), 0o644); err != nil {
			rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
			report.Errors = append(report.Errors, "write settings: "+err.Error())
			return report, err
		}
	}

	// Optionally update manifest.
	m, hasManifest, err := loadManifest(manifestPath)
	if err != nil {
		rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
		report.Errors = append(report.Errors, "load manifest: "+err.Error())
		return report, err
	}

	if hasManifest {
		// Compute EntrySHA256 from the entry being removed. The marker was
		// captured before removal; fall back to the path-verified lookup only
		// when the marker could not be extracted (defensive).
		entryRaw := markerRaw
		if len(entryRaw) == 0 {
			entryRaw = findOMREntryRaw(raw, opts.OmrCommand)
		}
		if len(entryRaw) == 0 {
			rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
			err := fmt.Errorf("removed OMR Hook entry cannot be verified")
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}
		entryHash := sha256Hex(entryRaw)
		m.Hook = &manifest.HookRecord{
			Enabled:      false,
			SettingsPath: HookSettingsRel,
			Event:        "PreToolUse",
			Description:  OMRDescription,
			EntrySHA256:  entryHash,
		}
		if err := manifest.Write(manifestPath, m); err != nil {
			rollbackSettingsOnly(settingsPath, raw, backupCreated, backupFile, backupDir)
			report.Errors = append(report.Errors, "write manifest: "+err.Error())
			return report, err
		}
	}

	report.Written = true
	return report, nil
}

// rollbackSettingsOnly restores settings and cleans up backup on failure.
func rollbackSettingsOnly(settingsPath string, raw []byte, backupCreated bool, backupFile, backupDir string) {
	if len(raw) > 0 {
		_ = fileutil.AtomicWrite(settingsPath, raw, 0o644)
	} else {
		_ = os.Remove(settingsPath)
	}
	if backupCreated {
		_ = os.Remove(backupFile)
		_ = os.Remove(backupDir)
	}
}

func writeBackupExclusive(backupDir, backupFile string, data []byte) (bool, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return false, err
	}
	file, err := os.OpenFile(backupFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		existing, readErr := os.ReadFile(backupFile)
		if readErr != nil {
			return false, readErr
		}
		if !bytes.Equal(existing, data) {
			return false, fmt.Errorf("existing backup content does not match source settings")
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	created := true
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(backupFile)
		return false, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(backupFile)
		return false, err
	}
	return created, nil
}

// ResolveProjectRoot resolves and cleans the project directory.
func ResolveProjectRoot(dir string) (string, error) {
	return resolveProjectRoot(dir)
}

// ValidateManagedPath verifies that an OMR-managed path remains inside root.
func ValidateManagedPath(path, root string) error {
	return checkPathSafety(path, root)
}

// resolveProjectRoot resolves and cleans the project directory.
func resolveProjectRoot(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	return filepath.Abs(dir)
}

// manifestPathFor returns the manifest path for a project root.
func manifestPathFor(root string) string {
	return filepath.Join(root, ".reasonix", "omr", "manifest.lock.yaml")
}

// loadManifest loads the manifest from path. Returns (Manifest, true, nil) on success,
// (Manifest{}, false, nil) when the file does not exist.
func loadManifest(path string) (manifest.Manifest, bool, error) {
	m, err := manifest.Load(path)
	if err == nil {
		return m, true, nil
	}
	if os.IsNotExist(err) {
		return manifest.Manifest{}, false, nil
	}
	return manifest.Manifest{}, false, err
}

// checkPathSafety validates that target is lexically and physically contained
// by projectRoot. The nearest existing target component is resolved so the
// target file itself and any intermediate directory symlink are checked.
func checkPathSafety(target, projectRoot string) error {
	absProject, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(absProject, absTarget)
	if err != nil {
		return fmt.Errorf("cannot compute relative path: %w", err)
	}
	if pathEscapesRoot(rel) {
		return fmt.Errorf("path %q escapes project root %q", target, projectRoot)
	}
	if info, err := os.Lstat(absTarget); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed file %q must not be a symlink", target)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect managed file %q: %w", target, err)
	}

	resolvedProject, err := filepath.EvalSymlinks(absProject)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}

	existing := absTarget
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect path %q: %w", existing, err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return fmt.Errorf("cannot find existing ancestor for %q", target)
		}
		existing = parent
	}

	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", existing, err)
	}
	resolvedRel, err := filepath.Rel(resolvedProject, resolvedExisting)
	if err != nil {
		return fmt.Errorf("compare resolved path: %w", err)
	}
	if pathEscapesRoot(resolvedRel) {
		return fmt.Errorf("path resolves outside project root via symlink")
	}
	return nil
}

func pathEscapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

// readSettingsFile reads settings.json with proper error handling.
// os.IsNotExist returns (nil, nil) — treated as empty config.
// All other errors (permission, is-directory, I/O) are returned for fail-closed.
func readSettingsFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

// sha256Hex returns the hex-encoded SHA-256 of data.
func sha256Hex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// findMarkerRaw returns the raw bytes of the first entry carrying the OMR
// description marker, or nil if none exists.
func findMarkerRaw(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	settings, err := ParseSettings(raw)
	if err != nil {
		return nil
	}
	for _, re := range settings.Hooks["PreToolUse"] {
		if re.HasOMRDescription() {
			return re.Raw
		}
	}
	return nil
}

// findOMREntryRaw extracts the raw JSON bytes of the OMR entry from settings data.
// Returns nil if not found.
func findOMREntryRaw(raw []byte, omrPath string) []byte {
	if len(raw) == 0 {
		return nil
	}
	settings, err := ParseSettings(raw)
	if err != nil {
		return nil
	}
	for _, re := range settings.Hooks["PreToolUse"] {
		if rawIsOMROwnedFor(re, omrPath) {
			return re.Raw
		}
	}
	return nil
}

// ResolveOmrPath resolves the absolute path to the omr binary.
// Priority:
//  1. os.Executable() — the current running binary, unless it's a go run temp path.
//  2. exec.LookPath("omr") — stable install in PATH.
//
// Returns ("", error) if neither resolves to a valid non-temp binary.
func ResolveOmrPath() (string, error) {
	return resolveOmrPath(os.Executable, exec.LookPath)
}

func resolveOmrPath(
	executable func() (string, error),
	lookPath func(string) (string, error),
) (string, error) {
	if exe, err := executable(); err == nil {
		if path, err := stableExecutablePath(exe); err == nil {
			return path, nil
		}
	}
	if path, err := lookPath("omr"); err == nil {
		if path, err := stableExecutablePath(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("cannot resolve a stable omr executable path")
}

func stableExecutablePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := validateShellExecutablePath(abs, runtime.GOOS); err != nil {
		return "", err
	}
	for _, part := range strings.Split(filepath.ToSlash(abs), "/") {
		if strings.HasPrefix(part, "go-build") {
			return "", fmt.Errorf("temporary go run executable is not stable")
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("executable path is a directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("executable path is not executable")
	}
	return abs, nil
}

func validateShellExecutablePath(path, goos string) error {
	if strings.ContainsAny(path, "\x00\r\n") {
		return fmt.Errorf("executable path contains control characters")
	}
	if goos == "windows" && strings.ContainsAny(path, "%!") {
		return fmt.Errorf("executable path contains Windows command expansion characters")
	}
	return nil
}
