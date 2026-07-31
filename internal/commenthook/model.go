// Package commenthook manages the project-scoped Comment Checker Hook (T14).
// It does not modify global configuration or read Reasonix private state.
package commenthook

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// OMRDescription is the canonical ownership marker for OMR Hook entries.
const OMRDescription = "[oh-my-reasonix] Comment Checker before git commit"

// OMRCommandSuffix is the invariant suffix of the OMR Hook command.
// The full command is: <absolute-path> hook comment-check guard --project-dir .
const OMRCommandSuffix = " hook comment-check guard --project-dir ."

// OMRCommandLegacy is the legacy relative command for backward compatibility.
const OMRCommandLegacy = "omr hook comment-check guard --project-dir ."

// OMRCanonicalKeys is the set of field keys that OMR Hook entry must contain.
var OMRCanonicalKeys = map[string]bool{
	"match": true, "command": true, "description": true, "timeout": true,
}

// Entry represents a single Hook entry's canonical fields.
// Used ONLY for OMR ownership detection. User entries use RawEntry.
type Entry struct {
	Match       string `json:"match"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Timeout     int    `json:"timeout"`
}

// RawEntry holds the raw JSON bytes of a hook entry.
// This preserves all user fields (cwd, env, contextFile, etc.) that
// OMR does not know about.
type RawEntry struct {
	Raw []byte
}

// Entry decodes the raw bytes into an Entry struct.
// Returns a zero Entry and false if decoding fails (e.g. non-object).
func (r RawEntry) Entry() (Entry, bool) {
	var e Entry
	if err := json.Unmarshal(r.Raw, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// IsOMROwned returns true when this entry is exactly the legacy canonical
// OMR Hook. Use IsOMROwnedFor when validating a current absolute command.
// OMR-owned means:
//  1. The JSON object contains ONLY the 4 canonical fields.
//  2. Each field has the correct type.
//  3. description matches OMRDescription.
//  4. command matches the legacy relative command.
//  5. match is "bash".
//  6. timeout is 10000.
//
// This is the single source of truth for OMR ownership detection.
func (r RawEntry) IsOMROwned() bool {
	return rawIsOMROwned(r)
}

// IsOMROwnedFor returns true when this entry is either the exact legacy entry
// or the exact entry generated for omrPath.
func (r RawEntry) IsOMROwnedFor(omrPath string) bool {
	return rawIsOMROwnedFor(r, omrPath)
}

// HasOMRDescription returns true when this entry's description field
// matches the OMR marker. Inspects the raw JSON independently
// without requiring full Entry decoding.
func (r RawEntry) HasOMRDescription() bool {
	return rawHasOMRDescription(r)
}

// IsOMRCommand checks the legacy command retained for backward compatibility.
// Dynamic absolute commands require IsOMRCommandFor so arbitrary paths cannot
// claim OMR ownership.
func IsOMRCommand(cmd string) bool {
	return cmd == OMRCommandLegacy
}

// IsOMRCommandFor checks whether cmd is the exact legacy command or the exact
// command generated for the resolved OMR executable path.
func IsOMRCommandFor(cmd, omrPath string) bool {
	if IsOMRCommand(cmd) {
		return true
	}
	return omrPath != "" && cmd == BuildHookCommand(omrPath)
}

// BuildHookCommand builds the shell command stored in Reasonix settings.
// Safe paths stay unquoted for readability; other paths are quoted without
// allowing shell expansion.
func BuildHookCommand(omrPath string) string {
	if omrPath == "" {
		return OMRCommandLegacy
	}
	return shellQuoteExecutable(omrPath) + OMRCommandSuffix
}

func shellQuoteExecutable(path string) string {
	if shellSafeUnquoted(path) {
		return path
	}
	// The Hook runs under bash (match: "bash"), so single quotes are always
	// the correct POSIX quoting on every host platform. Double quotes would
	// allow $, ` and backslash expansion inside the path.
	return "'" + strings.ReplaceAll(path, "'", `'"'"'`) + "'"
}

func shellSafeUnquoted(path string) bool {
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("/._-", r):
		case runtime.GOOS == "windows" && strings.ContainsRune(`:\`, r):
		default:
			return false
		}
	}
	return path != ""
}

// rawIsOMROwned is the internal implementation of IsOMROwned.
func rawIsOMROwned(raw RawEntry) bool {
	return rawIsOMROwnedFor(raw, "")
}

func rawIsOMROwnedFor(raw RawEntry, omrPath string) bool {
	if rawIsOMROwnedCommand(raw, OMRCommandLegacy) {
		return true
	}
	if omrPath == "" {
		return false
	}
	return rawIsOMROwnedCommand(raw, BuildHookCommand(omrPath))
}

func rawIsOMROwnedCommand(raw RawEntry, expectedCommand string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return false
	}
	if len(m) != len(OMRCanonicalKeys) {
		return false
	}
	for k := range m {
		if !OMRCanonicalKeys[k] {
			return false
		}
	}
	match, _ := m["match"].(string)
	if match != "bash" {
		return false
	}
	cmd, _ := m["command"].(string)
	if cmd != expectedCommand {
		return false
	}
	desc, _ := m["description"].(string)
	if desc != OMRDescription {
		return false
	}
	tout, _ := m["timeout"].(float64)
	if tout != 10000 {
		return false
	}
	return true
}

// rawHasOMRDescription checks if a RawEntry has the OMR description marker.
func rawHasOMRDescription(raw RawEntry) bool {
	var m map[string]any
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return false
	}
	desc, ok := m["description"].(string)
	return ok && desc == OMRDescription
}

// Settings represents a parsed .reasonix/settings.json.
type Settings struct {
	Hooks   map[string][]RawEntry `json:"hooks,omitempty"`
	unknown map[string]any        // preserves unknown top-level fields
}

// ParseSettings parses a raw settings.json byte slice.
func ParseSettings(raw []byte) (*Settings, error) {
	s := &Settings{
		Hooks:   make(map[string][]RawEntry),
		unknown: make(map[string]any),
	}
	if len(raw) == 0 {
		return s, nil
	}

	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return nil, err
	}
	if rawMap == nil {
		return s, nil
	}

	for k, v := range rawMap {
		switch k {
		case "hooks":
			hooksMap, ok := v.(map[string]any)
			if !ok {
				return nil, &MergeError{Conflict: true, Detail: `"hooks" must be a JSON object`}
			}
			for eventKey, eventVal := range hooksMap {
				arr, ok := eventVal.([]any)
				if !ok {
					return nil, &MergeError{Conflict: true, Detail: `"` + eventKey + `" must be a JSON array`}
				}
				for _, item := range arr {
					itemMap, ok := item.(map[string]any)
					if !ok {
						return nil, &MergeError{Conflict: true, Detail: fmt.Sprintf("Hook entry in %q is not a JSON object", eventKey)}
					}
					rawEntry, err := json.Marshal(itemMap)
					if err != nil {
						return nil, &MergeError{Conflict: true, Detail: fmt.Sprintf("failed to marshal hook entry in %q", eventKey)}
					}
					s.Hooks[eventKey] = append(s.Hooks[eventKey], RawEntry{Raw: rawEntry})
				}
			}
		default:
			s.unknown[k] = v
		}
	}
	return s, nil
}

// GuardInput is the JSON payload Reasonix sends to Hook commands via stdin.
type GuardInput struct {
	Event    string          `json:"event"`
	ToolName string          `json:"toolName"`
	ToolArgs json.RawMessage `json:"toolArgs"`
	Cwd      string          `json:"cwd"`
}

// GetCommand extracts the bash command from ToolArgs.
func (g GuardInput) GetCommand() (string, error) {
	if len(g.ToolArgs) == 0 {
		return "", nil
	}
	var obj struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(g.ToolArgs, &obj); err == nil {
		if obj.Command != "" {
			return obj.Command, nil
		}
		return "", fmt.Errorf("no command in toolArgs")
	}
	var s string
	if err := json.Unmarshal(g.ToolArgs, &s); err == nil {
		return s, nil
	}
	return "", fmt.Errorf("unrecognized toolArgs format")
}

// GuardResult describes the guard outcome.
type GuardResult struct {
	Block    bool   `json:"block"`
	ExitCode int    `json:"exit_code"`
	Message  string `json:"message,omitempty"`
}
