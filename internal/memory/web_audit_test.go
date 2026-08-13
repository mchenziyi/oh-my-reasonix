package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildMemoryAuditWebExportUsesDerivedStateAndIsReadOnly(t *testing.T) {
	store, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	rev.MemoryID = "mem_audit_01"
	rev.ContentSHA256, err = rev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	a, err := BuildMemoryAuditWebExport(context.Background(), store, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildMemoryAuditWebExport(context.Background(), store, now)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) || !strings.Contains(string(a), "mem_audit_01") || !strings.Contains(string(a), "probation") || !strings.Contains(string(a), "href=\"/\"") || !strings.Contains(string(a), "href=\"/manager\"") {
		t.Fatal("audit export must be deterministic and include derived state")
	}
}

func TestBuildMemoryAuditWebExportRequiresExplicitNow(t *testing.T) {
	store, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildMemoryAuditWebExport(context.Background(), store, time.Time{}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero now must fail closed, got %v", err)
	}
}
