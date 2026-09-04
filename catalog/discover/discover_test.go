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
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

func TestDiscoverCatalog_MergesProviderModelsWithAPIKey(t *testing.T) {
	t.Parallel()
	orServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-or-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"vendor/special-model","context_length":32000,"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer orServer.Close()

	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	result, err := discover.Run(context.Background(), discover.Options{
		LoadCatalogOptions: catalog.LoadCatalogOptions{
			CachePath:     cachePath,
			RefreshRemote: false,
		},
		Credentials: catalog.Credentials{APIKeys: map[string]string{
			"OPENROUTER_API_KEY":  "test-or-key",
			"OPENROUTER_BASE_URL": orServer.URL,
		}},
	})
	if err != nil {
		t.Fatalf("discover.Run: %v", err)
	}
	if result == nil || result.Compiled == nil {
		t.Fatal("expected compiled catalog")
	}
	found := false
	for id := range result.Compiled.ModelsByID {
		if id != "" {
			found = true
			break
		}
	}
	if !found && len(result.Compiled.OfferingsByID) == 0 {
		t.Fatal("expected models or offerings after discover")
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache not written: %v", err)
	}
	if result.Compiled.Catalog.Provenance == nil || result.Compiled.Catalog.Provenance.Source != result.Source {
		t.Fatalf("catalog provenance = %+v, want source %q", result.Compiled.Catalog.Provenance, result.Source)
	}
	if _, ok := catalog.LoadValidCatalogCache(cachePath); !ok {
		t.Fatal("discover persisted a catalog that cannot be loaded as valid")
	}
	if len(result.LiveProviders) != len(registry.All()) {
		t.Fatalf("LiveProviders: got %d want %d", len(result.LiveProviders), len(registry.All()))
	}
	var openrouter *catalog.LiveProviderEnrichment
	for i := range result.LiveProviders {
		if result.LiveProviders[i].Provider == "openrouter" {
			openrouter = &result.LiveProviders[i]
			break
		}
	}
	if openrouter == nil || openrouter.ModelCount < 1 {
		t.Fatalf("openrouter enrichment: %+v", openrouter)
	}
}
