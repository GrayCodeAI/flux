package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEntryFromOpenAICompatJSON_ParsesLongCatContextWindow(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"id": "LongCat-2.0",
		"object": "model",
		"owned_by": "LongCat",
		"display_name": "LongCat-2.0",
		"context_window": 1048576,
		"max_output_tokens": 131072
	}`)
	entry, ok := entryFromOpenAICompatJSON(raw)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if entry.ID != "LongCat-2.0" {
		t.Fatalf("id = %q", entry.ID)
	}
	if entry.ContextWindow != 1_048_576 || entry.MaxOutput != 131_072 {
		t.Fatalf("context/max = %d/%d", entry.ContextWindow, entry.MaxOutput)
	}
}

func TestFetchLongCat_Mock(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id":                "LongCat-2.0",
				"object":            "model",
				"owned_by":          "LongCat",
				"display_name":      "LongCat-2.0",
				"context_window":    1048576,
				"max_output_tokens": 131072,
			}},
		})
	}))
	defer srv.Close()

	entries, err := FetchLongCat(map[string]string{
		"LONGCAT_API_KEY":  "test-key-12345678",
		"LONGCAT_BASE_URL": srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ContextWindow != 1_048_576 || entries[0].MaxOutput != 131_072 {
		t.Fatalf("context/max = %d/%d", entries[0].ContextWindow, entries[0].MaxOutput)
	}
	if entries[0].InputPricePer1M >= 0 || entries[0].OutputPricePer1M >= 0 {
		t.Fatalf("expected unknown pricing, got in=%f out=%f", entries[0].InputPricePer1M, entries[0].OutputPricePer1M)
	}
	if !entries[0].ThinkingEnabled || entries[0].ImageInput {
		t.Fatalf("longcat caps thinking=%v image=%v", entries[0].ThinkingEnabled, entries[0].ImageInput)
	}
	foundTools := false
	for _, f := range entries[0].Features {
		if f == "tools" {
			foundTools = true
		}
	}
	if !foundTools {
		t.Fatalf("missing tools feature: %v", entries[0].Features)
	}
}
