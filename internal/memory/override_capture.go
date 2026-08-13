package memory

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

// AttributionOverrideRequest is an explicit user correction of one Outcome.
// It creates a new immutable JudgmentFact and never mutates the Outcome.
type AttributionOverrideRequest struct {
	Store          *FactStore
	OutcomeID      string
	PreviousEffect string
	NewEffect      string
	Reason         string
	SourceType     string
	SourceID       string
	Now            time.Time
}

type AttributionOverrideResult struct {
	Status   WriteStatus  `json:"status"`
	Judgment JudgmentFact `json:"judgment"`
}

func BuildAttributionOverride(ctx context.Context, req AttributionOverrideRequest) (JudgmentFact, error) {
	if req.Store == nil || validateID(req.OutcomeID, "outcome_id") != nil || req.Now.IsZero() {
		return JudgmentFact{}, storeError(CodeDerivedInvalidInput, "attribution override request is invalid")
	}
	if validEffect(req.PreviousEffect, "previous_effect") != nil || validEffect(req.NewEffect, "new_effect") != nil || validateText(req.Reason, maxReasonLen, "reason", true) != nil {
		return JudgmentFact{}, storeError(CodeSchemaInvalid, "attribution override payload is invalid")
	}
	if validateID(req.SourceType, "source_type") != nil || validateID(req.SourceID, "source_id") != nil {
		return JudgmentFact{}, storeError(CodeSchemaInvalid, "attribution override source is invalid")
	}
	b, err := req.Store.Get(ctx, FactKindOutcome, req.OutcomeID)
	if err != nil {
		return JudgmentFact{}, err
	}
	outcome, err := DecodeStrict[Outcome](b)
	if err != nil {
		return JudgmentFact{}, classifyDecodeError(err)
	}
	current := outcome.Effect
	var supersedes *JudgmentRef
	judgments, err := loadJudgmentsForOutcome(ctx, req.Store, req.OutcomeID)
	if err != nil {
		return JudgmentFact{}, err
	}
	if len(judgments) > 0 {
		latest := judgments[len(judgments)-1]
		if latest.AttributionOverride == nil {
			return JudgmentFact{}, storeError(CodeSchemaInvalid, "attribution override history is invalid")
		}
		if latest.AttributionOverride.NewEffect == req.NewEffect && latest.AttributionOverride.Reason == req.Reason && latest.Source.SourceType == req.SourceType && latest.Source.SourceID == req.SourceID && latest.CreatedAt == req.Now.UTC().Format(time.RFC3339Nano) {
			return latest, nil
		}
		current = latest.AttributionOverride.NewEffect
		supersedes = &JudgmentRef{Scope: latest.Scope, JudgmentType: latest.JudgmentType, JudgmentID: latest.JudgmentID, ContentSHA256: latest.ContentSHA256}
	}
	if req.PreviousEffect != current {
		return JudgmentFact{}, storeError(CodeDerivedInvalidInput, "attribution override previous effect does not match current effect")
	}
	identity, _ := json.Marshal(map[string]any{"scope": outcome.Scope, "outcome_id": req.OutcomeID, "previous_effect": req.PreviousEffect, "new_effect": req.NewEffect, "reason": req.Reason, "source_type": req.SourceType, "source_id": req.SourceID, "created_at": req.Now.UTC().Format(time.RFC3339Nano), "supersedes": supersedes})
	idHash := hashOf(identity)
	j := JudgmentFact{SchemaVersion: SchemaVersion, JudgmentID: "judgment_override_" + idHash[len("sha256_"):len("sha256_")+24], JudgmentType: JudgmentTypeAttributionOverride, Scope: outcome.Scope, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: req.OutcomeID}, Source: JudgmentSource{SourceType: req.SourceType, SourceID: req.SourceID}, AttributionOverride: &AttributionOverridePayload{PreviousEffect: req.PreviousEffect, NewEffect: req.NewEffect, Reason: req.Reason}, SupersedesJudgmentRef: supersedes, CreatedAt: req.Now.UTC().Format(time.RFC3339Nano)}
	h, err := j.ContentHash()
	if err != nil {
		return JudgmentFact{}, storeError(CodeSchemaInvalid, "attribution override cannot be hashed")
	}
	j.ContentSHA256 = h
	if err := j.Validate(); err != nil {
		return JudgmentFact{}, classifyValidateError(err)
	}
	return j, nil
}

func loadJudgmentsForOutcome(ctx context.Context, store *FactStore, outcomeID string) ([]JudgmentFact, error) {
	keys, err := store.List(ctx, FactKindJudgment)
	if err != nil {
		return nil, err
	}
	var out []JudgmentFact
	for _, key := range keys {
		b, err := store.Get(ctx, FactKindJudgment, key)
		if err != nil {
			return nil, err
		}
		j, err := DecodeStrict[JudgmentFact](b)
		if err != nil {
			return nil, classifyDecodeError(err)
		}
		if j.JudgmentType == JudgmentTypeAttributionOverride && j.Subject.SubjectType == "memory_outcome" && j.Subject.OutcomeID == outcomeID {
			out = append(out, j)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].JudgmentID < out[j].JudgmentID
	})
	// The append-only chain must advance from the latest judgment; a broken
	// supersede reference is not silently treated as current state.
	if len(out) > 1 {
		for i := 1; i < len(out); i++ {
			if out[i].SupersedesJudgmentRef == nil || out[i].SupersedesJudgmentRef.JudgmentID != out[i-1].JudgmentID || out[i].SupersedesJudgmentRef.ContentSHA256 != out[i-1].ContentSHA256 {
				return nil, storeError(CodeSchemaInvalid, "attribution override history is not a linear chain")
			}
		}
	}
	return out, nil
}

func CommitAttributionOverride(ctx context.Context, req AttributionOverrideRequest) (AttributionOverrideResult, error) {
	j, err := BuildAttributionOverride(ctx, req)
	if err != nil {
		return AttributionOverrideResult{}, err
	}
	r, err := req.Store.Put(ctx, j)
	if err != nil {
		return AttributionOverrideResult{}, err
	}
	return AttributionOverrideResult{Status: r.Status, Judgment: j}, nil
}
