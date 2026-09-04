package adapters

import (
	"context"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
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
	for _, p := range config.APIProviderDetectionOrder {
		if providerCredentialsPresent(ctx, p) {
			return p
		}
	}
	return "anthropic"
}

// providerCredentialsPresent derives ordinary API-key checks from the
// authoritative runtime profile and keeps only providers with multi-field
// credentials explicit. This prevents new catalog providers from being
// silently omitted from automatic detection.
func providerCredentialsPresent(ctx context.Context, provider string) bool {
	if provider == config.ProviderOllama {
		return ResolveEnvSecret("OLLAMA_BASE_URL") != ""
	}
	profile, ok := config.RuntimeProfileByKey(provider)
	if !ok {
		return false
	}
	if provider == config.ProviderAzure {
		return credentials.HasSecret(ctx, "AZURE_OPENAI_API_KEY") && ResolveEnvSecret("AZURE_OPENAI_ENDPOINT") != ""
	}
	if provider == config.ProviderBedrock {
		return credentials.HasSecret(ctx, "AWS_ACCESS_KEY_ID") && credentials.HasSecret(ctx, "AWS_SECRET_ACCESS_KEY")
	}
	if provider == config.ProviderVertex {
		return credentials.HasSecret(ctx, "VERTEX_PROJECT_ID") && credentials.HasSecret(ctx, "VERTEX_ACCESS_TOKEN")
	}
	for _, env := range profile.DetectionEnv {
		if credentials.HasSecret(ctx, env) {
			return true
		}
	}
	return false
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
