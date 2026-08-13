package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// RepairPlan is a read-only diagnosis. It never rebuilds or changes CURRENT.
type RepairPlan struct {
	Operation           string `json:"operation"`
	Scope               Scope  `json:"scope"`
	CurrentGenerationID string `json:"current_generation_id,omitempty"`
	Action              string `json:"action"`
	Rebuildable         bool   `json:"rebuildable"`
	ExpectedOutputHash  string `json:"expected_output_hash,omitempty"`
	BlockedReason       string `json:"blocked_reason,omitempty"`
}

// BuildRepairPlan verifies only the effective generation and reports whether
// its derived views can be rebuilt from the durable manifest/compiler pair.
func BuildRepairPlan(ctx context.Context, store GenerationStore) (RepairPlan, error) {
	gs, ok := store.(*generationStore)
	if !ok || gs == nil {
		return RepairPlan{}, errors.New("repair plan: unsupported generation store")
	}
	cur, err := gs.readCurrent(ctx)
	if err != nil {
		return RepairPlan{}, err
	}
	plan := RepairPlan{Operation: "repair", Scope: scopeForStore(gs)}
	if cur == nil {
		plan.Action = "none"
		return plan, nil
	}
	plan.CurrentGenerationID = cur.GenerationID
	doc, dir, err := readPublishedGeneration(gs, cur.GenerationID)
	if err != nil {
		plan.Action = "blocked"
		plan.BlockedReason = "current generation is unreadable"
		return plan, nil
	}
	plan.ExpectedOutputHash = doc.OutputGenerationSHA256
	if err := gs.verifyCompiledOutputIntegrity(ctx, dir, doc); err == nil {
		plan.Action = "none"
		return plan, nil
	}
	if _, err := os.Stat(filepath.Join(gs.store.root, "facts", string(FactKindGenerationInputManifest), cur.GenerationID+".json")); err != nil {
		plan.Action = "blocked"
		plan.BlockedReason = "generation input manifest is unavailable"
		return plan, nil
	}
	if _, ok := supportedGenerationCompilers[doc.CompilerVersion]; !ok {
		plan.Action = "blocked"
		plan.BlockedReason = "generation compiler is unavailable"
		return plan, nil
	}
	plan.Action = "rebuild_derived_views"
	plan.Rebuildable = true
	return plan, nil
}

// RollbackPlan is a read-only CAS preview. The target is never activated by
// this function; a later transaction must explicitly approve the switch.
type RollbackPlan struct {
	Operation           string `json:"operation"`
	Scope               Scope  `json:"scope"`
	CurrentGenerationID string `json:"current_generation_id"`
	TargetGenerationID  string `json:"target_generation_id"`
	TargetOutputHash    string `json:"target_output_hash"`
	Eligible            bool   `json:"eligible"`
	BlockedReason       string `json:"blocked_reason,omitempty"`
}

func BuildRollbackPlan(ctx context.Context, store GenerationStore, targetID string) (RollbackPlan, error) {
	gs, ok := store.(*generationStore)
	if !ok || gs == nil {
		return RollbackPlan{}, errors.New("rollback plan: unsupported generation store")
	}
	if err := validateID(targetID, "generation_id"); err != nil {
		return RollbackPlan{}, errors.New("rollback plan: invalid target generation")
	}
	cur, err := gs.readCurrent(ctx)
	if err != nil {
		return RollbackPlan{}, err
	}
	plan := RollbackPlan{Operation: "rollback", Scope: scopeForStore(gs), TargetGenerationID: targetID}
	if cur == nil {
		plan.BlockedReason = "CURRENT is empty"
		return plan, nil
	}
	plan.CurrentGenerationID = cur.GenerationID
	if cur.GenerationID == targetID {
		plan.BlockedReason = "target generation is already current"
		return plan, nil
	}
	doc, dir, err := readPublishedGeneration(gs, targetID)
	if err != nil {
		plan.BlockedReason = "target generation is unreadable"
		return plan, nil
	}
	if doc.Scope != scopeForStore(gs) {
		plan.BlockedReason = "target generation scope does not match"
		return plan, nil
	}
	if err := gs.verifyCompiledOutputIntegrity(ctx, dir, doc); err != nil {
		plan.BlockedReason = "target generation integrity check failed"
		return plan, nil
	}
	plan.TargetOutputHash = doc.OutputGenerationSHA256
	plan.Eligible = true
	return plan, nil
}

func readPublishedGeneration(gs *generationStore, id string) (generationDoc, string, error) {
	dir, err := secureJoin(gs.store.root, []string{"generations", id}, false, false)
	if err != nil {
		return generationDoc{}, "", err
	}
	doc, err := readJSONFile[generationDoc](filepath.Join(dir, "generation.json"))
	if err != nil || doc.GenerationID != id {
		return generationDoc{}, "", errors.New("generation is invalid")
	}
	h, err := doc.outputHash()
	if err != nil || h != doc.OutputGenerationSHA256 {
		return generationDoc{}, "", errors.New("generation hash is invalid")
	}
	return doc, dir, nil
}

func scopeForStore(gs *generationStore) Scope {
	if gs.store.storeScope == StoreScopeGlobal {
		return ScopeGlobal
	}
	return ScopeProject
}

func (p RepairPlan) CanonicalBytes() ([]byte, error)   { return json.Marshal(p) }
func (p RollbackPlan) CanonicalBytes() ([]byte, error) { return json.Marshal(p) }
func (p RepairPlan) PlanHash() string                  { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
func (p RollbackPlan) PlanHash() string                { b, _ := p.CanonicalBytes(); return NewContentHash(b) }
