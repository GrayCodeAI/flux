package live

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchFireworks_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatalf("missing bearer authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"accounts/fireworks/models/deepseek-v4-flash","owned_by":"fireworks"}]}`))
	}))
	defer server.Close()

	entries, err := FetchFireworks(map[string]string{
		"FIREWORKS_API_KEY":  "fw-test-key",
		"FIREWORKS_BASE_URL": server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "accounts/fireworks/models/deepseek-v4-flash" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestFetchFireworks_NoKey(t *testing.T) {
	entries, err := FetchFireworks(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
}
