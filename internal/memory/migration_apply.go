package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// MigrationApplyRequest enables the complete same-scope migration after a
// verified preview. The target receives a new Generation and CURRENT CAS;
// the source remains untouched. Cross-scope requests are rejected.
type MigrationApplyRequest struct {
	Plan           MigrationPlan
	IdempotencyKey string
}

type MigrationApplyResult struct {
	Copy         MigrationCopyResult
	Commit       CommitResult
	GenerationID string
}

// ApplyMigration performs copy, target compilation from the source's verified
// derived views, and a normal Generation transaction. It never reuses the
// source Generation ID or mutates the source store.
func ApplyMigration(ctx context.Context, source, target *FactStore, req MigrationApplyRequest) (MigrationApplyResult, error) {
	if source == nil || target == nil || source == target || source.root == target.root || source.storeScope != target.storeScope || !req.Plan.Eligible || req.Plan.SourceScope != req.Plan.TargetScope {
		return MigrationApplyResult{}, storeError(CodeGenerationTxConflict, "migration apply plan is not eligible")
	}
	targetGS := NewGenerationStore(target).(*generationStore)
	sourceGen, sourceDir, sourceManifest, facts, err := loadMigrationSource(ctx, source, req.Plan)
	if err != nil {
		return MigrationApplyResult{}, err
	}
	copyResult, err := copyMigrationFacts(ctx, target, facts, sourceManifest)
	if err != nil {
		return MigrationApplyResult{}, err
	}
	outputs, err := readCompiledOutputs(sourceDir)
	if err != nil {
		return MigrationApplyResult{}, err
	}
	cur, err := targetGS.readCurrent(ctx)
	if err != nil {
		return MigrationApplyResult{}, err
	}
	var base *string
	if cur != nil {
		base = &cur.GenerationID
	}
	tx, err := targetGS.Begin(ctx, BeginGenerationRequest{Scope: req.Plan.TargetScope, BaseGeneration: base, CompilerVersion: sourceGen.CompilerVersion, CanonicalizationVersion: sourceGen.CanonicalizationVersion, SchemaVersion: SchemaVersion, IdempotencyKey: req.IdempotencyKey})
	if err != nil {
		return MigrationApplyResult{}, err
	}
	for _, input := range facts {
		if err := targetGS.PrepareFact(ctx, tx, input); err != nil {
			_ = targetGS.Abort(ctx, tx, "migration prepare failed")
			return MigrationApplyResult{}, err
		}
	}
	if err := targetGS.WriteCompiledOutput(ctx, tx, outputs); err != nil {
		_ = targetGS.Abort(ctx, tx, "migration output staging failed")
		return MigrationApplyResult{}, err
	}
	staging, err := targetGS.stagingDir(ctx, tx.GenerationID)
	if err != nil {
		_ = targetGS.Abort(ctx, tx, "migration staging path failed")
		return MigrationApplyResult{}, err
	}
	compiledHash, err := targetGS.compiledOutputHash(ctx, staging)
	if err != nil {
		_ = targetGS.Abort(ctx, tx, "migration output hash failed")
		return MigrationApplyResult{}, err
	}
	gen := generationDoc{SchemaVersion: SchemaVersion, GenerationID: tx.GenerationID, Scope: tx.Scope, BaseGeneration: baseOrNil(base), CompilerVersion: tx.CompilerVersion, CanonicalizationVersion: tx.CanonicalizationVersion, TransactionID: tx.TransactionID, CompiledOutputSHA256: compiledHash}
	gen.OutputGenerationSHA256, err = gen.outputHash()
	if err != nil {
		_ = targetGS.Abort(ctx, tx, "migration generation hash failed")
		return MigrationApplyResult{}, err
	}
	mf := manifestForMigration(sourceManifest, tx, gen.OutputGenerationSHA256)
	if err := targetGS.PrepareManifest(ctx, tx, mf); err != nil {
		_ = targetGS.Abort(ctx, tx, "migration manifest failed")
		return MigrationApplyResult{}, err
	}
	commit, err := targetGS.Commit(ctx, tx)
	if err != nil && commit.Status != CommitPendingRecovery {
		return MigrationApplyResult{}, err
	}
	return MigrationApplyResult{Copy: copyResult, Commit: commit, GenerationID: commit.GenerationID}, err
}

func baseOrNil(base *string) any {
	if base == nil {
		return nil
	}
	return *base
}

func manifestForMigration(source GenerationInputManifest, tx *GenerationTx, outputHash string) GenerationInputManifest {
	m := source
	m.GenerationID = tx.GenerationID
	m.Scope = tx.Scope
	m.BaseGeneration = tx.BaseGeneration
	m.CompilerVersion = tx.CompilerVersion
	m.CanonicalizationVersion = tx.CanonicalizationVersion
	m.OutputSHA256 = outputHash
	m.TransactionID = tx.TransactionID
	m.InputManifestSHA256 = ""
	h, _ := m.ContentHash()
	m.InputManifestSHA256 = h
	return m
}

func readCompiledOutputs(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, sub := range []string{"wiki", "state"} {
		root := filepath.Join(dir, sub)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("migration source compiled output contains a symbolic link")
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				return errors.New("migration source output path is unsafe")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = data
			return nil
		})
		if err != nil {
			return nil, storeError(CodeGenerationStagingInvalid, "migration source compiled output is unreadable")
		}
	}
	return out, nil
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
	_, _, mf, facts, err := loadMigrationSource(ctx, source, req.Plan)
	if err != nil {
		return MigrationCopyResult{}, err
	}
	return copyMigrationFacts(ctx, target, facts, mf)
}

func loadMigrationSource(ctx context.Context, source *FactStore, plan MigrationPlan) (generationDoc, string, GenerationInputManifest, []Fact, error) {
	gs := NewGenerationStore(source).(*generationStore)
	gen, dir, err := readPublishedGeneration(gs, plan.GenerationID)
	if err != nil || gen.Scope != scopeOfStore(source) {
		return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationStagingInvalid, "migration source generation is invalid")
	}
	if err := gs.verifyCompiledOutputIntegrity(ctx, dir, gen); err != nil {
		return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationStagingInvalid, "migration source compiled output is invalid")
	}
	mfBytes, err := source.Get(ctx, FactKindGenerationInputManifest, plan.GenerationID)
	if err != nil {
		return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationManifestMismatch, "migration source manifest is unavailable")
	}
	mf, err := DecodeStrict[GenerationInputManifest](mfBytes)
	if err != nil || mf.InputManifestSHA256 != plan.InputManifestSHA256 || mf.GenerationID != plan.GenerationID {
		return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationManifestMismatch, "migration source manifest does not match plan")
	}
	facts := make([]Fact, 0, len(mf.Inputs))
	for _, input := range mf.Inputs {
		kind, key, err := resolveManifestInput(input.FactType, input.FactID)
		if err != nil {
			return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationManifestMismatch, "migration input identity is invalid")
		}
		data, err := source.Get(ctx, kind, key)
		if err != nil {
			data, err = preparedMigrationFact(ctx, gs, gen.TransactionID, input)
		}
		if err != nil {
			return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationManifestMismatch, "migration input fact is unavailable")
		}
		fact, err := decodeKind(kind, data)
		if err != nil {
			return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeGenerationManifestMismatch, "migration input fact is invalid")
		}
		if sc, ok := factScope(fact); !ok || sc != plan.SourceScope {
			return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeScopeMismatch, "migration input scope mismatch")
		}
		h, err := fact.ContentHash()
		if err != nil || h != input.ContentSHA256 {
			return generationDoc{}, "", GenerationInputManifest{}, nil, storeError(CodeHashMismatch, "migration input hash mismatch")
		}
		facts = append(facts, fact)
	}
	return gen, dir, mf, facts, nil
}

func copyMigrationFacts(ctx context.Context, target *FactStore, facts []Fact, mf GenerationInputManifest) (MigrationCopyResult, error) {
	all := append(append([]Fact{}, facts...), mf)
	results, err := target.PutBatch(ctx, all)
	if err != nil {
		return MigrationCopyResult{}, err
	}
	result := MigrationCopyResult{GenerationID: mf.GenerationID, FactCount: len(results)}
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
