package catalog_test

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/live"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestAllProviders_LiveFetchParity(t *testing.T) {
	t.Parallel()
	specs := registry.All()
	if len(specs) != 26 {
		t.Fatalf("expected 26 providers, got %d", len(specs))
	}
	for _, spec := range specs {
		t.Run(spec.ProviderID, func(t *testing.T) {
			if spec.LiveFetcherKey == "" {
				t.Fatal("missing LiveFetcherKey")
			}
			if spec.LiveCatalogKey == "" {
				t.Fatal("missing LiveCatalogKey")
			}
			if _, ok := live.Registry[spec.LiveFetcherKey]; !ok {
				t.Fatalf("live.Registry missing fetcher %q", spec.LiveFetcherKey)
			}
			dep := catalog.DeploymentIDForLiveCatalogKey(spec.LiveCatalogKey)
			if dep != spec.DeploymentID {
				t.Fatalf("DeploymentIDForLiveCatalogKey = %q, want %q", dep, spec.DeploymentID)
			}
		})
	}
}

func TestAllProviders_AllReturnEmptyWithoutCatalog(t *testing.T) {
	t.Parallel()
	empty := &catalog.ModelCatalog{}
	// All providers are fully dynamic — should return empty without catalog
	for _, spec := range registry.All() {
		got := catalog.GetProviderDefaultModel(spec.ProviderID, empty)
		if got != "" {
			t.Errorf("%s: expected empty default without catalog (fully dynamic), got %q", spec.ProviderID, got)
		}
	}
}

func TestAllProviders_FirstModelFromCompiledCache(t *testing.T) {
	t.Parallel()
	base := catalog.SeedCatalog()
	for _, spec := range registry.All() {
		native := "live-" + spec.ProviderID + "-model"
		owner := catalog.CanonicalProviderID(spec.ProviderID)
		canonical := owner + "/" + native
		switch spec.ProviderID {
		case "gemini", "vertex":
			owner = "google"
			canonical = "google/" + native
		case "azure":
			owner = "openai"
			canonical = "openai/" + native
		case "bedrock":
			owner = "anthropic"
			canonical = "anthropic/" + native
		}
		// Ensure provider exists in catalog
		if _, ok := base.Providers[owner]; !ok {
			base.Providers[owner] = catalog.Provider{ID: owner, Name: owner}
		}
		base.Models[canonical] = catalog.Model{
			ID: canonical, ProviderID: owner, Name: native,
		}
	}
	compiled, err := catalog.CompileCatalog(&base)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range registry.All() {
		id := catalog.FirstModelForProvider(compiled, spec.ProviderID)
		if id == "" {
			t.Errorf("%s: FirstModelForProvider returned empty", spec.ProviderID)
		}
	}
}
