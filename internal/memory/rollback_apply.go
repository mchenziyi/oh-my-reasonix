package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// RollbackRequest is the explicit operator authorization for a rollback.
// Now is required so the audit record is deterministic and replayable.
type RollbackRequest struct {
	Plan           RollbackPlan
	Operator       string
	Reason         string
	Now            time.Time
	IdempotencyKey string
}

type RollbackResult struct {
	RollbackID         string `json:"rollback_id"`
	Status             string `json:"status"`
	SourceGenerationID string `json:"source_generation_id"`
	TargetGenerationID string `json:"target_generation_id"`
	TargetOutputHash   string `json:"target_output_hash"`
	AuditSHA256        string `json:"audit_sha256"`
}

type rollbackRecord struct {
	SchemaVersion      int    `json:"schema_version"`
	RollbackID         string `json:"rollback_id"`
	RequestSHA256      string `json:"request_sha256"`
	Scope              Scope  `json:"scope"`
	Operator           string `json:"operator"`
	Reason             string `json:"reason"`
	SourceGenerationID string `json:"source_generation_id"`
	TargetGenerationID string `json:"target_generation_id"`
	TargetOutputHash   string `json:"target_output_hash"`
	CreatedAt          string `json:"created_at"`
	Status             string `json:"status"`
}

type rollbackClaim struct {
	SchemaVersion  int    `json:"schema_version"`
	IdempotencyKey string `json:"idempotency_key"`
	RequestSHA256  string `json:"request_sha256"`
	RollbackID     string `json:"rollback_id"`
	Status         string `json:"status"`
}

// ApplyRollbackPlan performs one audited CURRENT switch. It never changes or
// deletes a Generation or a normative Fact; a later generation may be built
// on top of the rolled-back CURRENT.
func ApplyRollbackPlan(ctx context.Context, store GenerationStore, req RollbackRequest) (RollbackResult, error) {
	gs, ok := store.(*generationStore)
	if !ok || gs == nil {
		return RollbackResult{}, errors.New("rollback: unsupported generation store")
	}
	if !req.Plan.Eligible || req.Plan.Operation != "rollback" || req.Plan.Scope != scopeForStore(gs) {
		return RollbackResult{}, storeError(CodeGenerationTxConflict, "rollback plan is not eligible")
	}
	if err := validateID(req.Plan.CurrentGenerationID, "generation_id"); err != nil {
		return RollbackResult{}, storeError(CodePathUnsafe, "invalid rollback source")
	}
	if err := validateID(req.Plan.TargetGenerationID, "generation_id"); err != nil {
		return RollbackResult{}, storeError(CodePathUnsafe, "invalid rollback target")
	}
	if err := validateID(req.IdempotencyKey, "idempotency_key"); err != nil {
		return RollbackResult{}, storeError(CodePathUnsafe, "invalid rollback idempotency key")
	}
	if req.Now.IsZero() {
		return RollbackResult{}, storeError(CodeDerivedInvalidInput, "rollback requires an explicit now timestamp")
	}
	if req.Operator == "" || len(req.Operator) > 128 || strings.ContainsAny(req.Operator, "\r\n") || req.Reason == "" || len(req.Reason) > 1024 || strings.ContainsAny(req.Reason, "\r\n") {
		return RollbackResult{}, storeError(CodeSchemaInvalid, "rollback audit fields are invalid")
	}
	requestBytes, _ := json.Marshal(struct {
		PlanHash, Operator, Reason, Now string
	}{req.Plan.PlanHash(), req.Operator, req.Reason, req.Now.UTC().Format(time.RFC3339Nano)})
	reqHash := hashOf(requestBytes)

	unlock, err := gs.store.acquireWriteLock(ctx)
	if err != nil {
		return RollbackResult{}, err
	}
	defer unlock()
	claimPath, err := secureJoin(gs.store.root, []string{"rollback-idempotency", req.IdempotencyKey + ".json"}, true, true)
	if err != nil {
		return RollbackResult{}, err
	}
	if _, statErr := os.Lstat(claimPath); statErr == nil {
		existing, rerr := readJSONFile[rollbackClaim](claimPath)
		if rerr != nil {
			return RollbackResult{}, storeError(CodeGenerationRecoveryBlocked, "rollback claim is unreadable")
		}
		if existing.RequestSHA256 != reqHash {
			return RollbackResult{}, storeError(CodeGenerationIdempotency, "rollback idempotency key is already used")
		}
		recPath, perr := secureJoin(gs.store.root, []string{"rollbacks", existing.RollbackID + ".json"}, false, true)
		if perr != nil {
			return RollbackResult{}, perr
		}
		rec, perr := readJSONFile[rollbackRecord](recPath)
		if perr != nil || rec.Status != "committed" {
			return RollbackResult{}, storeError(CodeGenerationRecoveryBlocked, "rollback recovery is pending")
		}
		return rollbackResult(rec), nil
	} else if !os.IsNotExist(statErr) {
		return RollbackResult{}, storeError(CodePermissionDenied, "cannot inspect rollback claim")
	}
	cur, err := gs.readCurrent(ctx)
	if err != nil {
		return RollbackResult{}, err
	}
	if cur == nil || cur.GenerationID != req.Plan.CurrentGenerationID {
		return RollbackResult{}, storeError(CodeGenerationCurrentCAS, "CURRENT does not match rollback source")
	}
	target, targetDir, err := readPublishedGeneration(gs, req.Plan.TargetGenerationID)
	if err != nil {
		return RollbackResult{}, storeError(CodeGenerationStagingInvalid, "rollback target is unreadable")
	}
	if target.OutputGenerationSHA256 != req.Plan.TargetOutputHash || target.Scope != scopeForStore(gs) {
		return RollbackResult{}, storeError(CodeGenerationStagingInvalid, "rollback target does not match plan")
	}
	if err := gs.verifyCompiledOutputIntegrity(ctx, targetDir, target); err != nil {
		return RollbackResult{}, storeError(CodeGenerationStagingInvalid, "rollback target integrity check failed")
	}
	rbID, err := newRandomID("rollback")
	if err != nil {
		return RollbackResult{}, storeError(CodePermissionDenied, "cannot create rollback id")
	}
	claim := rollbackClaim{SchemaVersion: SchemaVersion, IdempotencyKey: req.IdempotencyKey, RequestSHA256: reqHash, RollbackID: rbID, Status: "pending"}
	claimData, _ := json.Marshal(claim)
	if err := gs.store.atomicWriteFile(claimPath, claimData); err != nil {
		return RollbackResult{}, err
	}
	rec := rollbackRecord{SchemaVersion: SchemaVersion, RollbackID: rbID, RequestSHA256: reqHash, Scope: scopeForStore(gs), Operator: req.Operator, Reason: req.Reason, SourceGenerationID: cur.GenerationID, TargetGenerationID: target.GenerationID, TargetOutputHash: target.OutputGenerationSHA256, CreatedAt: req.Now.UTC().Format(time.RFC3339Nano), Status: "pending"}
	recPath, err := secureJoin(gs.store.root, []string{"rollbacks", rbID + ".json"}, true, true)
	if err != nil {
		return RollbackResult{}, err
	}
	if err := gs.store.atomicWriteFile(recPath, rollbackJSON(rec)); err != nil {
		return RollbackResult{}, err
	}
	cur.GenerationID = target.GenerationID
	cur.OutputGenerationSHA256 = target.OutputGenerationSHA256
	cur.TransactionID = rbID
	cur.CreatedAt = req.Now.UTC().Format(time.RFC3339Nano)
	curPath, err := gs.currentPath()
	if err != nil {
		return RollbackResult{}, err
	}
	if err := gs.atomicReplace(curPath, rollbackJSON(cur)); err != nil {
		return RollbackResult{}, err
	}
	rec.Status = "committed"
	if err := gs.atomicReplace(recPath, rollbackJSON(rec)); err != nil {
		return RollbackResult{}, storeError(CodeGenerationRecoveryPending, "rollback is effective but audit is pending")
	}
	claim.Status = "committed"
	if err := gs.atomicReplace(claimPath, rollbackJSON(claim)); err != nil {
		return RollbackResult{}, storeError(CodeGenerationRecoveryPending, "rollback is effective but claim is pending")
	}
	return rollbackResult(rec), nil
}

func rollbackResult(rec rollbackRecord) RollbackResult {
	data := rollbackJSON(rec)
	return RollbackResult{RollbackID: rec.RollbackID, Status: rec.Status, SourceGenerationID: rec.SourceGenerationID, TargetGenerationID: rec.TargetGenerationID, TargetOutputHash: rec.TargetOutputHash, AuditSHA256: hashOf(data)}
}

func rollbackJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
