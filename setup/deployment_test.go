package setup

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestProviderForDeploymentAnthropicBedrockFromConfig(t *testing.T) {
	t.Parallel()
	p, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{
		Region:          "us-east-1",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "secret",
	})
	if !ok {
		t.Fatal("expected bedrock deployment to be configured")
	}
	if p.Name() != "anthropic-bedrock" {
		t.Fatalf("provider name = %q, want anthropic-bedrock", p.Name())
	}
}

func TestProviderForDeploymentAnthropicBedrockFromStore(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	ctx := context.Background()
	_ = store.Set(ctx, credentials.AccountForEnv("AWS_ACCESS_KEY_ID"), "AKIATEST")
	_ = store.Set(ctx, credentials.AccountForEnv("AWS_SECRET_ACCESS_KEY"), "secret")
	t.Setenv("AWS_REGION", "us-west-2")

	p, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{})
	if !ok {
		t.Fatal("expected bedrock deployment to be configured from credential store")
	}
	if p.Name() != "anthropic-bedrock" {
		t.Fatalf("provider name = %q, want anthropic-bedrock", p.Name())
	}
}

func TestProviderForDeploymentAnthropicBedrockRequiresCredentials(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	if _, ok := ProviderForDeployment("anthropic-bedrock", config.DeploymentConfig{}); ok {
		t.Fatal("expected bedrock deployment to be unavailable without credentials")
	}
}

func TestDeploymentProviderFromStateRejectsAmbientCredentialsAndLegacyDetection(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "ambient-store-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "ambient-env-secret-1234567890")
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []*config.ProviderConfig{
		{},
		{Deployments: map[string]config.DeploymentConfig{"openai-direct": {}}},
	} {
		if provider, err := DeploymentProviderFromState(cfg, compiled); err == nil || provider != nil {
			t.Fatalf("strict provider used ambient state: provider=%T err=%v", provider, err)
		}
	}
}

func TestDeploymentProviderFromStateAcceptsExplicitHydratedDeployment(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{Deployments: map[string]config.DeploymentConfig{
		"openai-direct": {APIKey: "injected-explicit-secret-1234567890", BaseURL: "https://explicit.example.test/v1"},
	}}
	provider, err := DeploymentProviderFromState(cfg, compiled)
	if err != nil || provider == nil {
		t.Fatalf("strict provider rejected explicit state: provider=%T err=%v", provider, err)
	}
}

func TestProviderForDeploymentPoolsideUsesReasoningRecoveryClient(t *testing.T) {
	provider, ok := ProviderForDeployment("poolside", config.DeploymentConfig{
		APIKey: "poolside-test-key-1234567890",
	})
	if !ok {
		t.Fatal("expected Poolside deployment provider")
	}
	if _, ok := provider.(*client.PoolsideClient); !ok {
		t.Fatalf("provider type = %T, want *client.PoolsideClient", provider)
	}
}

// --- UseDeploymentRouting ---

func TestUseDeploymentRouting_EnvOverrideTrue(t *testing.T) {
	for _, val := range []string{"1", "true", "yes", "on", "TRUE", "Yes"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv("EYRIE_DEPLOYMENT_ROUTING", val)
			if !UseDeploymentRouting(nil) {
				t.Fatalf("expected true for env %q", val)
			}
		})
	}
}

func TestUseDeploymentRouting_EnvOverrideFalse(t *testing.T) {
	for _, val := range []string{"0", "false", "no", "off", "FALSE", "No"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv("EYRIE_DEPLOYMENT_ROUTING", val)
			if UseDeploymentRouting(&config.ProviderConfig{ConfigVersion: 2}) {
				t.Fatalf("expected false for env %q", val)
			}
		})
	}
}

func TestUseDeploymentRouting_NilConfig(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	if UseDeploymentRouting(nil) {
		t.Fatal("expected false for nil config")
	}
}

func TestUseDeploymentRouting_ConfigVersion2(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	cfg := &config.ProviderConfig{ConfigVersion: 2}
	if !UseDeploymentRouting(cfg) {
		t.Fatal("expected true for config_version >= 2")
	}
}

func TestUseDeploymentRouting_WithDeployments(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	cfg := &config.ProviderConfig{
		Deployments: map[string]config.DeploymentConfig{
			"anthropic-direct": {APIKey: "key"},
		},
	}
	if !UseDeploymentRouting(cfg) {
		t.Fatal("expected true when deployments present")
	}
}

func TestUseDeploymentRouting_WithRouting(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	cfg := &config.ProviderConfig{
		Routing: &config.RoutingPolicy{},
	}
	if !UseDeploymentRouting(cfg) {
		t.Fatal("expected true when routing present")
	}
}

func TestUseDeploymentRouting_LegacyConfig(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "")
	cfg := &config.ProviderConfig{ConfigVersion: 0}
	if UseDeploymentRouting(cfg) {
		t.Fatal("expected false for legacy config without deployments/routing")
	}
}

func TestDeploymentRoutingFromStateIgnoresAmbientOverride(t *testing.T) {
	t.Setenv("EYRIE_DEPLOYMENT_ROUTING", "true")
	if DeploymentRoutingFromState(nil) {
		t.Fatal("pure deployment routing read process environment")
	}
	if !DeploymentRoutingFromState(&config.ProviderConfig{ConfigVersion: 2}) {
		t.Fatal("pure deployment routing ignored explicit provider state")
	}
}

// --- FirstNonEmpty ---

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"all empty", []string{"", "", ""}, ""},
		{"first wins", []string{"a", "b", "c"}, "a"},
		{"middle wins", []string{"", "b", "c"}, "b"},
		{"last wins", []string{"", "", "c"}, "c"},
		{"trims whitespace", []string{"  ", " b ", "c"}, "b"},
		{"empty slice", []string{}, ""},
		{"single empty", []string{""}, ""},
		{"single value", []string{"only"}, "only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FirstNonEmpty(tt.values...)
			if got != tt.want {
				t.Fatalf("FirstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}

// --- CloneStringMap ---

func TestCloneStringMap_Nil(t *testing.T) {
	t.Parallel()
	if got := CloneStringMap(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCloneStringMap_Empty(t *testing.T) {
	if got := CloneStringMap(map[string]string{}); got != nil {
		t.Fatalf("expected nil for empty map, got %v", got)
	}
}

func TestCloneStringMap_Copy(t *testing.T) {
	in := map[string]string{"a": "1", "b": "2"}
	out := CloneStringMap(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	// Mutate original; copy should be unaffected.
	in["c"] = "3"
	if _, ok := out["c"]; ok {
		t.Fatal("clone should not reflect mutations to original")
	}
}

// --- RouterRoutingPolicy ---

func TestRouterRoutingPolicy_Nil(t *testing.T) {
	p := RouterRoutingPolicy(nil)
	if p.Default != nil {
		t.Fatal("expected nil default for nil policy")
	}
	if p.Providers != nil {
		t.Fatal("expected nil providers for nil policy")
	}
	if p.Models != nil {
		t.Fatal("expected nil models for nil policy")
	}
}

func TestRouterRoutingPolicy_WithStages(t *testing.T) {
	policy := &config.RoutingPolicy{
		Default: []config.RoutingStage{
			{
				Retries: 2,
				Deployments: []config.DeploymentChoice{
					{DeploymentID: "anthropic-direct", Weight: 100},
				},
			},
		},
		Providers: map[string][]config.RoutingStage{
			"anthropic": {
				{
					Retries: 1,
					Deployments: []config.DeploymentChoice{
						{DeploymentID: "anthropic-direct", Weight: 100},
					},
				},
			},
		},
		Models: map[string][]config.RoutingStage{
			"claude": {
				{
					Retries: 3,
					Deployments: []config.DeploymentChoice{
						{DeploymentID: "anthropic-direct", Weight: 50},
						{DeploymentID: "bedrock", Weight: 50},
					},
				},
			},
		},
	}
	got := RouterRoutingPolicy(policy)
	if len(got.Default) != 1 {
		t.Fatalf("expected 1 default stage, got %d", len(got.Default))
	}
	if got.Default[0].Retries != 2 {
		t.Fatalf("default retries = %d, want 2", got.Default[0].Retries)
	}
	if len(got.Default[0].Deployments) != 1 {
		t.Fatalf("default deployments = %d, want 1", len(got.Default[0].Deployments))
	}
	if got.Default[0].Deployments[0].DeploymentID != "anthropic-direct" {
		t.Fatalf("deployment ID = %q, want anthropic-direct", got.Default[0].Deployments[0].DeploymentID)
	}
	if got.Default[0].Deployments[0].Weight != 100 {
		t.Fatalf("weight = %d, want 100", got.Default[0].Deployments[0].Weight)
	}
	if len(got.Providers) != 1 {
		t.Fatalf("providers map len = %d, want 1", len(got.Providers))
	}
	if len(got.Models) != 1 {
		t.Fatalf("models map len = %d, want 1", len(got.Models))
	}
	if got.Models["claude"][0].Retries != 3 {
		t.Fatalf("model retries = %d, want 3", got.Models["claude"][0].Retries)
	}
	if len(got.Models["claude"][0].Deployments) != 2 {
		t.Fatalf("model deployments = %d, want 2", len(got.Models["claude"][0].Deployments))
	}
}

// --- DefaultDeploymentForProvider ---

func TestDefaultDeploymentForProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{config.ProviderAnthropic, "anthropic-direct"},
		{config.ProviderOpenAI, "openai-direct"},
		{config.ProviderGrok, "grok-direct"},
		{config.ProviderGemini, "gemini-direct"},
		{config.ProviderOpenRouter, "openrouter"},
		{config.ProviderConcentrate, "concentrate-direct"},
		{config.ProviderCanopyWave, "canopywave"},
		{config.ProviderDeepSeek, "deepseek-direct"},
		{config.ProviderZAIPayg, "zai_payg-direct"},
		{config.ProviderZAICoding, "zai_coding-direct"},
		{config.ProviderOllama, "ollama-local"},
		{config.ProviderOpenCodeGo, "opencodego"},
		{config.ProviderKimi, "kimi-direct"},
		{config.ProviderXiaomiMimoPayg, "xiaomi_mimo_payg-direct"},
		{"unknown", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := DefaultDeploymentForProvider(tt.provider)
			if got != tt.want {
				t.Fatalf("DefaultDeploymentForProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

// --- LegacyDeploymentConfig ---

func TestLegacyDeploymentConfig_NilConfig(t *testing.T) {
	got := LegacyDeploymentConfig(nil, config.ProviderAnthropic)
	if got.APIKey != "" || got.BaseURL != "" {
		t.Fatalf("expected empty DeploymentConfig for nil config, got %+v", got)
	}
}

func TestLegacyDeploymentConfig_Anthropic(t *testing.T) {
	cfg := &config.ProviderConfig{
		AnthropicAPIKey:  "key123",
		AnthropicBaseURL: "https://custom.api.com",
	}
	got := LegacyDeploymentConfig(cfg, config.ProviderAnthropic)
	if got.APIKey != "key123" {
		t.Fatalf("APIKey = %q, want key123", got.APIKey)
	}
	if got.BaseURL != "https://custom.api.com" {
		t.Fatalf("BaseURL = %q, want https://custom.api.com", got.BaseURL)
	}
}

func TestLegacyDeploymentConfig_OpenAI(t *testing.T) {
	cfg := &config.ProviderConfig{
		OpenAIAPIKey:  "oai-key",
		OpenAIBaseURL: "https://api.openai.com/v1",
	}
	got := LegacyDeploymentConfig(cfg, config.ProviderOpenAI)
	if got.APIKey != "oai-key" {
		t.Fatalf("APIKey = %q, want oai-key", got.APIKey)
	}
}

func TestLegacyDeploymentConfig_Grok(t *testing.T) {
	cfg := &config.ProviderConfig{
		GrokAPIKey: "grok-key",
	}
	got := LegacyDeploymentConfig(cfg, config.ProviderGrok)
	if got.APIKey != "grok-key" {
		t.Fatalf("APIKey = %q, want grok-key", got.APIKey)
	}
}

func TestLegacyDeploymentConfig_GrokXAIFallback(t *testing.T) {
	cfg := &config.ProviderConfig{
		XAIAPIKey: "xai-key",
	}
	got := LegacyDeploymentConfig(cfg, config.ProviderGrok)
	if got.APIKey != "xai-key" {
		t.Fatalf("APIKey = %q, want xai-key (XAI fallback)", got.APIKey)
	}
}

func TestLegacyDeploymentConfig_Ollama(t *testing.T) {
	cfg := &config.ProviderConfig{
		OllamaBaseURL: "http://localhost:11434",
	}
	got := LegacyDeploymentConfig(cfg, config.ProviderOllama)
	if got.BaseURL != "http://localhost:11434" {
		t.Fatalf("BaseURL = %q, want http://localhost:11434", got.BaseURL)
	}
	if got.APIKey != "" {
		t.Fatalf("Ollama should not have APIKey, got %q", got.APIKey)
	}
}

func TestLegacyDeploymentConfig_Unknown(t *testing.T) {
	cfg := &config.ProviderConfig{AnthropicAPIKey: "key"}
	got := LegacyDeploymentConfig(cfg, "nonexistent")
	if got.APIKey != "" {
		t.Fatalf("expected empty for unknown provider, got %+v", got)
	}
}

// --- ProviderForDeployment additional cases ---

func TestProviderForDeployment_UnknownID(t *testing.T) {
	if _, ok := ProviderForDeployment("nonexistent-provider", config.DeploymentConfig{}); ok {
		t.Fatal("expected false for unknown deployment ID")
	}
}

func TestProviderForDeployment_AnthropicDirect(t *testing.T) {
	p, ok := ProviderForDeployment("anthropic-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected anthropic-direct to be configured")
	}
	if p.Name() != "anthropic" {
		t.Fatalf("provider name = %q, want anthropic", p.Name())
	}
}

func TestProviderForDeployment_AnthropicDirectRequiresKey(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("ANTHROPIC_API_KEY", "")

	if _, ok := ProviderForDeployment("anthropic-direct", config.DeploymentConfig{}); ok {
		t.Fatal("expected anthropic-direct to be unavailable without key")
	}
}

func TestProviderForDeployment_OpenAIDirect(t *testing.T) {
	p, ok := ProviderForDeployment("openai-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected openai-direct to be configured")
	}
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_OpenAIDirectRequiresKey(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("OPENAI_API_KEY", "")

	if _, ok := ProviderForDeployment("openai-direct", config.DeploymentConfig{}); ok {
		t.Fatal("expected openai-direct to be unavailable without key")
	}
}

func TestProviderForDeployment_GrokDirect(t *testing.T) {
	p, ok := ProviderForDeployment("grok-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected grok-direct to be configured")
	}
	// Grok uses OpenAIClient which reports "openai" as its name.
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_GeminiDirect(t *testing.T) {
	p, ok := ProviderForDeployment("gemini-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected gemini-direct to be configured")
	}
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_OpenRouter(t *testing.T) {
	p, ok := ProviderForDeployment("openrouter", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected openrouter to be configured")
	}
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_ConcentrateDirect(t *testing.T) {
	p, ok := ProviderForDeployment("concentrate-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected concentrate-direct to be configured")
	}
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_ConcentrateDirectRequiresKey(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("CONCENTRATE_API_KEY", "")

	if _, ok := ProviderForDeployment("concentrate-direct", config.DeploymentConfig{}); ok {
		t.Fatal("expected concentrate-direct to be unavailable without key")
	}
}

func TestProviderForDeployment_CanopyWave(t *testing.T) {
	p, ok := ProviderForDeployment("canopywave", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected canopywave to be configured")
	}
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_DeepSeekDirect(t *testing.T) {
	p, ok := ProviderForDeployment("deepseek-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected deepseek-direct to be configured")
	}
	if p.Name() != "deepseek" {
		t.Fatalf("provider name = %q, want deepseek", p.Name())
	}
}

func TestProviderForDeployment_ZAIDirect(t *testing.T) {
	p, ok := ProviderForDeployment("zai_payg-direct", config.DeploymentConfig{APIKey: "test-key"})
	if !ok {
		t.Fatal("expected zai_payg-direct to be configured")
	}
	if p.Name() != "zai_payg" {
		t.Fatalf("provider name = %q, want zai_payg", p.Name())
	}
}

func TestProviderForDeployment_OllamaLocal(t *testing.T) {
	p, ok := ProviderForDeployment("ollama-local", config.DeploymentConfig{})
	if !ok {
		t.Fatal("expected ollama-local to always be configured (no key needed)")
	}
	// Ollama uses OpenAIClient which reports "openai" as its name.
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_AnthropicVertex(t *testing.T) {
	p, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{
		ProjectID: "my-project",
		Region:    "us-central1",
		Token:     "token123",
	})
	if !ok {
		t.Fatal("expected anthropic-vertex to be configured")
	}
	if p.Name() != "anthropic-vertex" {
		t.Fatalf("provider name = %q, want anthropic-vertex", p.Name())
	}
}

func TestProviderForDeployment_AnthropicVertexRequiresAllFields(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("VERTEX_PROJECT_ID", "")
	t.Setenv("VERTEX_REGION", "")
	t.Setenv("VERTEX_ACCESS_TOKEN", "")
	t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "")

	// Missing project ID.
	if _, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{Region: "us-central1", Token: "tok"}); ok {
		t.Fatal("expected false when project ID missing")
	}
	// Missing region.
	if _, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{ProjectID: "proj", Token: "tok"}); ok {
		t.Fatal("expected false when region missing")
	}
	// Missing token.
	if _, ok := ProviderForDeployment("anthropic-vertex", config.DeploymentConfig{ProjectID: "proj", Region: "us-central1"}); ok {
		t.Fatal("expected false when token missing")
	}
}

func TestProviderForDeployment_OpenAIAzure(t *testing.T) {
	p, ok := ProviderForDeployment("openai-azure", config.DeploymentConfig{
		APIKey:     "azure-key",
		Endpoint:   "https://myendpoint.openai.azure.com",
		APIVersion: "2024-02-15-preview",
	})
	if !ok {
		t.Fatal("expected openai-azure to be configured")
	}
	if p.Name() != "azure" {
		t.Fatalf("provider name = %q, want azure", p.Name())
	}
}

func TestProviderForDeployment_OpenAIAzureRequiresKeyAndEndpoint(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	t.Setenv("AZURE_OPENAI_API_KEY", "")
	t.Setenv("AZURE_OPENAI_ENDPOINT", "")

	if _, ok := ProviderForDeployment("openai-azure", config.DeploymentConfig{}); ok {
		t.Fatal("expected false without key and endpoint")
	}
}

func TestProviderForDeployment_KimiDirect(t *testing.T) {
	p, ok := ProviderForDeployment("kimi-direct", config.DeploymentConfig{APIKey: "moonshot-key"})
	if !ok {
		t.Fatal("expected kimi-direct to be configured")
	}
	// Kimi uses OpenAIClient which reports "openai" as its name.
	if p.Name() != "openai" {
		t.Fatalf("provider name = %q, want openai", p.Name())
	}
}

func TestProviderForDeployment_XiaomiDirect(t *testing.T) {
	p, ok := ProviderForDeployment("xiaomi_mimo_payg-direct", config.DeploymentConfig{APIKey: "xiaomi-key"})
	if !ok {
		t.Fatal("expected xiaomi_mimo_payg-direct to be configured")
	}
	if p.Name() != config.ProviderXiaomiMimoPayg {
		t.Fatalf("provider name = %q, want %s", p.Name(), config.ProviderXiaomiMimoPayg)
	}
}

func TestProviderForDeployment_XiaomiTokenPlanDirect(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	cfg := &config.ProviderConfig{
		Version:                    "1",
		XiaomiMimoTokenPlanRegion:  "sgp",
		XiaomiMimoTokenPlanBaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
	}
	if err := config.SaveProviderConfig(cfg, ""); err != nil {
		t.Fatal(err)
	}
	p, ok := ProviderForDeployment("xiaomi_mimo_token_plan-direct", config.DeploymentConfig{
		APIKey:  "tp-test-key",
		BaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
	})
	if !ok {
		t.Fatal("expected token plan direct to be configured")
	}
	if p.Name() != config.ProviderXiaomiMimoTokenPlan {
		t.Fatalf("provider name = %q", p.Name())
	}
}

func TestProviderForDeployment_OpenCodeGo(t *testing.T) {
	p, ok := ProviderForDeployment("opencodego", config.DeploymentConfig{APIKey: "ocg-key"})
	if !ok {
		t.Fatal("expected opencodego to be configured")
	}
	if p.Name() != "opencodego" {
		t.Fatalf("provider name = %q, want opencodego", p.Name())
	}
	if _, ok := p.(*client.OpenCodeGoClient); !ok {
		t.Fatalf("provider type = %T, want *client.OpenCodeGoClient", p)
	}
}
