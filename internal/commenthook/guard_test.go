package commenthook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// guardPayload creates a JSON payload string in Reasonix v1.18 native format
// where toolArgs is a JSON object: {"command":"..."}
func guardPayload(t *testing.T, event, toolName, cmd string) string {
	t.Helper()
	args, _ := json.Marshal(map[string]string{"command": cmd})
	input := map[string]any{
		"event":    event,
		"toolName": toolName,
		"toolArgs": json.RawMessage(args),
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// guardPayloadLegacy creates a JSON payload where toolArgs is a plain string
// (backward compat format).
func guardPayloadLegacy(t *testing.T, event, toolName, toolArgs string) string {
	t.Helper()
	input := map[string]any{
		"event":    event,
		"toolName": toolName,
		"toolArgs": toolArgs,
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- IsGitCommit tests ---

func TestIsGitCommit_DirectCommit(t *testing.T) {
	if !IsGitCommit("git commit") {
		t.Fatal("expected 'git commit' to be detected")
	}
}

func TestIsGitCommit_CommitWithMessage(t *testing.T) {
	if !IsGitCommit(`git commit -m "fix: resolve issue"`) {
		t.Fatal("expected 'git commit -m ...' to be detected")
	}
}

func TestIsGitCommit_GitCPathCommit(t *testing.T) {
	if !IsGitCommit("git -C /path/to/repo commit -m msg") {
		t.Fatal("expected 'git -C path commit' to be detected")
	}
}

func TestIsGitCommit_EchoGitCommit(t *testing.T) {
	if IsGitCommit(`echo "git commit"`) {
		t.Fatal("must NOT detect echo 'git commit'")
	}
}

func TestIsGitCommit_EmptyArgs(t *testing.T) {
	if IsGitCommit("") {
		t.Fatal("must NOT detect empty args")
	}
}

func TestIsGitCommit_NonCommitGit(t *testing.T) {
	if IsGitCommit("git status") {
		t.Fatal("must NOT detect git status")
	}
}

func TestIsGitCommit_EchoGitDashCCommit(t *testing.T) {
	if IsGitCommit(`echo "git -C path commit -m msg"`) {
		t.Fatal("must NOT detect echo of 'git -C ... commit'")
	}
}

func TestIsGitCommit_GitOnly(t *testing.T) {
	if IsGitCommit("git") {
		t.Fatal("must NOT detect bare 'git'")
	}
}

func TestIsGitCommit_GitCommitDashC(t *testing.T) {
	if !IsGitCommit("git commit -C HEAD") {
		t.Fatal("'git commit -C HEAD' is a direct commit, should be detected")
	}
}

func TestIsGitCommit_GitCommitAlias(t *testing.T) {
	if IsGitCommit("gc") {
		t.Fatal("must NOT detect 'gc' alias")
	}
	if IsGitCommit("git ci") {
		t.Fatal("must NOT detect 'git ci' alias")
	}
}

func TestIsGitCommit_ShDashCGitCommit(t *testing.T) {
	if IsGitCommit(`sh -c "git commit -m msg"`) {
		t.Fatal("must NOT detect sh -c wrapped git commit")
	}
}

// --- GuardInput.GetCommand ---

func TestGetCommand_ObjectFormat(t *testing.T) {
	input := GuardInput{
		Event:    "PreToolUse",
		ToolName: "bash",
		ToolArgs: json.RawMessage(`{"command":"git commit -m test"}`),
	}
	cmd, err := input.GetCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "git commit -m test" {
		t.Fatalf("expected 'git commit -m test', got %q", cmd)
	}
}

func TestGetCommand_ObjectNoCommandField(t *testing.T) {
	input := GuardInput{
		Event:    "PreToolUse",
		ToolName: "bash",
		ToolArgs: json.RawMessage(`{"other":"value"}`),
	}
	_, err := input.GetCommand()
	if err == nil {
		t.Fatal("expected error for object without command field")
	}
}

func TestGetCommand_StringFormatBackwardCompat(t *testing.T) {
	input := GuardInput{
		Event:    "PreToolUse",
		ToolName: "bash",
		ToolArgs: json.RawMessage(`"git commit -m test"`),
	}
	cmd, err := input.GetCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "git commit -m test" {
		t.Fatalf("expected 'git commit -m test', got %q", cmd)
	}
}

func TestGetCommand_EmptyReturnsEmpty(t *testing.T) {
	input := GuardInput{Event: "PreToolUse", ToolName: "bash"}
	cmd, err := input.GetCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "" {
		t.Fatalf("expected empty, got %q", cmd)
	}
}

func TestGetCommand_UnrecognizedFormatReturnsError(t *testing.T) {
	input := GuardInput{
		Event:    "PreToolUse",
		ToolName: "bash",
		ToolArgs: json.RawMessage(`42`),
	}
	_, err := input.GetCommand()
	if err == nil {
		t.Fatal("expected error for unrecognized format")
	}
}

// --- RunGuard tests ---

func TestRunGuard_NonBashReturnsZero(t *testing.T) {
	payload := guardPayload(t, "PreToolUse", "read", "git commit")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for non-bash, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_NonCommitReturnsZero(t *testing.T) {
	payload := guardPayload(t, "PreToolUse", "bash", "echo hello")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for non-commit, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_EchoGitCommitReturnsZero(t *testing.T) {
	payload := guardPayload(t, "PreToolUse", "bash", `echo "git commit"`)
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for echo git commit, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_LegacyStringFormat(t *testing.T) {
	// Legacy string format should still work.
	payload := guardPayloadLegacy(t, "PreToolUse", "bash", "echo hello")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for legacy string format, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_LegacyStringGitCommit(t *testing.T) {
	payload := guardPayloadLegacy(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	// Clean project should pass.
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for legacy string git commit in clean project, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_InvalidJSONReturnsOne(t *testing.T) {
	result := RunGuard(bytes.NewReader([]byte("{invalid}")), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for invalid JSON, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_EmptyStdinReturnsOne(t *testing.T) {
	result := RunGuard(bytes.NewReader([]byte("")), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for empty stdin, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_WrongEventReturnsOne(t *testing.T) {
	payload := guardPayload(t, "PostToolUse", "bash", "git commit")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for wrong event, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_OversizedPayloadReturnsOne(t *testing.T) {
	largeCmd := strings.Repeat("a", MaxPayloadSize)
	payload := guardPayload(t, "PreToolUse", "bash", largeCmd)
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for oversized payload, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_GitCommitInCleanProjectReturnsZero(t *testing.T) {
	dir := t.TempDir()
	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for clean project, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_GitCommitWithBlockingReturnsTwo(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n// password = \"hunter2\"\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2 for blocking finding, got %d: %s", result.ExitCode, result.Message)
	}
	if !result.Block {
		t.Fatal("expected Block=true for blocking finding")
	}
}

func TestRunGuard_GitCommitWarningOnlyReturnsZero(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n// TODO: do this later\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for info-only finding, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_GuardDoesNotModifyFiles(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	orig := "package main\n// TODO: implement\nfunc main() {}\n"
	if err := os.WriteFile(filePath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	_ = RunGuard(bytes.NewReader([]byte(payload)), dir)

	after, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != orig {
		t.Fatal("guard must not modify project files")
	}
}

func TestRunGuard_ScanFailFailClosed(t *testing.T) {
	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), "/nonexistent/project")
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2 (fail closed) for scan failure, got %d: %s", result.ExitCode, result.Message)
	}
	if !result.Block {
		t.Fatal("expected Block=true for scan failure")
	}
}

func TestRunGuard_BlockingMessageRedactsCredentials(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "config.go")
	if err := os.WriteFile(filePath, []byte("package config\n// db_password = \"mysecret123\"\nfunc load() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", result.ExitCode)
	}
	if strings.Contains(result.Message, "mysecret123") {
		t.Fatalf("blocking message must not contain raw credentials: %s", result.Message)
	}
}

func TestRunGuard_GitCommitInEmptyDir(t *testing.T) {
	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for empty dir, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_GitCPathCommit(t *testing.T) {
	dir := t.TempDir()
	payload := guardPayload(t, "PreToolUse", "bash", "git -C /some/path commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for git -C commit in clean project, got %d: %s", result.ExitCode, result.Message)
	}
}

// --- Real Reasonix v1.18 native payload format ---

func TestRunGuard_ReasonixNativePayload(t *testing.T) {
	// Exact payload shape as Reasonix v1.18 sends it.
	payload := `{"event":"PreToolUse","cwd":"/tmp","toolName":"bash","toolArgs":{"command":"echo hello"}}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for non-commit in native payload, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_ReasonixNativePayloadGitCommitWithBlocking(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n// password = \"hunter2\"\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := `{"event":"PreToolUse","cwd":"/tmp","toolName":"bash","toolArgs":{"command":"git commit -m test"}}`
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 2 {
		t.Fatalf("expected exit code 2 for blocking finding, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_ReasonixNativePayloadCleanCommit(t *testing.T) {
	dir := t.TempDir()
	payload := `{"event":"PreToolUse","cwd":"/tmp","toolName":"bash","toolArgs":{"command":"git commit -m test"}}`
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 for clean, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_MessageDoesNotContainCommand(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n// password = \"hunter2\"\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := guardPayload(t, "PreToolUse", "bash", "git commit -m test")
	result := RunGuard(bytes.NewReader([]byte(payload)), dir)
	if strings.Contains(result.Message, "git commit") {
		t.Fatalf("guard message must not contain the full command: %s", result.Message)
	}
}

// --- Guard toolArgs fail-closed ---

func TestRunGuard_InvalidToolArgsFormatExitOne(t *testing.T) {
	// Number as toolArgs — invalid format for bash.
	payload := `{"event":"PreToolUse","toolName":"bash","toolArgs":42}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for invalid toolArgs format, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_ObjectMissingCommandExitOne(t *testing.T) {
	// Object with no command field.
	payload := `{"event":"PreToolUse","toolName":"bash","toolArgs":{"other":"value"}}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for object without command, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_CommandNotStringExitOne(t *testing.T) {
	// command is a number, not string.
	payload := `{"event":"PreToolUse","toolName":"bash","toolArgs":{"command":42}}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for non-string command, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_MissingToolArgsForBashExitOne(t *testing.T) {
	payload := `{"event":"PreToolUse","toolName":"bash"}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for missing toolArgs, got %d: %s", result.ExitCode, result.Message)
	}
}

func TestRunGuard_ErrorSanitizedNoRawToolArgs(t *testing.T) {
	// Error message must not contain raw toolArgs or command.
	payload := `{"event":"PreToolUse","toolName":"bash","toolArgs":42}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", result.ExitCode)
	}
	if strings.Contains(result.Message, "42") {
		t.Fatalf("error message must not contain raw toolArgs: %s", result.Message)
	}
}

func TestRunGuard_ArrayToolArgsIsInvalid(t *testing.T) {
	payload := `{"event":"PreToolUse","toolName":"bash","toolArgs":["git","commit"]}`
	result := RunGuard(bytes.NewReader([]byte(payload)), t.TempDir())
	// Array is neither object nor string — invalid.
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1 for array toolArgs, got %d: %s", result.ExitCode, result.Message)
	}
}
