package catalog_test

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
)

func TestFirstModelForProvider(t *testing.T) {
	t.Parallel()
	c := catalog.SeedCatalog()
	c.Models["zai_payg/glm-5.1"] = catalog.Model{ID: "zai_payg/glm-5.1", ProviderID: "zai_payg", Name: "GLM-5.1"}
	compiled, err := catalog.CompileCatalog(&c)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.FirstModelForProvider(compiled, "zai_payg"); got != "zai_payg/glm-5.1" {
		t.Fatalf("FirstModelForProvider = %q", got)
	}
}

func TestGetProviderDefaultModel_AllProvidersEmptyWithoutCatalog(t *testing.T) {
	t.Parallel()
	// All providers return empty without a catalog (fully dynamic)
	allProviders := []string{
		"anthropic", "openai", "gemini", "grok", "bedrock", "kimi",
	}
	for _, p := range allProviders {
		if got := catalog.GetProviderDefaultModel(p, &catalog.ModelCatalog{}); got != "" {
			t.Fatalf("%s: expected empty default without catalog, got %q", p, got)
		}
	}
}
