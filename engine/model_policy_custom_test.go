package engine

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/llm"
)

func TestModelPolicyIncludesInvocationScopedCustomGateways(t *testing.T) {
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: dir,
		CustomGateways: []CustomGateway{
			{ID: "custom-secondary", DisplayName: "Secondary", BaseURL: "https://secondary.example.test/v1", DefaultModel: "custom/secondary", SortOrder: 2, ChatPreference: 20},
			{ID: "custom-primary", DisplayName: "Primary", BaseURL: "https://primary.example.test/v1", DefaultModel: "custom/primary", ContextWindow: 128_000, SortOrder: 1, ChatPreference: 10, Capabilities: &CustomGatewayCapabilities{Streaming: true, Tools: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if got := eng.DefaultModel(ctx, "custom-primary", "fallback"); got != "custom/primary" {
		t.Fatalf("DefaultModel(custom) = %q", got)
	}
	if got := eng.PreferredModel(ctx, "custom-primary", llm.ModelClassPremium, "fallback"); got != "custom/primary" {
		t.Fatalf("PreferredModel(custom) = %q", got)
	}
	preferred := eng.PreferredModels(ctx, "custom-primary", llm.ModelClassBalanced, 2)
	if !slices.Equal(preferred, []string{"custom/primary", "custom/secondary"}) {
		t.Fatalf("PreferredModels(custom) = %v", preferred)
	}
	if got := eng.ProviderForModel(ctx, "custom/primary"); got != "custom_primary" {
		t.Fatalf("ProviderForModel(custom) = %q", got)
	}
	if got := eng.GatewayForModel(ctx, "custom/primary"); got != "custom_primary" {
		t.Fatalf("GatewayForModel(custom) = %q", got)
	}
	model, ok, err := eng.ModelInfo(ctx, "custom/primary")
	if err != nil || !ok {
		t.Fatalf("ModelInfo(custom) = %+v, %v, %v", model, ok, err)
	}
	if model.ID != "custom/primary" || model.CanonicalID != model.ID || model.ProviderID != "custom_primary" || model.GatewayID != "custom_primary" || model.Owner != "Primary" || model.ContextWindow != 128_000 || !slices.Contains(model.Capabilities, "tools") {
		t.Fatalf("custom model metadata = %+v", model)
	}
	listed, err := eng.ListModels(ctx, "custom-primary", false)
	if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0], model) {
		t.Fatalf("ListModels(custom) = %+v, err=%v; ModelInfo = %+v", listed, err, model)
	}
	if names := eng.ModelNames(ctx); !slices.Contains(names, "custom/primary") || !slices.Contains(names, "custom/secondary") {
		t.Fatalf("custom model names missing: %v", names)
	}
	providers, err := eng.ModelProviders(ctx)
	if err != nil || !slices.Contains(providers, "custom_primary") || !slices.Contains(providers, "custom_secondary") || !slices.IsSorted(providers) {
		t.Fatalf("custom model providers = %v, err=%v", providers, err)
	}
}

func TestCustomModelOwnershipUsesDeterministicInstancePreference(t *testing.T) {
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{
			{ID: "later", BaseURL: "https://later.example.test/v1", DefaultModel: "shared/model", SortOrder: 1, ChatPreference: 20},
			{ID: "preferred", BaseURL: "https://preferred.example.test/v1", DefaultModel: "shared/model", SortOrder: 99, ChatPreference: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if provider := eng.ProviderForModel(ctx, "shared/model"); provider != "preferred" {
			t.Fatalf("iteration %d provider = %q", i, provider)
		}
		if gateway := eng.GatewayForModel(ctx, "shared/model"); gateway != "preferred" {
			t.Fatalf("iteration %d gateway = %q", i, gateway)
		}
	}
}

func TestCustomModelInfoAndNamesDoNotRequireCatalog(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: dir,
		CustomGateways: []CustomGateway{{ID: "isolated", BaseURL: "https://isolated.example.test/v1", DefaultModel: "isolated/model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	eng.catalogPath = filepath.Join(dir, "missing", "catalog.json")
	ctx := context.Background()
	if model, ok, err := eng.ModelInfo(ctx, "isolated/model"); err != nil || !ok || model.GatewayID != "isolated" {
		t.Fatalf("ModelInfo(custom without catalog) = %+v, %v, %v", model, ok, err)
	}
	if names := eng.ModelNames(ctx); !slices.Contains(names, "isolated/model") {
		t.Fatalf("ModelNames(custom without catalog) = %v", names)
	}
	providers, err := eng.ModelProviders(ctx)
	if err != nil || !slices.Contains(providers, "isolated") {
		t.Fatalf("ModelProviders(custom without catalog) = %v, %v", providers, err)
	}
}
