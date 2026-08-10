package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ---- helpers ----

func validUsage() MemoryUsage {
	u := MemoryUsage{
		SchemaVersion: 1,
		UsageID:       "usage_01K",
		Scope:         ScopeProject,
		MemoryID:      "mem_01K7A9X2",
		Revision:      2,
		UsageStage:    "affected",
		EpisodeID:     "episode_01K",
		OccurredAt:    "2026-08-11T10:00:00Z",
		Source:        "local_user",
		CreatedAt:     "2026-08-11T10:05:00Z",
	}
	h, err := u.ContentHash()
	if err != nil {
		panic(err)
	}
	u.ContentSHA256 = h
	return u
}

func validOutcome() Outcome {
	o := Outcome{
		SchemaVersion: 1,
		OutcomeID:     "outcome_01K",
		Scope:         ScopeProject,
		UsageID:       "usage_01K",
		MemoryID:      "mem_01K7A9X2",
		Revision:      2,
		Effect:        "helped",
		CreatedAt:     "2026-08-11T10:06:00Z",
	}
	h, err := o.ContentHash()
	if err != nil {
		panic(err)
	}
	o.ContentSHA256 = h
	return o
}

// ---- schema validation ----

func TestMemoryUsageValidate(t *testing.T) {
	if err := validUsage().Validate(); err != nil {
		t.Fatalf("valid usage rejected: %v", err)
	}
	bad := []struct {
		name string
		mut  func(*MemoryUsage)
	}{
		{"schema version", func(u *MemoryUsage) { u.SchemaVersion = 2 }},
		{"usage id", func(u *MemoryUsage) { u.UsageID = "../evil" }},
		{"scope", func(u *MemoryUsage) { u.Scope = Scope("") }},
		{"memory id", func(u *MemoryUsage) { u.MemoryID = "" }},
		{"revision", func(u *MemoryUsage) { u.Revision = 0 }},
		{"usage_stage", func(u *MemoryUsage) { u.UsageStage = "invented" }},
		{"empty usage_stage", func(u *MemoryUsage) { u.UsageStage = "" }},
		{"episode_id", func(u *MemoryUsage) { u.EpisodeID = "../evil" }},
		{"occurred_at", func(u *MemoryUsage) { u.OccurredAt = "not-a-time" }},
		{"source", func(u *MemoryUsage) { u.Source = "/abs/path" }},
		{"created_at", func(u *MemoryUsage) { u.CreatedAt = "" }},
		{"hash", func(u *MemoryUsage) { u.ContentSHA256 = "" }},
	}
	for _, b := range bad {
		t.Run(b.name, func(t *testing.T) {
			u := validUsage()
			b.mut(&u)
			if err := u.Validate(); err == nil {
				t.Error("invalid usage must be rejected")
			}
		})
	}
}

func TestOutcomeValidate(t *testing.T) {
	if err := validOutcome().Validate(); err != nil {
		t.Fatalf("valid outcome rejected: %v", err)
	}
	for _, effect := range []string{"helped", "neutral", "harmed", "unknown"} {
		o := validOutcome()
		o.Effect = effect
		o = fillOutcomeHash(o)
		if err := o.Validate(); err != nil {
			t.Errorf("effect %q must be valid: %v", effect, err)
		}
	}
	bad := []struct {
		name string
		mut  func(*Outcome)
	}{
		{"schema version", func(o *Outcome) { o.SchemaVersion = 2 }},
		{"outcome id", func(o *Outcome) { o.OutcomeID = "../evil" }},
		{"usage id", func(o *Outcome) { o.UsageID = "" }},
		{"scope", func(o *Outcome) { o.Scope = Scope("") }},
		{"memory id", func(o *Outcome) { o.MemoryID = "" }},
		{"revision", func(o *Outcome) { o.Revision = 0 }},
		{"effect", func(o *Outcome) { o.Effect = "amazing" }},
		{"created_at", func(o *Outcome) { o.CreatedAt = "" }},
		{"hash", func(o *Outcome) { o.ContentSHA256 = "" }},
	}
	for _, b := range bad {
		t.Run(b.name, func(t *testing.T) {
			o := validOutcome()
			b.mut(&o)
			if err := o.Validate(); err == nil {
				t.Error("invalid outcome must be rejected")
			}
		})
	}
}

// ---- strict decode: unknown fields ----

func TestMemoryUsageDecodeStrictRejectsUnknownFields(t *testing.T) {
	in := `{"schema_version":1,"usage_id":"usage_01K","scope":"project","memory_id":"mem_01K7A9X2","revision":2,"occurred_at":"2026-08-11T10:00:00Z","source":"local_user","content_sha256":"sha256_x","created_at":"2026-08-11T10:05:00Z","extra":1}`
	if _, err := DecodeStrict[MemoryUsage]([]byte(in)); err == nil {
		t.Error("unknown fields must be rejected")
	}
}

func TestOutcomeDecodeStrictRejectsUnknownFields(t *testing.T) {
	in := `{"schema_version":1,"outcome_id":"outcome_01K","scope":"project","usage_id":"usage_01K","memory_id":"mem_01K7A9X2","revision":2,"effect":"helped","content_sha256":"sha256_x","created_at":"2026-08-11T10:06:00Z","note":"free text"}`
	if _, err := DecodeStrict[Outcome]([]byte(in)); err == nil {
		t.Error("unknown fields must be rejected")
	}
}

// ---- store routing, idempotency and scope ----

func TestUsageOutcomeStoreRoundTrip(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	u := validUsage()
	if _, err := s.Put(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	o := validOutcome()
	if _, err := s.Put(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), FactKindMemoryUsage, "usage_01K")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := u.EncodeCanonical()
	if string(got) != string(want) {
		t.Errorf("usage round trip mismatch")
	}
	got, err = s.Get(context.Background(), FactKindOutcome, "outcome_01K")
	if err != nil {
		t.Fatal(err)
	}
	want, _ = o.EncodeCanonical()
	if string(got) != string(want) {
		t.Errorf("outcome round trip mismatch")
	}
	// Route checks.
	for _, p := range []string{
		"facts/memory-usages/usage_01K.json",
		"facts/outcomes/outcome_01K.json",
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("expected file at %s: %v", p, err)
		}
	}
}

func TestUsageOutcomeIdempotentAndConflict(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	u := validUsage()
	if _, err := s.Put(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	// Same identity + same hash: NOOP.
	res, err := s.Put(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != WriteNoop {
		t.Errorf("repeat put must be NOOP, got %v", res.Status)
	}
	// Same identity + different hash: conflict, old fact untouched.
	u2 := validUsage()
	u2.Source = "another_user"
	u2 = fillUsageHash(u2)
	if _, err := s.Put(context.Background(), u2); ErrorCode(err) != CodeIdentityConflict {
		t.Fatalf("same usage id with different hash must conflict, got %v", err)
	}
	got, err := s.Get(context.Background(), FactKindMemoryUsage, "usage_01K")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := u.EncodeCanonical()
	if string(got) != string(want) {
		t.Error("conflict must leave the original fact untouched")
	}
}

func TestUsageOutcomeScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	u := validUsage()
	u.Scope = ScopeGlobal
	u = fillUsageHash(u)
	if _, err := s.Put(context.Background(), u); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("global usage in project store must fail closed, got %v", err)
	}
	o := validOutcome()
	o.Scope = ScopeGlobal
	o = fillOutcomeHash(o)
	if _, err := s.Put(context.Background(), o); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("global outcome in project store must fail closed, got %v", err)
	}
}

// ---- helpers used by later stages ----

func fillUsageHash(u MemoryUsage) MemoryUsage {
	h, err := u.ContentHash()
	if err != nil {
		panic(err)
	}
	u.ContentSHA256 = h
	return u
}

func fillOutcomeHash(o Outcome) Outcome {
	h, err := o.ContentHash()
	if err != nil {
		panic(err)
	}
	o.ContentSHA256 = h
	return o
}

// ---- deterministic canonical form ----

func TestUsageOutcomeCanonicalDeterministic(t *testing.T) {
	u := validUsage()
	c1, err := u.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	c2, err := validUsage().CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(c1) != string(c2) {
		t.Error("usage canonical bytes must be deterministic")
	}
	o := validOutcome()
	c1, err = o.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	c2, err = validOutcome().CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(c1) != string(c2) {
		t.Error("outcome canonical bytes must be deterministic")
	}
	// Trailing data must be rejected.
	if _, err := DecodeStrict[MemoryUsage](append(c1, byte('x'))); err == nil {
		t.Error("trailing data must be rejected")
	}
	if _, err := DecodeStrict[Outcome](append(c1, byte('x'))); err == nil {
		t.Error("trailing data must be rejected")
	}
}
