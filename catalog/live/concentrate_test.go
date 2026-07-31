package live

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchConcentrate_PreservesFunctionCallingCapability(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-v4-pro","display_name":"DeepSeek V4 Pro","owned_by":"deepseek","max_input_tokens":1040000,"max_tokens":384000,"capabilities":{"structured_outputs":{"supported":true}}}]}`))
	}))
	defer server.Close()

	entries, err := FetchConcentrate(map[string]string{"CONCENTRATE_BASE_URL": server.URL})
	if err != nil {
		t.Fatalf("FetchConcentrate: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if !strings.Contains(strings.Join(entries[0].Features, ","), "function_calling") {
		t.Fatalf("features = %v, want function_calling", entries[0].Features)
	}
}
