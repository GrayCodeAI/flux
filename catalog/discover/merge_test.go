package discover_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/discover"
)

func TestMergeCatalogWithPolicy_ReplacesDeploymentOfferings(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Offerings = append(dst.Offerings, catalog.ModelOffering{
		ID: "canopywave:old-model", CanonicalModelID: "z-ai/old", DeploymentID: "canopywave",
		NativeModelID: "old-model", Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
	})
	src := catalog.SeedCatalog()
	src.Providers["canopywave"] = catalog.Provider{ID: "canopywave", Name: "CanopyWave"}
	src.Deployments["canopywave"] = catalog.Deployment{ID: "canopywave"}
	src.Models["moonshotai/kimi-k2.6"] = catalog.Model{
		ID: "moonshotai/kimi-k2.6", ProviderID: "canopywave", Name: "Kimi K2.6",
	}
	src.Offerings = append(src.Offerings, catalog.ModelOffering{
		ID: "canopywave:moonshotai/kimi-k2.6", CanonicalModelID: "moonshotai/kimi-k2.6",
		DeploymentID: "canopywave", NativeModelID: "moonshotai/kimi-k2.6",
		Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
	})
	out := discover.MergeCatalogWithPolicy(&dst, &src, discover.MergePolicy{
		PreferLive:                 true,
		ReplaceDeploymentOfferings: []string{"canopywave"},
	})
	for _, o := range out.Offerings {
		if o.DeploymentID == "canopywave" && o.NativeModelID == "old-model" {
			t.Fatal("stale canopywave offering should be removed")
		}
	}
	found := false
	for _, o := range out.Offerings {
		if o.DeploymentID == "canopywave" && o.NativeModelID == "moonshotai/kimi-k2.6" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected live canopywave offering after replace merge")
	}
}

func TestMergeCatalogWithPolicy_PreferLiveReplacesExistingModel(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["anthropic/claude-sonnet-4-6"] = catalog.Model{
		ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic", Name: "Claude Sonnet",
		ContextWindow: 100000, MaxOutput: 4096, Aliases: []string{"claude-sonnet"},
	}
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"anthropic/claude-sonnet-4-6": {
				ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic", Name: "Claude Sonnet 4.6",
				ContextWindow: 200000, MaxOutput: 8192, Aliases: []string{"sonnet-4-6"},
				Provenance: &catalog.Provenance{Source: "live", ObservedAt: time.Now().UTC()},
			},
		},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{
		PreferLiveProviders: []string{"anthropic"},
	})
	got := out.Models["anthropic/claude-sonnet-4-6"]
	if got.ContextWindow != 200000 || got.MaxOutput != 8192 {
		t.Fatalf("context/max = %d/%d", got.ContextWindow, got.MaxOutput)
	}
	if got.Name != "Claude Sonnet 4.6" {
		t.Fatalf("name = %q", got.Name)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "sonnet-4-6" {
		t.Fatalf("aliases = %#v (expected live replacement)", got.Aliases)
	}
}

func TestMergeCatalogWithPolicy_PreferLiveUpdatesExistingOffering(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["anthropic/claude-sonnet-4-6"] = catalog.Model{
		ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic", Name: "Claude Sonnet",
	}
	dst.Offerings = []catalog.ModelOffering{{
		ID:               "anthropic-direct:claude-sonnet-4-6",
		CanonicalModelID: "anthropic/claude-sonnet-4-6",
		DeploymentID:     "anthropic-direct",
		NativeModelID:    "claude-sonnet-4-6",
		Pricing:          catalog.Pricing{Status: catalog.PricingUnknown},
	}}
	liveMeta, _ := json.Marshal(map[string]any{"mode": "live"})
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"anthropic/claude-sonnet-4-6": {
				ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic", Name: "Claude Sonnet 4.6",
			},
		},
		Offerings: []catalog.ModelOffering{{
			ID:               "anthropic-direct:claude-sonnet-4-6",
			CanonicalModelID: "anthropic/claude-sonnet-4-6",
			DeploymentID:     "anthropic-direct",
			NativeModelID:    "claude-sonnet-4-6",
			Capabilities: catalog.CapabilitySet{
				FunctionCalling:        "supported",
				ExplicitThinkingBudget: "supported",
			},
			Pricing: catalog.Pricing{
				Status:     catalog.PricingKnown,
				Currency:   "USD",
				RatesPer1M: map[string]float64{"input_tokens": 3, "output_tokens": 15},
				Source:     "live",
			},
			LiveMetadata: liveMeta,
			Provenance:   &catalog.Provenance{Source: "live", ObservedAt: time.Now().UTC()},
		}},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{
		PreferLiveProviders: []string{"anthropic"},
	})
	if len(out.Offerings) != 1 {
		t.Fatalf("offerings = %d", len(out.Offerings))
	}
	got := out.Offerings[0]
	if got.Pricing.Status != catalog.PricingKnown {
		t.Fatalf("pricing status = %q", got.Pricing.Status)
	}
	if got.Capabilities.FunctionCalling != "supported" {
		t.Fatalf("function calling = %q", got.Capabilities.FunctionCalling)
	}
	if string(got.LiveMetadata) == "" {
		t.Fatal("expected live metadata")
	}
}

func TestMergeCatalogWithPolicy_PreferLiveFullReplace(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["openrouter/model-a"] = catalog.Model{
		ID: "openrouter/model-a", ProviderID: "openrouter", Name: "Model A (old)",
		ContextWindow: 100000, MaxOutput: 4096,
	}
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"openrouter/model-a": {
				ID: "openrouter/model-a", ProviderID: "openrouter", Name: "Model A (live)",
				ContextWindow: 0, MaxOutput: 0,
			},
		},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{
		PreferLiveProviders: []string{"openrouter"},
	})
	got := out.Models["openrouter/model-a"]
	if got.Name != "Model A (live)" {
		t.Fatalf("name = %q (expected live replacement)", got.Name)
	}
	if got.ContextWindow != 0 {
		t.Fatalf("context = %d (expected 0 from live)", got.ContextWindow)
	}
	if got.MaxOutput != 0 {
		t.Fatalf("max_output = %d (expected 0 from live)", got.MaxOutput)
	}
}

func TestMergeCatalogWithPolicy_PreferLiveUnconditionalPricing(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["openrouter/model-a"] = catalog.Model{
		ID: "openrouter/model-a", ProviderID: "openrouter",
	}
	dst.Offerings = []catalog.ModelOffering{{
		ID: "openrouter:model-a", CanonicalModelID: "openrouter/model-a",
		DeploymentID: "openrouter", NativeModelID: "model-a",
		Pricing: catalog.Pricing{
			Status: catalog.PricingKnown, Currency: "USD",
			RatesPer1M: map[string]float64{"input_tokens": 5, "output_tokens": 15},
		},
	}}
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"openrouter/model-a": {ID: "openrouter/model-a", ProviderID: "openrouter"},
		},
		Offerings: []catalog.ModelOffering{{
			ID: "openrouter:model-a", CanonicalModelID: "openrouter/model-a",
			DeploymentID: "openrouter", NativeModelID: "model-a",
			Pricing: catalog.Pricing{
				Status: catalog.PricingKnown, Currency: "EUR",
				RatesPer1M: map[string]float64{"input_tokens": 3, "output_tokens": 10},
			},
		}},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{
		PreferLiveProviders: []string{"openrouter"},
	})
	if len(out.Offerings) != 1 {
		t.Fatalf("offerings = %d", len(out.Offerings))
	}
	got := out.Offerings[0]
	if got.Pricing.Currency != "EUR" {
		t.Fatalf("pricing currency = %q (expected live EUR)", got.Pricing.Currency)
	}
	if got.Pricing.RatesPer1M["input_tokens"] != 3 {
		t.Fatalf("input price = %f (expected live 3)", got.Pricing.RatesPer1M["input_tokens"])
	}
}

func TestMergeCatalogWithPolicy_PreferLiveZeroContextOverwrites(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["anthropic/claude-sonnet-4-6"] = catalog.Model{
		ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic",
		ContextWindow: 200000, MaxOutput: 8192,
	}
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"anthropic/claude-sonnet-4-6": {
				ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic",
				ContextWindow: 0, MaxOutput: 0,
			},
		},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{
		PreferLiveProviders: []string{"anthropic"},
	})
	got := out.Models["anthropic/claude-sonnet-4-6"]
	if got.ContextWindow != 0 {
		t.Fatalf("context = %d (expected 0 from prefer-live)", got.ContextWindow)
	}
	if got.MaxOutput != 0 {
		t.Fatalf("max_output = %d (expected 0 from prefer-live)", got.MaxOutput)
	}
}

func TestMergeCatalogWithPolicy_NonPreferLivePreservesExisting(t *testing.T) {
	t.Parallel()
	dst := catalog.SeedCatalog()
	dst.Models["anthropic/claude-sonnet-4-6"] = catalog.Model{
		ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic",
		ContextWindow: 200000, MaxOutput: 8192, Name: "Old",
	}
	src := &catalog.Catalog{
		Models: map[string]catalog.Model{
			"anthropic/claude-sonnet-4-6": {
				ID: "anthropic/claude-sonnet-4-6", ProviderID: "anthropic",
				ContextWindow: 0, MaxOutput: 0, Name: "New",
			},
		},
	}
	out := discover.MergeCatalogWithPolicy(&dst, src, discover.MergePolicy{})
	got := out.Models["anthropic/claude-sonnet-4-6"]
	if got.ContextWindow != 200000 {
		t.Fatalf("context = %d (expected preserved without live policy)", got.ContextWindow)
	}
	if got.MaxOutput != 8192 {
		t.Fatalf("max_output = %d (expected preserved)", got.MaxOutput)
	}
}
