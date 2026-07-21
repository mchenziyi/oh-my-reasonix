package reasonix

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func TestRunTaskBuildsNonInteractiveRunArgs(t *testing.T) {
	runner := Runner{commandFactory: helperCommand}
	result := runner.RunTask(context.Background(), TaskOptions{
		Prompt:   "fix the bug",
		Metrics:  ".run-metrics.json",
		Model:    "test-model",
		MaxSteps: 7,
	})
	if result.Err != nil || !strings.Contains(result.Stdout, "task complete") {
		t.Fatalf("RunTask failed: %#v", result)
	}
}

func TestReadMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := os.WriteFile(path, []byte(`{"prompt_tokens":10,"cache_hit_tokens":7,"steps":2,"cost":0.12,"currency":"USD"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, err := ReadMetrics(path)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.PromptTokens != 10 || metrics.CacheHitTokens != 7 || metrics.Steps != 2 || metrics.Cost != 0.12 {
		t.Fatalf("unexpected metrics: %#v", metrics)
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
		if len(args) > 0 && args[0] == "run" {
			fmt.Println("task complete")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "unexpected args: %v", args)
		os.Exit(8)
	}
	os.Exit(0)
}
