package catalog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestFetchLiveModelEntriesForProvider_ConcentratePublicCatalogNeedsNoKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"id":"anthropic/claude-test","display_name":"Claude Test","owned_by":"anthropic","max_input_tokens":200000,"max_tokens":8192}]}`)
	}))
	t.Cleanup(server.Close)

	entries, err := catalog.FetchLiveModelEntriesForProvider(map[string]string{
		"CONCENTRATE_BASE_URL": server.URL,
	}, "concentrate")
	if err != nil {
		t.Fatalf("FetchLiveModelEntriesForProvider: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if got := entries[0].ID; got != "anthropic/claude-test" {
		t.Fatalf("id = %q", got)
	}
}

func TestFetchLiveProviderCatalog_SkipsProvidersWithoutCredentials(t *testing.T) {
	t.Parallel()
	cat, enrichment := catalog.FetchLiveProviderCatalog(map[string]string{})
	if len(cat.Models) == 0 {
		t.Fatal("expected seed models in catalog")
	}
	if len(enrichment) != len(registry.All()) {
		t.Fatalf("expected enrichment for all %d providers, got %d", len(registry.All()), len(enrichment))
	}
	for _, item := range enrichment {
		if !strings.HasPrefix(item.Error, "skipped") {
			t.Fatalf("expected skipped enrichment, got %+v", item)
		}
	}
}

func TestFetchLiveProviderCatalog_AttemptsAllRegisteredFetchers(t *testing.T) {
	t.Parallel()
	env := map[string]string{}
	for _, spec := range registry.All() {
		if !spec.RequiresKey {
			continue
		}
		env[spec.CredentialEnv] = "test-key-should-fail-network-12345678"
	}
	_, enrichment := catalog.FetchLiveProviderCatalog(env)
	if len(enrichment) != len(registry.All()) {
		t.Fatalf("expected enrichment for all %d providers, got %d", len(registry.All()), len(enrichment))
	}
	attempted := 0
	for _, item := range enrichment {
		if strings.HasPrefix(item.Error, "skipped") {
			continue
		}
		attempted++
	}
	expectedAttempts := 0
	for _, spec := range registry.All() {
		if spec.RequiresKey {
			expectedAttempts++
		}
	}
	if attempted != expectedAttempts {
		t.Fatalf("expected %d live fetch attempts, got %d", expectedAttempts, attempted)
	}
	seen := map[string]bool{}
	for _, item := range enrichment {
		seen[item.Provider] = true
	}
	for _, spec := range registry.All() {
		if !spec.RequiresKey {
			continue
		}
		if !seen[spec.LiveCatalogKey] {
			t.Errorf("missing live fetch attempt for %s (catalog key %q)", spec.ProviderID, spec.LiveCatalogKey)
		}
	}
}
