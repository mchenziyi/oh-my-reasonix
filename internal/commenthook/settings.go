package commenthook

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

// MergeReport describes the result of an enable or disable merge operation.
type MergeReport struct {
	Conflict       bool   `json:"conflict,omitempty"`
	ConflictDetail string `json:"conflict_detail,omitempty"`
	NoOp           bool   `json:"noop,omitempty"`
	ResultJSON     string `json:"result_json,omitempty"`
}

// EnableOptions controls the enable merge behaviour.
type EnableOptions struct {
	OmrNotAvailable bool   // no stable omr executable path could be resolved
	OmrCommand      string // absolute path to omr binary, e.g. "/opt/homebrew/bin/omr"
	// ExpectedEntrySHA256, when set, allows DisableMerge to remove a
	// description-marked entry whose raw JSON hash matches the manifest
	// ownership record even when the current omr executable path differs
	// (e.g. the binary was moved or removed). This is the manifest-backed
	// proof that the entry was installed by OMR.
	ExpectedEntrySHA256 string
}

// MergeError is a conflict or parse error returned by merge operations.
type MergeError struct {
	Conflict bool
	Detail   string
	Err      error
}

func (e *MergeError) Error() string {
	if e.Err != nil {
		return e.Detail + ": " + e.Err.Error()
	}
	return e.Detail
}

func (e *MergeError) Unwrap() error { return e.Err }

// IsOMROwned returns true when entry matches the canonical OMR Hook pattern.
func IsOMROwned(entry Entry) bool {
	return entry.Match == "bash" &&
		IsOMRCommand(entry.Command) &&
		entry.Description == OMRDescription &&
		entry.Timeout == 10000
}

// IsOMROwnedFor returns true for the exact legacy entry or the exact entry
// generated for omrPath.
func IsOMROwnedFor(entry Entry, omrPath string) bool {
	return entry.Match == "bash" &&
		IsOMRCommandFor(entry.Command, omrPath) &&
		entry.Description == OMRDescription &&
		entry.Timeout == 10000
}

// EnableMerge computes the settings JSON that results from adding the OMR
// Hook entry. It never writes files.
func EnableMerge(raw []byte, opts *EnableOptions) (MergeReport, error) {
	if opts == nil {
		opts = &EnableOptions{}
	}
	if opts.OmrNotAvailable {
		return MergeReport{
			Conflict:       true,
			ConflictDetail: "stable omr executable path cannot be resolved; cannot enable Hook",
		}, &MergeError{Conflict: true, Detail: "stable omr executable path cannot be resolved; cannot enable Hook"}
	}

	settings, err := ParseSettings(raw)
	if err != nil {
		var me *MergeError
		if asMerge(err, &me) {
			return MergeReport{Conflict: true, ConflictDetail: err.Error()}, err
		}
		return MergeReport{Conflict: true, ConflictDetail: "invalid settings: " + err.Error()}, err
	}

	entries := settings.Hooks["PreToolUse"]
	expectedCommand := BuildHookCommand(opts.OmrCommand)

	markerIndexes := make([]int, 0, 1)
	for i, re := range entries {
		if rawHasOMRDescription(re) {
			markerIndexes = append(markerIndexes, i)
		}
	}
	if len(markerIndexes) > 1 {
		return MergeReport{
			Conflict:       true,
			ConflictDetail: "multiple OMR Hook entries found; manual resolution required",
		}, &MergeError{Conflict: true, Detail: "multiple OMR Hook entries found"}
	}
	if len(markerIndexes) == 1 {
		i := markerIndexes[0]
		re := entries[i]
		if rawIsOMROwnedCommand(re, expectedCommand) {
			return MergeReport{NoOp: true}, nil
		}
		if opts.OmrCommand != "" && rawIsOMROwnedCommand(re, OMRCommandLegacy) {
			entry := Entry{
				Match:       "bash",
				Command:     expectedCommand,
				Description: OMRDescription,
				Timeout:     10000,
			}
			entryRaw, err := json.Marshal(entry)
			if err != nil {
				return MergeReport{Conflict: true, ConflictDetail: "failed to marshal OMR entry"}, err
			}
			settings.Hooks["PreToolUse"][i] = RawEntry{Raw: entryRaw}
			result, err := serializeSettings(settings)
			if err != nil {
				return MergeReport{Conflict: true, ConflictDetail: "serialization error: " + err.Error()}, err
			}
			return MergeReport{ResultJSON: result}, nil
		}
		if !rawIsOMROwnedFor(re, opts.OmrCommand) {
			return MergeReport{
				Conflict:       true,
				ConflictDetail: fmt.Sprintf("OMR Hook entry with description %q exists but has different content; manual resolution required", OMRDescription),
			}, &MergeError{Conflict: true, Detail: "OMR Hook entry conflict"}
		}
	}

	// Build entry with resolved command.
	// If OmrCommand is set (absolute path), construct: <path> + suffix.
	// If empty, use the legacy relative command (backward compat).
	entry := Entry{
		Match:       "bash",
		Command:     expectedCommand,
		Description: OMRDescription,
		Timeout:     10000,
	}
	entryRaw, err := json.Marshal(entry)
	if err != nil {
		return MergeReport{Conflict: true, ConflictDetail: "failed to marshal OMR entry"}, err
	}
	settings.Hooks["PreToolUse"] = append(settings.Hooks["PreToolUse"], RawEntry{Raw: entryRaw})

	result, err := serializeSettings(settings)
	if err != nil {
		return MergeReport{Conflict: true, ConflictDetail: "serialization error: " + err.Error()}, err
	}
	return MergeReport{ResultJSON: result}, nil
}

// DisableMerge computes the settings JSON that results from removing the OMR
// Hook entry. It never writes files.
func DisableMerge(raw []byte, opts *EnableOptions) (MergeReport, error) {
	if opts == nil {
		opts = &EnableOptions{}
	}
	settings, err := ParseSettings(raw)
	if err != nil {
		var me *MergeError
		if asMerge(err, &me) {
			return MergeReport{Conflict: true, ConflictDetail: err.Error()}, err
		}
		return MergeReport{Conflict: true, ConflictDetail: "invalid settings: " + err.Error()}, err
	}

	entries := settings.Hooks["PreToolUse"]

	markerIndexes := make([]int, 0, 1)
	for i, re := range entries {
		if rawHasOMRDescription(re) {
			markerIndexes = append(markerIndexes, i)
		}
	}

	if len(markerIndexes) == 0 {
		return MergeReport{NoOp: true}, nil
	}
	if len(markerIndexes) > 1 {
		return MergeReport{
			Conflict:       true,
			ConflictDetail: "multiple OMR Hook entries found; cannot safely remove",
		}, &MergeError{Conflict: true, Detail: "multiple OMR Hook entries found"}
	}
	omrIndex := markerIndexes[0]
	if !rawIsOMROwnedFor(entries[omrIndex], opts.OmrCommand) {
		// Manifest-backed ownership: the entry hash matches what OMR recorded
		// at enable time, even if the current executable path differs (e.g. the
		// binary was moved or removed). This proves OMR installed the entry, so
		// it is safe to remove rather than a forged or tampered Hook.
		if opts.ExpectedEntrySHA256 != "" &&
			fileutil.SHA256(entries[omrIndex].Raw) == opts.ExpectedEntrySHA256 {
			// fall through to removal
		} else {
			return MergeReport{
				Conflict:       true,
				ConflictDetail: fmt.Sprintf("OMR Hook entry with description %q has been modified; cannot safely remove", OMRDescription),
			}, &MergeError{Conflict: true, Detail: "OMR Hook entry modified"}
		}
	}

	newEntries := make([]RawEntry, 0, len(entries)-1)
	for i, re := range entries {
		if i != omrIndex {
			newEntries = append(newEntries, re)
		}
	}

	if len(newEntries) == 0 {
		delete(settings.Hooks, "PreToolUse")
	} else {
		settings.Hooks["PreToolUse"] = newEntries
	}

	result, err := serializeSettings(settings)
	if err != nil {
		return MergeReport{Conflict: true, ConflictDetail: "serialization error: " + err.Error()}, err
	}
	return MergeReport{ResultJSON: result}, nil
}

// serializeSettings produces stable JSON that preserves all fields.
func serializeSettings(s *Settings) (string, error) {
	out := make(map[string]any)
	for k, v := range s.unknown {
		out[k] = v
	}

	if len(s.Hooks) > 0 {
		hooksMap := make(map[string]any)
		events := make([]string, 0, len(s.Hooks))
		for event := range s.Hooks {
			events = append(events, event)
		}
		sort.Strings(events)
		for _, event := range events {
			entries := s.Hooks[event]
			arr := make([]any, 0, len(entries))
			for _, re := range entries {
				if len(re.Raw) == 0 {
					arr = append(arr, map[string]any{})
					continue
				}
				var entryMap map[string]any
				if err := json.Unmarshal(re.Raw, &entryMap); err != nil {
					arr = append(arr, json.RawMessage(re.Raw))
					continue
				}
				arr = append(arr, entryMap)
			}
			hooksMap[event] = arr
		}
		out["hooks"] = hooksMap
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// OMRDescriptionMatch returns true if the description matches the OMR marker.
func OMRDescriptionMatch(desc string) bool {
	return desc == OMRDescription
}

// asMerge attempts to cast err to *MergeError.
func asMerge(err error, target **MergeError) bool {
	if err == nil {
		return false
	}
	me, ok := err.(*MergeError)
	if ok {
		*target = me
	}
	return ok
}

// IsGitCommit checks if a command string represents a direct git commit.
func IsGitCommit(cmd string) bool {
	args := strings.Fields(cmd)
	if len(args) == 0 {
		return false
	}
	if len(args) >= 2 && args[0] == "git" && args[1] == "commit" {
		return true
	}
	if len(args) >= 4 && args[0] == "git" && args[1] == "-C" && args[3] == "commit" {
		return true
	}
	return false
}

// FindOMREntry finds the OMR entry index in a raw entry slice.
func FindOMREntry(entries []RawEntry) int {
	return FindOMREntryFor(entries, "")
}

// FindOMREntryFor finds the legacy entry or the exact entry for omrPath.
func FindOMREntryFor(entries []RawEntry, omrPath string) int {
	for i, re := range entries {
		if rawIsOMROwnedFor(re, omrPath) {
			return i
		}
	}
	return -1
}

// HasOMREntry returns true if any entry is OMR-owned.
func HasOMREntry(entries []RawEntry) bool {
	return FindOMREntry(entries) >= 0
}

// MarshalJSON implements json.Marshaler for RawEntry.
func (r RawEntry) MarshalJSON() ([]byte, error) {
	return r.Raw, nil
}
