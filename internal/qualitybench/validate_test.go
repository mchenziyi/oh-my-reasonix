package qualitybench

import (
	"strings"
	"testing"
)

func TestValidateReportValid(t *testing.T) {
	r := Report{
		FixtureCount:   5,
		EvaluatedCount: 5,
		QualifiedCount: 5,
		QualifiedRate:  1,
		Metrics: Metrics{
			PromptTokens:        100,
			CompletionTokens:    50,
			CacheHitTokens:      30,
			CacheMissTokens:     70,
			Steps:               4,
			Cost:                0.25,
			Currency:            "USD",
			Compactions:         1,
			ReadinessChecks:     3,
			ReadinessBlocks:     1,
			ReadinessRecoveries: 1,
		},
		Evaluations: []Evaluation{
			{FixtureID: "fixture-1", QualifiedCompletion: true},
			{FixtureID: "fixture-2", QualifiedCompletion: true},
			{FixtureID: "fixture-3", QualifiedCompletion: true},
			{FixtureID: "fixture-4", QualifiedCompletion: true},
			{FixtureID: "fixture-5", QualifiedCompletion: true},
		},
	}
	if errs := ValidateReport(r); errs != nil {
		t.Fatalf("expected valid report, got errors: %v", errs)
	}
}

func TestValidateReportWithFailures(t *testing.T) {
	r := Report{
		FixtureCount:   3,
		EvaluatedCount: 3,
		QualifiedCount: 2,
		QualifiedRate:  2.0 / 3.0,
		Metrics:        Metrics{PromptTokens: 10, CompletionTokens: 5, CacheHitTokens: 0, CacheMissTokens: 10, Steps: 2, Cost: 0, Currency: "USD", Compactions: 0},
		Evaluations: []Evaluation{
			{FixtureID: "pass-1", QualifiedCompletion: true},
			{FixtureID: "fail-1", QualifiedCompletion: false, Failures: []string{"runtime error"}},
			{FixtureID: "pass-2", QualifiedCompletion: true},
		},
	}
	if errs := ValidateReport(r); errs != nil {
		t.Fatalf("expected valid report with failures, got: %v", errs)
	}
}

func TestValidateReportRejectsNegativeFixtureCount(t *testing.T) {
	r := Report{FixtureCount: 0, EvaluatedCount: 0, QualifiedCount: 0, QualifiedRate: 1}
	errs := ValidateReport(r)
	if !containsField(errs, "fixture_count") {
		t.Fatalf("expected fixture_count error, got: %v", errs)
	}
}

func TestValidateReportRejectsNegativeEvaluatedCount(t *testing.T) {
	r := Report{FixtureCount: 5, EvaluatedCount: -1, QualifiedCount: 0, QualifiedRate: 1}
	errs := ValidateReport(r)
	if !containsField(errs, "evaluated_count") {
		t.Fatalf("expected evaluated_count error, got: %v", errs)
	}
}

func TestValidateReportRejectsEvaluatedExceedsFixture(t *testing.T) {
	r := Report{FixtureCount: 3, EvaluatedCount: 5, QualifiedCount: 5, QualifiedRate: 1}
	errs := ValidateReport(r)
	if !containsField(errs, "evaluated_count") {
		t.Fatalf("expected evaluated_count error, got: %v", errs)
	}
}

func TestValidateReportRejectsQualifiedExceedsEvaluated(t *testing.T) {
	r := Report{FixtureCount: 5, EvaluatedCount: 3, QualifiedCount: 4, QualifiedRate: 4.0 / 3.0}
	errs := ValidateReport(r)
	if !containsField(errs, "qualified_count") {
		t.Fatalf("expected qualified_count error, got: %v", errs)
	}
}

func TestValidateReportRejectsQualifiedRateOutOfRange(t *testing.T) {
	r := Report{FixtureCount: 1, EvaluatedCount: 1, QualifiedCount: 1, QualifiedRate: 1.5}
	errs := ValidateReport(r)
	if !containsField(errs, "qualified_rate") {
		t.Fatalf("expected qualified_rate error, got: %v", errs)
	}
}

func TestValidateReportRejectsNegativeQualifiedRate(t *testing.T) {
	r := Report{FixtureCount: 1, EvaluatedCount: 1, QualifiedCount: 0, QualifiedRate: -0.5}
	errs := ValidateReport(r)
	if !containsField(errs, "qualified_rate") {
		t.Fatalf("expected qualified_rate error, got: %v", errs)
	}
}

func TestValidateReportRejectsMismatchedQualifiedRate(t *testing.T) {
	r := Report{FixtureCount: 4, EvaluatedCount: 4, QualifiedCount: 3, QualifiedRate: 0.5}
	errs := ValidateReport(r)
	if !containsField(errs, "qualified_rate") {
		t.Fatalf("expected qualified_rate error for mismatch, got: %v", errs)
	}
}

func TestValidateReportRejectsZeroEvaluatedNonOneRate(t *testing.T) {
	r := Report{FixtureCount: 0, EvaluatedCount: 0, QualifiedCount: 0, QualifiedRate: 0}
	errs := ValidateReport(r)
	if !containsField(errs, "qualified_rate") {
		t.Fatalf("expected qualified_rate error for zero evaluated, got: %v", errs)
	}
}

func TestValidateMetricsRejectsNegativeTokens(t *testing.T) {
	m := Metrics{PromptTokens: -1, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 0, Compactions: 0}
	errs := validateMetrics(m)
	if !containsField(errs, "metrics.prompt_tokens") {
		t.Fatalf("expected prompt_tokens error, got: %v", errs)
	}
}

func TestValidateMetricsRejectsNegativeCost(t *testing.T) {
	m := Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: -0.5, Compactions: 0}
	errs := validateMetrics(m)
	if !containsField(errs, "metrics.cost") {
		t.Fatalf("expected cost error, got: %v", errs)
	}
}

func TestValidateMetricsRejectsMissingCurrencyWhenCostPositive(t *testing.T) {
	m := Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 1.0, Currency: "", Compactions: 0}
	errs := validateMetrics(m)
	if !containsField(errs, "metrics.currency") {
		t.Fatalf("expected currency error, got: %v", errs)
	}
}

func TestValidateMetricsRejectsBlocksExceedingChecks(t *testing.T) {
	m := Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 0, Compactions: 0, ReadinessChecks: 2, ReadinessBlocks: 5}
	errs := validateMetrics(m)
	if !containsField(errs, "metrics.readiness_blocks") {
		t.Fatalf("expected readiness_blocks error, got: %v", errs)
	}
}

func TestValidateMetricsRejectsRecoveriesExceedingBlocks(t *testing.T) {
	m := Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 0, Compactions: 0, ReadinessChecks: 5, ReadinessBlocks: 3, ReadinessRecoveries: 4}
	errs := validateMetrics(m)
	if !containsField(errs, "metrics.readiness_recoveries") {
		t.Fatalf("expected readiness_recoveries error, got: %v", errs)
	}
}

func TestValidateEvaluationsRejectsCountMismatch(t *testing.T) {
	errs := validateEvaluations([]Evaluation{{FixtureID: "a"}}, 2)
	if !containsField(errs, "evaluations") {
		t.Fatalf("expected evaluations count error, got: %v", errs)
	}
}

func TestValidateEvaluationsRejectsEmptyFixtureID(t *testing.T) {
	errs := validateEvaluations([]Evaluation{{FixtureID: ""}}, 1)
	if !containsField(errs, "evaluations[0].fixture_id") {
		t.Fatalf("expected fixture_id error, got: %v", errs)
	}
}

func TestValidateEvaluationsRejectsUnqualifiedWithoutFailures(t *testing.T) {
	errs := validateEvaluations([]Evaluation{{FixtureID: "f", QualifiedCompletion: false}}, 1)
	if !containsField(errs, "evaluations[0].failures") {
		t.Fatalf("expected failures error for unqualified fixture, got: %v", errs)
	}
}

func TestValidateReportAllErrorTypes(t *testing.T) {
	// Report with multiple errors
	r := Report{
		FixtureCount:   0,
		EvaluatedCount: -1,
		QualifiedCount: 10,
		QualifiedRate:  -0.1,
		Metrics: Metrics{
			PromptTokens:        -5,
			CompletionTokens:    0,
			CacheHitTokens:      0,
			CacheMissTokens:     0,
			Steps:               0,
			Cost:                -1.0,
			Compactions:         0,
			ReadinessChecks:     5,
			ReadinessBlocks:     10,
			ReadinessRecoveries: 2,
		},
		Evaluations: []Evaluation{
			{FixtureID: "", QualifiedCompletion: false},
		},
	}
	errs := ValidateReport(r)
	if errs == nil {
		t.Fatal("expected validation errors")
	}
	// Should have errors for: fixture_count, evaluated_count, qualified_rate, metrics.prompt_tokens, metrics.cost, metrics.readiness_blocks, evaluations[0].fixture_id
	expectedFields := []string{
		"fixture_count",
		"evaluated_count",
		"qualified_rate",
		"metrics.prompt_tokens",
		"metrics.cost",
		"metrics.readiness_blocks",
		"evaluations[0].fixture_id",
		"evaluations[0].failures",
	}
	for _, field := range expectedFields {
		if !containsField(errs, field) {
			t.Errorf("expected error for field %q, got: %v", field, errs)
		}
	}
}

func TestValidateReportWithZeroCostAndEmptyCurrency(t *testing.T) {
	r := Report{
		FixtureCount:   1,
		EvaluatedCount: 1,
		QualifiedCount: 1,
		QualifiedRate:  1,
		Metrics:        Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 0, Currency: "", Compactions: 0},
		Evaluations:    []Evaluation{{FixtureID: "f", QualifiedCompletion: true}},
	}
	if errs := ValidateReport(r); errs != nil {
		t.Fatalf("expected valid report with zero cost and empty currency, got: %v", errs)
	}
}

func TestValidateReportWithZeroEvaluated(t *testing.T) {
	r := Report{
		FixtureCount:   1,
		EvaluatedCount: 0,
		QualifiedCount: 0,
		QualifiedRate:  1, // by convention, 1 when nothing was evaluated
		Metrics:        Metrics{PromptTokens: 0, CompletionTokens: 0, CacheHitTokens: 0, CacheMissTokens: 0, Steps: 0, Cost: 0, Currency: "", Compactions: 0},
		Evaluations:    []Evaluation{},
	}
	if errs := ValidateReport(r); errs != nil {
		t.Fatalf("expected valid zero-evaluated report, got: %v", errs)
	}
}

// containsField checks if any error has the given field name.
func containsField(errs []ValidationError, field string) bool {
	for _, e := range errs {
		if e.Field == field || strings.HasPrefix(e.Field, field) {
			return true
		}
	}
	return false
}
