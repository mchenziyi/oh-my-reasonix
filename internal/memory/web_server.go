package memory

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// MemoryWebHandler serves the local-only management API. The handler keeps no
// mutable memory state; every response is rebuilt from the bound FactStore.
type MemoryWebHandler struct {
	store *FactStore
	now   time.Time
}

func NewMemoryWebHandler(store *FactStore, now time.Time) (http.Handler, error) {
	if store == nil || now.IsZero() {
		return nil, storeError(CodeDerivedInvalidInput, "web handler requires store and explicit now")
	}
	return &MemoryWebHandler{store: store, now: now.UTC()}, nil
}

func (h *MemoryWebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.serveGet(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.servePost(w, r)
		return
	}
	writeWebError(w, http.StatusMethodNotAllowed)
}

func (h *MemoryWebHandler) serveGet(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error
	switch r.URL.Path {
	case "/", "/index.html":
		data, err = BuildMemoryWebExport(r.Context(), h.store, h.now)
	case "/audit":
		data, err = BuildMemoryAuditWebExport(r.Context(), h.store, h.now)
	case "/manager":
		data, err = BuildMemoryManagerPage(r.Context(), h.store, h.now)
	default:
		writeWebError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		writeWebError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline';")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *MemoryWebHandler) servePost(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/action/validate" && r.URL.Path != "/action/apply" {
		writeWebError(w, http.StatusNotFound)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		writeWebError(w, http.StatusBadRequest)
		return
	}
	var action WebManagementAction
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&action); err != nil || action.Validate() != nil {
		writeWebError(w, http.StatusBadRequest)
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeWebError(w, http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/action/validate" {
		hash, err := action.ContentHash()
		if err != nil {
			writeWebError(w, http.StatusBadRequest)
			return
		}
		writeWebJSON(w, http.StatusOK, map[string]any{"valid": true, "action_id": action.ActionID, "content_sha256": hash})
		return
	}
	if r.Header.Get("X-OMR-Confirm") != "yes" {
		writeWebError(w, http.StatusPreconditionRequired)
		return
	}
	result, err := ApplyWebManagementAction(r.Context(), h.store, action, true)
	if err != nil {
		writeWebError(w, http.StatusConflict)
		return
	}
	writeWebJSON(w, http.StatusOK, map[string]any{"status": result.Status.String(), "event_id": result.Event.EventID})
}

func writeWebJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeWebError(w http.ResponseWriter, status int) {
	writeWebJSON(w, status, map[string]string{"error": http.StatusText(status)})
}
