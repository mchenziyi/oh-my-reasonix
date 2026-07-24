package reasonix

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// mockCommand creates a commandFactory that returns a fixed stdout.
// For non-zero exit, it uses sh -c "echo <stdout> && exit <code>".
func mockCommand(stdout string, exitCode int) commandFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if exitCode == 0 {
			cmd := exec.CommandContext(ctx, "echo", stdout)
			return cmd
		}
		// Use sh to output and exit with code
		cmd := exec.CommandContext(ctx, "sh", "-c", "echo '"+strings.ReplaceAll(stdout, "'", "'\\''")+"'; exit "+string(rune('0'+exitCode)))
		return cmd
	}
}

func TestSessionListParsesOutput(t *testing.T) {
	jsonOutput := `{"sessions":[{"branch_id":"abc","status":"running","scope":"delivery","turn":5,"lifecycle":"active","recovered":false,"schema_version":1}],"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	result, err := r.SessionList(context.Background())
	if err != nil {
		t.Fatalf("SessionList: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
	if result.Sessions[0].BranchID != "abc" {
		t.Fatalf("expected branch_id=abc, got %q", result.Sessions[0].BranchID)
	}
}

func TestSessionListEmpty(t *testing.T) {
	jsonOutput := `{"sessions":[],"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	result, err := r.SessionList(context.Background())
	if err != nil {
		t.Fatalf("SessionList: %v", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(result.Sessions))
	}
}

func TestSessionListBinaryMissing(t *testing.T) {
	r := Runner{
		Binary:         "nonexistent-binary-xyz",
		commandFactory: nil,
	}
	_, err := r.SessionList(context.Background())
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestSessionListInvalidJSON(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand("not valid json", 0),
	}
	_, err := r.SessionList(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSessionStatusParsesOutput(t *testing.T) {
	jsonOutput := `{"branch_id":"test-branch","status":"completed","turn":3,"lifecycle":"done","recovered":true,"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	detail, err := r.SessionStatus(context.Background(), "test-branch")
	if err != nil {
		t.Fatalf("SessionStatus: %v", err)
	}
	if detail.BranchID != "test-branch" {
		t.Fatalf("expected branch_id=test-branch, got %q", detail.BranchID)
	}
	if !detail.Recovered {
		t.Fatal("expected recovered=true")
	}
}

func TestSessionShowParsesOutput(t *testing.T) {
	jsonOutput := `{"branch_id":"show-branch","status":"running","turn":2,"lifecycle":"active","recovered":false,"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	detail, err := r.SessionShow(context.Background(), "show-branch")
	if err != nil {
		t.Fatalf("SessionShow: %v", err)
	}
	if detail.BranchID != "show-branch" {
		t.Fatalf("expected branch_id=show-branch, got %q", detail.BranchID)
	}
}

func TestRunWithEventsArgsDoNotIncludeFilePath(t *testing.T) {
	var capturedArgs []string
	r := Runner{
		Binary: "reasonix",
		commandFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string{name}, args...)
			return exec.CommandContext(ctx, "echo", `{"event":"run_done","seq":1}`)
		},
	}
	dir := t.TempDir()
	outPath := dir + "/events.jsonl"
	result := r.RunWithEvents(context.Background(), "echo integration-test", outPath)
	if result.Err != nil {
		t.Fatalf("RunWithEvents: %v", result.Err)
	}
	// --events-jsonl must be a bare flag, no file path argument
	joined := strings.Join(capturedArgs, " ")
	if strings.Contains(joined, outPath) {
		t.Fatalf("events file path must not be passed as CLI argument, got: %s", joined)
	}
	if !strings.Contains(joined, "--events-jsonl") || !strings.Contains(joined, "run") {
		t.Fatalf("expected args to contain run --events-jsonl, got: %s", joined)
	}
}

func TestRunWithEventsWritesStdoutToFile(t *testing.T) {
	eventsJSONL := `{"event":"tool_call","seq":1,"kind":"bash","tool_name":"echo","prompt_tokens":10}
{"event":"tool_result","seq":2,"kind":"bash","completion_tokens":5}
{"event":"run_done","seq":3,"prompt_tokens":10,"completion_tokens":5}
`
	r := Runner{
		Binary: "reasonix",
		commandFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "printf", "%s", eventsJSONL)
		},
	}
	dir := t.TempDir()
	outPath := dir + "/events.jsonl"
	result := r.RunWithEvents(context.Background(), "echo test", outPath)
	if result.Err != nil {
		t.Fatalf("RunWithEvents: %v", result.Err)
	}
	// Verify file contents
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	if string(data) != eventsJSONL {
		t.Fatalf("file content mismatch.\nExpected:\n%s\nGot:\n%s", eventsJSONL, string(data))
	}
}

func TestRunWithEventsWriteFailure(t *testing.T) {
	r := Runner{
		Binary: "reasonix",
		commandFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo", `{"event":"run_done","seq":1}`)
		},
	}
	// Write to a path that cannot be created (non-existent directory)
	result := r.RunWithEvents(context.Background(), "echo test", "/nonexistent-dir/events.jsonl")
	if result.Err == nil {
		t.Fatal("expected write error for unwritable path")
	}
	if !strings.Contains(result.Err.Error(), "write events file") {
		t.Fatalf("expected 'write events file' error, got: %v", result.Err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("expected exit code -1 for write failure, got %d", result.ExitCode)
	}
}

func TestRunWithEventsPreservesExitError(t *testing.T) {
	// When the reasonix process fails with non-zero exit, the stdout JSONL
	// (containing run_done with ok=false) should still be saved.
	r := Runner{
		Binary: "reasonix",
		commandFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "echo '{\"event\":\"run_done\",\"seq\":1}'; exit 2")
		},
	}
	dir := t.TempDir()
	outPath := dir + "/events.jsonl"
	result := r.RunWithEvents(context.Background(), "echo test", outPath)
	if result.Err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", result.ExitCode)
	}
	// File should be saved even on failure (run_done events are still emitted)
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected events file to exist on failure: %v", err)
	}
	if !strings.Contains(string(data), `"event":"run_done"`) {
		t.Fatalf("expected run_done in saved events, got: %s", string(data))
	}
}

func TestRunWithEventsParsesRunDoneAndTokens(t *testing.T) {
	eventsJSONL := `{"event":"tool_call","seq":1,"prompt_tokens":10}
{"event":"run_done","seq":2,"prompt_tokens":10,"completion_tokens":5}
`
	r := Runner{
		Binary: "reasonix",
		commandFactory: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "printf", "%s", eventsJSONL)
		},
	}
	dir := t.TempDir()
	outPath := dir + "/events.jsonl"
	result := r.RunWithEvents(context.Background(), "echo test", outPath)
	if result.Err != nil {
		t.Fatalf("RunWithEvents: %v", result.Err)
	}
	// Parse the file and verify run_done, seq, tokens
	stream, err := ParseEventStream(outPath)
	if err != nil {
		t.Fatalf("ParseEventStream: %v", err)
	}
	if !stream.RunDone {
		t.Fatal("expected run_done=true")
	}
	if len(stream.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(stream.Events))
	}
	if stream.TotalTokens != 25 {
		t.Fatalf("expected TotalTokens=25, got %d", stream.TotalTokens)
	}
	if len(stream.Errors) > 0 {
		t.Fatalf("expected no errors, got: %v", stream.Errors)
	}
}
