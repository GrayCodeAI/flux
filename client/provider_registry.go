package client

import (
	"fmt"
	"log/slog"

	"github.com/GrayCodeAI/graycode-router/client/adapters"
	"github.com/GrayCodeAI/graycode-router/config"
)

// ProviderType classifies providers.
type ProviderType = adapters.ProviderType

// ProviderRegistryConfig holds provider registry info.
type ProviderRegistryConfig = adapters.ProviderRegistryConfig

// GetProviders lists all available providers.
func (c *GraycodeRouterClient) GetProviders() []string {
	var providers []string
	for k := range adapters.CoreProviders {
		providers = append(providers, k)
	}
	adapters.DynamicMu.RLock()
	for k := range adapters.OpenAICompatibleProviders {
		providers = append(providers, k)
	}
	adapters.DynamicMu.RUnlock()
	return providers
}

// GetProviderInfo returns config for a provider.
func (c *GraycodeRouterClient) GetProviderInfo(provider string) *ProviderRegistryConfig {
	if p, ok := adapters.CoreProviders[provider]; ok {
		return &p
	}
	adapters.DynamicMu.RLock()
	p, ok := adapters.OpenAICompatibleProviders[provider]
	adapters.DynamicMu.RUnlock()
	if ok {
		return &p
	}
	return nil
}

func (c *GraycodeRouterClient) getOrCreateProvider(providerName string) (Provider, error) {
	c.mu.RLock()
	if p, ok := c.providers[providerName]; ok {
		c.mu.RUnlock()
		return p, nil
	}
	hasKey := c.apiKeys[providerName] != ""
	needsRegistration := !hasKey && c.GetProviderInfo(providerName) == nil
	c.mu.RUnlock()

	if needsRegistration && adapters.DynamicProviderEnabled() {
		if fallbackURL := adapters.OpenAIBaseFallbackURL(); fallbackURL != "" {
			slog.Warn(
				"auto-registering OpenAI-compatible provider from OPENAI_API_BASE",
				"provider", providerName,
				"base_url", fallbackURL,
				"opt_in_env", adapters.DynamicProviderEnvVar,
			)
			_ = adapters.RegisterDynamicProvider(providerName, fallbackURL, "OPENAI_API_KEY")
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if p, ok := c.providers[providerName]; ok {
		return p, nil
	}

	apiKey := c.apiKeys[providerName]
	if apiKey == "" {
		info := c.GetProviderInfo(providerName)
		if info == nil {
			return nil, fmt.Errorf("graycode-router: unknown provider: %s", providerName)
		}
		apiKey = adapters.ResolveEnvSecret(info.EnvKey)
	}

	info := c.GetProviderInfo(providerName)
	if info == nil {
		return nil, fmt.Errorf("graycode-router: unknown provider: %s", providerName)
	}
	baseURL := c.baseURLs[providerName]
	if baseURL == "" {
		baseURL = info.BaseURL
	}

	if apiKey == "" && providerName != "ollama" {
		return nil, fmt.Errorf("graycode-router: no API key for %s; set %s or call SetAPIKey()", providerName, info.EnvKey)
	}

	var p Provider
	switch info.Type {
	case adapters.ProviderTypeAnthropic:
		p = adapters.NewAnthropicClient(apiKey, baseURL)
	case adapters.ProviderTypeAzure:
		endpoint := adapters.ResolveEnvSecret("AZURE_OPENAI_ENDPOINT")
		if endpoint == "" {
			endpoint = baseURL
		}
		apiVersion := adapters.ResolveEnvSecret("AZURE_OPENAI_API_VERSION")
		p = adapters.NewAzureClient(apiKey, endpoint, apiVersion)
	case adapters.ProviderTypeBedrock:
		region := adapters.ResolveEnvSecret("AWS_REGION")
		if region == "" {
			region = adapters.ResolveEnvSecret("AWS_DEFAULT_REGION")
		}
		if region == "" {
			region = "us-east-1"
		}
		accessKey := adapters.ResolveEnvSecret("AWS_ACCESS_KEY_ID")
		sessionToken := adapters.ResolveEnvSecret("AWS_SESSION_TOKEN")
		p = adapters.NewBedrockClient(accessKey, apiKey, sessionToken, region)
	case adapters.ProviderTypeVertex:
		projectID := adapters.ResolveEnvSecret("VERTEX_PROJECT_ID")
		if projectID == "" {
			return nil, fmt.Errorf("graycode-router: vertex requires VERTEX_PROJECT_ID")
		}
		region := adapters.ResolveEnvSecret("VERTEX_REGION")
		if region == "" {
			region = "us-central1"
		}
		p = adapters.NewVertexClient(projectID, region, apiKey)
	default:
		if config.IsZAIProvider(providerName) {
			providerCfg := config.LoadProviderConfig("")
			openAIBase, err := config.ResolveZAIOpenAIBase(providerName, providerCfg)
			if err != nil {
				return nil, err
			}
			anthropicBase := config.ResolveZAIAnthropicBase(providerCfg)
			p = adapters.NewZAIClient(apiKey, openAIBase, anthropicBase, info.Compat, providerName)
			break
		}
		if config.IsXiaomiMimoProvider(providerName) {
			providerCfg := config.LoadProviderConfig("")
			openAIBase, err := config.ResolveXiaomiOpenAIBase(providerName, providerCfg)
			if err != nil {
				return nil, err
			}
			p = adapters.NewMiMoClient(apiKey, openAIBase, info.Compat, providerName)
			break
		}
		if providerName == "opencodego" {
			p = adapters.NewOpenCodeGoClient(apiKey, baseURL)
			break
		}
		if providerName == "poolside" {
			p = adapters.NewPoolsideClient(apiKey, baseURL)
			break
		}
		p = adapters.NewOpenAIClient(apiKey, baseURL, info.Compat)
	}

	c.providers[providerName] = p
	return p, nil
}

// DetectProvider detects the active provider from the credential store.
func DetectProvider() string { return adapters.DetectProvider() }

// ResolveProviderModelEnvOverride resolves the model env override for a provider.
func ResolveProviderModelEnvOverride(provider string) string {
	return adapters.ResolveProviderModelEnvOverride(provider)
}
