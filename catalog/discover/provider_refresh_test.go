package discover_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/discover"
)

func TestRefreshProvider_MergesLiveModelsIntoCache(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../live/testdata/canopywave_models.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatal(err)
	}

	result, err := discover.RefreshProvider(context.Background(), "canopywave", catalog.Credentials{APIKeys: map[string]string{
		"CANOPYWAVE_API_KEY":  "test-key-12345678",
		"CANOPYWAVE_BASE_URL": srv.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Compiled == nil {
		t.Fatal("expected compiled catalog")
	}
	entries := catalog.ModelEntriesForProvider(result.Compiled, "canopywave")
	if len(entries) != 2 {
		t.Fatalf("expected 2 canopywave models, got %d", len(entries))
	}
	if len(entries[0].LiveMetadata) == 0 {
		t.Fatal("expected live metadata on cached offering")
	}
}

func TestRefreshProvider_QualifiesUnpricedConcentrateModelIDs(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("request path = %q, want /models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "claude-fable-5",
				"display_name": "Claude Fable 5",
				"owned_by": "anthropic",
				"max_input_tokens": 200000,
				"max_tokens": 8192
			}]
		}`))
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatal(err)
	}

	result, err := discover.RefreshProviderWithOptions(context.Background(), "concentrate", discover.ProviderRefreshOptions{
		Credentials: catalog.Credentials{APIKeys: map[string]string{
			"CONCENTRATE_API_KEY":  "test-key-12345678",
			"CONCENTRATE_BASE_URL": srv.URL,
		}},
		CachePath:                 cachePath,
		DisableCredentialFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Compiled == nil {
		t.Fatal("expected compiled catalog")
	}
	const canonicalID = "concentrate/claude-fable-5"
	if _, ok := result.Compiled.ModelsByID[canonicalID]; !ok {
		t.Fatalf("compiled catalog is missing canonical model %q", canonicalID)
	}
	if got, ok := catalog.CanonicalModelForProviderNative(result.Compiled, "concentrate", "claude-fable-5"); !ok || got != canonicalID {
		t.Fatalf("native model mapping = %q, %v; want %q, true", got, ok, canonicalID)
	}
}
