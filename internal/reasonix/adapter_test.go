package reasonix

import (
	"context"
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
