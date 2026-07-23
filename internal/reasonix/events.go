package reasonix

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// EventRecord corresponds to a single line in the events JSONL file.
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
