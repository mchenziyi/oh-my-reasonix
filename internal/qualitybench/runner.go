package qualitybench

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ExecuteFixture replays a deterministic fixture and runs its declared test
// commands in projectDir. Commands are fixture-owned checks; no provider or
// network access is performed by this runner.
func ExecuteFixture(ctx context.Context, fixture Fixture, projectDir string) (RunResult, error) {
	result, err := Replay(fixture)
	if err != nil {
		return RunResult{}, err
	}
	result.HiddenTestsPassed, err = runChecks(ctx, projectDir, fixture.HiddenTests)
	if err != nil {
		return result, fmt.Errorf("fixture %s hidden tests: %w", fixture.ID, err)
	}
	result.RegressionPassed, err = runChecks(ctx, projectDir, fixture.RegressionTests)
	if err != nil {
		return result, fmt.Errorf("fixture %s regression tests: %w", fixture.ID, err)
	}
	return result, nil
}

func runChecks(ctx context.Context, projectDir string, commands []string) (bool, error) {
	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/C", command)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", command)
		}
		cmd.Dir = projectDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(string(output)))
		}
	}
	return true, nil
}
