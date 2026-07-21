package promptcompose

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Segment is one deterministic input to the composed Reasonix system prompt.
type Segment struct {
	ID      string
	Content string
	Present bool
	Hash    string
	Order   int
}

// Composition is the byte-stable result of composing Prompt segments.
type Composition struct {
	Content  string
	Hash     string
	Segments []Segment
}

// Canonicalize applies the PRD's line-ending and blank-line rules without
// changing content inside a segment.
func Canonicalize(input string) string {
	input = strings.TrimPrefix(input, "\ufeff")
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func SHA256String(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

// Compose joins Base, optional User, and OMR segments in the fixed order.
func Compose(base, user, orchestrator string) Composition {
	inputs := []struct {
		id      string
		content string
		present bool
	}{
		{id: "reasonix-base", content: base, present: true},
		{id: "user", content: user, present: Canonicalize(user) != ""},
		{id: "omr-orchestrator", content: orchestrator, present: true},
	}

	segments := make([]Segment, 0, len(inputs))
	parts := make([]string, 0, len(inputs))
	order := 1
	for _, input := range inputs {
		if !input.present {
			continue
		}
		content := Canonicalize(input.content)
		segments = append(segments, Segment{
			ID:      input.id,
			Content: content,
			Present: true,
			Hash:    SHA256String(content),
			Order:   order,
		})
		parts = append(parts, content)
		order++
	}
	result := strings.Join(parts, "\n\n") + "\n"
	return Composition{Content: result, Hash: SHA256String(result), Segments: segments}
}
