package commenthook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
)

// LifecyclePlan is a read-only upgrade or uninstall plan for Hook settings.
type LifecyclePlan struct {
	SettingsPath    string
	BackupPath      string
	Before          []byte
	BeforeExisted   bool
	After           []byte
	SettingsChanged bool
	Record          *manifest.HookRecord
	RecordChanged   bool
	Action          string
}

// PlanUpgradeLifecycle plans migration of an enabled Hook to currentOmrPath.
func PlanUpgradeLifecycle(root, currentOmrPath string, record *manifest.HookRecord) (LifecyclePlan, error) {
	plan, settings, marker, err := loadLifecycleState(root)
	if err != nil {
		return plan, err
	}
	if marker == nil {
		if record != nil && record.Enabled {
			return plan, fmt.Errorf("Manifest declares an enabled OMR Hook but settings contains no OMR entry")
		}
		plan.Record = cloneHookRecord(record)
		return plan, nil
	}
	if record != nil && !record.Enabled {
		return plan, fmt.Errorf("settings contains an OMR Hook but Manifest marks it disabled")
	}
	if !lifecycleEntryOwned(*marker, currentOmrPath, record) {
		return plan, fmt.Errorf("OMR Hook entry differs from its owned state; manual resolution required")
	}
	if currentOmrPath == "" {
		return plan, fmt.Errorf("stable omr executable path cannot be resolved for Hook upgrade")
	}
	if _, err := stableExecutablePath(currentOmrPath); err != nil {
		return plan, fmt.Errorf("invalid omr executable path: %w", err)
	}

	expectedCommand := BuildHookCommand(currentOmrPath)
	entry, _ := marker.Entry()
	if entry.Command != expectedCommand {
		entry.Command = expectedCommand
		entryRaw, err := json.Marshal(entry)
		if err != nil {
			return plan, err
		}
		replaceLifecycleMarker(settings, *marker, RawEntry{Raw: entryRaw})
		serialized, err := serializeSettings(settings)
		if err != nil {
			return plan, err
		}
		plan.After = []byte(serialized)
		plan.SettingsChanged = string(plan.After) != string(plan.Before)
		plan.Action = "migrate"
	} else {
		plan.After = append([]byte(nil), plan.Before...)
	}

	finalSettings, err := ParseSettings(plan.After)
	if err != nil {
		return plan, err
	}
	finalEntry := findLifecycleMarker(finalSettings)
	if finalEntry == nil || !finalEntry.IsOMROwnedFor(currentOmrPath) {
		return plan, fmt.Errorf("upgraded OMR Hook entry cannot be verified")
	}
	hook := &manifest.HookRecord{
		Enabled:             true,
		SettingsPath:        HookSettingsRel,
		Event:               "PreToolUse",
		Description:         OMRDescription,
		EntrySHA256:         sha256Hex(finalEntry.Raw),
		InstalledFileSHA256: sha256Hex(plan.After),
	}
	if plan.SettingsChanged {
		hook.BaseFileSHA256 = sha256Hex(plan.Before)
	} else if record != nil {
		hook.BaseFileSHA256 = record.BaseFileSHA256
	}
	plan.Record = hook
	plan.RecordChanged = !hookRecordsEqual(record, hook)
	if plan.Action == "" && plan.RecordChanged {
		plan.Action = "repair-manifest"
	}
	setLifecycleBackup(&plan, root)
	return plan, nil
}

// PlanUninstallLifecycle plans removal of the OMR-owned Hook entry.
func PlanUninstallLifecycle(root, currentOmrPath string, record *manifest.HookRecord) (LifecyclePlan, error) {
	plan, settings, marker, err := loadLifecycleState(root)
	if err != nil {
		return plan, err
	}
	if marker == nil {
		return plan, nil
	}
	if !lifecycleEntryOwned(*marker, currentOmrPath, record) {
		return plan, fmt.Errorf("OMR Hook entry was modified; uninstall cannot remove it safely")
	}

	entries := settings.Hooks["PreToolUse"]
	kept := make([]RawEntry, 0, len(entries)-1)
	for _, entry := range entries {
		if string(entry.Raw) != string(marker.Raw) {
			kept = append(kept, entry)
		}
	}
	if len(kept) == 0 {
		delete(settings.Hooks, "PreToolUse")
	} else {
		settings.Hooks["PreToolUse"] = kept
	}
	serialized, err := serializeSettings(settings)
	if err != nil {
		return plan, err
	}
	plan.After = []byte(serialized)
	plan.SettingsChanged = string(plan.After) != string(plan.Before)
	plan.Action = "remove"
	setLifecycleBackup(&plan, root)
	return plan, nil
}

// EnsureLifecycleBackup creates or verifies the immutable settings backup.
func EnsureLifecycleBackup(plan LifecyclePlan, root string) (bool, error) {
	if !plan.SettingsChanged || !plan.BeforeExisted || plan.BackupPath == "" {
		return false, nil
	}
	if err := checkPathSafety(plan.BackupPath, root); err != nil {
		return false, err
	}
	return writeBackupExclusive(filepath.Dir(plan.BackupPath), plan.BackupPath, plan.Before)
}

func loadLifecycleState(root string) (LifecyclePlan, *Settings, *RawEntry, error) {
	settingsPath := SettingsPath(root)
	plan := LifecyclePlan{SettingsPath: settingsPath}
	if err := checkPathSafety(settingsPath, root); err != nil {
		return plan, nil, nil, err
	}
	raw, err := readSettingsFile(settingsPath)
	if err != nil {
		return plan, nil, nil, err
	}
	plan.Before = append([]byte(nil), raw...)
	plan.BeforeExisted = raw != nil
	plan.After = append([]byte(nil), raw...)

	settings, err := ParseSettings(raw)
	if err != nil {
		return plan, nil, nil, err
	}
	var marker *RawEntry
	for event, entries := range settings.Hooks {
		for i := range entries {
			if !entries[i].HasOMRDescription() {
				continue
			}
			if event != "PreToolUse" {
				return plan, nil, nil, fmt.Errorf("OMR Hook marker exists under unexpected event %q", event)
			}
			if marker != nil {
				return plan, nil, nil, fmt.Errorf("multiple OMR Hook entries found")
			}
			copy := entries[i]
			marker = &copy
		}
	}
	return plan, settings, marker, nil
}

func lifecycleEntryOwned(entry RawEntry, currentOmrPath string, record *manifest.HookRecord) bool {
	if entry.IsOMROwnedFor(currentOmrPath) {
		return true
	}
	return record != nil &&
		record.EntrySHA256 != "" &&
		record.EntrySHA256 == sha256Hex(entry.Raw) &&
		rawHasCanonicalOMRShape(entry)
}

func rawHasCanonicalOMRShape(raw RawEntry) bool {
	var fields map[string]any
	if err := json.Unmarshal(raw.Raw, &fields); err != nil || len(fields) != len(OMRCanonicalKeys) {
		return false
	}
	for key := range fields {
		if !OMRCanonicalKeys[key] {
			return false
		}
	}
	entry, ok := raw.Entry()
	if !ok || entry.Match != "bash" || entry.Description != OMRDescription || entry.Timeout != 10000 {
		return false
	}
	return entry.Command == OMRCommandLegacy ||
		(strings.HasSuffix(entry.Command, OMRCommandSuffix) && strings.TrimSuffix(entry.Command, OMRCommandSuffix) != "")
}

func replaceLifecycleMarker(settings *Settings, old, replacement RawEntry) {
	entries := settings.Hooks["PreToolUse"]
	for i := range entries {
		if string(entries[i].Raw) == string(old.Raw) {
			entries[i] = replacement
			return
		}
	}
}

func findLifecycleMarker(settings *Settings) *RawEntry {
	for _, entry := range settings.Hooks["PreToolUse"] {
		if entry.HasOMRDescription() {
			copy := entry
			return &copy
		}
	}
	return nil
}

func setLifecycleBackup(plan *LifecyclePlan, root string) {
	if !plan.SettingsChanged || !plan.BeforeExisted || len(plan.Before) == 0 {
		return
	}
	plan.BackupPath = filepath.Join(HookBackupPath(root), "settings.json."+sha256Hex(plan.Before)[:12])
}

func cloneHookRecord(record *manifest.HookRecord) *manifest.HookRecord {
	if record == nil {
		return nil
	}
	copy := *record
	return &copy
}

func hookRecordsEqual(a, b *manifest.HookRecord) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// RemoveLifecycleBackup removes only a backup created by the active transaction.
func RemoveLifecycleBackup(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
