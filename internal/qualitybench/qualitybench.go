package qualitybench

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	Metrics            Metrics  `json:"metrics,omitempty"`
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
	Metrics            Metrics  `json:"metrics,omitempty"`
	Error              string   `json:"error,omitempty"`
}

type Metrics struct {
	PromptTokens        int     `json:"prompt_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	CacheHitTokens      int     `json:"cache_hit_tokens"`
	CacheMissTokens     int     `json:"cache_miss_tokens"`
	Steps               int     `json:"steps"`
	Cost                float64 `json:"cost"`
	Currency            string  `json:"currency,omitempty"`
	Compactions         int     `json:"compactions"`
	ReadinessChecks     int     `json:"readiness_checks"`
	ReadinessBlocks     int     `json:"readiness_blocks"`
	ReadinessRecoveries int     `json:"readiness_recoveries"`
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
	Metrics        Metrics      `json:"metrics"`
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
	if result.Error != "" {
		evaluation.Failures = append(evaluation.Failures, "runtime error: "+result.Error)
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
		report.Metrics.PromptTokens += result.Metrics.PromptTokens
		report.Metrics.CompletionTokens += result.Metrics.CompletionTokens
		report.Metrics.CacheHitTokens += result.Metrics.CacheHitTokens
		report.Metrics.CacheMissTokens += result.Metrics.CacheMissTokens
		report.Metrics.Steps += result.Metrics.Steps
		report.Metrics.Cost += result.Metrics.Cost
		report.Metrics.Compactions += result.Metrics.Compactions
		report.Metrics.ReadinessChecks += result.Metrics.ReadinessChecks
		report.Metrics.ReadinessBlocks += result.Metrics.ReadinessBlocks
		report.Metrics.ReadinessRecoveries += result.Metrics.ReadinessRecoveries
		if report.Metrics.Currency == "" {
			report.Metrics.Currency = result.Metrics.Currency
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

// CheckCostGate rejects a report whose aggregate cost exceeds the configured
// budget. A zero budget disables this optional gate.
func CheckCostGate(report Report, maximumCost float64) error {
	if maximumCost < 0 {
		return fmt.Errorf("maximum cost must be non-negative")
	}
	if maximumCost > 0 && report.Metrics.Cost > maximumCost {
		return fmt.Errorf("cost %.4f exceeds maximum %.4f", report.Metrics.Cost, maximumCost)
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
		Metrics:            fixture.Replay.Metrics,
	}
	return result, nil
}

func Matches(path, pattern string) bool {
	if pattern == path {
		return true
	}
	if strings.ContainsAny(pattern, "*?[") {
		if strings.Contains(pattern, "**") {
			return globstarMatch(filepath.ToSlash(path), filepath.ToSlash(pattern))
		}
		matched, _ := filepath.Match(pattern, filepath.ToSlash(path))
		return matched
	}
	return false
}

func globstarMatch(path, pattern string) bool {
	var expression strings.Builder
	expression.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				expression.WriteString(".*")
				i++
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	expression.WriteString("$")
	matched, err := regexp.MatchString(expression.String(), path)
	return err == nil && matched
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
