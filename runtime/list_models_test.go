package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/runtime"
)

func TestListModels_RequiresProvider(t *testing.T) {
	_, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{})
	if err == nil {
		t.Fatal("expected error for empty provider")
	}
}

func TestListModels_WhitespaceProvider(t *testing.T) {
	_, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{ProviderID: "   "})
	if err == nil {
		t.Fatal("expected error for whitespace-only provider")
	}
}

func TestFormatSetupError_Ollama(t *testing.T) {
	err := runtime.FormatSetupError("ollama", context.DeadlineExceeded)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFormatSetupError_NilError(t *testing.T) {
	if err := runtime.FormatSetupError("openai", nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestListModels_CacheReadDoesNotRequireDiscover(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	now := time.Now().UTC().Truncate(time.Second)
	c := catalog.Catalog{
		SchemaVersion: catalog.CatalogSchemaVersion,
		GeneratedAt:   now,
		StaleAfter:    now.Add(time.Hour),
		Providers: map[string]catalog.Provider{
			"openai": {ID: "openai", Name: "OpenAI"},
		},
		Protocols: map[string]catalog.Protocol{
			"openai-chat-completions": {ID: "openai-chat-completions", Name: "OpenAI Chat Completions"},
		},
		Deployments: map[string]catalog.Deployment{
			"openai-direct": {
				ID: "openai-direct", Name: "OpenAI", ProviderID: "openai",
				APIProtocolID: "openai-chat-completions", AdapterConstructor: "openai",
				NativeModelIDSource: catalog.NativeModelIDCatalogKnown,
			},
		},
		Models: map[string]catalog.Model{
			"openai/gpt-test": {ID: "openai/gpt-test", ProviderID: "openai", Name: "GPT Test"},
		},
		Offerings: []catalog.ModelOffering{{
			ID: "openai-direct:gpt-test", CanonicalModelID: "openai/gpt-test",
			DeploymentID: "openai-direct", NativeModelID: "gpt-test",
			Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
		}},
	}
	if err := catalog.WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", cachePath)

	entries, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
		ProviderID: "openai",
		Source:     runtime.ListSourceCache,
	})
	if err != nil {
		t.Fatalf("ListModels cache: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "gpt-test" {
		t.Fatalf("entries: %+v", entries)
	}
}

func TestListModels_CacheEntriesHaveCorrectFields(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	now := time.Now().UTC().Truncate(time.Second)
	c := catalog.Catalog{
		SchemaVersion: catalog.CatalogSchemaVersion,
		GeneratedAt:   now,
		StaleAfter:    now.Add(time.Hour),
		Providers: map[string]catalog.Provider{
			"anthropic": {ID: "anthropic", Name: "Anthropic"},
		},
		Protocols: map[string]catalog.Protocol{
			"anthropic-messages": {ID: "anthropic-messages", Name: "Anthropic Messages"},
		},
		Deployments: map[string]catalog.Deployment{
			"anthropic-direct": {
				ID: "anthropic-direct", Name: "Anthropic", ProviderID: "anthropic",
				APIProtocolID: "anthropic-messages", AdapterConstructor: "anthropic",
				NativeModelIDSource: catalog.NativeModelIDCatalogKnown,
			},
		},
		Models: map[string]catalog.Model{
			"anthropic/claude-opus-4-6": {
				ID: "anthropic/claude-opus-4-6", ProviderID: "anthropic", Name: "Claude Opus 4.6",
				ContextWindow: 200000, MaxOutput: 32000,
			},
		},
		Offerings: []catalog.ModelOffering{{
			ID: "anthropic-direct:claude-opus-4-6", CanonicalModelID: "anthropic/claude-opus-4-6",
			DeploymentID: "anthropic-direct", NativeModelID: "claude-opus-4-6",
			Pricing: catalog.Pricing{Status: catalog.PricingUnknown}, LiveMetadata: []byte(`{"id":"claude-opus-4-6"}`),
		}},
	}
	if err := catalog.WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", cachePath)

	entries, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
		ProviderID: "anthropic",
		Source:     runtime.ListSourceCache,
	})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != "claude-opus-4-6" {
		t.Fatalf("expected ID 'claude-opus-4-6', got %q", e.ID)
	}
	if e.ProviderID != "anthropic" {
		t.Fatalf("expected provider 'anthropic', got %q", e.ProviderID)
	}
	if e.Source != "cache" {
		t.Fatalf("expected source 'cache', got %q", e.Source)
	}
	if e.ContextWindow != 200000 {
		t.Fatalf("expected context window 200000, got %d", e.ContextWindow)
	}
	if e.MaxOutput != 32000 {
		t.Fatalf("expected max output 32000, got %d", e.MaxOutput)
	}
	var metadata map[string]any
	if err := json.Unmarshal(e.LiveMetadata, &metadata); err != nil || metadata["id"] != "claude-opus-4-6" {
		t.Fatalf("unexpected live metadata: %s (err=%v)", e.LiveMetadata, err)
	}
	if e.Installed {
		t.Fatal("expected installed=false for cache source")
	}
}

func TestProviderSupportsLiveModels(t *testing.T) {
	if !runtime.ProviderSupportsLiveModels("openai") {
		t.Fatal("expected OpenAI to support live model listing")
	}
	if runtime.ProviderSupportsLiveModels("unknown-provider") {
		t.Fatal("unknown provider must not report live model support")
	}
}

func TestListModels_CacheMultipleModels(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	now := time.Now().UTC().Truncate(time.Second)
	c := catalog.Catalog{
		SchemaVersion: catalog.CatalogSchemaVersion,
		GeneratedAt:   now,
		StaleAfter:    now.Add(time.Hour),
		Providers: map[string]catalog.Provider{
			"openai": {ID: "openai", Name: "OpenAI"},
		},
		Protocols: map[string]catalog.Protocol{
			"openai-chat-completions": {ID: "openai-chat-completions", Name: "OpenAI Chat Completions"},
		},
		Deployments: map[string]catalog.Deployment{
			"openai-direct": {
				ID: "openai-direct", Name: "OpenAI", ProviderID: "openai",
				APIProtocolID: "openai-chat-completions", AdapterConstructor: "openai",
				NativeModelIDSource: catalog.NativeModelIDCatalogKnown,
			},
		},
		Models: map[string]catalog.Model{
			"openai/gpt-4o":      {ID: "openai/gpt-4o", ProviderID: "openai", Name: "GPT-4o"},
			"openai/gpt-4o-mini": {ID: "openai/gpt-4o-mini", ProviderID: "openai", Name: "GPT-4o Mini"},
		},
		Offerings: []catalog.ModelOffering{
			{
				ID: "openai-direct:gpt-4o", CanonicalModelID: "openai/gpt-4o",
				DeploymentID: "openai-direct", NativeModelID: "gpt-4o",
				Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
			},
			{
				ID: "openai-direct:gpt-4o-mini", CanonicalModelID: "openai/gpt-4o-mini",
				DeploymentID: "openai-direct", NativeModelID: "gpt-4o-mini",
				Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
			},
		},
	}
	if err := catalog.WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", cachePath)

	entries, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
		ProviderID: "openai",
		Source:     runtime.ListSourceCache,
	})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Verify both models present
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	if !ids["gpt-4o"] || !ids["gpt-4o-mini"] {
		t.Fatalf("expected gpt-4o and gpt-4o-mini, got: %v", ids)
	}
}

func TestListModels_CacheProviderNotInCatalog(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "model_catalog.json")
	now := time.Now().UTC().Truncate(time.Second)
	c := catalog.Catalog{
		SchemaVersion: catalog.CatalogSchemaVersion,
		GeneratedAt:   now,
		StaleAfter:    now.Add(time.Hour),
		Providers: map[string]catalog.Provider{
			"openai": {ID: "openai", Name: "OpenAI"},
		},
		Protocols: map[string]catalog.Protocol{
			"openai-chat-completions": {ID: "openai-chat-completions", Name: "OpenAI Chat Completions"},
		},
		Deployments: map[string]catalog.Deployment{
			"openai-direct": {
				ID: "openai-direct", Name: "OpenAI", ProviderID: "openai",
				APIProtocolID: "openai-chat-completions", AdapterConstructor: "openai",
				NativeModelIDSource: catalog.NativeModelIDCatalogKnown,
			},
		},
		Models: map[string]catalog.Model{
			"openai/gpt-4o": {ID: "openai/gpt-4o", ProviderID: "openai", Name: "GPT-4o"},
		},
		Offerings: []catalog.ModelOffering{{
			ID: "openai-direct:gpt-4o", CanonicalModelID: "openai/gpt-4o",
			DeploymentID: "openai-direct", NativeModelID: "gpt-4o",
			Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
		}},
	}
	if err := catalog.WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", cachePath)

	entries, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
		ProviderID: "nonexistent",
		Source:     runtime.ListSourceCache,
	})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for nonexistent provider, got %d", len(entries))
	}
}

func TestWriteCatalogCache_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model_catalog.json")
	c := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(path, &c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("expected temp file removed after atomic write")
	}
}

func TestWriteCatalogCache_CanBeReadBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model_catalog.json")
	c := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(path, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", path)

	entries, err := runtime.ListModels(context.Background(), runtime.ListModelsOpts{
		ProviderID: "anthropic",
		Source:     runtime.ListSourceCache,
	})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty entries after writing and reading back cache")
	}
}

func TestWriteCatalogCache_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model_catalog.json")

	// Write initial catalog
	c1 := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(path, &c1); err != nil {
		t.Fatal(err)
	}

	// Overwrite with same content (should succeed)
	if err := catalog.WriteCatalogCache(path, &c1); err != nil {
		t.Fatal(err)
	}

	// Verify file still exists and is valid
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
