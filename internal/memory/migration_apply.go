package memory

import (
	"context"
	"errors"
)

// MigrationCopyRequest authorizes copying the immutable input facts of a
// verified same-scope MigrationPlan. It does not switch CURRENT or publish a
// Generation; compilation and switching remain separate explicit steps.
type MigrationCopyRequest struct {
	Plan MigrationPlan
}

type MigrationCopyResult struct {
	GenerationID string
	FactCount    int
	Created      int
	Noop         int
}

// ApplyMigrationCopy verifies the source Generation/Manifest again and
// atomically copies its complete input fact set into the distinct target
// Store. PutBatch guarantees one lock, no-overwrite identities, and rollback
// of facts created by this batch if publication fails.
func ApplyMigrationCopy(ctx context.Context, source, target *FactStore, req MigrationCopyRequest) (MigrationCopyResult, error) {
	if source == nil || target == nil || source == target || source.root == target.root {
		return MigrationCopyResult{}, errors.New("migration copy: source and target stores must be distinct")
	}
	if req.Plan.Operation != "migration_preview" || !req.Plan.Eligible || req.Plan.SourceScope != req.Plan.TargetScope || source.storeScope != target.storeScope || req.Plan.SourceScope != scopeOfStore(source) {
		return MigrationCopyResult{}, storeError(CodeGenerationTxConflict, "migration copy plan is not eligible")
	}
	if err := validateID(req.Plan.GenerationID, "generation_id"); err != nil {
		return MigrationCopyResult{}, storeError(CodePathUnsafe, "invalid migration generation")
	}
	gs := NewGenerationStore(source).(*generationStore)
	gen, _, err := readPublishedGeneration(gs, req.Plan.GenerationID)
	if err != nil || gen.Scope != scopeOfStore(source) {
		return MigrationCopyResult{}, storeError(CodeGenerationStagingInvalid, "migration source generation is invalid")
	}
	mfBytes, err := source.Get(ctx, FactKindGenerationInputManifest, req.Plan.GenerationID)
	if err != nil {
		return MigrationCopyResult{}, storeError(CodeGenerationManifestMismatch, "migration source manifest is unavailable")
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfBytes)
	if err != nil || mf.InputManifestSHA256 != req.Plan.InputManifestSHA256 || mf.GenerationID != req.Plan.GenerationID {
		return MigrationCopyResult{}, storeError(CodeGenerationManifestMismatch, "migration source manifest does not match plan")
	}
	facts := make([]Fact, 0, len(mf.Inputs)+1)
	for _, input := range mf.Inputs {
		kind, key, err := resolveManifestInput(input.FactType, input.FactID)
		if err != nil {
			return MigrationCopyResult{}, storeError(CodeGenerationManifestMismatch, "migration input identity is invalid")
		}
		data, err := source.Get(ctx, kind, key)
		if err != nil {
			// MEM-01D transactions may intentionally keep prepared inputs
			// isolated from the ordinary FactStore. Recover the exact canonical
			// input from the committed transaction record instead of guessing or
			// scanning another fact.
			data, err = preparedMigrationFact(ctx, gs, gen.TransactionID, input)
			if err != nil {
				return MigrationCopyResult{}, storeError(CodeGenerationManifestMismatch, "migration input fact is unavailable")
			}
		}
		fact, err := decodeKind(kind, data)
		if err != nil {
			return MigrationCopyResult{}, storeError(CodeGenerationManifestMismatch, "migration input fact is invalid")
		}
		if factScope, ok := factScope(fact); !ok || factScope != req.Plan.SourceScope {
			return MigrationCopyResult{}, storeError(CodeScopeMismatch, "migration input scope mismatch")
		}
		h, err := fact.ContentHash()
		if err != nil || h != input.ContentSHA256 {
			return MigrationCopyResult{}, storeError(CodeHashMismatch, "migration input hash mismatch")
		}
		facts = append(facts, fact)
	}
	facts = append(facts, mf)
	results, err := target.PutBatch(ctx, facts)
	if err != nil {
		return MigrationCopyResult{}, err
	}
	result := MigrationCopyResult{GenerationID: req.Plan.GenerationID, FactCount: len(results)}
	for _, item := range results {
		if item.Status == WriteCreated {
			result.Created++
		} else {
			result.Noop++
		}
	}
	return result, nil
}

func preparedMigrationFact(ctx context.Context, gs *generationStore, txID string, input ManifestInput) ([]byte, error) {
	if err := validateID(txID, "transaction_id"); err != nil {
		return nil, err
	}
	dir, err := gs.txDir(ctx, txID)
	if err != nil {
		return nil, err
	}
	rec, err := readJSONFile[txRecord](dir + "/prepared.json")
	if err != nil {
		return nil, err
	}
	for _, prepared := range rec.PreparedFacts {
		if prepared.FactType == input.FactType && prepared.FactID == input.FactID && prepared.ContentSHA256 == input.ContentSHA256 && prepared.FactSchemaVersion == input.FactSchemaVersion {
			return prepared.Canonical, nil
		}
	}
	return nil, errors.New("prepared migration fact not found")
}
