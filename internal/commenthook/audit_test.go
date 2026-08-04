package commenthook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Audit entry / store ----------------------------------------------------

func TestAuditStoreWritesSanitizedEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAuditStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry := AuditEntry{
		Time:          "2026-08-04T10:00:00Z",
		Event:         "PreToolUse",
		Decision:      DecisionBlocking,
		RuleCounts:    map[string]int{"blocking": 2, "warning": 1},
		ExitCode:      2,
		DurationMs:    12,
		OMRVersion:    "2.0.7",
		TriggeredRule: "R004",
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.Decision != DecisionBlocking || got.ExitCode != 2 || got.Event != "PreToolUse" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if got.RuleCounts["blocking"] != 2 || got.TriggeredRule != "R004" {
		t.Fatalf("unexpected counts: %+v", got)
	}
}

func TestAuditStoreListIsStableOrder(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	for i := 0; i < 3; i++ {
		_ = store.Append(AuditEntry{Time: time.Now().UTC().Format(time.RFC3339Nano), Event: "PreToolUse", Decision: DecisionPass, ExitCode: 0})
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestAuditStoreRejectsCorruptEntry(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	// Write a corrupt JSONL entry directly.
	if err := os.MkdirAll(store.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "audit.jsonl"), []byte("{corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("expected corrupt log rejection")
	}
}

func TestAuditStoreRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "audit.jsonl"), store.Path()); err != nil {
		t.Skip(err)
	}
	if err := store.Append(AuditEntry{Time: "2026-01-01T00:00:00Z", Event: "PreToolUse", Decision: DecisionPass, ExitCode: 0}); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestAuditStoreClearDryRunAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	_ = store.Append(AuditEntry{Time: "2026-01-01T00:00:00Z", Event: "PreToolUse", Decision: DecisionPass, ExitCode: 0})

	// Dry-run: zero writes.
	dry, err := store.Clear(DryRun)
	if err != nil {
		t.Fatal(err)
	}
	if dry {
		t.Fatal("dry-run must not clear")
	}
	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatal("dry-run must not remove the log")
	}
	// Real clear.
	cleared, err := store.Clear(RealClear)
	if err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Fatal("expected clear result")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatal("log must be removed after clear")
	}
	// Idempotent: clearing again succeeds.
	if _, err := store.Clear(RealClear); err != nil {
		t.Fatal(err)
	}
}

func TestAuditStoreRetentionByCount(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	for i := 0; i < 15; i++ {
		_ = store.Append(AuditEntry{Time: time.Now().UTC().Format(time.RFC3339Nano), Event: "PreToolUse", Decision: DecisionPass, ExitCode: 0})
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > maxAuditEntries {
		t.Fatalf("retention failed: %d entries", len(entries))
	}
}

func TestAuditStoreRetentionByBytes(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	big := strings.Repeat("x", 200000)
	for i := 0; i < 3; i++ {
		_ = store.Append(AuditEntry{Time: "2026-01-01T00:00:00Z", Event: "PreToolUse", Decision: DecisionPass, ExitCode: 0, TriggeredRule: big})
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 1 {
		t.Fatalf("byte retention failed: %d entries", len(entries))
	}
}

func TestAuditEntryJSONShape(t *testing.T) {
	entry := AuditEntry{
		Time:          "2026-08-04T10:00:00Z",
		Event:         "PreToolUse",
		Decision:      DecisionBlocking,
		RuleCounts:    map[string]int{"blocking": 1},
		ExitCode:      2,
		DurationMs:    5,
		OMRVersion:    "2.0.7",
		TriggeredRule: "R004",
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	// Must never include command bodies, tool args, or credentials.
	for _, forbidden := range []string{"command", "toolArgs", "api_key", "secret", "password"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("audit entry leaked %q: %s", forbidden, raw)
		}
	}
	for _, required := range []string{"schema_version", "event", "decision", "exit_code", "omr_version"} {
		if !strings.Contains(raw, required) {
			t.Fatalf("audit entry missing %q: %s", required, raw)
		}
	}
}

// --- Guard integration ------------------------------------------------------

func TestRunGuardWritesAuditEntry(t *testing.T) {
	dir := t.TempDir()
	payload := guardPayload(t, "PreToolUse", "read", "git commit")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	store, err := NewAuditStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
	// Non-bash pass decision must be logged.
	if entries[len(entries)-1].Decision != DecisionPass {
		t.Fatalf("expected pass decision: %+v", entries[len(entries)-1])
	}
}

func TestRunGuardParsingFailureLogged(t *testing.T) {
	dir := t.TempDir()
	result := RunGuard(strings.NewReader("{invalid}"), dir)
	if result.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", result.ExitCode)
	}
	store, _ := NewAuditStore(dir)
	entries, _ := store.List()
	if len(entries) == 0 || entries[len(entries)-1].Decision != DecisionParseFailure {
		t.Fatalf("expected parse failure decision: %+v", entries)
	}
}

func TestRunGuardBlockingLoggedWithRuleCounts(t *testing.T) {
	// A commit in a project with a blocking comment.
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, "bad.go"), []byte("// api_key=secret-leak\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), projDir)
	if result.ExitCode != 2 {
		t.Fatalf("expected exit 2 (blocking), got %d: %s", result.ExitCode, result.Message)
	}
	store, _ := NewAuditStore(projDir)
	entries, _ := store.List()
	if len(entries) == 0 {
		t.Fatal("expected blocking entry")
	}
	last := entries[len(entries)-1]
	if last.Decision != DecisionBlocking || last.ExitCode != 2 {
		t.Fatalf("unexpected blocking entry: %+v", last)
	}
	// The audit entry must not contain the leaked credential.
	b, _ := json.Marshal(last)
	if strings.Contains(string(b), "api_key=secret-leak") {
		t.Fatal("audit entry leaked credential content")
	}
}

func TestAuditUnavailableFailsClosed(t *testing.T) {
	// When the audit directory cannot be written, the guard must not report
	// a silent success: the write failure is surfaced.
	dir := t.TempDir()
	store, _ := NewAuditStore(dir)
	// Occupy the audit directory slot with a plain file so writes fail.
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(store.Path())), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Dir, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	payload := guardPayload(t, "PreToolUse", "read", "git commit")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode == 0 && result.Message == "" {
		t.Fatal("audit write failure must be surfaced")
	}
}
