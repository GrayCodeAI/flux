package catalog

import (
	"strings"
	"testing"
)

func TestSeedCatalog_ReturnsCatalog(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	if c.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schema_version = %q", c.SchemaVersion)
	}
	if c.Provenance == nil || c.Provenance.Source != "test" {
		t.Fatalf("provenance source = %v, want test", c.Provenance)
	}
}

func TestSeedCatalog_HasProviders(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	expected := []string{"anthropic", "openai", "google", "xai", "openrouter", "concentrate", "ollama", "opencodego", "canopywave"}
	for _, id := range expected {
		if c.Providers[id].ID == "" {
			t.Errorf("missing provider %q in default catalog", id)
		}
	}
}

func TestSeedCatalog_HasDeployments(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	expected := []string{
		"anthropic-direct", "openai-direct",
		"grok-direct", "openrouter", "concentrate-payg", "ollama-local",
		"opencodego", "canopywave",
	}
	for _, id := range expected {
		if c.Deployments[id].ID == "" {
			t.Errorf("missing deployment %q in default catalog", id)
		}
	}
}

func TestSeedCatalog_ConcentrateIsPayAsYouGo(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()

	if got := c.Providers["concentrate"].Name; got != "Concentrate AI (Pay-as-you-go)" {
		t.Fatalf("Concentrate provider name = %q", got)
	}
	if got := c.Deployments["concentrate-payg"].Name; got != "Concentrate AI (Pay-as-you-go)" {
		t.Fatalf("Concentrate deployment name = %q", got)
	}
	if got := c.Deployments["concentrate-payg"].APIProtocolID; got != "openai-responses" {
		t.Fatalf("Concentrate protocol = %q", got)
	}
	if got := c.Deployments["concentrate-payg"].AdapterConstructor; got != "concentrate-responses" {
		t.Fatalf("Concentrate adapter = %q", got)
	}
}

func TestSeedCatalog_HasModels(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	if len(c.Models) == 0 {
		t.Fatal("seed catalog should have models")
	}
}

func TestSeedCatalog_DeploymentsReferenceProviders(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	for id, dep := range c.Deployments {
		if c.Providers[dep.ProviderID].ID == "" {
			t.Errorf("deployment %q references unknown provider %q", id, dep.ProviderID)
		}
		if c.Protocols[dep.APIProtocolID].ID == "" {
			t.Errorf("deployment %q references unknown protocol %q", id, dep.APIProtocolID)
		}
	}
}

func TestSeedCatalog_Validates(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	if err := ValidateCatalog(&c); err != nil {
		t.Fatalf("seed catalog should validate: %v", err)
	}
}

func TestSeedCatalog_Compiles(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	compiled, err := CompileCatalog(&c)
	if err != nil {
		t.Fatalf("seed catalog should compile: %v", err)
	}
	if compiled == nil {
		t.Fatal("compiled should not be nil")
	}
}

func TestSeedCatalog_NoDuplicateOfferings(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	seen := map[string]bool{}
	for _, o := range c.Offerings {
		if seen[o.ID] {
			t.Fatalf("duplicate offering %q", o.ID)
		}
		seen[o.ID] = true
	}
}

func TestBootstrapCatalog_HasEnvFallbacks(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	for id, dep := range c.Deployments {
		if len(dep.EnvFallbacks) > 0 {
			continue
		}
		if dep.AdapterConstructor == "" {
			continue
		}
		if dep.ModelMappingsRequired {
			continue
		}
		if dep.NativeModelIDSource == NativeModelIDUserConfigured {
			continue
		}
		if strings.HasSuffix(id, "-direct") || id == "openrouter" || id == "canopywave" || id == "opencodego" || id == "ollama-local" {
			if len(dep.EnvFallbacks) == 0 {
				t.Errorf("deployment %q should have env fallbacks", id)
			}
		}
	}
}

func TestIsBootstrapCatalog(t *testing.T) {
	t.Parallel()
	// SeedCatalog is NOT bootstrap (it's a full catalog)
	c := SeedCatalog()
	if IsBootstrapCatalog(&c) {
		t.Error("SeedCatalog should not be bootstrap")
	}
	if IsBootstrapCatalog(nil) {
		t.Error("nil should not be bootstrap")
	}
}
