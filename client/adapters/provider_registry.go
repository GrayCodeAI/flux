package adapters

import (
	"context"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// ProviderType classifies providers.
type ProviderType string

const (
	// ProviderTypeAnthropic uses the Anthropic Messages API.
	ProviderTypeAnthropic ProviderType = "anthropic"
	// ProviderTypeOpenAI uses the OpenAI Chat Completions API.
	ProviderTypeOpenAI ProviderType = "openai"
	// ProviderTypeOpenAICompatible uses OpenAI-compatible APIs with custom base URLs.
	ProviderTypeOpenAICompatible ProviderType = "openai-compatible"
	// ProviderTypeAzure uses Azure OpenAI.
	ProviderTypeAzure ProviderType = "azure"
	// ProviderTypeBedrock uses AWS Bedrock.
	ProviderTypeBedrock ProviderType = "bedrock"
	// ProviderTypeVertex uses Google Vertex AI.
	ProviderTypeVertex ProviderType = "vertex"
)

// ProviderRegistryConfig holds provider registry info.
type ProviderRegistryConfig struct {
	Name              string              `json:"name"`
	Type              ProviderType        `json:"type"`
	BaseURL           string              `json:"base_url,omitempty"`
	EnvKey            string              `json:"env_key"`
	SupportsStreaming bool                `json:"supports_streaming"`
	SupportsTools     bool                `json:"supports_tools"`
	SupportsReasoning bool                `json:"supports_reasoning"`
	Compat            *OpenAICompatConfig `json:"compat,omitempty"`
}

// CoreProviders and OpenAICompatibleProviders are derived from the canonical
// catalog registry. Dynamic providers are added only to the compatible map.
var CoreProviders, OpenAICompatibleProviders = staticProviderMaps()

func staticProviderMaps() (map[string]ProviderRegistryConfig, map[string]ProviderRegistryConfig) {
	coreMap := make(map[string]ProviderRegistryConfig)
	compatible := make(map[string]ProviderRegistryConfig)
	for _, spec := range registry.All() {
		providerType := ProviderTypeOpenAICompatible
		if spec.TransportKind != "" {
			providerType = ProviderType(spec.TransportKind)
		}
		baseURL := spec.RuntimeBaseURL
		if baseURL == "" {
			baseURL = spec.ProbeBaseURL
		}
		envKey := spec.RuntimeCredentialEnv
		if envKey == "" {
			envKey = spec.CredentialEnv
		}
		provider := ProviderRegistryConfig{
			Name: spec.ProviderID, Type: providerType, BaseURL: baseURL, EnvKey: envKey,
			SupportsStreaming: true, SupportsTools: true, SupportsReasoning: !spec.IsLocal,
		}
		if providerType == ProviderTypeOpenAICompatible {
			compatible[spec.ProviderID] = provider
		} else {
			coreMap[spec.ProviderID] = provider
		}
	}
	return coreMap, compatible
}

// DetectProvider detects the active provider from the credential store (not process env).
func DetectProvider() string {
	ctx := context.Background()
	checks := map[string]func() bool{
		"anthropic":  func() bool { return credentials.HasSecret(ctx, "ANTHROPIC_API_KEY") },
		"deepseek":   func() bool { return credentials.HasSecret(ctx, "DEEPSEEK_API_KEY") },
		"openrouter": func() bool { return credentials.HasSecret(ctx, "OPENROUTER_API_KEY") },
		"grok":       func() bool { return credentials.HasSecret(ctx, "XAI_API_KEY") },
		"gemini":     func() bool { return credentials.HasSecret(ctx, "GEMINI_API_KEY") },
		"zai_payg":   func() bool { return credentials.HasSecret(ctx, "ZAI_API_KEY") },
		"zai_coding": func() bool { return credentials.HasSecret(ctx, "ZAI_CODING_API_KEY") },
		"canopywave": func() bool { return credentials.HasSecret(ctx, "CANOPYWAVE_API_KEY") },
		"poolside":   func() bool { return credentials.HasSecret(ctx, "POOLSIDE_API_KEY") },
		"groq":       func() bool { return credentials.HasSecret(ctx, "GROQ_API_KEY") },
		"openai":     func() bool { return credentials.HasSecret(ctx, "OPENAI_API_KEY") },
		"opencodego": func() bool { return credentials.HasSecret(ctx, "OPENCODEGO_API_KEY") },
		"kimi":       func() bool { return credentials.HasSecret(ctx, "MOONSHOT_API_KEY") },
		"xiaomi_mimo_payg": func() bool {
			return credentials.HasSecret(ctx, config.EnvXiaomiPaygAPIKey)
		},
		"xiaomi_mimo_token_plan": func() bool {
			return credentials.HasSecret(ctx, config.EnvXiaomiTokenPlanAPIKey)
		},
		"minimax_token_plan": func() bool {
			return credentials.HasSecret(ctx, "MINIMAX_TOKEN_PLAN_API_KEY")
		},
		"minimax_payg": func() bool {
			return credentials.HasSecret(ctx, "MINIMAX_PAYG_API_KEY")
		},
		"ollama": func() bool { return ResolveEnvSecret("OLLAMA_BASE_URL") != "" },
		"azure": func() bool {
			return credentials.HasSecret(ctx, "AZURE_OPENAI_API_KEY") && ResolveEnvSecret("AZURE_OPENAI_ENDPOINT") != ""
		},
		"bedrock": func() bool {
			return credentials.HasSecret(ctx, "AWS_ACCESS_KEY_ID") && credentials.HasSecret(ctx, "AWS_SECRET_ACCESS_KEY")
		},
		"vertex": func() bool {
			return credentials.HasSecret(ctx, "VERTEX_PROJECT_ID") && credentials.HasSecret(ctx, "VERTEX_ACCESS_TOKEN")
		},
	}
	for _, p := range config.APIProviderDetectionOrder {
		if fn, ok := checks[p]; ok && fn() {
			return p
		}
	}
	return "anthropic"
}

// ResolveProviderModelEnvOverride resolves the model env override for a provider.
func ResolveProviderModelEnvOverride(provider string) string {
	if provider == "" {
		provider = DetectProvider()
	}
	for _, k := range config.ProviderModelEnvKeys[provider] {
		if v := ResolveEnvSecret(k); v != "" {
			return v
		}
	}
	return ""
}

// ResolveEnvSecret looks up an environment variable via the credential store
// with a bounded timeout.
func ResolveEnvSecret(envKey string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return credentials.LookupSecret(ctx, envKey)
}
