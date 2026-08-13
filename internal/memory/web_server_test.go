package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func webHandlerFixture(t *testing.T) (http.Handler, WebManagementAction) {
	t.Helper()
	store, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	rev.MemoryID = "mem_web_handler"
	rev.Title = "<script>bad()</script>"
	target := validRevision()
	target.MemoryID = "mem_web_target"
	target.ContentSHA256, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	rev.Relations = []MemoryRelation{{Predicate: "related_to", Target: memoryRefFromRevision(target)}}
	rev.ContentSHA256, err = rev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	action := validWebAction()
	action.Target = memoryRefFromRevision(rev)
	h, err := NewMemoryWebHandler(store, time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return h, action
}

func TestMemoryWebHandlerReadAndActionProtocol(t *testing.T) {
	h, action := webHandlerFixture(t)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/", nil))
	if get.Code != http.StatusOK || !bytes.Contains(get.Body.Bytes(), []byte("mem_web_handler")) || get.Header().Get("X-Content-Type-Options") != "nosniff" || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read endpoint failed: %d %s", get.Code, get.Body.String())
	}
	manager := httptest.NewRecorder()
	h.ServeHTTP(manager, httptest.NewRequest(http.MethodGet, "/manager", nil))
	if manager.Code != http.StatusOK || !bytes.Contains(manager.Body.Bytes(), []byte("X-OMR-Confirm")) || !bytes.Contains(manager.Body.Bytes(), []byte("/audit")) || !bytes.Contains(manager.Body.Bytes(), []byte("mem_web_handler")) || !bytes.Contains(manager.Body.Bytes(), []byte("Lifecycle")) || !bytes.Contains(manager.Body.Bytes(), []byte("probation")) || !bytes.Contains(manager.Body.Bytes(), []byte("healthy")) || !bytes.Contains(manager.Body.Bytes(), []byte("Usage")) || !bytes.Contains(manager.Body.Bytes(), []byte("Relations")) || !bytes.Contains(manager.Body.Bytes(), []byte("related_to")) || !bytes.Contains(manager.Body.Bytes(), []byte("Unfreeze")) || !bytes.Contains(manager.Body.Bytes(), []byte("basis-refs")) || !bytes.Contains(manager.Body.Bytes(), []byte("location.reload")) || bytes.Contains(manager.Body.Bytes(), []byte("<script>bad()")) || !bytes.Contains(manager.Body.Bytes(), []byte("&lt;script&gt;bad()")) {
		t.Fatalf("manager endpoint failed: %d %s", manager.Code, manager.Body.String())
	}
	data, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	validate := httptest.NewRecorder()
	h.ServeHTTP(validate, httptest.NewRequest(http.MethodPost, "/action/validate", bytes.NewReader(data)))
	if validate.Code != http.StatusOK || !bytes.Contains(validate.Body.Bytes(), []byte(`"valid":true`)) {
		t.Fatalf("validate endpoint failed: %d %s", validate.Code, validate.Body.String())
	}
	trailing := httptest.NewRecorder()
	h.ServeHTTP(trailing, httptest.NewRequest(http.MethodPost, "/action/validate", bytes.NewReader(append(data, []byte(` {}`)...))))
	if trailing.Code != http.StatusBadRequest {
		t.Fatal("trailing JSON must be rejected")
	}
	apply := httptest.NewRecorder()
	h.ServeHTTP(apply, httptest.NewRequest(http.MethodPost, "/action/apply", bytes.NewReader(data)))
	if apply.Code != http.StatusPreconditionRequired {
		t.Fatalf("apply without confirmation must be rejected: %d", apply.Code)
	}
	apply = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action/apply", bytes.NewReader(data))
	req.Header.Set("X-OMR-Confirm", "yes")
	h.ServeHTTP(apply, req)
	if apply.Code != http.StatusOK {
		t.Fatalf("confirmed apply failed: %d %s", apply.Code, apply.Body.String())
	}
}

func TestMemoryWebHandlerRejectsUnknownRoutesAndMethods(t *testing.T) {
	h, _ := webHandlerFixture(t)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if r.Code != http.StatusNotFound {
		t.Fatal("unknown route must be 404")
	}
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/", nil))
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatal("unsupported method must be 405")
	}
}
