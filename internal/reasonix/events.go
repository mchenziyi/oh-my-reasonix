package reasonix

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EventRecord corresponds to a single line in the events JSONL file.
// Only includes safe, sanitized fields — no prompt, tool_args, tool_result, reasoning.
type EventRecord struct {
	Event             string `json:"event"`
	Seq               int    `json:"seq,omitempty"`
	Kind              string `json:"kind,omitempty"`
	ToolID            string `json:"tool_id,omitempty"`
	ToolName          string `json:"tool_name,omitempty"`
	Status            string `json:"status,omitempty"`
	PromptTokens      int    `json:"prompt_tokens,omitempty"`
	CompletionTokens  int    `json:"completion_tokens,omitempty"`
}

// EventStream is the parsed result of an events JSONL file.
type EventStream struct {
	Events      []EventRecord `json:"events"`
	RunDone     bool          `json:"run_done"`
	TotalTokens int           `json:"total_tokens,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
}

// ParseEventStream reads a JSONL file, parses each line into an EventRecord,
// validates sequence order, and checks for the final run_done event.
func ParseEventStream(path string) (EventStream, error) {
	f, err := os.Open(path)
	if err != nil {
		return EventStream{}, fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	var stream EventStream
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	maxLineBytes := 1024 * 1024
	lineNum := 0
	lastSeq := -1

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(line) > maxLineBytes {
			stream.Errors = append(stream.Errors, fmt.Sprintf("line %d: exceeds max size (%d bytes)", lineNum, maxLineBytes))
			continue
		}
		var rec EventRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			stream.Errors = append(stream.Errors, fmt.Sprintf("line %d: invalid JSON: %v", lineNum, err))
			continue
		}
		// Check for run_done
		if rec.Event == "run_done" {
			stream.RunDone = true
		}
		// Validate monotonic seq
		if rec.Seq > 0 {
			if rec.Seq <= lastSeq {
				stream.Errors = append(stream.Errors, fmt.Sprintf("line %d: non-monotonic seq %d (last=%d)", lineNum, rec.Seq, lastSeq))
			}
			lastSeq = rec.Seq
		}
		stream.Events = append(stream.Events, rec)
		stream.TotalTokens += rec.PromptTokens + rec.CompletionTokens
	}
	if err := scanner.Err(); err != nil {
		return stream, fmt.Errorf("read events file: %w", err)
	}
	if !stream.RunDone {
		stream.Errors = append(stream.Errors, "no run_done event found at end of stream")
	}
	return stream, nil
}
