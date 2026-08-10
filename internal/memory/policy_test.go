package memory

import (
	"strings"
	"testing"
)

func validPolicy() PolicyFact {
	p := PolicyFact{
		SchemaVersion: 1,
		PolicyID:      "freshness_policy_v1",
		PolicyType:    PolicyTypeFreshness,
		PolicyVersion: 1,
		Config:        PolicyConfig{Freshness: &PolicyConfigFreshness{}},
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
	h, err := p.ContentHash()
	if err != nil {
		panic(err)
	}
	p.ContentSHA256 = h
	return p
}

func TestPolicyFactValidation(t *testing.T) {
	p := validPolicy()
	if err := p.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	// Every policy type with its matching config key is valid.
	for _, pt := range []PolicyType{PolicyTypeFreshness, PolicyTypeTrust, PolicyTypeContentClassifier, PolicyTypeIndex, PolicyTypeBenchmark} {
		pp := p
		pp.PolicyType = pt
		switch pt {
		case PolicyTypeFreshness:
			pp.Config = PolicyConfig{Freshness: &PolicyConfigFreshness{}}
		case PolicyTypeTrust:
			pp.Config = PolicyConfig{Trust: &PolicyConfigTrust{}}
		case PolicyTypeContentClassifier:
			pp.Config = PolicyConfig{ContentClassifier: &PolicyConfigContentClassifier{}}
		case PolicyTypeIndex:
			pp.Config = PolicyConfig{Index: &PolicyConfigIndex{}}
		case PolicyTypeBenchmark:
			pp.Config = PolicyConfig{Benchmark: &PolicyConfigBenchmark{}}
		}
		h, err := pp.ContentHash()
		if err != nil {
			t.Fatalf("hash failed: %v", err)
		}
		pp.ContentSHA256 = h
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
		{"config key mismatch", func(p *PolicyFact) { p.Config = PolicyConfig{Trust: &PolicyConfigTrust{}} }},
		{"config empty union", func(p *PolicyFact) { p.Config = PolicyConfig{} }},
		{"config two keys", func(p *PolicyFact) {
			p.Config = PolicyConfig{Freshness: &PolicyConfigFreshness{}, Trust: &PolicyConfigTrust{}}
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
