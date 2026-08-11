package memory

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validRetrievalEvaluation(t *testing.T) RetrievalEvaluation {
	t.Helper()
	ctx := MemoryContext{
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion: SchemaVersion, Scope: ScopeProject,
			GenerationID: "generation_retrieval_01", InputManifestID: "generation_retrieval_01",
			InputManifestSHA256: HashPrefix + "1c4f2600b580f4feecf4741f5d7ae115b94858f654d4770484285ec415a1e9cc",
		},
	}
	r := RetrievalEvaluation{
		SchemaVersion:   SchemaVersion,
		EvaluationID:    "evaluation_retrieval_01",
		Scope:           ScopeProject,
		RetrievalID:     "retrieval_01",
		MemoryContext:   ctx,
		EvaluationScope: "fixture",
		JudgmentRef: JudgmentRef{
			Scope: ScopeProject, JudgmentType: JudgmentTypeRetrievalRelevance,
			JudgmentID:    "judgment_retrieval_01",
			ContentSHA256: HashPrefix + "9f4f2600b580f4feecf4741f5d7ae115b94858f654d4770484285ec415a1e9cc",
		},
		CreatedAt: "2026-08-11T00:00:00Z",
	}
	h, err := r.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	r.ContentSHA256 = h
	return r
}

func TestRetrievalSubjectSchema(t *testing.T) {
	valid := JudgmentSubject{SubjectType: "retrieval", RetrievalID: "retrieval_01"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid retrieval subject: %v", err)
	}
	for _, bad := range []JudgmentSubject{
		{SubjectType: "retrieval"},
		{SubjectType: "retrieval", RetrievalID: "../escape"},
		{SubjectType: "retrieval", RetrievalID: "retrieval_01", OutcomeID: "outcome_01"},
	} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid retrieval subject accepted: %#v", bad)
		}
	}
}

func TestRetrievalEvaluationRoundTripAndStore(t *testing.T) {
	r := validRetrievalEvaluation(t)
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	b, err := r.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeStrict[RetrievalEvaluation](b)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvaluationID != r.EvaluationID || got.ContentSHA256 != r.ContentSHA256 {
		t.Fatalf("round trip mismatch: %#v", got)
	}

	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	wr, err := s.Put(context.Background(), r)
	if err != nil || wr.Status != WriteCreated {
		t.Fatalf("put: result=%#v err=%v", wr, err)
	}
	if _, err := s.Get(context.Background(), FactKindRetrievalEvaluation, r.EvaluationID); err != nil {
		t.Fatal(err)
	}
	wr, err = s.Put(context.Background(), r)
	if err != nil || wr.Status != WriteNoop {
		t.Fatalf("noop: result=%#v err=%v", wr, err)
	}
}

func TestRetrievalEvaluationScopeEnum(t *testing.T) {
	for _, scope := range []string{"fixture", "generation_full_scan", "expanded_index_scan", "sampled_audit"} {
		r := validRetrievalEvaluation(t)
		r.EvaluationScope = scope
		r.ContentSHA256, _ = r.ContentHash()
		if err := r.Validate(); err != nil {
			t.Fatalf("scope %s: %v", scope, err)
		}
	}
	r := validRetrievalEvaluation(t)
	r.EvaluationScope = "../../../escape"
	r.ContentSHA256, _ = r.ContentHash()
	if err := r.Validate(); err == nil {
		t.Fatal("unknown evaluation scope accepted")
	}
}

func TestRetrievalRelevanceRejectsDuplicateRefs(t *testing.T) {
	ref := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeDecision, MemoryID: "mem_dup_01", Revision: 1, ContentSHA256: testHash}
	p := RetrievalRelevancePayload{Result: "hit_relevant", ExpectedMemoryRefs: []MemoryRef{ref, ref}}
	if err := p.Validate(); err == nil {
		t.Fatal("duplicate memory refs accepted")
	}
	ev := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_dup_01", ContentSHA256: testHash}
	p = RetrievalRelevancePayload{Result: "hit_relevant", EvidenceRefs: []EvidenceRef{ev, ev}}
	if err := p.Validate(); err == nil {
		t.Fatal("duplicate evidence refs accepted")
	}
}

func retrievalWorld(t *testing.T) (*FactStore, *GenerationTx, MemoryRevision, MemoryEvidenceGeneration) {
	t.Helper()
	tx, s, _ := commitOKFGeneration(t, tempRoot(t), "retrieval_eval_world", nil)
	rev := validRevision()
	ev := validEvidenceGeneration()
	return s, tx, rev, ev
}

func putRetrievalJudgment(t *testing.T, s *FactStore, retrievalID, source, result string, rev MemoryRevision, ev MemoryEvidenceGeneration) JudgmentFact {
	t.Helper()
	mref := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
	j := JudgmentFact{
		SchemaVersion: SchemaVersion, JudgmentID: "judgment_" + retrievalID,
		JudgmentType: JudgmentTypeRetrievalRelevance, Scope: ScopeProject,
		Subject: JudgmentSubject{SubjectType: "retrieval", RetrievalID: retrievalID},
		Source:  JudgmentSource{SourceType: source, SourceID: "source_01"},
		RetrievalRelevance: &RetrievalRelevancePayload{
			Result: result, ExpectedMemoryRefs: []MemoryRef{mref}, RetrievedMemoryRefs: []MemoryRef{mref},
			EvidenceRefs: append([]EvidenceRef(nil), ev.EvidenceRefs...),
		},
		CreatedAt: "2026-08-11T00:00:00Z",
	}
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	return j
}

func putRetrievalEvaluation(t *testing.T, s *FactStore, tx *GenerationTx, j JudgmentFact, retrievalID string) RetrievalEvaluation {
	t.Helper()
	r := validRetrievalEvaluation(t)
	r.RetrievalID = retrievalID
	r.EvaluationID = "evaluation_" + retrievalID
	r.MemoryContext = expectedContext(t, s, tx)
	r.JudgmentRef = JudgmentRef{Scope: j.Scope, JudgmentType: j.JudgmentType, JudgmentID: j.JudgmentID, ContentSHA256: j.ContentSHA256}
	r.ContentSHA256, _ = r.ContentHash()
	put(t, s, r)
	return r
}

func TestValidateRetrievalEvaluationFixture(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_fixture_01", "fixture_oracle", "hit_relevant", rev, ev)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_fixture_01")
	now := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	got, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RetrievalEvaluationVerified || got.JudgmentResult != "hit_relevant" || got.SourceType != "fixture_oracle" {
		t.Fatalf("unexpected result: %#v", got)
	}
	b1, err := got.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	got2, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := got2.EncodeCanonical()
	if !bytes.Equal(b1, b2) {
		t.Fatalf("derived result is not byte stable:\n%s\n%s", b1, b2)
	}
}

func TestValidateRetrievalEvaluationRequiresExplicitNow(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_now_01", "fixture_oracle", "hit_relevant", rev, ev)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_now_01")
	_, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
	})
	if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateRetrievalEvaluationRejectsSourceAndRetrievalMismatch(t *testing.T) {
	for _, tc := range []struct{ name, source, judgmentRetrieval, evaluationRetrieval string }{
		{"source", "model_guess", "retrieval_source_01", "retrieval_source_01"},
		{"retrieval", "user_review", "retrieval_subject_01", "retrieval_other_01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tx, rev, ev := retrievalWorld(t)
			j := putRetrievalJudgment(t, s, tc.judgmentRetrieval, tc.source, "missed_relevant", rev, ev)
			r := putRetrievalEvaluation(t, s, tx, j, tc.evaluationRetrieval)
			_, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
				Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
				Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
			})
			if err == nil {
				t.Fatal("invalid retrieval evaluation accepted")
			}
		})
	}
}

func TestValidateRetrievalEvaluationPathBodyIdentityMismatch(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_identity_01", "fixture_oracle", "hit_relevant", rev, ev)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_identity_01")
	other := r
	other.EvaluationID = "evaluation_body_other"
	other.ContentSHA256, _ = other.ContentHash()
	raw, _ := other.EncodeCanonical()
	if _, err := plantRawFact(s.root, FactKindRetrievalEvaluation, "evaluation_path_other", raw); err != nil {
		t.Fatal(err)
	}
	_, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: "evaluation_path_other", ProjectStore: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("identity mismatch code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateRetrievalEvaluationFutureFact(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_future_01", "fixture_oracle", "hit_relevant", rev, ev)
	r := validRetrievalEvaluation(t)
	r.RetrievalID = "retrieval_future_01"
	r.EvaluationID = "evaluation_future_01"
	r.MemoryContext = expectedContext(t, s, tx)
	r.JudgmentRef = JudgmentRef{Scope: j.Scope, JudgmentType: j.JudgmentType, JudgmentID: j.JudgmentID, ContentSHA256: j.ContentSHA256}
	r.CreatedAt = "2026-08-12T00:00:00Z"
	r.ContentSHA256, _ = r.ContentHash()
	put(t, s, r)
	_, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("future evaluation code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateRetrievalEvaluationZeroWrites(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_readonly_01", "user_review", "missed_relevant", rev, ev)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_readonly_01")
	count := func() int {
		n := 0
		_ = filepath.Walk(s.root, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	before := count()
	_, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if after := count(); after != before {
		t.Fatalf("read-only evaluation changed file count: before=%d after=%d", before, after)
	}
}

func commitRetrievalScopeWorld(t *testing.T, scope Scope, key string) (*FactStore, *GenerationTx, MemoryRevision, MemoryEvidenceGeneration) {
	t.Helper()
	root := tempRoot(t)
	var s *FactStore
	var err error
	if scope == ScopeProject {
		s, err = OpenProject(root, Options{})
	} else {
		s, err = OpenGlobal(root, Options{})
	}
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	rev.MemoryID = "mem_" + string(scope) + "_retrieval"
	rev.CanonicalKey = string(scope) + "-retrieval"
	rev.Scope = scope
	rev = fillRevisionHash(rev)
	ev := validEvidenceGeneration()
	ev.MemoryID = rev.MemoryID
	ev.Revision = rev.Revision
	for i := range ev.EvidenceRefs {
		ev.EvidenceRefs[i].Scope = scope
	}
	ev = fillEvidenceHash(ev)
	putRevisionEvidence(t, s, rev, ev)
	res, err := CompileOKF(context.Background(), s, okfRequest(rev, ev))
	if err != nil {
		t.Fatal(err)
	}
	gs := NewGenerationStore(s)
	req := beginReq(key, nil)
	req.Scope = scope
	req.CompilerVersion = OKFCompilerVersion
	req.CanonicalizationVersion = OKFCanonicalizationVersion
	tx, err := gs.Begin(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, res.Inputs)); err != nil {
		t.Fatal(err)
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, res.Outputs); err != nil {
		t.Fatal(err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	return s, tx, rev, ev
}

func TestValidateRetrievalEvaluationProjectAndGlobalWorld(t *testing.T) {
	project, ptx, prev, pev := commitRetrievalScopeWorld(t, ScopeProject, "retrieval_pair_project")
	global, gtx, grev, gev := commitRetrievalScopeWorld(t, ScopeGlobal, "retrieval_pair_global")
	ctx := expectedContext(t, project, ptx)
	gmfData, err := global.Get(context.Background(), FactKindGenerationInputManifest, gtx.GenerationID)
	if err != nil {
		t.Fatal(err)
	}
	gmf, err := DecodeStrict[GenerationInputManifest](gmfData)
	if err != nil {
		t.Fatal(err)
	}
	ctx.GlobalGenerationRef = &GlobalGenerationRef{
		SchemaVersion: SchemaVersion, Scope: ScopeGlobal, GenerationID: gtx.GenerationID,
		InputManifestID: gmf.GenerationID, InputManifestSHA256: gmf.InputManifestSHA256,
	}
	refs := []MemoryRef{
		{Scope: prev.Scope, MemoryType: prev.MemoryType, MemoryID: prev.MemoryID, Revision: prev.Revision, ContentSHA256: prev.ContentSHA256},
		{Scope: grev.Scope, MemoryType: grev.MemoryType, MemoryID: grev.MemoryID, Revision: grev.Revision, ContentSHA256: grev.ContentSHA256},
	}
	j := JudgmentFact{
		SchemaVersion: SchemaVersion, JudgmentID: "judgment_retrieval_pair", JudgmentType: JudgmentTypeRetrievalRelevance,
		Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "retrieval", RetrievalID: "retrieval_pair"},
		Source: JudgmentSource{SourceType: "retrieval_critic", SourceID: "critic_01"},
		RetrievalRelevance: &RetrievalRelevancePayload{Result: "hit_relevant", ExpectedMemoryRefs: refs, RetrievedMemoryRefs: refs,
			EvidenceRefs: []EvidenceRef{pev.EvidenceRefs[0], gev.EvidenceRefs[0]}},
		CreatedAt: "2026-08-11T00:00:00Z",
	}
	j.ContentSHA256, _ = j.ContentHash()
	put(t, project, j)
	r := validRetrievalEvaluation(t)
	r.EvaluationID = "evaluation_retrieval_pair"
	r.RetrievalID = "retrieval_pair"
	r.MemoryContext = ctx
	r.JudgmentRef = JudgmentRef{Scope: j.Scope, JudgmentType: j.JudgmentType, JudgmentID: j.JudgmentID, ContentSHA256: j.ContentSHA256}
	r.ContentSHA256, _ = r.ContentHash()
	put(t, project, r)
	got, err := ValidateRetrievalEvaluation(context.Background(), project, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: project, GlobalStore: global,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RetrievalEvaluationVerified {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestValidateRetrievalEvaluationRebuildAndUnavailable(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_rebuild_01", "fixture_oracle", "hit_relevant", rev, ev)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_rebuild_01")
	genDir := filepath.Join(s.root, "generations", tx.GenerationID)
	cleanedDir := filepath.Join(s.root, "generations", tx.GenerationID+".cleaned")
	if err := os.Rename(genDir, cleanedDir); err != nil {
		t.Fatal(err)
	}
	req := RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	}
	got, err := ValidateRetrievalEvaluation(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RetrievalEvaluationVerified {
		t.Fatalf("rebuild status=%s", got.Status)
	}
	revPath := filepath.Join(s.root, "facts", string(FactKindMemoryRevision), rev.MemoryID, fmt.Sprintf("%d.json", rev.Revision))
	if err := os.Rename(revPath, revPath+".cleaned"); err != nil {
		t.Fatal(err)
	}
	got, err = ValidateRetrievalEvaluation(context.Background(), s, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RetrievalEvaluationUnavailable {
		t.Fatalf("missing rebuild input status=%s", got.Status)
	}
}

func TestValidateRetrievalEvaluationAbsentScopeIsUnavailable(t *testing.T) {
	s, tx, rev, ev := retrievalWorld(t)
	j := putRetrievalJudgment(t, s, "retrieval_absent_scope", "fixture_oracle", "hit_relevant", rev, ev)
	j.RetrievalRelevance.ExpectedMemoryRefs[0].Scope = ScopeGlobal
	j.RetrievalRelevance.ExpectedMemoryRefs[0].ContentSHA256 = rev.ContentSHA256
	j.ContentSHA256, _ = j.ContentHash()
	// The helper already stored the original immutable judgment, so use a new
	// identity for this structurally valid but context-incomplete evaluation.
	j.JudgmentID = "judgment_retrieval_absent_scope_global"
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	r := putRetrievalEvaluation(t, s, tx, j, "retrieval_absent_scope")
	got, err := ValidateRetrievalEvaluation(context.Background(), s, RetrievalEvaluationRequest{
		Scope: ScopeProject, EvaluationID: r.EvaluationID, ProjectStore: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RetrievalEvaluationUnavailable {
		t.Fatalf("status=%s", got.Status)
	}
}
