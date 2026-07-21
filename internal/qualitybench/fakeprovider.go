package qualitybench

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ReplayProvider is a deterministic local OpenAI-compatible response source
// for fixed behavior assertions. It never calls a remote Provider.
type ReplayProvider struct {
	mu      sync.Mutex
	outputs []ReplayOutput
	index   int
}

func NewReplayProvider(outputs []ReplayOutput) *ReplayProvider {
	copyOutputs := append([]ReplayOutput(nil), outputs...)
	return &ReplayProvider{outputs: copyOutputs}
}

func (p *ReplayProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.index = 0
}

func (p *ReplayProvider) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.index >= len(p.outputs) {
			http.Error(w, "replay exhausted", http.StatusGone)
			return
		}
		output := p.outputs[p.index]
		p.index++
		response := map[string]interface{}{
			"id":      fmt.Sprintf("omr-replay-%d", p.index),
			"object":  "chat.completion",
			"choices": []map[string]interface{}{{"index": 0, "message": map[string]string{"role": "assistant", "content": output.Output}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 0, "completion_tokens": len(output.Output), "total_tokens": len(output.Output)},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}
