package setup

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
)

func TestBuildSetupUI_NilCatalog(t *testing.T) {
	ui := BuildSetupUI(nil, "")
	if ui == nil {
		t.Fatal("expected non-nil SetupUI for nil catalog")
	}
	if len(ui.Providers) != 0 {
		t.Fatalf("expected 0 providers, got %d", len(ui.Providers))
	}
}

func TestBuildSetupUI_EmptyCatalog(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 0 {
		t.Fatalf("expected 0 providers for empty catalog, got %d", len(ui.Providers))
	}
}

func TestBuildSetupUI_WithModels(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"anthropic/claude-sonnet-4": {
				ID:         "anthropic/claude-sonnet-4",
				ProviderID: "anthropic",
				Name:       "Claude Sonnet 4",
			},
			"anthropic/claude-haiku-3.5": {
				ID:         "anthropic/claude-haiku-3.5",
				ProviderID: "anthropic",
				Name:       "Claude Haiku 3.5",
			},
			"openai/gpt-4o": {
				ID:         "openai/gpt-4o",
				ProviderID: "openai",
				Name:       "GPT-4o",
			},
		},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(ui.Providers))
	}
	// Providers should be sorted.
	if ui.Providers[0].ID != "anthropic" {
		t.Fatalf("first provider = %q, want anthropic", ui.Providers[0].ID)
	}
	if ui.Providers[1].ID != "openai" {
		t.Fatalf("second provider = %q, want openai", ui.Providers[1].ID)
	}
	// Anthropic provider should have 2 models sorted.
	if len(ui.Providers[0].Models) != 2 {
		t.Fatalf("expected 2 anthropic models, got %d", len(ui.Providers[0].Models))
	}
	if ui.Providers[0].Models[0].CanonicalID != "anthropic/claude-haiku-3.5" {
		t.Fatalf("first model = %q, want anthropic/claude-haiku-3.5", ui.Providers[0].Models[0].CanonicalID)
	}
	if ui.Providers[0].Models[0].DisplayName != "Claude Haiku 3.5" {
		t.Fatalf("first model display = %q, want Claude Haiku 3.5", ui.Providers[0].Models[0].DisplayName)
	}
}

func TestBuildSetupUI_ProviderFilter(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"anthropic/claude-sonnet-4": {
				ID:         "anthropic/claude-sonnet-4",
				ProviderID: "anthropic",
				Name:       "Claude Sonnet 4",
			},
			"openai/gpt-4o": {
				ID:         "openai/gpt-4o",
				ProviderID: "openai",
				Name:       "GPT-4o",
			},
		},
	}
	ui := BuildSetupUI(compiled, "anthropic")
	if len(ui.Providers) != 1 {
		t.Fatalf("expected 1 provider with filter, got %d", len(ui.Providers))
	}
	if ui.Providers[0].ID != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", ui.Providers[0].ID)
	}
}

func TestBuildSetupUI_DisplayNameFallback(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"anthropic/my-model": {
				ID:         "anthropic/my-model",
				ProviderID: "anthropic",
				Name:       "", // empty name triggers fallback
			},
		},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 1 {
		t.Fatal("expected 1 provider")
	}
	if len(ui.Providers[0].Models) != 1 {
		t.Fatal("expected 1 model")
	}
	// Fallback: strip prefix, use last segment.
	if ui.Providers[0].Models[0].DisplayName != "my-model" {
		t.Fatalf("display name = %q, want my-model", ui.Providers[0].Models[0].DisplayName)
	}
}

func TestBuildSetupUI_DisplayNameFallbackNoSlash(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"anthropic/claude": {
				ID:         "anthropic/claude",
				ProviderID: "anthropic",
				Name:       "   ", // whitespace-only triggers fallback
			},
		},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 1 {
		t.Fatal("expected 1 provider")
	}
	if ui.Providers[0].Models[0].DisplayName != "claude" {
		t.Fatalf("display name = %q, want claude", ui.Providers[0].Models[0].DisplayName)
	}
}

func TestBuildSetupUI_ModelProvenanceSource(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"anthropic/claude": {
				ID:         "anthropic/claude",
				ProviderID: "anthropic",
				Name:       "Claude",
				Provenance: &catalog.Provenance{Source: "remote"},
			},
			"openai/gpt": {
				ID:         "openai/gpt",
				ProviderID: "openai",
				Name:       "GPT",
				Provenance: &catalog.Provenance{Source: "live"},
			},
			"gemini/pro": {
				ID:         "gemini/pro",
				ProviderID: "gemini",
				Name:       "Pro",
				Provenance: nil,
			},
		},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(ui.Providers))
	}
	// Find each provider and check source.
	sources := map[string]string{}
	for _, p := range ui.Providers {
		for _, m := range p.Models {
			sources[m.CanonicalID] = m.Source
		}
	}
	if sources["anthropic/claude"] != "remote" {
		t.Fatalf("anthropic source = %q, want remote", sources["anthropic/claude"])
	}
	if sources["openai/gpt"] != "live" {
		t.Fatalf("openai source = %q, want live", sources["openai/gpt"])
	}
	if sources["gemini/pro"] != "" {
		t.Fatalf("gemini source = %q, want empty", sources["gemini/pro"])
	}
}

func TestProviderIDForDeployment_NilCatalog(t *testing.T) {
	got := ProviderIDForDeployment(nil, "anthropic-direct")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestProviderIDForDeployment_NilDeployments(t *testing.T) {
	compiled := &catalog.CompiledCatalog{}
	got := ProviderIDForDeployment(compiled, "anthropic-direct")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestProviderIDForDeployment_Found(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		DeploymentsByID: map[string]catalog.Deployment{
			"anthropic-direct": {ProviderID: "anthropic"},
		},
	}
	got := ProviderIDForDeployment(compiled, "anthropic-direct")
	if got != "anthropic" {
		t.Fatalf("got %q, want anthropic", got)
	}
}

func TestProviderIDForDeployment_NotFound(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		DeploymentsByID: map[string]catalog.Deployment{
			"anthropic-direct": {ProviderID: "anthropic"},
		},
	}
	got := ProviderIDForDeployment(compiled, "openai-direct")
	if got != "" {
		t.Fatalf("expected empty for missing deployment, got %q", got)
	}
}

func TestBuildSetupUI_SkipsModelsWithEmptyProviderID(t *testing.T) {
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"no-provider/model": {
				ID:         "no-provider/model",
				ProviderID: "",
				Name:       "No Provider",
			},
		},
	}
	ui := BuildSetupUI(compiled, "")
	if len(ui.Providers) != 0 {
		t.Fatalf("expected 0 providers for empty provider_id, got %d", len(ui.Providers))
	}
}

func TestBuildSetupUI_ProviderWithNoModelsExcluded(t *testing.T) {
	// All models belong to "openai"; filter for "anthropic" should yield nothing.
	compiled := &catalog.CompiledCatalog{
		ModelsByID: map[string]catalog.Model{
			"openai/gpt-4o": {
				ID:         "openai/gpt-4o",
				ProviderID: "openai",
				Name:       "GPT-4o",
			},
		},
	}
	ui := BuildSetupUI(compiled, "anthropic")
	if len(ui.Providers) != 0 {
		t.Fatalf("expected 0 providers when filter has no models, got %d", len(ui.Providers))
	}
}
