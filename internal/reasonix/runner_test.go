package reasonix

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProbeUsesOnlyReadOnlyCLICommands(t *testing.T) {
	runner := Runner{commandFactory: helperCommand}
	probe, err := runner.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if probe.Version != "reasonix 464d494" {
		t.Fatalf("unexpected version: %q", probe.Version)
	}
	for _, name := range []string{"version", "cli", "subagent", "subagent.try", "subagent.run", "profile.list", "profile.review"} {
		if !hasAvailable(probe.Checks, name) {
			t.Fatalf("capability %q was not detected: %#v", name, probe.Checks)
		}
	}
}

func TestRunCapturesExitCodeAndOutput(t *testing.T) {
	runner := Runner{commandFactory: helperCommand}
	result := runner.Run(context.Background(), "fail")
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "synthetic failure") {
		t.Fatalf("stderr = %q", result.Stderr)
	}
}

func hasAvailable(checks []Check, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return check.Available
		}
	}
	return false
}

func helperCommand(ctx context.Context, _ string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestReasonixHelper", "--")
	cmd.Env = append(os.Environ(),
		"OMR_REASONIX_HELPER=1",
		"OMR_REASONIX_ARGS="+strings.Join(args, "\x1f"),
	)
	return cmd
}

func TestReasonixHelper(t *testing.T) {
	if os.Getenv("OMR_REASONIX_HELPER") != "1" {
		return
	}
	args := strings.Split(os.Getenv("OMR_REASONIX_ARGS"), "\x1f")
	switch strings.Join(args, " ") {
	case "--version", "version":
		fmt.Println("reasonix 464d494")
	case "--help":
		fmt.Println("run subagent doctor")
	case "subagent --help":
		fmt.Println("try run list")
	case "subagent list":
		fmt.Println("explore review security-review")
	case "fail":
		fmt.Fprintln(os.Stderr, "synthetic failure")
		os.Exit(7)
	default:
		fmt.Fprintf(os.Stderr, "unexpected args: %v", args)
		os.Exit(8)
	}
	os.Exit(0)
}
