package memory

import (
	"context"
	"time"
)

type EvidenceValidatedGateRequest struct {
	Scope                 Scope
	MemoryID              string
	Revision              int
	ExpectedMemoryContext MemoryContext
	ProjectStore          *FactStore
	GlobalStore           *FactStore
	Now                   time.Time
}

type EvidenceValidatedGateResult struct {
	EvidenceCount  int                       `json:"evidence_count"`
	RootTaskCount  int                       `json:"root_task_count"`
	CriticStatus   CriticRequirementStatus   `json:"critic_status"`
	ConflictStatus ConflictRequirementStatus `json:"conflict_status"`
	Satisfied      bool                      `json:"satisfied"`
}

// EvaluateEvidenceValidatedGate composes already-frozen, read-only facts. It
// does not mutate Lifecycle, indexes, CURRENT, or any canonical fact.
func EvaluateEvidenceValidatedGate(ctx context.Context, store *FactStore, req EvidenceValidatedGateRequest) (*EvidenceValidatedGateResult, error) {
	criticReq := CriticRequirementRequest{
		Scope: req.Scope, MemoryID: req.MemoryID, Revision: req.Revision,
		ExpectedMemoryContext: req.ExpectedMemoryContext,
		ProjectStore:          req.ProjectStore, GlobalStore: req.GlobalStore, Now: req.Now,
	}
	critic, err := EvaluateCriticRequirement(ctx, store, criticReq)
	if err != nil {
		return nil, err
	}
	conflict, err := EvaluateConflictRequirement(ctx, store, ConflictRequirementRequest{
		Scope: req.Scope, MemoryID: req.MemoryID, Revision: req.Revision,
		ExpectedMemoryContext: req.ExpectedMemoryContext,
		ProjectStore:          req.ProjectStore, GlobalStore: req.GlobalStore, Now: req.Now,
	})
	if err != nil {
		return nil, err
	}
	result := &EvidenceValidatedGateResult{
		CriticStatus: critic.Status, ConflictStatus: conflict.Status,
	}
	world, err := loadFixedMemoryWorld(ctx, req.ExpectedMemoryContext, req.Scope, req.ProjectStore, req.GlobalStore)
	if err != nil {
		if err == errWorldUnavailable {
			return result, nil
		}
		return nil, err
	}
	evidence, roots, err := world.collectEvidence(ctx, req.MemoryID, req.Revision, req.Now)
	if err != nil {
		return nil, err
	}
	result.EvidenceCount = len(evidence)
	result.RootTaskCount = len(roots)
	result.Satisfied = critic.UsagePolicy == UsagePolicyEvidenceValidated &&
		result.EvidenceCount >= 3 && result.RootTaskCount >= 2 &&
		critic.Status == CriticRequirementPassed && conflict.Status == ConflictRequirementClear
	return result, nil
}
