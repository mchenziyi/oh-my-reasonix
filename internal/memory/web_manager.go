package memory

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// WebManagementAction is a validated, deterministic intent emitted by a
// local management UI. It is not a Fact and never performs the operation.
type WebManagementAction struct {
	SchemaVersion int        `json:"schema_version"`
	ActionID      string     `json:"action_id"`
	Scope         Scope      `json:"scope"`
	Target        MemoryRef  `json:"target"`
	Operation     string     `json:"operation"`
	Reason        string     `json:"reason"`
	BasisRefs     []BasisRef `json:"basis_refs"`
	RequestedAt   string     `json:"requested_at"`
}

func (a WebManagementAction) Validate() error {
	if a.SchemaVersion != SchemaVersion {
		return errors.New("web action: schema_version is invalid")
	}
	if err := validateID(a.ActionID, "action_id"); err != nil {
		return errors.New("web action: action_id is invalid")
	}
	if err := a.Scope.Validate(); err != nil || a.Scope == ScopePortable {
		return errors.New("web action: scope is invalid")
	}
	if err := a.Target.Validate(); err != nil || a.Target.Scope != a.Scope {
		return errors.New("web action: target is invalid")
	}
	switch a.Operation {
	case "pin", "unpin", "freeze", "unfreeze", "archive":
	default:
		return errors.New("web action: operation is invalid")
	}
	if err := validateSummary(a.Reason); err != nil {
		return errors.New("web action: reason is invalid")
	}
	if a.Operation == "unfreeze" && len(a.BasisRefs) == 0 {
		return errors.New("web action: unfreeze requires basis_refs")
	}
	for _, ref := range a.BasisRefs {
		if err := ref.Validate(); err != nil {
			return errors.New("web action: basis_refs is invalid")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, a.RequestedAt); err != nil {
		return errors.New("web action: requested_at is invalid")
	}
	return nil
}

func (a WebManagementAction) CanonicalBytes() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

func (a WebManagementAction) ContentHash() (string, error) {
	b, err := a.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return NewContentHash(b), nil
}

// ApplyWebManagementAction executes a validated action only after an explicit
// second confirmation. It delegates all persistence and lifecycle checks to
// the existing Governance API; the web protocol adds no write path of its own.
func ApplyWebManagementAction(ctx context.Context, store *FactStore, action WebManagementAction, confirmed bool) (GovernanceResult, error) {
	if store == nil {
		return GovernanceResult{}, storeError(CodeDerivedInvalidInput, "web action store is unavailable")
	}
	if !confirmed {
		return GovernanceResult{}, storeError(CodeDerivedInvalidInput, "web action requires explicit confirmation")
	}
	if err := action.Validate(); err != nil {
		return GovernanceResult{}, storeError(CodeSchemaInvalid, "web action is invalid")
	}
	now, err := time.Parse(time.RFC3339Nano, action.RequestedAt)
	if err != nil {
		return GovernanceResult{}, storeError(CodeDerivedInvalidInput, "web action timestamp is invalid")
	}
	operation := action.Operation
	if operation == "freeze" {
		operation = "manual_freeze"
	}
	return CommitGovernanceEvent(ctx, GovernanceRequest{
		Store: store, Target: action.Target, Operation: operation,
		Reason: action.Reason, Source: "local_web", BasisRefs: action.BasisRefs, Now: now.UTC(),
	})
}
