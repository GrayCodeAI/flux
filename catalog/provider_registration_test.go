package catalog

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestSpecByProviderID_RegisteredProviders(t *testing.T) {
	t.Parallel()
	providers := []string{"anthropic", "openai", "gemini", "grok", "openrouter", "zai_payg", "zai_coding", "canopywave", "ollama", "opencodego", "kimi", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan"}
	for _, id := range providers {
		spec, ok := SpecByProviderID(id)
		if !ok {
			t.Errorf("SpecByProviderID(%q) not found", id)
			continue
		}
		if spec.ProviderID != id {
			t.Errorf("SpecByProviderID(%q).ProviderID = %q", id, spec.ProviderID)
		}
		if spec.DisplayName == "" {
			t.Errorf("SpecByProviderID(%q).DisplayName is empty", id)
		}
		if spec.DeploymentID == "" {
			t.Errorf("SpecByProviderID(%q).DeploymentID is empty", id)
		}
	}
}

func TestSpecByProviderID_CatalogAliases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		alias, wantProviderID string
	}{
		{"google", "gemini"},
		{"xai", "grok"},
	}
	for _, tt := range tests {
		spec, ok := SpecByProviderID(tt.alias)
		if !ok {
			t.Errorf("SpecByProviderID(%q) not found", tt.alias)
			continue
		}
		if spec.ProviderID != tt.wantProviderID {
			t.Errorf("SpecByProviderID(%q).ProviderID = %q, want %q", tt.alias, spec.ProviderID, tt.wantProviderID)
		}
	}
}

func TestSpecByProviderID_Unknown(t *testing.T) {
	t.Parallel()
	_, ok := SpecByProviderID("nonexistent_provider_xyz")
	if ok {
		t.Error("expected not found for unknown provider")
	}
}

func TestSpecByEnvVar_FindsProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		envVar         string
		wantProviderID string
	}{
		{"ANTHROPIC_API_KEY", "anthropic"},
		{"OPENAI_API_KEY", "openai"},
		{"GEMINI_API_KEY", "gemini"},
		{"OPENROUTER_API_KEY", "openrouter"},
		{"XAI_API_KEY", "grok"},
		{"OLLAMA_BASE_URL", "ollama"},
	}
	for _, tt := range tests {
		spec, ok := SpecByEnvVar(tt.envVar)
		if !ok {
			t.Errorf("SpecByEnvVar(%q) not found", tt.envVar)
			continue
		}
		if spec.ProviderID != tt.wantProviderID {
			t.Errorf("SpecByEnvVar(%q).ProviderID = %q, want %q", tt.envVar, spec.ProviderID, tt.wantProviderID)
		}
	}
}

func TestSpecByEnvVar_Unknown(t *testing.T) {
	t.Parallel()
	_, ok := SpecByEnvVar("NONEXISTENT_ENV_VAR")
	if ok {
		t.Error("expected not found for unknown env var")
	}
}

func TestProviderDisplayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id, want string
	}{
		{"anthropic", "Anthropic"},
		{"openai", "OpenAI"},
		{"gemini", "Gemini API"},
		{"nonexistent", "nonexistent"},
	}
	for _, tt := range tests {
		got := ProviderDisplayName(tt.id)
		if got != tt.want {
			t.Errorf("ProviderDisplayName(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestEnsureCredentialRegistryInCatalog_AddsMissingProviders(t *testing.T) {
	t.Parallel()
	c := &Catalog{
		Providers:   map[string]Provider{},
		Deployments: map[string]Deployment{},
		Protocols:   map[string]Protocol{},
	}
	EnsureCredentialRegistryInCatalog(c)
	for _, spec := range registry.DefaultRegistry.All() {
		pid := CanonicalProviderID(spec.ProviderID)
		if c.Providers[pid].ID == "" {
			t.Errorf("missing provider %q after EnsureCredentialRegistryInCatalog", pid)
		}
		if c.Deployments[spec.DeploymentID].ID == "" {
			t.Errorf("missing deployment %q after EnsureCredentialRegistryInCatalog", spec.DeploymentID)
		}
	}
}

func TestEnsureCredentialRegistryInCatalog_PreservesExisting(t *testing.T) {
	t.Parallel()
	c := &Catalog{
		Providers: map[string]Provider{
			"anthropic": {ID: "anthropic", Name: "Custom Anthropic"},
		},
		Deployments: map[string]Deployment{
			"anthropic-direct": {ID: "anthropic-direct", Name: "Custom Anthropic Direct"},
		},
		Protocols: map[string]Protocol{},
	}
	EnsureCredentialRegistryInCatalog(c)
	if c.Providers["anthropic"].Name != "Custom Anthropic" {
		t.Errorf("existing provider name overwritten: %q", c.Providers["anthropic"].Name)
	}
	if c.Deployments["anthropic-direct"].Name != "Custom Anthropic Direct" {
		t.Errorf("existing deployment name overwritten: %q", c.Deployments["anthropic-direct"].Name)
	}
}

func TestEnsureCredentialRegistryInCatalog_NilCatalog(t *testing.T) {
	t.Parallel()
	EnsureCredentialRegistryInCatalog(nil) // should not panic
}

func TestProviderIDsFromCompiled_LegacyCatalog(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	ids := ProviderIDsFromCompiled(compiled)
	if len(ids) == 0 {
		t.Fatal("expected provider IDs")
	}
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["anthropic"] {
		t.Error("expected anthropic in provider IDs")
	}
	if !found["openai"] {
		t.Error("expected openai in provider IDs")
	}
}

func TestProviderIDsFromCompiled_ReturnsNilForNil(t *testing.T) {
	t.Parallel()
	ids := ProviderIDsFromCompiled(nil)
	if ids != nil {
		t.Fatalf("expected nil, got %v", ids)
	}
}

func TestPrimaryAPIKeyEnvForProvider_LegacyCatalog(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	tests := []struct {
		provider, wantEnv string
	}{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
	}
	for _, tt := range tests {
		got := PrimaryAPIKeyEnvForProvider(compiled, tt.provider)
		if got != tt.wantEnv {
			t.Errorf("PrimaryAPIKeyEnvForProvider(%q) = %q, want %q", tt.provider, got, tt.wantEnv)
		}
	}
}

func TestPrimaryAPIKeyEnvForProvider_ReturnsEmptyForNilCompiled(t *testing.T) {
	t.Parallel()
	got := PrimaryAPIKeyEnvForProvider(nil, "anthropic")
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCredentialStatusForProvider_LegacyCatalog(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	compiled, _ := CompileCatalog(&c)
	if got := CredentialStatusForProvider(compiled, "anthropic"); got != "required" {
		t.Errorf("anthropic status = %q, want required", got)
	}
	// ollama has api_key env fallbacks (OLLAMA_API_KEY, OPENAI_API_KEY) so status is "required"
	if got := CredentialStatusForProvider(compiled, "ollama"); got != "required" {
		t.Errorf("ollama status = %q, want required", got)
	}
	if got := CredentialStatusForProvider(compiled, ""); got != "empty" {
		t.Errorf("empty status = %q, want empty", got)
	}
}
