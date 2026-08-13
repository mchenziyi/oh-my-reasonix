package memory

import (
	"encoding/json"
	"errors"
)

type MigrationRequest struct {
	SourceScope          Scope
	TargetScope          Scope
	ProjectGenerationRef *ProjectGenerationRef
	GlobalGenerationRef  *GlobalGenerationRef
	FactCount            int
	InputManifestSHA256  string
}

// MigrationPlan is a read-only preview. It never copies facts or switches
// CURRENT; Project->Global must use the Promotion gate instead.
type MigrationPlan struct {
	Operation           string   `json:"operation"`
	SourceScope         Scope    `json:"source_scope"`
	TargetScope         Scope    `json:"target_scope"`
	GenerationID        string   `json:"generation_id"`
	InputManifestSHA256 string   `json:"input_manifest_sha256"`
	FactCount           int      `json:"fact_count"`
	SnapshotRequired    bool     `json:"snapshot_required"`
	Steps               []string `json:"steps"`
	Eligible            bool     `json:"eligible"`
	BlockedReason       string   `json:"blocked_reason,omitempty"`
}

func BuildMigrationPlan(req MigrationRequest) (MigrationPlan, error) {
	plan := MigrationPlan{Operation: "migration_preview", SourceScope: req.SourceScope, TargetScope: req.TargetScope, FactCount: req.FactCount, SnapshotRequired: true, Steps: []string{"preview", "snapshot", "copy", "compile", "doctor", "switch"}}
	if req.SourceScope != ScopeProject && req.SourceScope != ScopeGlobal || req.TargetScope != ScopeProject && req.TargetScope != ScopeGlobal {
		return MigrationPlan{}, errors.New("migration plan: invalid scope")
	}
	if req.SourceScope != req.TargetScope {
		plan.BlockedReason = "cross-scope migration requires promotion"
		return plan, nil
	}
	if req.FactCount < 0 {
		return MigrationPlan{}, errors.New("migration plan: fact count must not be negative")
	}
	if err := validateHash(req.InputManifestSHA256, "input_manifest_sha256"); err != nil {
		return MigrationPlan{}, errors.New("migration plan: invalid manifest hash")
	}
	switch req.SourceScope {
	case ScopeProject:
		if req.ProjectGenerationRef == nil {
			return MigrationPlan{}, errors.New("migration plan: project generation ref is required")
		}
		if err := req.ProjectGenerationRef.Validate(); err != nil {
			return MigrationPlan{}, errors.New("migration plan: project generation ref is invalid")
		}
		plan.GenerationID = req.ProjectGenerationRef.GenerationID
	case ScopeGlobal:
		if req.GlobalGenerationRef == nil {
			return MigrationPlan{}, errors.New("migration plan: global generation ref is required")
		}
		if err := req.GlobalGenerationRef.Validate(); err != nil {
			return MigrationPlan{}, errors.New("migration plan: global generation ref is invalid")
		}
		plan.GenerationID = req.GlobalGenerationRef.GenerationID
	}
	plan.InputManifestSHA256 = req.InputManifestSHA256
	plan.Eligible = true
	return plan, nil
}

func (p MigrationPlan) CanonicalBytes() ([]byte, error) { return json.Marshal(p) }
func (p MigrationPlan) PlanHash() string                { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
