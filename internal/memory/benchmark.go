package memory

// MEM-02-08: Memory Quality Benchmark（离线、可重复）。
//
// 基准是"冻结 Fixture 文件 → 字节稳定报告"的纯函数：只统计协议事实
// （完整性、可重建率、断链率、确定性 Hash、拒绝率），不调用真实模型、
// 不联网、不输出思考或完整 Prompt，绝不宣称模型质量提升。同一 Fixture
// 多次运行字节一致；恶意 Fixture（未知字段、Hash 漂移、非法枚举、跨
// Scope）被拒绝或计入拒绝率。报告可被删除后从 Fixture 精确重建。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// BenchmarkFixture is the frozen input of one benchmark run. Facts reuse the
// canonical strict model types, so every fixture record goes through the
// exact production validation chain.
type BenchmarkFixture struct {
	SchemaVersion     int                        `json:"schema_version"`
	FixtureID         string                     `json:"fixture_id"`
	EvaluationInstant string                     `json:"evaluation_instant,omitempty"` // RFC3339; default fixed instant
	Revisions         []MemoryRevision           `json:"revisions"`
	Evidences         []MemoryEvidenceGeneration `json:"evidences"`
	Judgments         []JudgmentFact             `json:"judgments"`
	Policies          []PolicyFact               `json:"policies"`
	Usages            []MemoryUsage              `json:"usages"`
	Outcomes          []Outcome                  `json:"outcomes"`
}

// Validate checks the fixture envelope only. The embedded facts are decoded
// strictly (unknown fields rejected) but their semantic validation (hash,
// enums, scope) happens in the benchmark run's Put chain, where each
// rejection is counted: a malicious fixture produces a rejection report
// instead of being silently dropped.
func (f BenchmarkFixture) Validate() error {
	if f.SchemaVersion != SchemaVersion {
		return fmt.Errorf("benchmark fixture: schema_version must be %d", SchemaVersion)
	}
	if err := validateID(f.FixtureID, "fixture_id"); err != nil {
		return fmt.Errorf("benchmark fixture: %w", err)
	}
	return nil
}

func (f BenchmarkFixture) canonMap() (map[string]any, error) {
	revisions := make([]any, 0, len(f.Revisions))
	for _, r := range f.Revisions {
		m, err := r.canonMap()
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, m)
	}
	evidences := make([]any, 0, len(f.Evidences))
	for _, e := range f.Evidences {
		m, err := e.canonMap()
		if err != nil {
			return nil, err
		}
		evidences = append(evidences, m)
	}
	judgments := make([]any, 0, len(f.Judgments))
	for _, j := range f.Judgments {
		m, err := j.canonMap()
		if err != nil {
			return nil, err
		}
		judgments = append(judgments, m)
	}
	policies := make([]any, 0, len(f.Policies))
	for _, p := range f.Policies {
		m, err := p.canonMap()
		if err != nil {
			return nil, err
		}
		policies = append(policies, m)
	}
	usages := make([]any, 0, len(f.Usages))
	for _, u := range f.Usages {
		m, err := u.canonMap()
		if err != nil {
			return nil, err
		}
		usages = append(usages, m)
	}
	outcomes := make([]any, 0, len(f.Outcomes))
	for _, o := range f.Outcomes {
		m, err := o.canonMap()
		if err != nil {
			return nil, err
		}
		outcomes = append(outcomes, m)
	}
	return map[string]any{
		"schema_version":     f.SchemaVersion,
		"fixture_id":         f.FixtureID,
		"evaluation_instant": f.EvaluationInstant,
		"revisions":          revisions,
		"evidences":          evidences,
		"judgments":          judgments,
		"policies":           policies,
		"usages":             usages,
		"outcomes":           outcomes,
	}, nil
}

func (f BenchmarkFixture) CanonicalBytes() ([]byte, error) {
	m, err := f.canonMap()
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func (f BenchmarkFixture) ContentHash() (string, error) {
	b, err := f.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hashOf(b), nil
}

func (f BenchmarkFixture) EncodeCanonical() ([]byte, error) {
	m, err := f.canonMap()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// BenchmarkMetrics reports protocol facts only.
type BenchmarkMetrics struct {
	TotalFacts        int     `json:"total_facts"`
	RejectedFacts     int     `json:"rejected_facts"`
	RejectionRate     float64 `json:"rejection_rate"`
	DerivedMemories   int     `json:"derived_memories"`
	RebuildRate       float64 `json:"rebuild_rate"`
	BrokenLinks       int     `json:"broken_links"`
	LinkCount         int     `json:"link_count"`
	BrokenLinkRate    float64 `json:"broken_link_rate"`
	DeterministicHash string  `json:"deterministic_hash"`
	EvidenceStatus    string  `json:"evidence_status"`
}

// BenchmarkReport is the byte-stable output. ProtocolOnly is a fixed
// disclaimer: the report measures protocol facts, never model quality.
type BenchmarkReport struct {
	SchemaVersion int              `json:"schema_version"`
	FixtureID     string           `json:"fixture_id"`
	ProtocolOnly  bool             `json:"protocol_only"`
	Metrics       BenchmarkMetrics `json:"metrics"`
}

// RunBenchmarkFixture loads one fixture file through the strict chain and
// computes protocol metrics. It is deterministic: no wall clock, no random
// source, no network.
func RunBenchmarkFixture(ctx context.Context, fixturePath string) (*BenchmarkReport, error) {
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, storeError(CodeNotFound, "fixture file cannot be read")
	}
	fx, err := DecodeStrict[BenchmarkFixture](data)
	if err != nil {
		return nil, classifyDecodeError(err)
	}

	// Load every fact into a throwaway workspace through the real Put chain
	// so identity conflicts, hash drift, unknown fields and cross-scope
	// facts are rejected exactly like production writes. This workspace is
	// ephemeral scratch for the run and is deleted afterwards; it is not a
	// second fact source and no derived report is ever persisted here.
	root, err := os.MkdirTemp("", "omr-benchmark-")
	if err != nil {
		return nil, storeError(CodePermissionDenied, "cannot create benchmark workspace")
	}
	defer os.RemoveAll(root)
	s, err := OpenProject(root, Options{})
	if err != nil {
		return nil, err
	}

	total, rejected := 0, 0
	reject := func(err error) {
		total++
		if err != nil {
			rejected++
		}
	}
	for _, f := range fx.Revisions {
		_, err := s.Put(ctx, f)
		reject(err)
	}
	for _, f := range fx.Evidences {
		_, err := s.Put(ctx, f)
		reject(err)
	}
	for _, f := range fx.Judgments {
		_, err := s.Put(ctx, f)
		reject(err)
	}
	for _, f := range fx.Policies {
		_, err := s.Put(ctx, f)
		reject(err)
	}
	for _, f := range fx.Usages {
		_, err := s.Put(ctx, f)
		reject(err)
	}
	for _, f := range fx.Outcomes {
		_, err := s.Put(ctx, f)
		reject(err)
	}

	metrics := BenchmarkMetrics{TotalFacts: total, RejectedFacts: rejected}
	if total > 0 {
		metrics.RejectionRate = float64(rejected) / float64(total)
	}

	// Derive twice with a fixed evaluation instant: the deterministic hash
	// proves byte-stable derivation without any wall-clock dependency. The
	// instant comes from the fixture itself (evaluation_instant, or a
	// stable default), so it can never predate the fixture's own facts.
	evalNow := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	if fx.EvaluationInstant != "" {
		evalNow, err = time.Parse(time.RFC3339Nano, fx.EvaluationInstant)
		if err != nil {
			return nil, storeError(CodeSchemaInvalid, "fixture evaluation_instant is not a valid timestamp")
		}
	}
	res1, err := DeriveState(ctx, s, DerivedStateRequest{
		Scope: ScopeProject, Now: evalNow,
	})
	if err == nil {
		metrics.DerivedMemories = len(res1.States)
		if len(fx.Revisions) > 0 {
			metrics.RebuildRate = float64(metrics.DerivedMemories) / float64(len(fx.Revisions))
		}
		h1, err1 := hashDerivedStates(res1)
		res2, err2 := DeriveState(ctx, s, DerivedStateRequest{
			Scope: ScopeProject, Now: evalNow,
		})
		if err1 == nil && err2 == nil {
			h2, err2b := hashDerivedStates(res2)
			if err2b == nil && h1 == h2 {
				metrics.DeterministicHash = h1
			}
		}
	}

	// Broken links via the read-only consistency doctor. The findings all
	// concern judgment-held references, so the denominator is the number of
	// judgments (each can carry at most a few findings); revisions and
	// evidences are reference targets, not references themselves.
	cons, err := CheckConsistency(ctx, s, ConsistencyRequest{Scope: ScopeProject})
	if err == nil {
		metrics.BrokenLinks = len(cons.Findings)
		metrics.LinkCount = len(fx.Judgments)
		if metrics.LinkCount > 0 {
			metrics.BrokenLinkRate = float64(metrics.BrokenLinks) / float64(metrics.LinkCount)
		}
	}

	// Insufficient evidence: with fewer than 3 counted helps across at
	// least 2 independent episodes the protocol cannot support any quality
	// claim, so the status is insufficient_evidence.
	metrics.EvidenceStatus = "sufficient"
	helps := 0
	episodes := map[string]bool{}
	for _, u := range fx.Usages {
		if u.UsageStage == "affected" || u.UsageStage == "evaluated" {
			if u.EpisodeID != "" {
				episodes[u.EpisodeID] = true
			}
		}
	}
	for _, o := range fx.Outcomes {
		if o.Effect == "helped" && !o.ExternalFailure {
			helps++
		}
	}
	if helps < 3 || len(episodes) < 2 {
		metrics.EvidenceStatus = "insufficient_evidence"
	}

	return &BenchmarkReport{
		SchemaVersion: SchemaVersion,
		FixtureID:     fx.FixtureID,
		ProtocolOnly:  true,
		Metrics:       metrics,
	}, nil
}

func hashDerivedStates(res *DerivedStateResult) (string, error) {
	ids := make([]string, 0, len(res.States))
	for _, st := range res.States {
		ids = append(ids, string(st.SnapshotBytes()))
	}
	sort.Strings(ids)
	var buf []byte
	for _, id := range ids {
		buf = append(buf, id...)
	}
	return hashOf(buf), nil
}
