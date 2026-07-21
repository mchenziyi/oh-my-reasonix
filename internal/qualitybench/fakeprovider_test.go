package qualitybench

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplayProviderIsDeterministic(t *testing.T) {
	provider := NewReplayProvider([]ReplayOutput{{Output: "first"}, {Output: "second"}})
	server := httptest.NewServer(provider.Handler())
	defer server.Close()
	for _, expected := range []string{"first", "second"} {
		response, err := http.Post(server.URL, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !containsString(string(body), expected) {
			t.Fatalf("unexpected replay response: %d %s", response.StatusCode, body)
		}
	}
	response, err := http.Post(server.URL, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusGone {
		t.Fatalf("expected exhausted replay, got %d", response.StatusCode)
	}
}

func containsString(value, wanted string) bool {
	for i := 0; i+len(wanted) <= len(value); i++ {
		if value[i:i+len(wanted)] == wanted {
			return true
		}
	}
	return false
}
