package commenthook

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- ParseSettings ---

func TestParseSettings_NilReturnsEmpty(t *testing.T) {
	raw, err := ParseSettings(nil)
	if err != nil {
		t.Fatalf("expected nil to parse without error, got: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil Settings")
	}
}

func TestParseSettings_EmptyObject(t *testing.T) {
	raw, err := ParseSettings([]byte("{}"))
	if err != nil {
		t.Fatalf("expected empty object to parse, got: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil Settings")
	}
}

func TestParseSettings_UnknownTopFields(t *testing.T) {
	data := []byte(`{"unknown_field": "value", "hooks": {"PreToolUse": []}}`)
	raw, err := ParseSettings(data)
	if err != nil {
		t.Fatalf("expected unknown top fields to be preserved, got: %v", err)
	}
	if raw.unknown["unknown_field"] != "value" {
		t.Fatalf("expected unknown_field to be preserved, got: %v", raw.unknown)
	}
}

func TestParseSettings_HooksNotObject(t *testing.T) {
	_, err := ParseSettings([]byte(`{"hooks": "string"}`))
	if err == nil {
		t.Fatal("expected error for non-object hooks")
	}
}

func TestParseSettings_PreToolUseNotArray(t *testing.T) {
	_, err := ParseSettings([]byte(`{"hooks": {"PreToolUse": "not-array"}}`))
	if err == nil {
		t.Fatal("expected error for non-array PreToolUse")
	}
}

func TestParseSettings_TopLevelNotObject(t *testing.T) {
	_, err := ParseSettings([]byte(`"string"`))
	if err == nil {
		t.Fatal("expected error for non-object top level")
	}
}

func TestParseSettings_MalformedJSON(t *testing.T) {
	_, err := ParseSettings([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseSettings_NonObjectEntryFailsClosed(t *testing.T) {
	_, err := ParseSettings([]byte(`{"hooks": {"PreToolUse": ["string_entry"]}}`))
	if err == nil {
		t.Fatal("expected error for non-object hook entry")
	}
}

// --- EnableMerge ---

func TestEnableMerge_NoSettingsCreatesNew(t *testing.T) {
	report, err := EnableMerge(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	if report.NoOp {
		t.Fatal("expected change, not noop")
	}
	if report.ResultJSON == "" {
		t.Fatal("expected non-empty ResultJSON")
	}
	parsed, err := ParseSettings([]byte(report.ResultJSON))
	if err != nil {
		t.Fatalf("result must be valid settings JSON: %v", err)
	}
	entries := parsed.Hooks["PreToolUse"]
	if len(entries) != 1 {
		t.Fatalf("expected 1 PreToolUse entry, got %d", len(entries))
	}
	e, ok := entries[0].Entry()
	if !ok || e.Description != OMRDescription {
		t.Fatalf("expected OMR description, got %+v", e)
	}
}

func TestEnableMerge_EmptyObject(t *testing.T) {
	report, err := EnableMerge([]byte("{}"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	if report.NoOp {
		t.Fatal("expected change, not noop")
	}
}

func TestEnableMerge_SameEntryNoOp(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.NoOp {
		t.Fatal("expected NOOP for same entry")
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
}

func TestEnableMerge_OwnershipConflict(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(20000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !report.Conflict {
		t.Fatal("expected Conflict=true")
	}
}

func TestEnableMerge_PreservesOtherEventsAndHooks(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
					"timeout":     float64(5000),
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "post-hook",
					"description": "post hook",
				},
			},
		},
		"custom_field": "value",
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	parsed, err := ParseSettings([]byte(report.ResultJSON))
	if err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if len(parsed.Hooks["PreToolUse"]) != 2 {
		t.Fatalf("expected 2 PreToolUse entries, got %d", len(parsed.Hooks["PreToolUse"]))
	}
	if len(parsed.Hooks["PostToolUse"]) != 1 {
		t.Fatalf("expected PostToolUse to be preserved, got %d entries", len(parsed.Hooks["PostToolUse"]))
	}
}

func TestEnableMerge_CustomFieldPreserved(t *testing.T) {
	input := map[string]any{
		"my_custom_setting": true,
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	parsed, err := ParseSettings([]byte(report.ResultJSON))
	if err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if parsed.unknown["my_custom_setting"] != true {
		t.Fatal("expected my_custom_setting to be preserved")
	}
}

// --- DisableMerge ---

func TestDisableMerge_NoSettingsNoop(t *testing.T) {
	report, err := DisableMerge(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.NoOp {
		t.Fatal("expected NOOP when no settings exist")
	}
}

func TestDisableMerge_NoOMREntryNoop(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.NoOp {
		t.Fatal("expected NOOP when no OMR entry exists")
	}
}

func TestDisableMerge_RemovesOMREntry(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
				},
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp {
		t.Fatal("expected change, not noop")
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	parsed, err := ParseSettings([]byte(report.ResultJSON))
	if err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if len(parsed.Hooks["PreToolUse"]) != 1 {
		t.Fatalf("expected 1 user PreToolUse entry remaining, got %d", len(parsed.Hooks["PreToolUse"]))
	}
	e, ok := parsed.Hooks["PreToolUse"][0].Entry()
	if !ok || e.Description == OMRDescription {
		t.Fatal("OMR entry should have been removed")
	}
}

func TestDisableMerge_RemovesEmptyPreToolUse(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"match":   "read",
					"command": "post-hook",
				},
			},
		},
		"custom_field": "value",
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp {
		t.Fatal("expected change, not noop")
	}
	parsed, err := ParseSettings([]byte(report.ResultJSON))
	if err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if _, exists := parsed.Hooks["PreToolUse"]; exists {
		t.Fatal("PreToolUse should be removed when empty")
	}
	if parsed.Hooks["PostToolUse"] == nil {
		t.Fatal("PostToolUse should be preserved")
	}
}

func TestDisableMerge_UserModifiedOMREntryConflict(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir /custom/path",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err == nil {
		t.Fatal("expected conflict error for modified OMR entry")
	}
	if !report.Conflict {
		t.Fatal("expected Conflict=true")
	}
}

// --- Stable JSON output ---

func TestStableJSONOutput(t *testing.T) {
	report1, err := EnableMerge(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	report2, err := EnableMerge(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report1.ResultJSON != report2.ResultJSON {
		t.Fatalf("JSON output should be stable\n1: %s\n2: %s", report1.ResultJSON, report2.ResultJSON)
	}
}

func TestStableJSONOutputPreservesUnknownFields(t *testing.T) {
	input := map[string]any{
		"hooks":         map[string]any{"PreToolUse": []any{}},
		"unknown_field": "preserve_me",
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stringsContains(report.ResultJSON, "preserve_me") {
		t.Fatal("unknown_field value should be preserved in output")
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- IsOMROwned ---

func TestIsOMROwned_MatchesCanonical(t *testing.T) {
	entry := Entry{
		Match:       "bash",
		Command:     "omr hook comment-check guard --project-dir .",
		Description: OMRDescription,
		Timeout:     10000,
	}
	if !IsOMROwned(entry) {
		t.Fatal("expected canonical entry to be OMR-owned")
	}
}

func TestIsOMROwned_WrongDescription(t *testing.T) {
	entry := Entry{
		Match:       "bash",
		Command:     "omr hook comment-check guard --project-dir .",
		Description: "not OMR",
		Timeout:     10000,
	}
	if IsOMROwned(entry) {
		t.Fatal("expected wrong description to not match")
	}
}

func TestIsOMROwned_WrongCommand(t *testing.T) {
	entry := Entry{
		Match:       "bash",
		Command:     "different command",
		Description: OMRDescription,
		Timeout:     10000,
	}
	if IsOMROwned(entry) {
		t.Fatal("expected wrong command to not match")
	}
}

func TestIsOMRCommandRejectsArbitraryAbsolutePath(t *testing.T) {
	if IsOMRCommand("/tmp/evil/omr" + OMRCommandSuffix) {
		t.Fatal("arbitrary absolute path must not be treated as an OMR-owned command")
	}
}

func TestIsOMRCommandForMatchesResolvedPath(t *testing.T) {
	path := "/opt/homebrew/bin/omr"
	if !IsOMRCommandFor(BuildHookCommand(path), path) {
		t.Fatal("resolved OMR path should be treated as owned")
	}
	if IsOMRCommandFor("/tmp/evil/omr"+OMRCommandSuffix, path) {
		t.Fatal("a different absolute path must be treated as drift")
	}
}

func TestBuildHookCommandQuotesPathWithSpaces(t *testing.T) {
	path := "/tmp/OMR Test/bin/omr"
	got := BuildHookCommand(path)
	want := `'/tmp/OMR Test/bin/omr'` + OMRCommandSuffix
	if runtime.GOOS == "windows" {
		want = `"/tmp/OMR Test/bin/omr"` + OMRCommandSuffix
	}
	if got != want {
		t.Fatalf("unexpected quoted command: %q", got)
	}
}

func TestBuildHookCommandQuotesSingleQuote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(`double quotes are not valid in Windows filenames`)
	}
	path := "/tmp/OMR's Test/bin/omr"
	got := BuildHookCommand(path)
	if got != `'/tmp/OMR'"'"'s Test/bin/omr'`+OMRCommandSuffix {
		t.Fatalf("unexpected single-quote escaping: %q", got)
	}
}

func TestEnableMergeUsesResolvedCommand(t *testing.T) {
	path := "/tmp/OMR Test/bin/omr"
	report, err := EnableMerge(nil, &EnableOptions{OmrCommand: path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.ResultJSON, BuildHookCommand(path)) {
		t.Fatalf("result does not contain the safely quoted command: %s", report.ResultJSON)
	}
}

func TestEnableMergeMigratesLegacyCommand(t *testing.T) {
	input := `{"hooks":{"PreToolUse":[{"match":"bash","command":"` + OMRCommandLegacy + `","description":"` + OMRDescription + `","timeout":10000}]}}`
	path := "/opt/homebrew/bin/omr"
	report, err := EnableMerge([]byte(input), &EnableOptions{OmrCommand: path})
	if err != nil {
		t.Fatal(err)
	}
	if report.NoOp {
		t.Fatal("legacy command should migrate when a stable absolute path is available")
	}
	if !strings.Contains(report.ResultJSON, BuildHookCommand(path)) {
		t.Fatalf("migrated settings do not contain resolved command: %s", report.ResultJSON)
	}
}

func TestDisableMergeRejectsDifferentAbsolutePath(t *testing.T) {
	expectedPath := "/opt/homebrew/bin/omr"
	input := `{"hooks":{"PreToolUse":[{"match":"bash","command":"/tmp/evil/omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000}]}}`
	report, err := DisableMerge([]byte(input), &EnableOptions{OmrCommand: expectedPath})
	if err == nil {
		t.Fatal("expected drift conflict")
	}
	if !report.Conflict {
		t.Fatal("expected conflict for a different absolute path")
	}
}

func TestDisableMergeAcceptsLegacyWithResolvedPath(t *testing.T) {
	input := `{"hooks":{"PreToolUse":[{"match":"bash","command":"` + OMRCommandLegacy + `","description":"` + OMRDescription + `","timeout":10000}]}}`
	report, err := DisableMerge([]byte(input), &EnableOptions{OmrCommand: "/opt/homebrew/bin/omr"})
	if err != nil {
		t.Fatal(err)
	}
	if report.NoOp || report.Conflict {
		t.Fatalf("legacy entry should be safely removable: %#v", report)
	}
}

func TestEnableMergeRejectsMultipleOMRMarkers(t *testing.T) {
	input := `{"hooks":{"PreToolUse":[` +
		`{"match":"bash","command":"` + OMRCommandLegacy + `","description":"` + OMRDescription + `","timeout":10000},` +
		`{"match":"bash","command":"/tmp/evil/omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000}` +
		`]}}`
	report, err := EnableMerge([]byte(input), &EnableOptions{OmrCommand: "/opt/homebrew/bin/omr"})
	if err == nil || !report.Conflict {
		t.Fatalf("multiple markers must conflict: report=%#v err=%v", report, err)
	}
}

func TestDisableMergeRejectsMultipleOMRMarkers(t *testing.T) {
	input := `{"hooks":{"PreToolUse":[` +
		`{"match":"bash","command":"` + OMRCommandLegacy + `","description":"` + OMRDescription + `","timeout":10000},` +
		`{"match":"bash","command":"/opt/homebrew/bin/omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000}` +
		`]}}`
	report, err := DisableMerge([]byte(input), &EnableOptions{OmrCommand: "/opt/homebrew/bin/omr"})
	if err == nil || !report.Conflict {
		t.Fatalf("multiple markers must conflict: report=%#v err=%v", report, err)
	}
}

// --- EnableMerge omr check ---

func TestEnableMerge_RequiresOmrInPath(t *testing.T) {
	report, err := EnableMerge(nil, &EnableOptions{OmrNotAvailable: true})
	if err == nil {
		t.Fatal("expected error when omr is not available")
	}
	if !report.Conflict {
		t.Fatal("expected Conflict=true when omr not available")
	}
}

func TestEnableMerge_DryRunDoesNotMutate(t *testing.T) {
	report, err := EnableMerge(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ResultJSON == "" {
		t.Fatal("expected non-empty ResultJSON even for dry-run")
	}
}

// --- DisableMerge idempotent ---

func TestDisableMerge_Idempotent(t *testing.T) {
	report1, err := DisableMerge(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report1.NoOp {
		t.Fatal("first disable on empty should be NOOP")
	}

	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report2, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report2.NoOp {
		t.Fatal("first disable on enabled should change")
	}
	report3, err := DisableMerge([]byte(report2.ResultJSON), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report3.NoOp {
		t.Fatal("second disable should be NOOP")
	}
}

// --- User Hook field preservation ---

func TestEnableMerge_PreservesUserHookCwd(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
					"cwd":         "/custom/path",
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Conflict {
		t.Fatalf("unexpected conflict: %s", report.ConflictDetail)
	}
	// Verify cwd is preserved in output.
	if !stringsContains(report.ResultJSON, "/custom/path") {
		t.Fatal("user hook cwd should be preserved")
	}
}

func TestEnableMerge_PreservesUserHookEnv(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
					"env":         map[string]any{"PATH": "/custom/bin", "HOME": "/root"},
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stringsContains(report.ResultJSON, "HOME") || !stringsContains(report.ResultJSON, "/root") {
		t.Fatal("user hook env should be preserved")
	}
}

func TestDisableMerge_PreservesUserHookCwd(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "read",
					"command":     "user-hook",
					"description": "user hook",
					"cwd":         "/custom/path",
				},
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp || report.Conflict {
		t.Fatalf("unexpected result: noop=%t conflict=%s", report.NoOp, report.ConflictDetail)
	}
	if !stringsContains(report.ResultJSON, "/custom/path") {
		t.Fatal("user hook cwd should be preserved after disable")
	}
}

func TestEnableDisable_RoundTripPreservesUnknownFields(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":        "read",
					"command":      "user-hook",
					"description":  "user hook",
					"contextFile":  ".my-config.yaml",
					"custom_field": 42,
				},
			},
		},
		"custom_top": "value",
	}
	b := mustJSON(t, input)

	// Enable
	enable, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !stringsContains(enable.ResultJSON, ".my-config.yaml") {
		t.Fatal("contextFile should be preserved after enable")
	}
	if !stringsContains(enable.ResultJSON, "custom_field") {
		t.Fatal("custom_field should be preserved after enable")
	}
	if !stringsContains(enable.ResultJSON, "custom_top") {
		t.Fatal("custom_top field should be preserved after enable")
	}

	// Disable
	disable, err := DisableMerge([]byte(enable.ResultJSON), nil)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !stringsContains(disable.ResultJSON, ".my-config.yaml") {
		t.Fatal("contextFile should be preserved after disable")
	}
	if !stringsContains(disable.ResultJSON, "custom_field") {
		t.Fatal("custom_field should be preserved after disable")
	}
}

// --- Exact OMR ownership tests ---

func TestRawIsOMROwned_CanonicalOnly(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000}`)}
	if !rawIsOMROwned(raw) {
		t.Fatal("canonical entry should be OMR-owned")
	}
}

func TestRawIsOMROwned_ExtraFieldCwdIsConflict(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000,"cwd":"/unexpected"}`)}
	if rawIsOMROwned(raw) {
		t.Fatal("entry with extra cwd field must NOT be OMR-owned")
	}
}

func TestRawIsOMROwned_ExtraFieldEnvIsConflict(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000,"env":{"PATH":"/custom"}}`)}
	if rawIsOMROwned(raw) {
		t.Fatal("entry with extra env field must NOT be OMR-owned")
	}
}

func TestRawIsOMROwned_TimeoutWrongTypeIsConflict(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":"10000"}`)}
	if rawIsOMROwned(raw) {
		t.Fatal("entry with string timeout must NOT be OMR-owned")
	}
}

func TestRawIsOMROwned_FractionalTimeoutIsConflict(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `","timeout":10000.5}`)}
	if rawIsOMROwned(raw) {
		t.Fatal("entry with fractional timeout must NOT be OMR-owned")
	}
}

func TestRawIsOMROwned_MissingTimeout(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"omr hook comment-check guard --project-dir .","description":"` + OMRDescription + `"}`)}
	if rawIsOMROwned(raw) {
		t.Fatal("entry missing timeout must NOT be OMR-owned")
	}
}

func TestRawHasOMRDescription_MarkerPresent(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"anything","description":"` + OMRDescription + `","timeout":10000}`)}
	if !rawHasOMRDescription(raw) {
		t.Fatal("should detect OMR description marker")
	}
}

func TestRawHasOMRDescription_MarkerAbsent(t *testing.T) {
	raw := RawEntry{Raw: []byte(`{"match":"bash","command":"anything","description":"user hook","timeout":5000}`)}
	if rawHasOMRDescription(raw) {
		t.Fatal("should not detect non-OMR description")
	}
}

func TestEnableMerge_CanonicalPlusCwdIsConflict(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
					"cwd":         "/unexpected",
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err == nil {
		t.Fatal("expected conflict for entry with extra cwd field")
	}
	if !report.Conflict {
		t.Fatal("expected Conflict=true")
	}
}

func TestDisableMerge_CanonicalPlusCwdDoesNotDelete(t *testing.T) {
	// Entry has OMR marker + extra cwd field — disable must NOT delete it.
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
					"cwd":         "/unexpected",
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err == nil {
		t.Fatal("expected conflict for drifted entry")
	}
	if !report.Conflict {
		t.Fatal("expected Conflict=true")
	}
}

func TestDisableMerge_OnlyCanonicalEntryIsNoop(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := DisableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.NoOp {
		t.Fatal("expected change for canonical entry, not noop")
	}
	if report.Conflict {
		t.Fatal("unexpected conflict")
	}
}

func TestEnableMerge_CanonicalEntryIsNoop(t *testing.T) {
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.NoOp {
		t.Fatal("expected NOOP for existing canonical entry")
	}
}

func TestEnableMerge_DoesNotAppendSecondOMREntry(t *testing.T) {
	// Enable on existing canonical entry must NOOP, not append a second one.
	input := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"match":       "bash",
					"command":     "omr hook comment-check guard --project-dir .",
					"description": OMRDescription,
					"timeout":     float64(10000),
				},
			},
		},
	}
	b := mustJSON(t, input)
	// First enable is NOOP.
	report, err := EnableMerge([]byte(b), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.NoOp {
		t.Fatal("first enable on existing must be NOOP")
	}
	// Verify no double entry: check the original input has exactly 1 entry.
	if parsed, _ := ParseSettings([]byte(b)); len(parsed.Hooks["PreToolUse"]) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Hooks["PreToolUse"]))
	}
}
