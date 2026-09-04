package discover_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/discover"
	"github.com/GrayCodeAI/graycode-router/catalog/live"
	graycoderoutercfg "github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestRun_DisableCredentialFallbackIgnoresAmbientStoreAndConfigPath(t *testing.T) {
	var requests atomic.Int32
	stubCanopyFetcher(t, func(map[string]string) ([]live.Entry, error) {
		requests.Add(1)
		return []live.Entry{{ID: "ambient/model"}}, nil
	})

	t.Setenv("GRAYCODE_ROUTER_CONFIG_DIR", t.TempDir())
	if err := graycoderoutercfg.SaveProviderConfig(&graycoderoutercfg.ProviderConfig{
		Version:           "1",
		CanopyWaveBaseURL: "https://ambient-canopy.invalid/v1",
	}, ""); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("CANOPYWAVE_API_KEY"), "ambient-canopy-key-1234567890"); err != nil {
		t.Fatal(err)
	}
	if graycoderoutercfg.DiscoveryCredentials(context.Background()).APIKeys["CANOPYWAVE_API_KEY"] == "" {
		t.Fatal("test setup: ambient fallback credential is not visible")
	}

	cachePath := filepath.Join(t.TempDir(), "isolated-catalog.json")
	base := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &base); err != nil {
		t.Fatal(err)
	}
	result, err := discover.Run(context.Background(), discover.Options{
		LoadCatalogOptions:        catalog.LoadCatalogOptions{CachePath: cachePath},
		DisableCredentialFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.LiveProviders) != 0 {
		t.Fatalf("live providers = %+v, want none without injected credentials", result.LiveProviders)
	}
	if requests.Load() != 0 {
		t.Fatalf("ambient provider endpoint received %d requests", requests.Load())
	}
}

func TestRunDoesNotReportRefreshWhenEveryLiveProviderFails(t *testing.T) {
	stubCanopyFetcher(t, func(map[string]string) ([]live.Entry, error) {
		return nil, errors.New("injected provider failure")
	})
	cachePath := filepath.Join(t.TempDir(), "catalog.json")
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &seed); err != nil {
		t.Fatal(err)
	}
	result, err := discover.Run(context.Background(), discover.Options{
		LoadCatalogOptions:        catalog.LoadCatalogOptions{CachePath: cachePath},
		Credentials:               catalog.Credentials{APIKeys: map[string]string{"CANOPYWAVE_API_KEY": "explicit-canopy-key-1234567890"}},
		DisableCredentialFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Refreshed || strings.Contains(result.Source, "providers") {
		t.Fatalf("failed live refresh reported success: %+v", result)
	}
}

func TestRefreshProviderWithOptions_DisableCredentialFallback(t *testing.T) {
	var requests atomic.Int32
	stubCanopyFetcher(t, func(map[string]string) ([]live.Entry, error) {
		requests.Add(1)
		return []live.Entry{{ID: "ambient/model"}}, nil
	})

	t.Setenv("GRAYCODE_ROUTER_CONFIG_DIR", t.TempDir())
	if err := graycoderoutercfg.SaveProviderConfig(&graycoderoutercfg.ProviderConfig{
		Version:           "1",
		CanopyWaveBaseURL: "https://ambient-canopy.invalid/v1",
	}, ""); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("CANOPYWAVE_API_KEY"), "ambient-canopy-key-1234567890"); err != nil {
		t.Fatal(err)
	}

	_, err := discover.RefreshProviderWithOptions(context.Background(), "canopywave", discover.ProviderRefreshOptions{
		CachePath:                 filepath.Join(t.TempDir(), "catalog.json"),
		DisableCredentialFallback: true,
	})
	if err == nil || !strings.Contains(err.Error(), "no credentials") {
		t.Fatalf("error = %v, want no credentials", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("ambient provider endpoint received %d requests", requests.Load())
	}
}

func TestRefreshProviderWithOptions_UsesOnlyScopedCachePath(t *testing.T) {
	var requests atomic.Int32
	stubCanopyFetcher(t, func(env map[string]string) ([]live.Entry, error) {
		requests.Add(1)
		if got := env["CANOPYWAVE_API_KEY"]; got != "explicit-canopy-key-1234567890" {
			t.Errorf("CANOPYWAVE_API_KEY = %q", got)
		}
		if got := env["CANOPYWAVE_BASE_URL"]; got != "https://scoped-canopy.example/v1" {
			t.Errorf("CANOPYWAVE_BASE_URL = %q", got)
		}
		return []live.Entry{{
			ID:            "scoped/live-model",
			ContextWindow: 32768,
			RawJSON:       []byte(`{"id":"scoped/live-model","context_length":32768}`),
		}}, nil
	})

	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global-catalog.json")
	scopedPath := filepath.Join(dir, "engine-a", "catalog.json")
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", globalPath)

	global := catalog.SeedCatalog()
	global.Aliases["global-cache-marker"] = "global-only"
	if err := catalog.WriteCatalogCache(globalPath, &global); err != nil {
		t.Fatal(err)
	}
	globalBefore, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}

	scoped := catalog.SeedCatalog()
	scoped.Aliases["scoped-cache-marker"] = "scoped-only"
	if err := catalog.WriteCatalogCache(scopedPath, &scoped); err != nil {
		t.Fatal(err)
	}

	result, err := discover.RefreshProviderWithOptions(context.Background(), "canopywave", discover.ProviderRefreshOptions{
		Credentials: catalog.Credentials{APIKeys: map[string]string{
			"CANOPYWAVE_API_KEY":  "explicit-canopy-key-1234567890",
			"CANOPYWAVE_BASE_URL": "https://scoped-canopy.example/v1",
		}},
		CachePath:                 scopedPath,
		DisableCredentialFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CachePath != scopedPath {
		t.Fatalf("result cache path = %q, want %q", result.CachePath, scopedPath)
	}
	if requests.Load() != 1 {
		t.Fatalf("provider endpoint requests = %d, want 1", requests.Load())
	}
	if result.Compiled.Catalog.Aliases["scoped-cache-marker"] != "scoped-only" {
		t.Fatal("refresh did not load the provider-scoped cache")
	}
	if _, ok := result.Compiled.Catalog.Aliases["global-cache-marker"]; ok {
		t.Fatal("refresh loaded state from the process-global cache path")
	}
	if len(catalog.ModelEntriesForProvider(result.Compiled, "canopywave")) == 0 {
		t.Fatal("provider-scoped cache is missing live CanopyWave models")
	}
	if catalog.IsBootstrapCatalog(result.Compiled.Catalog) {
		t.Fatal("provider refresh left populated catalog marked as bootstrap")
	}
	if _, ok := catalog.LoadValidCatalogCache(scopedPath); !ok {
		t.Fatal("provider refresh persisted a catalog that cannot be loaded as valid")
	}

	globalAfter, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(globalBefore, globalAfter) {
		t.Fatal("provider refresh modified the process-global cache")
	}
}

func stubCanopyFetcher(t *testing.T, fetch live.FetchFunc) {
	t.Helper()
	previous := live.Registry["canopywave"]
	live.Registry["canopywave"] = fetch
	t.Cleanup(func() { live.Registry["canopywave"] = previous })
}
