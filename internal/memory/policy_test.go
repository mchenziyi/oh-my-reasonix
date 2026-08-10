package memory

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func validFreshnessConfig() *PolicyConfigFreshness {
	return &PolicyConfigFreshness{
		EvaluationWindowDays:      90,
		AgingAfterDays:            180,
		StaleAfterDays:            365,
		RevalidationEvidenceTypes: []string{"test_result", "usage_outcome"},
		Version:                   PolicyConfigSchemaVersion,
	}
}

func validTrustConfig() *PolicyConfigTrust {
	return &PolicyConfigTrust{
		AllowedAcquisitionMethods:            []string{"local_file", "test_run", "user_confirmation"},
		RequireProvenance:                    true,
		RequireVerificationStatus:            true,
		ExternalUnverifiedInstructionAllowed: false,
		PromotionRequiresPolicyEvidence:      true,
		Version:                              PolicyConfigSchemaVersion,
	}
}

func validClassifierConfig() *PolicyConfigContentClassifier {
	return &PolicyConfigContentClassifier{
		ClassifierID:                "omr_builtin_classifier_v1",
		AllowedClasses:              []string{"instructional", "descriptive", "secret", "unsafe"},
		DefaultClass:                "descriptive",
		SecretClassesBlockPromotion: true,
		Version:                     PolicyConfigSchemaVersion,
	}
}

func validIndexConfig() *PolicyConfigIndex {
	return &PolicyConfigIndex{
		MaxEntriesPerPage: 64,
		MaxPageBytes:      32768,
		MaxShardDepth:     4,
		SplitOrder:        []string{"component", "operation", "memory_type", "stable_id_prefix"},
		OverflowBucket:    "other",
		Version:           PolicyConfigSchemaVersion,
	}
}

func validBenchmarkConfig() *PolicyConfigBenchmark {
	return &PolicyConfigBenchmark{
		FixtureSetID:             "mnemosyne_memory_quality_v1",
		MinimumCases:             1,
		RequiredMetrics:          []string{"retrieval_recall", "citation_accuracy", "safety_regression"},
		PassThresholds:           map[string]float64{"retrieval_recall": 0.0, "citation_accuracy": 0.0, "safety_regression": 1.0},
		PairedComparisonRequired: true,
		Version:                  PolicyConfigSchemaVersion,
	}
}

func validPolicy() PolicyFact {
	p := PolicyFact{
		SchemaVersion: 1,
		PolicyID:      "freshness_policy_v1",
		PolicyType:    PolicyTypeFreshness,
		PolicyVersion: 1,
		Config:        PolicyConfig{Freshness: validFreshnessConfig()},
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
	h, err := p.ContentHash()
	if err != nil {
		panic(err)
	}
	p.ContentSHA256 = h
	return p
}

// policyOf builds a schema-valid PolicyFact of the given type.
func policyOf(pt PolicyType) PolicyFact {
	p := PolicyFact{
		SchemaVersion: 1,
		PolicyID:      "policy_of_" + string(pt),
		PolicyType:    pt,
		PolicyVersion: 1,
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
	switch pt {
	case PolicyTypeFreshness:
		p.Config = PolicyConfig{Freshness: validFreshnessConfig()}
	case PolicyTypeTrust:
		p.Config = PolicyConfig{Trust: validTrustConfig()}
	case PolicyTypeContentClassifier:
		p.Config = PolicyConfig{ContentClassifier: validClassifierConfig()}
	case PolicyTypeIndex:
		p.Config = PolicyConfig{Index: validIndexConfig()}
	case PolicyTypeBenchmark:
		p.Config = PolicyConfig{Benchmark: validBenchmarkConfig()}
	}
	return fillPolicyHash(p)
}

func TestPolicyFactValidation(t *testing.T) {
	p := validPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	// Every policy type with its matching config key is valid.
	for _, pt := range []PolicyType{PolicyTypeFreshness, PolicyTypeTrust, PolicyTypeContentClassifier, PolicyTypeIndex, PolicyTypeBenchmark} {
		pp := policyOf(pt)
		if err := pp.Validate(); err != nil {
			t.Errorf("policy type %q should be valid: %v", pt, err)
		}
	}

	cases := []struct {
		name string
		mut  func(*PolicyFact)
	}{
		{"schema_version zero", func(p *PolicyFact) { p.SchemaVersion = 0 }},
		{"empty policy_id", func(p *PolicyFact) { p.PolicyID = "" }},
		{"path policy_id", func(p *PolicyFact) { p.PolicyID = "etc/policies/1" }},
		{"invalid policy_type", func(p *PolicyFact) { p.PolicyType = PolicyType("retention") }},
		{"policy_version zero", func(p *PolicyFact) { p.PolicyVersion = 0 }},
		{"config key mismatch", func(p *PolicyFact) { p.Config = PolicyConfig{Trust: validTrustConfig()} }},
		{"config empty union", func(p *PolicyFact) { p.Config = PolicyConfig{} }},
		{"config two keys", func(p *PolicyFact) {
			p.Config = PolicyConfig{Freshness: validFreshnessConfig(), Trust: validTrustConfig()}
		}},
		{"empty hash", func(p *PolicyFact) { p.ContentSHA256 = "" }},
		{"hash mismatch", func(p *PolicyFact) { p.ContentSHA256 = "sha256_" + strings.Repeat("c", 64) }},
		{"bad created_at", func(p *PolicyFact) { p.CreatedAt = "whenever" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pp := p
			c.mut(&pp)
			if err := pp.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestPolicyConfigValidation(t *testing.T) {
	// Per-type valid configs must pass.
	for _, pt := range []PolicyType{PolicyTypeFreshness, PolicyTypeTrust, PolicyTypeContentClassifier, PolicyTypeIndex, PolicyTypeBenchmark} {
		pp := policyOf(pt)
		if err := pp.Config.Validate(pt); err != nil {
			t.Errorf("valid %s config rejected: %v", pt, err)
		}
	}

	// Freshness window ordering and positive integers.
	f := validFreshnessConfig()
	f.EvaluationWindowDays = 0
	if err := (PolicyConfig{Freshness: f}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("zero evaluation window must be rejected")
	}
	f = validFreshnessConfig()
	f.AgingAfterDays = 90 // must be strictly > evaluation_window
	if err := (PolicyConfig{Freshness: f}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("aging_after must be strictly greater than evaluation_window")
	}
	f = validFreshnessConfig()
	f.StaleAfterDays = 180 // must be strictly > aging_after
	if err := (PolicyConfig{Freshness: f}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("stale_after must be strictly greater than aging_after")
	}
	f = validFreshnessConfig()
	f.RevalidationEvidenceTypes = nil
	if err := (PolicyConfig{Freshness: f}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("empty evidence types must be rejected")
	}
	f = validFreshnessConfig()
	f.RevalidationEvidenceTypes = []string{"/etc/passwd"}
	if err := (PolicyConfig{Freshness: f}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("path-like evidence type must be rejected")
	}

	// Trust policy: safety hard constraints cannot be disabled.
	tr := validTrustConfig()
	tr.RequireProvenance = false
	if err := (PolicyConfig{Trust: tr}).Validate(PolicyTypeTrust); err == nil {
		t.Error("trust with require_provenance=false must be rejected")
	}
	tr = validTrustConfig()
	tr.RequireVerificationStatus = false
	if err := (PolicyConfig{Trust: tr}).Validate(PolicyTypeTrust); err == nil {
		t.Error("trust with require_verification_status=false must be rejected")
	}
	tr = validTrustConfig()
	tr.ExternalUnverifiedInstructionAllowed = true
	if err := (PolicyConfig{Trust: tr}).Validate(PolicyTypeTrust); err == nil {
		t.Error("trust allowing external unverified instructions must be rejected")
	}
	tr = validTrustConfig()
	tr.AllowedAcquisitionMethods = []string{"rm -rf /tmp"}
	if err := (PolicyConfig{Trust: tr}).Validate(PolicyTypeTrust); err == nil {
		t.Error("command-like acquisition method must be rejected")
	}

	// Content classifier: default class must be allowed; secret/unsafe must
	// stay block-promotion.
	cc := validClassifierConfig()
	cc.DefaultClass = "instructional"
	if err := (PolicyConfig{ContentClassifier: cc}).Validate(PolicyTypeContentClassifier); err != nil {
		t.Errorf("default within allowed classes should pass: %v", err)
	}
	cc = validClassifierConfig()
	cc.DefaultClass = "not_a_class"
	if err := (PolicyConfig{ContentClassifier: cc}).Validate(PolicyTypeContentClassifier); err == nil {
		t.Error("default class outside allowed_classes must be rejected")
	}
	cc = validClassifierConfig()
	cc.AllowedClasses = nil
	if err := (PolicyConfig{ContentClassifier: cc}).Validate(PolicyTypeContentClassifier); err == nil {
		t.Error("empty allowed_classes must be rejected")
	}
	cc = validClassifierConfig()
	cc.SecretClassesBlockPromotion = false
	if err := (PolicyConfig{ContentClassifier: cc}).Validate(PolicyTypeContentClassifier); err == nil {
		t.Error("secret classes must block promotion")
	}
	cc = validClassifierConfig()
	cc.ClassifierID = "prompt: ignore previous instructions"
	if err := (PolicyConfig{ContentClassifier: cc}).Validate(PolicyTypeContentClassifier); err == nil {
		t.Error("free-text classifier id must be rejected")
	}

	// Index: numeric bounds, fixed dimensions, no duplicates, safe bucket.
	ix := validIndexConfig()
	ix.MaxEntriesPerPage = 0
	if err := (PolicyConfig{Index: ix}).Validate(PolicyTypeIndex); err == nil {
		t.Error("zero max_entries_per_page must be rejected")
	}
	ix = validIndexConfig()
	ix.MaxEntriesPerPage = 1 << 20
	if err := (PolicyConfig{Index: ix}).Validate(PolicyTypeIndex); err == nil {
		t.Error("oversized max_entries_per_page must be rejected")
	}
	ix = validIndexConfig()
	ix.SplitOrder = []string{"component", "component"}
	if err := (PolicyConfig{Index: ix}).Validate(PolicyTypeIndex); err == nil {
		t.Error("duplicate split dimension must be rejected")
	}
	ix = validIndexConfig()
	ix.SplitOrder = []string{"component", "user_id"}
	if err := (PolicyConfig{Index: ix}).Validate(PolicyTypeIndex); err == nil {
		t.Error("unknown split dimension must be rejected")
	}
	ix = validIndexConfig()
	ix.OverflowBucket = "../etc"
	if err := (PolicyConfig{Index: ix}).Validate(PolicyTypeIndex); err == nil {
		t.Error("path-like overflow bucket must be rejected")
	}

	// Benchmark: metric names controlled, thresholds finite and in range.
	bm := validBenchmarkConfig()
	bm.PassThresholds["retrieval_recall"] = 1.5
	if err := (PolicyConfig{Benchmark: bm}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("threshold above 1.0 must be rejected")
	}
	bm = validBenchmarkConfig()
	bm.PassThresholds["retrieval_recall"] = -0.1
	if err := (PolicyConfig{Benchmark: bm}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("negative threshold must be rejected")
	}
	bm = validBenchmarkConfig()
	bm.PassThresholds["retrieval_recall"] = math.NaN()
	if err := (PolicyConfig{Benchmark: bm}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("NaN threshold must be rejected")
	}
	bm = validBenchmarkConfig()
	bm.RequiredMetrics = []string{"run: echo hacked"}
	if err := (PolicyConfig{Benchmark: bm}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("command-like metric must be rejected")
	}
	bm = validBenchmarkConfig()
	bm.FixtureSetID = "/opt/fixtures/set"
	if err := (PolicyConfig{Benchmark: bm}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("absolute-path fixture_set_id must be rejected")
	}

	// The frozen config schema version cannot be changed by any type.
	fv := validFreshnessConfig()
	fv.Version = 2
	if err := (PolicyConfig{Freshness: fv}).Validate(PolicyTypeFreshness); err == nil {
		t.Error("freshness config with version 2 must be rejected")
	}
	tv := validTrustConfig()
	tv.Version = 2
	if err := (PolicyConfig{Trust: tv}).Validate(PolicyTypeTrust); err == nil {
		t.Error("trust config with version 2 must be rejected")
	}
	cv := validClassifierConfig()
	cv.Version = 2
	if err := (PolicyConfig{ContentClassifier: cv}).Validate(PolicyTypeContentClassifier); err == nil {
		t.Error("classifier config with version 2 must be rejected")
	}
	iv := validIndexConfig()
	iv.Version = 2
	if err := (PolicyConfig{Index: iv}).Validate(PolicyTypeIndex); err == nil {
		t.Error("index config with version 2 must be rejected")
	}
	bv := validBenchmarkConfig()
	bv.Version = 2
	if err := (PolicyConfig{Benchmark: bv}).Validate(PolicyTypeBenchmark); err == nil {
		t.Error("benchmark config with version 2 must be rejected")
	}

	// Mathematically identical thresholds must hash identically: -0.0 and
	// 0.0 must not split the content hash.
	neg := validBenchmarkConfig()
	neg.PassThresholds["retrieval_recall"] = math.Copysign(0, -1)
	pos := validBenchmarkConfig()
	pos.PassThresholds["retrieval_recall"] = 0.0
	mNeg, err := neg.canonMap()
	if err != nil {
		t.Fatal(err)
	}
	mPos, err := pos.canonMap()
	if err != nil {
		t.Fatal(err)
	}
	if NewContentHash(mustMarshal(mNeg)) != NewContentHash(mustMarshal(mPos)) {
		t.Error("-0.0 and 0.0 thresholds must hash identically")
	}
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestPolicyFactConfigStrictUnion(t *testing.T) {
	// Unknown key inside the discriminated config must be rejected at decode time.
	in := `{"schema_version":1,"policy_id":"freshness_policy_v1","policy_type":"freshness","policy_version":1,"config":{"freshness":{"max_age_days":30}},"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[PolicyFact]([]byte(in)); err == nil {
		t.Error("unknown config field should be rejected (MEM-01C freezes config fields)")
	}
	// Unknown union key must be rejected.
	in2 := `{"schema_version":1,"policy_id":"freshness_policy_v1","policy_type":"freshness","policy_version":1,"config":{"retention":{}},"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[PolicyFact]([]byte(in2)); err == nil {
		t.Error("unknown config union key should be rejected")
	}
}

func validManifest() GenerationInputManifest {
	m := GenerationInputManifest{
		SchemaVersion:           1,
		GenerationID:            "gen_000013",
		Scope:                   ScopeProject,
		BaseGeneration:          strPtr("gen_000012"),
		CompilerVersion:         "mnemosyne-compiler/1",
		CanonicalizationVersion: 1,
		Inputs: []ManifestInput{
			{FactType: "memory_revision", FactID: "mem_abc@2", FactSchemaVersion: 1, ContentSHA256: testHash},
			{FactType: "memory_evidence_generation", FactID: "mem_abc@2:evidence@3", FactSchemaVersion: 1, ContentSHA256: testHash},
		},
		OutputSHA256:  "sha256_" + strings.Repeat("d", 64),
		TransactionID: "tx_01K",
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
	h, err := m.ContentHash()
	if err != nil {
		panic(err)
	}
	m.InputManifestSHA256 = h
	return m
}

func TestManifestValidation(t *testing.T) {
	m := validManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	first := m
	first.BaseGeneration = nil
	h, err := first.ContentHash()
	if err != nil {
		panic(err)
	}
	first.InputManifestSHA256 = h
	if err := first.Validate(); err != nil {
		t.Errorf("manifest without base_generation should be valid: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*GenerationInputManifest)
	}{
		{"schema_version zero", func(m *GenerationInputManifest) { m.SchemaVersion = 0 }},
		{"empty generation_id", func(m *GenerationInputManifest) { m.GenerationID = "" }},
		{"path generation_id", func(m *GenerationInputManifest) { m.GenerationID = "gens/000013" }},
		{"invalid scope", func(m *GenerationInputManifest) { m.Scope = Scope("x") }},
		{"path base_generation", func(m *GenerationInputManifest) { m.BaseGeneration = strPtr("../gen") }},
		{"empty compiler_version", func(m *GenerationInputManifest) { m.CompilerVersion = "" }},
		{"canonicalization_version zero", func(m *GenerationInputManifest) { m.CanonicalizationVersion = 0 }},
		{"empty fact_type", func(m *GenerationInputManifest) { m.Inputs[0].FactType = "" }},
		{"path fact_type", func(m *GenerationInputManifest) { m.Inputs[0].FactType = "../facts" }},
		{"empty fact_id", func(m *GenerationInputManifest) { m.Inputs[0].FactID = "" }},
		{"path fact_id", func(m *GenerationInputManifest) { m.Inputs[0].FactID = "data/mem@1" }},
		{"fact_schema_version zero", func(m *GenerationInputManifest) { m.Inputs[0].FactSchemaVersion = 0 }},
		{"invalid fact hash", func(m *GenerationInputManifest) { m.Inputs[0].ContentSHA256 = "sha256_zz" }},
		{"invalid output hash", func(m *GenerationInputManifest) { m.OutputSHA256 = "md5_abcd" }},
		{"empty transaction_id", func(m *GenerationInputManifest) { m.TransactionID = "" }},
		{"bad created_at", func(m *GenerationInputManifest) { m.CreatedAt = "later" }},
		{"empty manifest hash", func(m *GenerationInputManifest) { m.InputManifestSHA256 = "" }},
		{"manifest hash mismatch", func(m *GenerationInputManifest) { m.InputManifestSHA256 = "sha256_" + strings.Repeat("e", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mm := m
			c.mut(&mm)
			if err := mm.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestManifestInputsDeterministicDedupe(t *testing.T) {
	a := ManifestInput{FactType: "memory_revision", FactID: "mem_abc@2", FactSchemaVersion: 1, ContentSHA256: testHash}
	b := ManifestInput{FactType: "memory_evidence_generation", FactID: "mem_abc@2:evidence@3", FactSchemaVersion: 1, ContentSHA256: testHash}

	ordered := validManifest()
	ordered.Inputs = []ManifestInput{a, b}
	ordered = fillManifestHash(ordered)
	if err := ordered.Validate(); err != nil {
		t.Fatalf("ordered manifest invalid: %v", err)
	}

	shuffled := validManifest()
	shuffled.Inputs = []ManifestInput{b, a, b, a}
	shuffled = fillManifestHash(shuffled)
	if err := shuffled.Validate(); err != nil {
		t.Fatalf("shuffled manifest invalid: %v", err)
	}

	ha, _ := ordered.ContentHash()
	hb, _ := shuffled.ContentHash()
	if ha != hb {
		t.Error("manifest hash must be independent of input order and duplicates")
	}

	// Same fact_type+fact_id with conflicting hashes must be rejected.
	// Input deduplication runs before hash verification, so the stale
	// manifest hash is not what rejects this.
	conflict := validManifest()
	conflict.Inputs = []ManifestInput{
		{FactType: "memory_revision", FactID: "mem_abc@2", FactSchemaVersion: 1, ContentSHA256: testHash},
		{FactType: "memory_revision", FactID: "mem_abc@2", FactSchemaVersion: 1, ContentSHA256: "sha256_" + strings.Repeat("1", 64)},
	}
	if err := conflict.Validate(); err == nil {
		t.Error("conflicting fact entries for same fact_type+fact_id should be rejected")
	}
}

func fillManifestHash(m GenerationInputManifest) GenerationInputManifest {
	h, err := m.ContentHash()
	if err != nil {
		panic(err)
	}
	m.InputManifestSHA256 = h
	return m
}
