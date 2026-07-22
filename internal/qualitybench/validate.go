package qualitybench

import (
	"fmt"
	"sort"
	"strings"
)

// ValidationError represents a single schema validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", ve.Field, ve.Message)
}

// ValidateReport checks that a Report conforms to the quality report schema.
// Returns nil if valid, or a slice of validation errors.
func ValidateReport(r Report) []ValidationError {
	var errs []ValidationError

	errs = append(errs, validateReportFields(r)...)
	errs = append(errs, validateMetrics(r.Metrics)...)
	errs = append(errs, validateEvaluations(r.Evaluations, r.EvaluatedCount)...)

	if len(errs) == 0 {
		return nil
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Field < errs[j].Field
	})
	return errs
}

func validateReportFields(r Report) []ValidationError {
	var errs []ValidationError

	if r.FixtureCount < 1 {
		errs = append(errs, ValidationError{
			Field:   "fixture_count",
			Message: fmt.Sprintf("must be >= 1, got %d", r.FixtureCount),
		})
	}
	if r.EvaluatedCount < 0 {
		errs = append(errs, ValidationError{
			Field:   "evaluated_count",
			Message: fmt.Sprintf("must be >= 0, got %d", r.EvaluatedCount),
		})
	}
	if r.EvaluatedCount > r.FixtureCount {
		errs = append(errs, ValidationError{
			Field:   "evaluated_count",
			Message: fmt.Sprintf("must be <= fixture_count (%d), got %d", r.FixtureCount, r.EvaluatedCount),
		})
	}
	if r.QualifiedCount < 0 {
		errs = append(errs, ValidationError{
			Field:   "qualified_count",
			Message: fmt.Sprintf("must be >= 0, got %d", r.QualifiedCount),
		})
	}
	if r.QualifiedCount > r.EvaluatedCount {
		errs = append(errs, ValidationError{
			Field:   "qualified_count",
			Message: fmt.Sprintf("must be <= evaluated_count (%d), got %d", r.EvaluatedCount, r.QualifiedCount),
		})
	}
	if r.QualifiedRate < 0 || r.QualifiedRate > 1 {
		errs = append(errs, ValidationError{
			Field:   "qualified_rate",
			Message: fmt.Sprintf("must be in [0, 1], got %f", r.QualifiedRate),
		})
	}
	// qualified_rate should match qualified_count / evaluated_count when evaluated_count > 0
	if r.EvaluatedCount > 0 {
		expectedRate := float64(r.QualifiedCount) / float64(r.EvaluatedCount)
		if absFloat(r.QualifiedRate-expectedRate) > 0.0001 {
			errs = append(errs, ValidationError{
				Field:   "qualified_rate",
				Message: fmt.Sprintf("expected %f (based on %d/%d), got %f", expectedRate, r.QualifiedCount, r.EvaluatedCount, r.QualifiedRate),
			})
		}
	}
	// When evaluated_count == 0, qualified_rate should be 1 (by convention)
	if r.EvaluatedCount == 0 && r.QualifiedRate != 1 {
		errs = append(errs, ValidationError{
			Field:   "qualified_rate",
			Message: fmt.Sprintf("must be 1 when evaluated_count is 0, got %f", r.QualifiedRate),
		})
	}

	return errs
}

func validateMetrics(m Metrics) []ValidationError {
	var errs []ValidationError

	// All integer metrics must be >= 0
	intFields := []struct {
		name  string
		value int
	}{
		{"metrics.prompt_tokens", m.PromptTokens},
		{"metrics.completion_tokens", m.CompletionTokens},
		{"metrics.cache_hit_tokens", m.CacheHitTokens},
		{"metrics.cache_miss_tokens", m.CacheMissTokens},
		{"metrics.steps", m.Steps},
		{"metrics.compactions", m.Compactions},
		{"metrics.readiness_checks", m.ReadinessChecks},
		{"metrics.readiness_blocks", m.ReadinessBlocks},
		{"metrics.readiness_recoveries", m.ReadinessRecoveries},
	}
	for _, f := range intFields {
		if f.value < 0 {
			errs = append(errs, ValidationError{
				Field:   f.name,
				Message: fmt.Sprintf("must be >= 0, got %d", f.value),
			})
		}
	}

	// Cost must be >= 0
	if m.Cost < 0 {
		errs = append(errs, ValidationError{
			Field:   "metrics.cost",
			Message: fmt.Sprintf("must be >= 0, got %f", m.Cost),
		})
	}

	// Cost > 0 should have a non-empty currency
	if m.Cost > 0 && strings.TrimSpace(m.Currency) == "" {
		errs = append(errs, ValidationError{
			Field:   "metrics.currency",
			Message: "must be non-empty when cost > 0",
		})
	}

	// Readiness invariants
	if m.ReadinessBlocks > m.ReadinessChecks {
		errs = append(errs, ValidationError{
			Field:   "metrics.readiness_blocks",
			Message: fmt.Sprintf("must be <= readiness_checks (%d), got %d", m.ReadinessChecks, m.ReadinessBlocks),
		})
	}
	if m.ReadinessRecoveries > m.ReadinessBlocks {
		errs = append(errs, ValidationError{
			Field:   "metrics.readiness_recoveries",
			Message: fmt.Sprintf("must be <= readiness_blocks (%d), got %d", m.ReadinessBlocks, m.ReadinessRecoveries),
		})
	}

	return errs
}

func validateEvaluations(evaluations []Evaluation, expectedCount int) []ValidationError {
	var errs []ValidationError

	// Evaluations should have the same count as evaluated_count
	if len(evaluations) != expectedCount {
		errs = append(errs, ValidationError{
			Field:   "evaluations",
			Message: fmt.Sprintf("expected %d evaluations (matching evaluated_count), got %d", expectedCount, len(evaluations)),
		})
	}

	for i, eval := range evaluations {
		prefix := fmt.Sprintf("evaluations[%d]", i)
		if strings.TrimSpace(eval.FixtureID) == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".fixture_id",
				Message: "must be non-empty",
			})
		}
		// When not qualified, must have at least one failure
		if !eval.QualifiedCompletion && len(eval.Failures) == 0 {
			errs = append(errs, ValidationError{
				Field:   prefix + ".failures",
				Message: fmt.Sprintf("fixture %q is not qualified but has no failure reasons", eval.FixtureID),
			})
		}
	}

	return errs
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
