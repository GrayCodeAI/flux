package live

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchOpenGateway_ParsesPricingAndContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		// No auth required for the public listing.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("did not expect Authorization header on public /models, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "auto",
					"name": "Auto (smart routing)",
					"description": "picks the cheapest capable model",
					"context_window": null,
					"aliases": ["gitlawb/auto"],
					"pricing": null,
					"effective_pricing": null
				},
				{
					"id": "xiaomi/mimo-v2.5-pro",
					"name": "MiMo V2.5-Pro",
					"description": "general large language model",
					"aliases": [],
					"context_window": 262144,
					"pricing": {"prompt": "0.000000435", "completion": "0.00000087", "input_cache_read": "0.0000000036"},
					"effective_pricing": {"prompt": "0.000000522", "completion": "0.000001044", "input_cache_read": "0.00000000432"}
				},
				{
					"id": "nvidia/nemotron-3-ultra-550b-a55b:free",
					"name": "Nemotron 3 UltraFREE",
					"description": "frontier reasoning MoE",
					"aliases": [],
					"context_window": 131072,
					"pricing": {"prompt": "0", "completion": "0", "input_cache_read": "0"},
					"effective_pricing": {"prompt": "0", "completion": "0", "input_cache_read": "0"}
				}
			]
		}`))
	}))
	defer server.Close()

	entries, err := FetchOpenGateway(map[string]string{"OPENGATEWAY_BASE_URL": server.URL})
	if err != nil {
		t.Fatalf("FetchOpenGateway: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}

	want := map[string]struct {
		in, out, cached float64
		ctx             int
	}{
		"auto":                                   {0, 0, 0, 0},
		"xiaomi/mimo-v2.5-pro":                   {0.522, 1.044, 0.00432, 262144},
		"nvidia/nemotron-3-ultra-550b-a55b:free": {0, 0, 0, 131072},
	}
	for _, e := range entries {
		w, ok := want[e.ID]
		if !ok {
			t.Errorf("unexpected entry %q", e.ID)
			continue
		}
		if e.ContextWindow != w.ctx {
			t.Errorf("%s context_window = %d, want %d", e.ID, e.ContextWindow, w.ctx)
		}
		if e.InputPricePer1M != w.in {
			t.Errorf("%s input = %v, want %v", e.ID, e.InputPricePer1M, w.in)
		}
		if e.OutputPricePer1M != w.out {
			t.Errorf("%s output = %v, want %v", e.ID, e.OutputPricePer1M, w.out)
		}
		if e.CachedReadPricePer1M != w.cached {
			t.Errorf("%s cached_read = %v, want %v", e.ID, e.CachedReadPricePer1M, w.cached)
		}
	}
}

func TestFetchOpenGateway_EmptyCatalog(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	entries, err := FetchOpenGateway(map[string]string{"OPENGATEWAY_BASE_URL": server.URL})
	if err != nil {
		t.Fatalf("FetchOpenGateway: unexpected error on empty catalog: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0 on empty catalog", len(entries))
	}
}

func TestFetchOpenGateway_SendsAuthWhenKeyProvided(t *testing.T) {
	t.Parallel()

	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"auto","name":"Auto","effective_pricing":{},"pricing":{}}]}`))
	}))
	defer server.Close()

	if _, err := FetchOpenGateway(map[string]string{"OPENGATEWAY_BASE_URL": server.URL, "OPENGATEWAY_API_KEY": "ogw_live_test"}); err != nil {
		t.Fatalf("FetchOpenGateway: %v", err)
	}
	if seenAuth != "Bearer ogw_live_test" {
		t.Errorf("Authorization header = %q, want %q", seenAuth, "Bearer ogw_live_test")
	}
}
