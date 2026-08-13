package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildMemoryWebExportDeterministicAndReadOnly(t *testing.T) {
	store, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	first := validRevision()
	first.MemoryID = "mem_web_a"
	first.Title = "<script>alert(1)</script>"
	first.Summary = "Summary & details"
	first.ContentSHA256, err = first.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	second := validRevision()
	second.MemoryID = "mem_web_b"
	second.Title = "Second"
	second.Relations = []MemoryRelation{{Predicate: "supports", Target: memoryRefFromRevision(first)}}
	second.ContentSHA256, err = second.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	for _, rev := range []MemoryRevision{first, second} {
		if _, err := store.Put(context.Background(), rev); err != nil {
			t.Fatal(err)
		}
	}
	before, err := store.List(context.Background(), FactKindMemoryRevision)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	a, err := BuildMemoryWebExport(context.Background(), store, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildMemoryWebExport(context.Background(), store, now)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("web export must be byte deterministic")
	}
	if !strings.Contains(string(a), "&lt;script&gt;alert(1)&lt;/script&gt;") || strings.Contains(string(a), "<script>alert") {
		t.Fatal("web export must HTML-escape memory text")
	}
	if !strings.Contains(string(a), "supports") || !strings.Contains(string(a), "mem_web_a") || !strings.Contains(string(a), "mem_web_b") || !strings.Contains(string(a), "href=\"/manager\"") || !strings.Contains(string(a), "href=\"/audit\"") {
		t.Fatal("web export must include nodes and relations")
	}
	after, err := store.List(context.Background(), FactKindMemoryRevision)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatal("web export must not modify the store")
	}
}

func TestBuildMemoryWebExportRequiresExplicitNow(t *testing.T) {
	store, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildMemoryWebExport(context.Background(), store, time.Time{}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero now must fail closed, got %v", err)
	}
}
