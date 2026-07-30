package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncodeJSONCompact(t *testing.T) {
	var output bytes.Buffer
	if err := encodeJSON(&output, map[string]int{"value": 1}, false); err != nil {
		t.Fatalf("encode compact JSON: %v", err)
	}
	if got, want := output.String(), "{\"value\":1}\n"; got != want {
		t.Fatalf("compact JSON = %q, want %q", got, want)
	}
}

func TestEncodeJSONPretty(t *testing.T) {
	var output bytes.Buffer
	if err := encodeJSON(&output, map[string]int{"value": 1}, true); err != nil {
		t.Fatalf("encode pretty JSON: %v", err)
	}
	if got, want := output.String(), "{\n  \"value\": 1\n}\n"; got != want {
		t.Fatalf("pretty JSON = %q, want %q", got, want)
	}
}

func TestNormalizeLeadingTargetArgs(t *testing.T) {
	got := normalizeLeadingTargetArgs([]string{"session-1", "--project-dir", "/tmp/project", "--json"})
	want := []string{"--project-dir", "/tmp/project", "--json", "session-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLeadingTargetArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeLeadingTargetArgsKeepsFlagsFirst(t *testing.T) {
	args := []string{"--project-dir", "/tmp/project", "session-1"}
	got := normalizeLeadingTargetArgs(args)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("normalizeLeadingTargetArgs() = %#v, want %#v", got, args)
	}
}
