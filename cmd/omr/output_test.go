package main

import (
	"bytes"
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
