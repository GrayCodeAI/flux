package discover_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/discover"
)

func TestDiscoverRun_RemoteFailureUsesCacheFallback(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failServer.Close()

	t.Setenv("EYRIE_MODEL_CATALOG_URL", failServer.URL)

	result, err := discover.Run(context.Background(), discover.Options{
		LoadCatalogOptions: catalog.LoadCatalogOptions{
			CachePath:     cachePath,
			RefreshRemote: true,
			RemoteURL:     failServer.URL,
		},
		DisableCredentialFallback: true,
	})
	if err != nil {
		t.Fatalf("discover.Run: %v", err)
	}
	if result == nil || result.Compiled == nil {
		t.Fatal("expected compiled catalog from cache fallback")
	}
	if len(result.Compiled.ModelsByID) == 0 {
		t.Fatal("expected models from seeded cache")
	}
	if result.Source != "cache-fallback" && result.Source != "cache-fallback+providers" {
		t.Fatalf("unexpected source %q", result.Source)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache not written: %v", err)
	}
}

func TestDiscoverRun_ConcurrentCallsSerialized(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	opts := discover.Options{
		LoadCatalogOptions: catalog.LoadCatalogOptions{
			CachePath:     cachePath,
			RefreshRemote: false,
		},
		DisableCredentialFallback: true,
	}
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := discover.Run(context.Background(), opts)
			done <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent discover: %v", err)
		}
	}
}
