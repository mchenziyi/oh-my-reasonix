package qualitybench

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fixture is deliberately JSON-shaped because JSON is a valid YAML document;
// this keeps the benchmark reader dependency-free and deterministic.
type Fixture struct {
	ID              string         `json:"id"`
	Description     string         `json:"description"`
	Task            string         `json:"task"`
	AllowedPaths    []string       `json:"allowed_paths"`
	ForbiddenPaths  []string       `json:"forbidden_paths"`
	HiddenTests     []string       `json:"hidden_tests"`
	RegressionTests []string       `json:"regression_tests"`
	ExpectedEvents  []string       `json:"expected_events"`
	ReplayOutputs   []ReplayOutput `json:"replay_outputs,omitempty"`
	Replay          *ReplaySpec    `json:"replay,omitempty"`
}

// ReplaySpec describes the deterministic outcome expected from a local replay.
// It deliberately contains no provider or filesystem commands.
type ReplaySpec struct {
	ChangedPaths       []string `json:"changed_paths"`
	HiddenTestsPassed  bool     `json:"hidden_tests_passed"`
	RegressionPassed   bool     `json:"regression_passed"`
	RequiredEffectsMet bool     `json:"required_effects_met"`
	Events             []string `json:"events"`
	TestsSkipped       bool     `json:"tests_skipped"`
}

type ReplayOutput struct {
	Role   string `json:"role"`
	Output string `json:"output"`
	Kind   string `json:"kind"`
}

type RunResult struct {
	ChangedPaths       []string `json:"changed_paths"`
	HiddenTestsPassed  bool     `json:"hidden_tests_passed"`
	RegressionPassed   bool     `json:"regression_passed"`
	RequiredEffectsMet bool     `json:"required_effects_met"`
	Events             []string `json:"events"`
	TestsSkipped       bool     `json:"tests_skipped"`
}

type Evaluation struct {
	FixtureID           string   `json:"fixture_id"`
	QualifiedCompletion bool     `json:"qualified_completion"`
	Failures            []string `json:"failures,omitempty"`
}

type Report struct {
	FixtureCount   int          `json:"fixture_count"`
	EvaluatedCount int          `json:"evaluated_count"`
	QualifiedCount int          `json:"qualified_count"`
	QualifiedRate  float64      `json:"qualified_rate"`
	Evaluations    []Evaluation `json:"evaluations"`
}

func LoadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, err
	}
	var fixture Fixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return Fixture{}, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	if fixture.ID == "" || fixture.Task == "" {
		return Fixture{}, fmt.Errorf("fixture %s requires id and task", path)
	}
	return fixture, nil
}

func Discover(root string) ([]Fixture, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "fixture.yaml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	fixtures := make([]Fixture, 0, len(paths))
	for _, path := range paths {
		fixture, err := LoadFixture(path)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures, nil
}

func Evaluate(fixture Fixture, result RunResult) Evaluation {
	evaluation := Evaluation{FixtureID: fixture.ID, QualifiedCompletion: true}
	if !result.HiddenTestsPassed {
		evaluation.Failures = append(evaluation.Failures, "hidden tests failed")
	}
	if !result.RegressionPassed {
		evaluation.Failures = append(evaluation.Failures, "regression tests failed")
	}
	if !result.RequiredEffectsMet {
		evaluation.Failures = append(evaluation.Failures, "required effects not met")
	}
	if result.TestsSkipped {
		evaluation.Failures = append(evaluation.Failures, "tests were skipped")
	}
	for _, changed := range result.ChangedPaths {
		if matchesAny(changed, fixture.ForbiddenPaths) || (len(fixture.AllowedPaths) > 0 && !matchesAny(changed, fixture.AllowedPaths)) {
			evaluation.Failures = append(evaluation.Failures, "modified path outside fixture scope: "+changed)
		}
	}
	for _, expected := range fixture.ExpectedEvents {
		if !contains(result.Events, expected) {
			evaluation.Failures = append(evaluation.Failures, "missing expected event: "+expected)
		}
	}
	evaluation.QualifiedCompletion = len(evaluation.Failures) == 0
	return evaluation
}

func EvaluateAll(fixtures []Fixture, results map[string]RunResult) Report {
	report := Report{FixtureCount: len(fixtures)}
	for _, fixture := range fixtures {
		result, ok := results[fixture.ID]
		if !ok {
			continue
		}
		report.EvaluatedCount++
		evaluation := Evaluate(fixture, result)
		report.Evaluations = append(report.Evaluations, evaluation)
		if evaluation.QualifiedCompletion {
			report.QualifiedCount++
		}
	}
	if report.EvaluatedCount > 0 {
		report.QualifiedRate = float64(report.QualifiedCount) / float64(report.EvaluatedCount)
	}
	return report
}

// CheckGate validates a benchmark report against the requested qualified rate.
// A perfect-rate gate also requires every discovered fixture to be evaluated.
func CheckGate(report Report, minimumRate float64) error {
	if minimumRate < 0 || minimumRate > 1 {
		return fmt.Errorf("minimum qualified rate must be between 0 and 1")
	}
	if minimumRate >= 1 && report.EvaluatedCount != report.FixtureCount {
		return fmt.Errorf("missing results for one or more fixtures")
	}
	if report.QualifiedRate < minimumRate {
		return fmt.Errorf("qualified rate %.3f is below minimum %.3f", report.QualifiedRate, minimumRate)
	}
	return nil
}

// Replay executes a fixture's deterministic transcript without contacting a
// provider. The explicit outcome is kept in the fixture so the same run is
// reproducible on every platform.
func Replay(fixture Fixture) (RunResult, error) {
	if fixture.Replay == nil {
		return RunResult{}, fmt.Errorf("fixture %s has no replay outcome", fixture.ID)
	}
	result := RunResult{
		ChangedPaths:       append([]string(nil), fixture.Replay.ChangedPaths...),
		HiddenTestsPassed:  fixture.Replay.HiddenTestsPassed,
		RegressionPassed:   fixture.Replay.RegressionPassed,
		RequiredEffectsMet: fixture.Replay.RequiredEffectsMet,
		Events:             append([]string(nil), fixture.Replay.Events...),
		TestsSkipped:       fixture.Replay.TestsSkipped,
	}
	return result, nil
}

func Matches(path, pattern string) bool {
	if pattern == path {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		matched, _ := filepath.Match(pattern, filepath.ToSlash(path))
		return matched
	}
	return false
}

func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if Matches(filepath.ToSlash(path), filepath.ToSlash(pattern)) {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
