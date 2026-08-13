package memory

import (
	"bytes"
	"testing"
	"time"
)

func validWebAction() WebManagementAction {
	rev := validRevision()
	return WebManagementAction{
		SchemaVersion: SchemaVersion,
		ActionID:      "web_action_01",
		Scope:         rev.Scope,
		Target:        memoryRefFromRevision(rev),
		Operation:     "freeze",
		Reason:        "manual review",
		RequestedAt:   time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
}

func TestWebManagementActionCanonicalAndHashDeterministic(t *testing.T) {
	a := validWebAction()
	b, err := a.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := a.CanonicalBytes()
	if err != nil || !bytes.Equal(b, b2) {
		t.Fatal("web action canonical bytes must be stable")
	}
	h, err := a.ContentHash()
	if err != nil || len(h) < 64 || h[:7] != "sha256_" {
		t.Fatalf("web action hash invalid: %q %v", h, err)
	}
}

func TestWebManagementActionRejectsUnsafeOrIncompleteInput(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*WebManagementAction)
	}{
		{"unknown operation", func(a *WebManagementAction) { a.Operation = "../../etc" }},
		{"portable scope", func(a *WebManagementAction) { a.Scope = ScopePortable }},
		{"scope mismatch", func(a *WebManagementAction) { a.Target.Scope = ScopeGlobal }},
		{"bad timestamp", func(a *WebManagementAction) { a.RequestedAt = "now" }},
		{"unfreeze basis missing", func(a *WebManagementAction) { a.Operation = "unfreeze" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := validWebAction()
			tc.mut(&a)
			if err := a.Validate(); err == nil {
				t.Fatal("invalid web action must fail closed")
			}
		})
	}
}
