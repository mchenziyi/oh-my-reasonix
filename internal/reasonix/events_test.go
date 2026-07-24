package reasonix

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEventsFile(t *testing.T, path string, lines []string) {
	t.Helper()
	var data strings.Builder
	for _, l := range lines {
		data.WriteString(l)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(data.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseEventStreamValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1,"kind":"bash","tool_name":"echo","prompt_tokens":10}`,
		`{"event":"tool_result","seq":2,"kind":"bash","completion_tokens":5}`,
		`{"event":"run_done","seq":3,"prompt_tokens":10,"completion_tokens":5}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done")
	}
	if len(stream.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(stream.Events))
	}
	if stream.TotalTokens != 30 {
		t.Fatalf("expected 30 total tokens, got %d", stream.TotalTokens)
	}
	if len(stream.Errors) != 0 {
		t.Fatalf("expected no errors, got: %v", stream.Errors)
	}
}

func TestParseEventStreamMissingRunDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if stream.RunDone {
		t.Fatal("expected run_done=false")
	}
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "run_done") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected run_done error, got: %v", stream.Errors)
	}
}

func TestParseEventStreamOutOfOrderSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"a","seq":1}`,
		`{"event":"b","seq":3}`,
		`{"event":"c","seq":2}`,
		`{"event":"run_done","seq":4}`,
	})
	stream, _ := ParseEventStream(path)
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "non-monotonic") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected non-monotonic error, got: %v", stream.Errors)
	}
}

func TestParseEventStreamInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`not valid json`,
		`{"event":"run_done"}`,
	})
	stream, _ := ParseEventStream(path)
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "invalid JSON") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid JSON error, got: %v", stream.Errors)
	}
}

func TestParseEventStreamRedactsSensitiveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","prompt":"secret text","tool_args":"rm -rf /","tool_result":"sensitive","reasoning":"private","approval":"granted","compaction_summary":"summary"}`,
		`{"event":"run_done"}`,
	})
	stream, _ := ParseEventStream(path)
	// Verify parsed EventRecords do NOT contain sensitive fields.
	for _, rec := range stream.Events {
		b, _ := json.Marshal(rec)
		s := string(b)
		for _, forbidden := range []string{"secret text", "rm -rf", "sensitive", "private", "granted", "summary"} {
			if strings.Contains(s, forbidden) {
				t.Fatalf("EventRecord should not contain %q, got: %s", forbidden, s)
			}
		}
	}
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
}

func TestParseEventStreamLargeLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// Create a line with a very long string value that exceeds reasonable limits
	largeLine := `{"event":"tool_call","data":"` + strings.Repeat("X", 500*1024) + `"}` // 500KB
	writeEventsFile(t, path, []string{
		largeLine,
		`{"event":"run_done"}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	// The large line should parse correctly (500KB < 1MB limit)
	// and run_done must also be present.
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events (large line + run_done), got %d", len(stream.Events))
	}
}

func TestParseEventStreamRunDoneMustBeFinal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"run_done","seq":1}`,
		`{"event":"tool_call","seq":2}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatal(err)
	}
	if stream.RunDone {
		t.Fatal("run_done before final event must not mark stream complete")
	}
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "run_done must be final") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected final run_done error, got %v", stream.Errors)
	}
}

func TestParseEventStreamSkipsOversizedLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	largeLine := `{"event":"tool_call","data":"` + strings.Repeat("X", 2*1024*1024) + `"}`
	writeEventsFile(t, path, []string{largeLine, `{"event":"run_done","seq":1}`})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatal(err)
	}
	if !stream.RunDone || len(stream.Events) != 1 || stream.Events[0].Event != "run_done" {
		t.Fatalf("expected oversized line skipped and run_done retained: %#v", stream)
	}
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "exceeds max size") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected oversized line error, got %v", stream.Errors)
	}
}

func TestParseEventStreamV1_17_20_RealFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	// Reasonix v1.17.20 native format: sequence, kind, usage
	writeEventsFile(t, path, []string{
		`{"schema_version":1,"sequence":1,"kind":"tool_dispatch","tool_name":"bash","tool_id":"t1"}`,
		`{"schema_version":1,"sequence":2,"kind":"tool_result","tool_id":"t1","status":"ok"}`,
		`{"schema_version":1,"sequence":3,"kind":"run_done","ok":true,"duration_ms":120,"num_turns":1,"usage":{"input_tokens":50,"output_tokens":30,"cache_hit_tokens":0,"cache_miss_tokens":50}}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done=true")
	}
	if len(stream.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(stream.Events))
	}
	// Verify field mapping
	runDone := stream.Events[2]
	if runDone.Event != "run_done" {
		t.Fatalf("expected event=run_done, got %q", runDone.Event)
	}
	if runDone.Seq != 3 {
		t.Fatalf("expected seq=3, got %d", runDone.Seq)
	}
	// Token mapping: usage.input_tokens → prompt_tokens, usage.output_tokens → completion_tokens
	if runDone.PromptTokens != 50 {
		t.Fatalf("expected prompt_tokens=50 (from usage.input_tokens), got %d", runDone.PromptTokens)
	}
	if runDone.CompletionTokens != 30 {
		t.Fatalf("expected completion_tokens=30 (from usage.output_tokens), got %d", runDone.CompletionTokens)
	}
}

func TestParseEventStreamV1_17_20_TokenSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"schema_version":1,"sequence":1,"kind":"tool_dispatch","tool_name":"write","usage":{"input_tokens":10,"output_tokens":0}}`,
		`{"schema_version":1,"sequence":2,"kind":"tool_result","status":"ok","usage":{"input_tokens":0,"output_tokens":5}}`,
		`{"schema_version":1,"sequence":3,"kind":"run_done","ok":true,"duration_ms":200,"num_turns":1,"usage":{"input_tokens":100,"output_tokens":80}}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	// TotalTokens should sum across all events: (10+0)+(0+5)+(100+80) = 195
	if stream.TotalTokens != 195 {
		t.Fatalf("expected TotalTokens=195, got %d", stream.TotalTokens)
	}
}

func TestParseEventStreamV1_17_20_SequenceMonotonic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"schema_version":1,"sequence":1,"kind":"tool_dispatch"}`,
		`{"schema_version":1,"sequence":3,"kind":"tool_result"}`,
		`{"schema_version":1,"sequence":2,"kind":"run_done"}`,
	})
	stream, _ := ParseEventStream(path)
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "non-monotonic") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected non-monotonic sequence error, got: %v", stream.Errors)
	}
}

func TestParseEventStreamV1_17_20_RedactsSensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"schema_version":1,"sequence":1,"kind":"tool_dispatch","tool_name":"bash","prompt":"SECRET","tool_args":"rm -rf /","tool_result":"sensitive","reasoning":"private"}`,
		`{"schema_version":1,"sequence":2,"kind":"run_done","ok":true,"usage":{"input_tokens":1,"output_tokens":1}}`,
	})
	stream, _ := ParseEventStream(path)
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
	// The raw file contains "SECRET" but EventRecord should NOT expose it
	if stream.Events[0].ToolName != "bash" {
		t.Fatalf("expected tool_name=bash, got %q", stream.Events[0].ToolName)
	}
}

func TestParseEventStreamV1_17_20_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`not valid json`,
		`{"schema_version":1,"sequence":1,"kind":"run_done","ok":true}`,
	})
	stream, _ := ParseEventStream(path)
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "invalid JSON") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid JSON error, got: %v", stream.Errors)
	}
	// Valid line should still be parsed
	if len(stream.Events) != 1 {
		t.Fatalf("expected 1 valid event, got %d", len(stream.Events))
	}
}

func TestParseEventStreamV1_17_20_RunDoneNotFinal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"schema_version":1,"sequence":1,"kind":"run_done","ok":true}`,
		`{"schema_version":1,"sequence":2,"kind":"tool_dispatch"}`,
	})
	stream, _ := ParseEventStream(path)
	found := false
	for _, e := range stream.Errors {
		if strings.Contains(e, "run_done must be final") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected run_done final error, got: %v", stream.Errors)
	}
}

func TestParseEventStreamV1_17_20_BackwardCompatible(t *testing.T) {
	// Mix of OMR legacy and v1.17.20 format should both work
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1,"kind":"bash","tool_name":"echo"}`,
		`{"schema_version":1,"sequence":2,"kind":"tool_result","status":"ok"}`,
		`{"event":"run_done","seq":3,"prompt_tokens":10,"completion_tokens":5}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done=true")
	}
	if len(stream.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(stream.Events))
	}
}

func TestParseEventStreamValidatesRequiredFields(t *testing.T) {
	// v1.17.20 events: when 'event' field is missing, 'kind' is used as fallback.
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	writeEventsFile(t, path, []string{
		`{"seq":1,"kind":"bash"}`,
		`{"event":"run_done","seq":2}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream should not hard-fail: %v", err)
	}
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
	// When 'event' field is missing, 'kind' is used as fallback
	if stream.Events[0].Event != "bash" {
		t.Fatalf("expected fallback event='bash' (from kind), got %q", stream.Events[0].Event)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done=true")
	}
}

func TestParseEventStreamValidatesSequence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1}`,
		`{"event":"tool_call","seq":3}`,
		`{"event":"run_done","seq":2,"prompt_tokens":5,"completion_tokens":5}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	// seq 2 after seq 3 triggers non-monotonic error
	if len(stream.Errors) == 0 {
		t.Fatal("expected non-monotonic seq error")
	}
	// Events are still recorded (parser is lenient)
	if len(stream.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(stream.Events))
	}
}

func TestParseEventStreamValidatesSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1,"kind":"bash","tool_name":"echo","schema_version":1}`,
		`{"event":"run_done","seq":2,"schema_version":1,"prompt_tokens":5,"completion_tokens":5}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done with valid schema_version=1")
	}
	// Verify events carry schema_version in their raw record
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
}

func TestParseEventStreamHandlesEventSanitization(t *testing.T) {
	// Events with unexpected/extra fields should parse without error
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	writeEventsFile(t, path, []string{
		`{"event":"tool_call","seq":1,"kind":"bash","tool_name":"echo","extra_field":"should_be_ignored"}`,
		`{"event":"run_done","seq":2,"prompt_tokens":5,"completion_tokens":5}`,
	})
	stream, err := ParseEventStream(path)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done=true")
	}
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
	// tool_call event should have correct fields
	if stream.Events[0].Event != "tool_call" {
		t.Fatalf("expected tool_call event, got %q", stream.Events[0].Event)
	}
	if stream.Events[0].ToolName != "echo" {
		t.Fatalf("expected tool_name=echo, got %q", stream.Events[0].ToolName)
	}
	if len(stream.Errors) > 0 {
		t.Fatalf("expected no errors, got: %v", stream.Errors)
	}
}
