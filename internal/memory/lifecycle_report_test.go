package memory

import (
	"context"
	"testing"
	"time"
)

func TestBuildLifecycleReportEmptyStore(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	report, err := BuildLifecycleReport(context.Background(), LifecycleReportRequest{Store: s, Scope: ScopeProject, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if report.MemoryCount != 0 || len(report.States) != 0 || report.EvaluatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected empty report: %+v", report)
	}
}

func TestBuildLifecycleReportRequiresExplicitNow(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	if _, err := BuildLifecycleReport(context.Background(), LifecycleReportRequest{Store: s, Scope: ScopeProject}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now must fail closed: %v", err)
	}
}
