package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mchenziyi/oh-my-reasonix/internal/commenthook"
)

func TestHookCommentCheckLogsEmptyJSON(t *testing.T) {
	dir := t.TempDir()
	out, err := captureRunOutput(func() error {
		return runHookCommentCheck([]string{"logs", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var logs auditLogsOutput
	if err := json.Unmarshal([]byte(out), &logs); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if logs.SchemaVersion != 1 || logs.Entries != 0 {
		t.Fatalf("unexpected empty logs: %+v", logs)
	}
}

func TestHookCommentCheckLogsAfterGuard(t *testing.T) {
	dir := t.TempDir()
	store, _ := commenthook.NewAuditStore(dir)
	if err := store.Append(commenthook.AuditEntry{Event: "PreToolUse", Decision: commenthook.DecisionPass, ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	out, err := captureRunOutput(func() error {
		return runHookCommentCheck([]string{"logs", "--project-dir", dir, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var logs auditLogsOutput
	if err := json.Unmarshal([]byte(out), &logs); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if logs.Entries != 1 || logs.Summary["pass"] != 1 {
		t.Fatalf("unexpected logs: %+v", logs)
	}
	if len(logs.Logs) != 1 || logs.Logs[0].OMRVersion == "" {
		t.Fatalf("logs must carry omr version: %+v", logs.Logs)
	}
}

func TestHookCommentCheckLogsClearDryRunAndReal(t *testing.T) {
	dir := t.TempDir()
	store, _ := commenthook.NewAuditStore(dir)
	if err := store.Append(commenthook.AuditEntry{Event: "PreToolUse", Decision: commenthook.DecisionPass, ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	// Dry-run clear: no write.
	out, err := captureRunOutput(func() error {
		return runHookCommentCheck([]string{"logs", "--project-dir", dir, "--clear", "--dry-run", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var dry auditLogsOutput
	if err := json.Unmarshal([]byte(out), &dry); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if !dry.DryRun || dry.Cleared {
		t.Fatalf("unexpected dry-run: %+v", dry)
	}
	if entries, _ := store.List(); len(entries) != 1 {
		t.Fatal("dry-run must not clear the log")
	}
	// Real clear.
	out2, err := captureRunOutput(func() error {
		return runHookCommentCheck([]string{"logs", "--project-dir", dir, "--clear", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var cleared auditLogsOutput
	if err := json.Unmarshal([]byte(out2), &cleared); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out2)
	}
	if !cleared.Cleared || cleared.DryRun {
		t.Fatalf("unexpected clear: %+v", cleared)
	}
	if entries, _ := store.List(); len(entries) != 0 {
		t.Fatal("log must be cleared")
	}
}

func TestHookCommentCheckLogsRejectsCorruptLog(t *testing.T) {
	dir := t.TempDir()
	store, _ := commenthook.NewAuditStore(dir)
	// Corrupt log file.
	if err := store.Append(commenthook.AuditEntry{Event: "PreToolUse", Decision: commenthook.DecisionPass, ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	// Append garbage directly.
	data := `{"schema_version":1,"time":"x"` + "\n"
	if err := appendFileBytes(store.Path(), []byte(data)); err != nil {
		t.Fatal(err)
	}
	_, err := captureRunOutput(func() error {
		return runHookCommentCheck([]string{"logs", "--project-dir", dir, "--json"})
	})
	if err == nil {
		t.Fatal("expected corrupt log rejection")
	}
	if !strings.Contains(err.Error(), "audit") && !strings.Contains(err.Error(), "log") {
		t.Fatalf("expected audit/log error, got: %v", err)
	}
}
