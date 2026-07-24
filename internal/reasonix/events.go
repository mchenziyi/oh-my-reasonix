package reasonix

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// rawEventLine is an intermediate representation that can decode both
// OMR legacy event format and Reasonix v1.17.20 native format.
type rawEventLine struct {
	// OMR legacy fields
	Event string `json:"event"`
	Seq   int    `json:"seq"`
	Kind  string `json:"kind"`

	// Reasonix v1.17.20 native fields
	Sequence int       `json:"sequence"`
	Usage    *rawUsage `json:"usage"`
	Ok       *bool     `json:"ok"`

	// Common fields
	ToolID           string `json:"tool_id"`
	ToolName         string `json:"tool_name"`
	Status           string `json:"status"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	SessionID        string `json:"session_id"`
	DurationMs       int    `json:"duration_ms"`
	NumTurns         int    `json:"num_turns"`
}

type rawUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheHitTokens  int `json:"cache_hit_tokens"`
	CacheMissTokens int `json:"cache_miss_tokens"`
}

// normalizeEvent converts a rawEventLine into the stable EventRecord form.
// It handles both OMR legacy and Reasonix v1.17.20 native formats.
func normalizeEvent(raw rawEventLine) EventRecord {
	rec := EventRecord{
		ToolID:           raw.ToolID,
		ToolName:         raw.ToolName,
		Status:           raw.Status,
		PromptTokens:     raw.PromptTokens,
		CompletionTokens: raw.CompletionTokens,
	}

	// Detect format: Reasonix v1.17.20 uses "sequence" instead of "seq"
	if raw.Sequence > 0 && raw.Seq == 0 {
		rec.Seq = raw.Sequence
		// Map kind → event
		rec.Kind = raw.Kind
		if raw.Kind == "run_done" {
			rec.Event = "run_done"
		} else {
			rec.Event = raw.Kind
		}
		// Map usage → token fields
		if raw.Usage != nil {
			rec.PromptTokens = raw.Usage.InputTokens
			rec.CompletionTokens = raw.Usage.OutputTokens
		}
	} else {
		// OMR legacy format
		rec.Seq = raw.Seq
		rec.Event = raw.Event
		rec.Kind = raw.Kind
		if rec.Event == "" {
			rec.Event = raw.Kind
		}
	}

	return rec
}

// Only includes safe, sanitized fields — no prompt, tool_args, tool_result, reasoning.
type EventRecord struct {
	Event            string `json:"event"`
	Seq              int    `json:"seq,omitempty"`
	Kind             string `json:"kind,omitempty"`
	ToolID           string `json:"tool_id,omitempty"`
	ToolName         string `json:"tool_name,omitempty"`
	Status           string `json:"status,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
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
	maxLineBytes := 1024 * 1024
	lineNum := 0
	lastSeq := -1
	reader := bufio.NewReaderSize(f, 64*1024)

	for {
		lineBytes, tooLarge, readErr := readEventLine(reader, maxLineBytes)
		if len(lineBytes) == 0 && readErr == io.EOF {
			break
		}
		lineNum++
		if tooLarge {
			stream.Errors = append(stream.Errors, fmt.Sprintf("line %d: exceeds max size (%d bytes)", lineNum, maxLineBytes))
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return stream, fmt.Errorf("read events file: %w", readErr)
			}
			continue
		}
		if readErr != nil && readErr != io.EOF {
			return stream, fmt.Errorf("read events file: %w", readErr)
		}
		line := strings.TrimSpace(string(lineBytes))
		if line == "" {
			if readErr == io.EOF {
				break
			}
			continue
		}
		var raw rawEventLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			stream.Errors = append(stream.Errors, fmt.Sprintf("line %d: invalid JSON: %v", lineNum, err))
			continue
		}
		rec := normalizeEvent(raw)
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
		if readErr == io.EOF {
			break
		}
	}
	if len(stream.Events) == 0 || stream.Events[len(stream.Events)-1].Event != "run_done" {
		stream.RunDone = false
		stream.Errors = append(stream.Errors, "run_done event must be the final event in the stream")
	} else {
		stream.RunDone = true
	}
	if len(stream.Events) > 0 {
		for i, event := range stream.Events[:len(stream.Events)-1] {
			if event.Event == "run_done" {
				stream.Errors = append(stream.Errors, fmt.Sprintf("event %d: run_done must be final", i+1))
			}
		}
	}
	return stream, nil
}

func readEventLine(reader *bufio.Reader, maxBytes int) ([]byte, bool, error) {
	var line []byte
	tooLarge := false
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > maxBytes {
			tooLarge = true
		} else if !tooLarge {
			line = append(line, part...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, tooLarge, err
	}
}
