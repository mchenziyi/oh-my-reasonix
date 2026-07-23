package qualitybench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

// ReadEventNames reads a JSONL event stream without depending on Reasonix's
// private event package. Accepted records use an event, kind, or name field.
func ReadEventNames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var events []string
	for scanner.Scan() {
		var record struct {
			Event string `json:"event"`
			Kind  string `json:"kind"`
			Name  string `json:"name"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("parse event log %s: %w", path, err)
		}
		name := record.Event
		if name == "" {
			name = record.Kind
		}
		if name == "" {
			name = record.Name
		}
		if name != "" {
			events = append(events, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

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

// ExecuteRuntime runs a fixture through the real Reasonix CLI. It intentionally
// leaves Events empty: runtime event evidence must come from a structured event
// sink, never from scraping human-readable stdout.
func ExecuteRuntime(ctx context.Context, fixture Fixture, projectDir, binary, metricsDir, model string, maxSteps int) (RunResult, error) {
	if strings.TrimSpace(fixture.Task) == "" {
		return RunResult{}, fmt.Errorf("fixture %s has no task", fixture.ID)
	}
	if metricsDir == "" {
		metricsDir = filepath.Join(projectDir, ".reasonix", "omr", "metrics")
	}
	if err := os.MkdirAll(metricsDir, 0o755); err != nil {
		return RunResult{}, err
	}
	metricsPath := filepath.Join(metricsDir, fixture.ID+".json")
	run := (reasonix.Runner{Binary: binary, ProjectDir: projectDir}).RunTask(ctx, reasonix.TaskOptions{
		Prompt: fixture.Task, Metrics: metricsPath, Model: model, MaxSteps: maxSteps,
	})
	result := RunResult{RequiredEffectsMet: run.Err == nil}
	if run.Err != nil {
		result.Error = run.Err.Error()
		result.Failed = true
	}
	if metrics, err := reasonix.ReadMetrics(metricsPath); err == nil {
		result.Metrics = Metrics{
			PromptTokens: metrics.PromptTokens, CompletionTokens: metrics.CompletionTokens,
			CacheHitTokens: metrics.CacheHitTokens, CacheMissTokens: metrics.CacheMissTokens,
			Steps: metrics.Steps, Cost: metrics.Cost, Currency: metrics.Currency,
			Compactions: metrics.Compactions, ReadinessChecks: metrics.ReadinessChecks,
			ReadinessBlocks: metrics.ReadinessBlocks, ReadinessRecoveries: metrics.ReadinessRecoveries,
		}
	}
	hidden, err := runChecks(ctx, projectDir, fixture.HiddenTests)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("fixture %s hidden tests: %w", fixture.ID, err)
	}
	regression, err := runChecks(ctx, projectDir, fixture.RegressionTests)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("fixture %s regression tests: %w", fixture.ID, err)
	}
	result.HiddenTestsPassed = hidden
	result.RegressionPassed = regression
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
