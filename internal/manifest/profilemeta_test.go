package manifest

import (
	"testing"
)

func TestParseProfileMetaBasic(t *testing.T) {
	content := []byte(`---
name: omr-explore
description: Read-only exploration
read-only: true
allowed-tools: [read_file, grep, glob]
---

# Title

## 输入

- task_id: the task identifier
- goal: the objective

## 输出

1. Findings by file
2. Key decisions
`)
	meta, err := ParseProfileMeta(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ID != "omr-explore" {
		t.Fatalf("expected omr-explore, got %q", meta.ID)
	}
	if !meta.ReadOnly {
		t.Fatal("expected read-only")
	}
	if len(meta.AllowedTools) != 3 {
		t.Fatalf("expected 3 allowed tools, got %d", len(meta.AllowedTools))
	}
	if len(meta.InputTypes) != 2 {
		t.Fatalf("expected 2 input types, got %d: %v", len(meta.InputTypes), meta.InputTypes)
	}
	if len(meta.OutputSections) != 2 {
		t.Fatalf("expected 2 output sections, got %d: %v", len(meta.OutputSections), meta.OutputSections)
	}
}

func TestParseProfileMetaNoFrontmatter(t *testing.T) {
	_, err := ParseProfileMeta([]byte("no frontmatter"))
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseProfileMetaMinimal(t *testing.T) {
	content := []byte(`---
name: minimal
---
`)
	meta, err := ParseProfileMeta(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ID != "minimal" {
		t.Fatalf("expected minimal, got %q", meta.ID)
	}
	if meta.ReadOnly {
		t.Fatal("expected non-read-only by default")
	}
}
