package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFetchOpenRouter_Mock(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		resp := struct {
			Data []json.RawMessage `json:"data"`
		}{
			Data: []json.RawMessage{json.RawMessage(`{
				"id":"anthropic/claude-sonnet-4-6",
				"context_length":200000,
				"top_provider":{"max_completion_tokens":32000},
				"architecture":{"modality":"text"}
			}`)},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	entries, err := FetchOpenRouter(map[string]string{
		"OPENROUTER_API_KEY":  "test-key-12345",
		"OPENROUTER_BASE_URL": srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].RawJSON) == 0 {
		t.Fatal("expected full provider JSON preserved")
	}
	if !strings.Contains(string(entries[0].RawJSON), "architecture") {
		t.Fatalf("expected raw metadata, got %s", entries[0].RawJSON)
	}
}

func TestFetchCanopyWave_ParsesProviderFields(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"id": "vendor/sample",
		"display_name": "Sample Model",
		"description": "Synthetic parser test",
		"context_size": 2048,
		"status": 1,
		"max_output_tokens": 512,
		"input_token_price_per_m": 11,
		"output_token_price_per_m": 22,
		"features": ["function-calling","vision"],
		"tags": ["synthetic-test-only"]
	}`)
	entry, ok := entryFromOpenAICompatJSON(raw)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if entry.ID != "vendor/sample" {
		t.Fatalf("id = %q", entry.ID)
	}
	if entry.DisplayName != "Sample Model" {
		t.Fatalf("display = %q", entry.DisplayName)
	}
	if entry.ContextWindow != 2048 || entry.MaxOutput != 512 {
		t.Fatalf("context/max = %d/%d", entry.ContextWindow, entry.MaxOutput)
	}
	if entry.InputPricePer1M != 11 || entry.OutputPricePer1M != 22 {
		t.Fatalf("pricing = %v/%v", entry.InputPricePer1M, entry.OutputPricePer1M)
	}
	if len(entry.Features) != 2 {
		t.Fatalf("features = %v", entry.Features)
	}
	if !strings.Contains(string(entry.RawJSON), "synthetic-test-only") {
		t.Fatalf("raw json not preserved: %s", entry.RawJSON)
	}
}

func TestFetchCanopyWave_MockHTTPServer(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/canopywave_models.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	entries, err := FetchCanopyWave(map[string]string{
		"CANOPYWAVE_API_KEY":  "test-key-12345678",
		"CANOPYWAVE_BASE_URL": srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 models, got %d", len(entries))
	}
	byID := map[string]Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	alpha, ok := byID["vendor/alpha"]
	if !ok {
		t.Fatal("missing vendor/alpha")
	}
	if alpha.DisplayName != "Alpha Model" || alpha.ContextWindow != 100000 {
		t.Fatalf("alpha = %+v", alpha)
	}
	beta, ok := byID["vendor/beta"]
	if !ok {
		t.Fatal("missing vendor/beta")
	}
	if beta.ContextWindow != 0 || beta.MaxOutput != 0 {
		t.Fatalf("beta nulls should be unknown (0): %+v", beta)
	}
	for _, e := range entries {
		if len(e.RawJSON) == 0 {
			t.Fatalf("%s missing raw json", e.ID)
		}
	}
}

func TestFetchOllama_EmptyModels(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	}))
	defer srv.Close()

	entries, err := FetchOllama(map[string]string{"OLLAMA_BASE_URL": srv.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestFetchOllama_EnrichesCapabilities(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []any{map[string]any{"name": "deepseek-r1:7b"}},
			})
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"capabilities": []string{"completion", "thinking", "tools"},
				"model_info":   map[string]any{"qwen2.context_length": 131072},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	entries, err := FetchOllama(map[string]string{"OLLAMA_BASE_URL": srv.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.ContextWindow != 131072 {
		t.Fatalf("context window = %d, want 131072", entry.ContextWindow)
	}
	if len(entry.Features) != 2 || entry.Features[0] != "thinking:enabled" || entry.Features[1] != "tools" {
		t.Fatalf("features = %v, want [thinking:enabled tools]", entry.Features)
	}
}

func TestFetchOllama_UsesTagsWhenShowUnavailable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []any{map[string]any{"name": "text-model"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	entries, err := FetchOllama(map[string]string{"OLLAMA_BASE_URL": srv.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "text-model" {
		t.Fatalf("entries = %+v", entries)
	}
	if len(entries[0].Features) != 0 {
		t.Fatalf("unexpected features after unavailable show endpoint: %v", entries[0].Features)
	}
}
