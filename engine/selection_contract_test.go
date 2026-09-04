package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestSetSelectionPreservesCustomModelIDThatMatchesCatalogAlias(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	seed.Aliases["shared-native-id"] = "openai/gpt-4o"
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: dir,
		CustomGateways: []CustomGateway{{ID: "private", BaseURL: "https://private.example.test/v1", DefaultModel: "shared-native-id"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "private", "shared-native-id"); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadProviderConfigWithError(eng.providerConfigPath)
	if err != nil || saved == nil || saved.ActiveModel != "shared-native-id" || saved.ActiveProvider != "private" {
		t.Fatalf("custom model ID was globally canonicalized: %#v err=%v", saved, err)
	}
}

func TestEffectiveSelectionClearsModelWhenProviderOverrideChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	for envKey, secret := range map[string]string{
		"ANTHROPIC_API_KEY": "anthropic-live-secret-1234567890",
		"OPENAI_API_KEY":    "openai-live-secret-1234567890",
	} {
		if err := store.Set(ctx, credentials.AccountForEnv(envKey), secret); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := New(Options{SecretStore: store, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "anthropic", catalog.ClaudeOpusV4_6); err != nil {
		t.Fatal(err)
	}
	selection := eng.EffectiveSelection(ctx, SelectionOptions{ProviderOverride: "openai"})
	if selection.Provider != "openai" || selection.Model == "" || selection.Model == "anthropic/"+catalog.ClaudeOpusV4_6 {
		t.Fatalf("provider override retained stale model: %+v", selection)
	}
}

func TestEffectiveSelectionPreservesUnconfiguredProviderOverride(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	if err := store.Set(ctx, credentials.AccountForEnv("CLINE_API_KEY"), "cline-live-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	selection := eng.EffectiveSelection(ctx, SelectionOptions{
		ProviderOverride: "anthropic",
		ModelOverride:    catalog.ClaudeOpusV4_6,
	})
	if selection.Provider != "anthropic" {
		t.Fatalf("explicit provider override was replaced by a configured provider: %+v", selection)
	}
	if !selection.HasConfiguredDeployment {
		t.Fatal("configured deployment state was lost while preserving the explicit override")
	}
}

func TestCustomProviderOverrideDoesNotReuseOtherCustomModel(t *testing.T) {
	ctx := context.Background()
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{
			{ID: "first", BaseURL: "https://first.example.test/v1", DefaultModel: "first/model"},
			{ID: "second", BaseURL: "https://second.example.test/v1", DefaultModel: "second/model"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "first", "first/model"); err != nil {
		t.Fatal(err)
	}
	route, err := eng.Resolve(ctx, SelectionRequest{Preference: Preference{PreferredProvider: "second"}})
	if err != nil {
		t.Fatal(err)
	}
	if route.Provider != "second" || route.Model != "second/model" {
		t.Fatalf("custom override reused persisted model: %+v", route)
	}
}

func TestSetSelectionRejectsModelOwnedByDifferentProvider(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	modelID := "anthropic/" + catalog.ClaudeOpusV4_6
	if err := eng.SetSelection(ctx, "openai", modelID); !IsCode(err, ErrorModelUnavailable) {
		t.Fatalf("cross-provider selection error = %v, want model unavailable", err)
	}
	if saved, err := config.LoadProviderConfigWithError(eng.providerConfigPath); err != nil || saved != nil {
		t.Fatalf("rejected selection changed provider state: %#v err=%v", saved, err)
	}
}

func TestSetSelectionInfersInvocationScopedCustomGateway(t *testing.T) {
	ctx := context.Background()
	eng, err := New(Options{
		SecretStore: &credentials.MapStore{}, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{{ID: "private", BaseURL: "https://private.example.test/v1", DefaultModel: "private/model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "", "private/model"); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadProviderConfigWithError(eng.providerConfigPath)
	if err != nil || saved == nil || saved.ActiveProvider != "private" || saved.ActiveModel != "private/model" {
		t.Fatalf("custom gateway ownership was not inferred: %#v err=%v", saved, err)
	}
}
