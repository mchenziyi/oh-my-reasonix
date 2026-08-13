package memory

import (
	"context"
	"encoding/json"
	"sort"
)

// MigrationDoctorFinding is a deterministic, redacted readiness finding.
type MigrationDoctorFinding struct {
	Code   string `json:"code"`
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	Detail string `json:"detail"`
}

// MigrationDoctorReport audits a verified source and target without writing
// facts, snapshots, generations, or CURRENT.
type MigrationDoctorReport struct {
	SourceScope         Scope                    `json:"source_scope"`
	TargetScope         Scope                    `json:"target_scope"`
	GenerationID        string                   `json:"generation_id"`
	SourceFactCount     int                      `json:"source_fact_count"`
	TargetMissingCount  int                      `json:"target_missing_count"`
	TargetExistingCount int                      `json:"target_existing_count"`
	Healthy             bool                     `json:"healthy"`
	Findings            []MigrationDoctorFinding `json:"findings"`
}

func (r MigrationDoctorReport) EncodeCanonical() ([]byte, error) { return json.Marshal(r) }

// CheckMigrationReadiness verifies the source snapshot and reports whether
// each immutable input is absent or already present in the target. It never
// performs the copy or treats an existing target fact as implicitly correct.
func CheckMigrationReadiness(ctx context.Context, source, target *FactStore, plan MigrationPlan) (*MigrationDoctorReport, error) {
	if source == nil || target == nil || source == target || source.root == target.root || source.storeScope != target.storeScope {
		return nil, storeError(CodeGenerationTxConflict, "migration doctor stores are invalid")
	}
	if !plan.Eligible || plan.SourceScope != plan.TargetScope || plan.SourceScope != scopeOfStore(source) || plan.TargetScope != scopeOfStore(target) {
		return nil, storeError(CodeGenerationTxConflict, "migration doctor plan is not eligible")
	}
	_, _, manifest, facts, err := loadMigrationSource(ctx, source, plan)
	report := &MigrationDoctorReport{SourceScope: plan.SourceScope, TargetScope: plan.TargetScope, GenerationID: plan.GenerationID, Findings: []MigrationDoctorFinding{}}
	if err != nil {
		report.Healthy = false
		report.Findings = append(report.Findings, MigrationDoctorFinding{Code: "source_invalid", Kind: "generation", Key: plan.GenerationID, Detail: "source generation or manifest is unavailable"})
		return report, nil
	}
	report.SourceFactCount = len(facts) + 1 // include the permanent input manifest
	for _, fact := range append(append([]Fact{}, facts...), manifest) {
		kind, key, keyErr := factKey(fact)
		if keyErr != nil {
			report.Healthy = false
			report.Findings = append(report.Findings, MigrationDoctorFinding{Code: "source_invalid", Kind: "fact", Detail: "source fact identity is invalid"})
			continue
		}
		data, getErr := target.Get(ctx, kind, key)
		if getErr != nil {
			if ErrorCode(getErr) == CodeNotFound {
				report.TargetMissingCount++
				continue
			}
			report.Healthy = false
			report.Findings = append(report.Findings, MigrationDoctorFinding{Code: "target_unreadable", Kind: string(kind), Key: key, Detail: "target fact cannot be read"})
			continue
		}
		stored, decodeErr := decodeKind(kind, data)
		if decodeErr != nil {
			report.Healthy = false
			report.Findings = append(report.Findings, MigrationDoctorFinding{Code: "target_corrupt", Kind: string(kind), Key: key, Detail: "target fact is invalid"})
			continue
		}
		want, _ := fact.ContentHash()
		got, _ := stored.ContentHash()
		if want != got {
			report.Healthy = false
			report.Findings = append(report.Findings, MigrationDoctorFinding{Code: "target_conflict", Kind: string(kind), Key: key, Detail: "target fact differs from source"})
			continue
		}
		report.TargetExistingCount++
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Code != report.Findings[j].Code {
			return report.Findings[i].Code < report.Findings[j].Code
		}
		if report.Findings[i].Kind != report.Findings[j].Kind {
			return report.Findings[i].Kind < report.Findings[j].Kind
		}
		return report.Findings[i].Key < report.Findings[j].Key
	})
	if len(report.Findings) == 0 {
		report.Healthy = true
	}
	return report, nil
}
