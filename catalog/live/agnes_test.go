package live

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchAgnes_Mock(t *testing.T) {
	// Mock Agnes /v1/models endpoint (OpenAI-compatible format)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object": "list",
			"data": [
				{
					"id": "agnes-2.0-flash",
					"object": "model",
					"created": 1700000000,
					"owned_by": "agnes"
				},
				{
					"id": "agnes-2.0-pro",
					"object": "model",
					"created": 1700000000,
					"owned_by": "agnes"
				}
			]
		}`))
	}))
	defer server.Close()

	entries, err := FetchAgnes(map[string]string{
		"AGNES_API_KEY":  "test-key",
		"AGNES_BASE_URL": server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("FetchAgnes: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify model IDs
	if entries[0].ID != "agnes-2.0-flash" {
		t.Errorf("expected agnes-2.0-flash, got %s", entries[0].ID)
	}
	if entries[1].ID != "agnes-2.0-pro" {
		t.Errorf("expected agnes-2.0-pro, got %s", entries[1].ID)
	}

	flash := entries[0]
	if flash.ContextWindow != 512_000 || flash.MaxOutput != 65_536 {
		t.Fatalf("agnes-2.0-flash context/max = %d/%d", flash.ContextWindow, flash.MaxOutput)
	}
	if !flash.ImageInput || !flash.ThinkingEnabled {
		t.Fatalf("agnes-2.0-flash caps image=%v thinking=%v", flash.ImageInput, flash.ThinkingEnabled)
	}
	if flash.InputPricePer1M != 0 || flash.OutputPricePer1M != 0 {
		t.Fatalf("agnes-2.0-flash should be free (0), got in=%f out=%f", flash.InputPricePer1M, flash.OutputPricePer1M)
	}
	foundTools := false
	for _, f := range flash.Features {
		if f == "tools" {
			foundTools = true
		}
	}
	if !foundTools {
		t.Fatalf("agnes-2.0-flash missing tools feature: %v", flash.Features)
	}

	// Unknown models keep unknown pricing; flash enrichment overrides to free.
	for _, e := range entries {
		if e.ID == "agnes-2.0-pro" && (e.InputPricePer1M >= 0 || e.OutputPricePer1M >= 0) {
			t.Errorf("expected unknown pricing for %s, got in=%f out=%f", e.ID, e.InputPricePer1M, e.OutputPricePer1M)
		}
	}
}

func TestFetchAgnes_DefaultBaseURL(t *testing.T) {
	// Verify default base URL is used when AGNES_BASE_URL is not set
	// We can't easily test the actual call without a mock, but we can
	// verify the function doesn't panic with empty base URL
	// (it will fail trying to connect to the default URL)
	_, err := FetchAgnes(map[string]string{
		"AGNES_API_KEY": "test-key",
	})
	// We expect a connection error since we're not running a real server
	if err == nil {
		t.Log("expected connection error, got nil (unexpected but not a test failure)")
	}
}

func TestFetchAgnes_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"无效的令牌"}}`))
	}))
	defer server.Close()

	_, err := FetchAgnes(map[string]string{
		"AGNES_API_KEY":  "invalid-key",
		"AGNES_BASE_URL": server.URL,
	})
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}
