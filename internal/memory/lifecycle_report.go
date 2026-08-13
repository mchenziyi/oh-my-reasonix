package memory

import (
	"context"
	"sort"
	"time"
)

// LifecycleReportRequest fixes the derivation instant for a reproducible
// summary. The report is a read-only view of canonical facts.
type LifecycleReportRequest struct {
	Store *FactStore
	Scope Scope
	Now   time.Time
}

type LifecycleReport struct {
	Scope             Scope                `json:"scope"`
	EvaluatedAt       string               `json:"evaluated_at"`
	MemoryCount       int                  `json:"memory_count"`
	LifecycleCounts   map[Lifecycle]int    `json:"lifecycle_counts"`
	HealthCounts      map[Health]int       `json:"health_counts"`
	UsageCount        int                  `json:"usage_count"`
	CountedHelpCount  int                  `json:"counted_help_count"`
	CountedHarmCount  int                  `json:"counted_harm_count"`
	InsufficientCount int                  `json:"insufficient_evidence_count"`
	States            []DerivedMemoryState `json:"states"`
}

func BuildLifecycleReport(ctx context.Context, req LifecycleReportRequest) (LifecycleReport, error) {
	if req.Store == nil || (req.Scope != ScopeProject && req.Scope != ScopeGlobal) || req.Now.IsZero() || !req.Store.scopeMatches(req.Scope) {
		return LifecycleReport{}, storeError(CodeDerivedInvalidInput, "lifecycle report request is invalid")
	}
	derived, err := DeriveState(ctx, req.Store, DerivedStateRequest{Scope: req.Scope, Now: req.Now})
	if err != nil {
		return LifecycleReport{}, err
	}
	report := LifecycleReport{Scope: req.Scope, EvaluatedAt: req.Now.UTC().Format(time.RFC3339Nano), LifecycleCounts: map[Lifecycle]int{}, HealthCounts: map[Health]int{}, States: append([]DerivedMemoryState(nil), derived.States...)}
	sort.Slice(report.States, func(i, j int) bool {
		if report.States[i].MemoryID != report.States[j].MemoryID {
			return report.States[i].MemoryID < report.States[j].MemoryID
		}
		return report.States[i].Revision < report.States[j].Revision
	})
	for _, state := range report.States {
		report.MemoryCount++
		report.LifecycleCounts[state.Lifecycle]++
		report.HealthCounts[state.Health]++
		report.UsageCount += state.Usage.UsageCount
		report.CountedHelpCount += state.Usage.CountedHelpCount
		report.CountedHarmCount += state.Usage.CountedHarmCount
		if state.Usage.InsufficientEvidence {
			report.InsufficientCount++
		}
	}
	return report, nil
}
