package registry_test

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestAllProviders_Count(t *testing.T) {
	t.Parallel()
	if n := len(registry.All()); n != 27 {
		t.Fatalf("expected 27 providers, got %d", n)
	}
}

func TestProviderSpecs_AgnesOpenAIOnlyLongCatOpenAIPrimary(t *testing.T) {
	t.Parallel()
	// Agnes: official docs advertise OpenAI-compatible only — no Anthropic surface.
	agnes, ok := registry.SpecByProviderID("agnes")
	if !ok {
		t.Fatal("missing agnes")
	}
	if agnes.ProtocolID != "openai-chat-completions" || agnes.AdapterID != "openai" {
		t.Fatalf("agnes protocol/adapter = %q/%q", agnes.ProtocolID, agnes.AdapterID)
	}
	if got := registry.DirectFallbackProviderIDs("agnes"); len(got) != 0 {
		t.Fatalf("agnes DirectFallbacks = %v, want none", got)
	}

	// LongCat: official docs expose BOTH OpenAI (/openai) and Anthropic (/anthropic).
	// Hawk uses the OpenAI primary only — Anthropic is not required when OpenAI works.
	longcat, ok := registry.SpecByProviderID("longcat")
	if !ok {
		t.Fatal("missing longcat")
	}
	if longcat.ProtocolID != "openai-chat-completions" || longcat.AdapterID != "openai" {
		t.Fatalf("longcat protocol/adapter = %q/%q", longcat.ProtocolID, longcat.AdapterID)
	}
}

func TestLiveFetcherKeys_AllProviders(t *testing.T) {
	t.Parallel()
	keys := registry.LiveFetcherKeys()
	if len(keys) != 27 {
		t.Fatalf("expected 27 live fetcher keys, got %d", len(keys))
	}
}

func TestOpenCodeGo_HasProbeBaseURL(t *testing.T) {
	t.Parallel()
	spec, ok := registry.SpecByProviderID("opencodego")
	if !ok {
		t.Fatal("missing opencodego spec")
	}
	if spec.ProbeBaseURL == "" {
		t.Fatal("opencodego must have a ProbeBaseURL")
	}
	if spec.ProbeBaseURL != opencodego.DefaultBaseURL {
		t.Fatalf("opencodego probe base URL = %q", spec.ProbeBaseURL)
	}
	if spec.ProbeKind != registry.ProbeOpenAIModels {
		t.Fatalf("opencodego probe kind = %q", spec.ProbeKind)
	}
}

func TestConcentrateUsesResponsesAPI(t *testing.T) {
	t.Parallel()
	spec, ok := registry.SpecByProviderID("concentrate")
	if !ok {
		t.Fatal("missing Concentrate provider spec")
	}
	if spec.ProtocolID != "openai-responses" {
		t.Fatalf("protocol = %q", spec.ProtocolID)
	}
	if spec.AdapterID != "concentrate-responses" {
		t.Fatalf("adapter = %q", spec.AdapterID)
	}
	if !spec.PublicModelCatalog {
		t.Fatal("Concentrate model catalog must be public")
	}
}

func TestOpenGatewaySpec(t *testing.T) {
	t.Parallel()
	spec, ok := registry.SpecByProviderID("opengateway")
	if !ok {
		t.Fatal("missing OpenGateway provider spec")
	}
	if spec.ProtocolID != "openai-chat-completions" {
		t.Fatalf("protocol = %q, want openai-chat-completions", spec.ProtocolID)
	}
	if spec.AdapterID != "openai" {
		t.Fatalf("adapter = %q, want openai", spec.AdapterID)
	}
	if !spec.PublicModelCatalog {
		t.Fatal("OpenGateway model catalog must be public")
	}
	if !spec.RequiresKey {
		t.Fatal("OpenGateway should require a key for inference (OPENGATEWAY_API_KEY)")
	}
	if spec.LiveFetcherKey != "opengateway" {
		t.Fatalf("fetcher = %q", spec.LiveFetcherKey)
	}
}

func TestProviderRuntimePolicy_Metadata(t *testing.T) {
	t.Parallel()

	order := registry.ChatProviderPreferenceOrder()
	if len(order) < 3 {
		t.Fatalf("runtime preference order too short: %v", order)
	}
	if order[0] != "openai" || order[1] != "anthropic" || order[2] != "openrouter" {
		t.Fatalf("unexpected runtime preference prefix: %v", order[:3])
	}

	if got := registry.DirectFallbackProviderIDs("openai"); len(got) != 1 || got[0] != "anthropic" {
		t.Fatalf("openai direct fallbacks = %v, want [anthropic]", got)
	}
	if got := registry.DirectFallbackProviderIDs("anthropic"); len(got) != 1 || got[0] != "openai" {
		t.Fatalf("anthropic direct fallbacks = %v, want [openai]", got)
	}

	if got := registry.CredentialAliases("anthropic"); len(got) != 1 || got[0] != "CLAUDE_API_KEY" {
		t.Fatalf("anthropic credential aliases = %v", got)
	}

	prepared := registry.CredentialEnvPreparedProviders()
	wantPrepared := map[string]bool{
		"xiaomi_mimo_token_plan": true,
		"zai_coding":             true,
		"zai_payg":               true,
	}
	if len(prepared) != len(wantPrepared) {
		t.Fatalf("prepared providers = %v", prepared)
	}
	for _, providerID := range prepared {
		if !wantPrepared[providerID] {
			t.Fatalf("unexpected prepared provider %q in %v", providerID, prepared)
		}
	}
}

func TestProviderSpecs_TableDriven(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		providerID       string
		wantKey          bool
		wantProbeKind    registry.ProbeKind
		wantLiveFetcher  bool
		wantDeploymentID string
	}{
		{"anthropic", "anthropic", true, registry.ProbeAnthropic, true, "anthropic-direct"},
		{"openai", "openai", true, registry.ProbeOpenAIModels, true, "openai-direct"},
		{"azure", "azure", true, registry.ProbeNone, true, "openai-azure"},
		{"gemini", "gemini", true, registry.ProbeGemini, true, "gemini-direct"},
		{"bedrock", "bedrock", true, registry.ProbeNone, true, "anthropic-bedrock"},
		{"vertex", "vertex", true, registry.ProbeNone, true, "gemini-vertex"},
		{"openrouter", "openrouter", true, registry.ProbeOpenAIModels, true, "openrouter"},
		{"concentrate", "concentrate", true, registry.ProbeOpenAIModels, true, "concentrate-payg"},
		{"grok", "grok", true, registry.ProbeOpenAIModels, true, "grok-direct"},
		{"zai_payg", "zai_payg", true, registry.ProbeOpenAIModels, true, "zai_payg-direct"},
		{"zai_coding", "zai_coding", true, registry.ProbeOpenAIModels, true, "zai_coding-direct"},
		{"canopywave", "canopywave", true, registry.ProbeOpenAIModels, true, "canopywave"},
		{"opencodego", "opencodego", true, registry.ProbeOpenAIModels, true, "opencodego"},
		{"kimi", "kimi", true, registry.ProbeOpenAIModels, true, "kimi-direct"},
		{"xiaomi_mimo_payg", "xiaomi_mimo_payg", true, registry.ProbeOpenAIModels, true, "xiaomi_mimo_payg-direct"},
		{"xiaomi_mimo_token_plan", "xiaomi_mimo_token_plan", true, registry.ProbeOpenAIModels, true, "xiaomi_mimo_token_plan-direct"},
		{"ollama", "ollama", false, registry.ProbeOllama, true, "ollama-local"},
		{"deepseek", "deepseek", true, registry.ProbeOpenAIModels, true, "deepseek-direct"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := registry.SpecByProviderID(tt.providerID)
			if !ok {
				t.Fatalf("provider %q not found", tt.providerID)
			}
			if spec.RequiresKey != tt.wantKey {
				t.Errorf("RequiresKey = %v, want %v", spec.RequiresKey, tt.wantKey)
			}
			if spec.ProbeKind != tt.wantProbeKind {
				t.Errorf("ProbeKind = %q, want %q", spec.ProbeKind, tt.wantProbeKind)
			}
			if (spec.LiveFetcherKey != "") != tt.wantLiveFetcher {
				t.Errorf("LiveFetcherKey presence = %v, want %v", spec.LiveFetcherKey != "", tt.wantLiveFetcher)
			}
			if spec.DeploymentID != tt.wantDeploymentID {
				t.Errorf("DeploymentID = %q, want %q", spec.DeploymentID, tt.wantDeploymentID)
			}
			if spec.CredentialEnv == "" && spec.RequiresKey {
				t.Error("RequiresKey=true but CredentialEnv is empty")
			}
			if spec.ProbeBaseURL == "" && spec.ProbeKind != registry.ProbeAnthropic && spec.ProbeKind != registry.ProbeGemini && spec.ProbeKind != registry.ProbeOllama && spec.ProbeKind != registry.ProbeNone && spec.ProviderID != "xiaomi_mimo_token_plan" {
				t.Error("ProbeBaseURL is empty for probe kind that requires it")
			}
		})
	}
}
