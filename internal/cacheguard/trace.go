package cacheguard

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type Usage struct {
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

type RequestRecord struct {
	RunID         string          `json:"run_id"`
	RequestID     string          `json:"request_id"`
	StreamRole    string          `json:"stream_role,omitempty"`
	Body          json.RawMessage `json:"body"`
	ForwardedBody json.RawMessage `json:"forwarded_body,omitempty"`
	Usage         *Usage          `json:"usage,omitempty"`
	DeclaredReset bool            `json:"declared_reset,omitempty"`
}

type AnalyzedRecord struct {
	RunID                       string `json:"run_id"`
	RequestID                   string `json:"request_id"`
	LogicalStreamID             string `json:"logical_stream_id"`
	StreamRole                  string `json:"stream_role"`
	RawRequestBodySHA256        string `json:"raw_request_body_sha256"`
	ForwardedRequestBodySHA256  string `json:"forwarded_request_body_sha256"`
	SystemPromptSHA256          string `json:"system_prompt_sha256,omitempty"`
	CanonicalToolSchemaSHA256   string `json:"canonical_tool_schema_sha256,omitempty"`
	MessagesSHA256              string `json:"messages_sha256,omitempty"`
	PreviousMessagesPrefixMatch bool   `json:"previous_messages_prefix_match"`
	Classification              string `json:"classification"`
	Error                       string `json:"error,omitempty"`
}

type Stream struct {
	ID       string `json:"id"`
	RunID    string `json:"run_id"`
	Role     string `json:"role"`
	Requests int    `json:"requests"`
}

type Report struct {
	Records               []AnalyzedRecord `json:"records"`
	Streams               []Stream         `json:"streams"`
	AmbiguousStream       int              `json:"ambiguous_stream"`
	UnexpectedDivergence  int              `json:"unexpected_divergence"`
	BodyMismatch          int              `json:"body_mismatch"`
	WarmEligible          int              `json:"warm_eligible"`
	Cold                  int              `json:"cold"`
	PromptCacheHitTokens  int              `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int              `json:"prompt_cache_miss_tokens"`
	SteadyStateHitRate    float64          `json:"steady_state_hit_rate"`
	Passed                bool             `json:"passed"`
}

type message struct {
	Raw json.RawMessage
}

type requestPayload struct {
	Messages       []json.RawMessage `json:"messages"`
	SystemPrompt   string            `json:"system_prompt,omitempty"`
	Tools          json.RawMessage   `json:"tools,omitempty"`
	PromptHash     string            `json:"system_prompt_sha256,omitempty"`
	ToolSchemaHash string            `json:"tool_schema_sha256,omitempty"`
}

type streamState struct {
	id       string
	runID    string
	role     string
	messages []string
	requests int
}

func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Analyze(records []RequestRecord) Report {
	result := Report{Passed: true}
	streamsByRun := map[string][]*streamState{}
	streamCount := map[string]int{}
	for _, record := range records {
		rawHash := SHA256(record.Body)
		forwarded := record.ForwardedBody
		if len(forwarded) == 0 {
			forwarded = record.Body
		}
		forwardedHash := SHA256(forwarded)
		analyzed := AnalyzedRecord{
			RunID:                      record.RunID,
			RequestID:                  record.RequestID,
			RawRequestBodySHA256:       rawHash,
			ForwardedRequestBodySHA256: forwardedHash,
			StreamRole:                 record.StreamRole,
		}
		if record.Usage != nil {
			result.PromptCacheHitTokens += record.Usage.PromptCacheHitTokens
			result.PromptCacheMissTokens += record.Usage.PromptCacheMissTokens
		}
		if rawHash != forwardedHash {
			result.BodyMismatch++
			result.Passed = false
			analyzed.Error = "forwarded request body differs from original body"
		}
		payload, messages, err := parsePayload(record.Body)
		if err != nil {
			analyzed.Classification = "unknown"
			analyzed.Error = err.Error()
			result.Records = append(result.Records, analyzed)
			result.Passed = false
			continue
		}
		analyzed.SystemPromptSHA256 = payload.PromptHash
		if analyzed.SystemPromptSHA256 == "" && payload.SystemPrompt != "" {
			analyzed.SystemPromptSHA256 = SHA256([]byte(payload.SystemPrompt))
		}
		analyzed.CanonicalToolSchemaSHA256 = payload.ToolSchemaHash
		if analyzed.CanonicalToolSchemaSHA256 == "" && len(payload.Tools) > 0 {
			analyzed.CanonicalToolSchemaSHA256 = SHA256(canonicalJSON(payload.Tools))
		}
		analyzed.MessagesSHA256 = SHA256([]byte(strings.Join(messages, "\n")))

		candidates := make([]*streamState, 0)
		for _, state := range streamsByRun[record.RunID] {
			if record.DeclaredReset {
				continue
			}
			if isPrefix(state.messages, messages) {
				candidates = append(candidates, state)
			}
		}
		var selected *streamState
		switch {
		case record.DeclaredReset:
			analyzed.Classification = "declared_reset"
		case len(candidates) > 1:
			analyzed.Classification = "ambiguous_stream"
			analyzed.Error = "request matches multiple logical streams"
			result.AmbiguousStream++
			result.Passed = false
		case len(candidates) == 1 && equalMessages(candidates[0].messages, messages):
			// Equal starting bodies are not distinguishable from a concurrent
			// duplicate, so fail closed instead of inventing a stream join.
			analyzed.Classification = "ambiguous_stream"
			analyzed.Error = "request duplicates an existing stream prefix"
			result.AmbiguousStream++
			result.Passed = false
		case len(candidates) == 1:
			selected = candidates[0]
			analyzed.Classification = "warm_eligible"
			analyzed.PreviousMessagesPrefixMatch = true
			result.WarmEligible++
		default:
			if len(streamsByRun[record.RunID]) == 0 {
				analyzed.Classification = "cold"
				result.Cold++
			} else {
				analyzed.Classification = "unexpected_divergence"
				analyzed.Error = "request does not continue any known logical stream"
				result.UnexpectedDivergence++
				result.Passed = false
			}
		}
		if selected == nil && analyzed.Classification != "ambiguous_stream" {
			streamCount[record.RunID]++
			selected = &streamState{id: fmt.Sprintf("%s/stream-%d", record.RunID, streamCount[record.RunID]), runID: record.RunID, role: record.StreamRole, messages: messages}
			streamsByRun[record.RunID] = append(streamsByRun[record.RunID], selected)
		} else if selected != nil {
			selected.messages = messages
		}
		if selected != nil {
			selected.requests++
			analyzed.LogicalStreamID = selected.id
			if analyzed.StreamRole == "" {
				analyzed.StreamRole = selected.role
			}
		}
		result.Records = append(result.Records, analyzed)
	}
	for _, states := range streamsByRun {
		for _, state := range states {
			result.Streams = append(result.Streams, Stream{ID: state.id, RunID: state.runID, Role: state.role, Requests: state.requests})
		}
	}
	if result.AmbiguousStream > 0 || result.UnexpectedDivergence > 0 || result.BodyMismatch > 0 {
		result.Passed = false
	}
	denominator := result.PromptCacheHitTokens + result.PromptCacheMissTokens
	if denominator > 0 {
		result.SteadyStateHitRate = float64(result.PromptCacheHitTokens) / float64(denominator)
	}
	return result
}

func AnalyzeJSONL(r io.Reader) (Report, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var records []RequestRecord
	line := 0
	for scanner.Scan() {
		line++
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		var record RequestRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return Report{}, fmt.Errorf("parse trace line %d: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return Report{}, err
	}
	return Analyze(records), nil
}

func ReadJSONL(path string) (Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer file.Close()
	return AnalyzeJSONL(file)
}

func WriteReport(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func parsePayload(body json.RawMessage) (requestPayload, []string, error) {
	var payload requestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, nil, err
	}
	if len(payload.Messages) == 0 {
		return payload, nil, fmt.Errorf("request body has no messages")
	}
	messages := make([]string, 0, len(payload.Messages))
	for _, raw := range payload.Messages {
		messages = append(messages, string(canonicalJSON(raw)))
	}
	return payload, messages, nil
}

func canonicalJSON(raw []byte) []byte {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return bytes.TrimSpace(raw)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return bytes.TrimSpace(raw)
	}
	return canonical
}

func isPrefix(previous, current []string) bool {
	if len(previous) > len(current) {
		return false
	}
	for i := range previous {
		if previous[i] != current[i] {
			return false
		}
	}
	return true
}

func equalMessages(a, b []string) bool {
	return len(a) == len(b) && isPrefix(a, b)
}
