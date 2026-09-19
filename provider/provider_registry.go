package provider

import (
	"fmt"
	"sort"

	"github.com/GrayCodeAI/flux/config"
	"github.com/GrayCodeAI/flux/provider/adapters"
	"github.com/GrayCodeAI/flux/provider/core"
)

// GetProviders lists all available providers.
func (c *FluxClient) GetProviders() []string {
	seen := make(map[string]struct{}, len(adapters.CoreProviders)+len(adapters.OpenAICompatibleProviders))
	for k := range adapters.CoreProviders {
		seen[k] = struct{}{}
	}
	for k := range adapters.OpenAICompatibleProviders {
		seen[k] = struct{}{}
	}
	if c != nil {
		c.mu.RLock()
		for k := range c.customProviders {
			seen[k] = struct{}{}
		}
		if _, ok := seen[c.defaultProvider]; !ok && c.baseURLs[c.defaultProvider] != "" {
			seen[c.defaultProvider] = struct{}{}
		}
		c.mu.RUnlock()
	}
	providers := make([]string, 0, len(seen))
	for k := range seen {
		providers = append(providers, k)
	}
	sort.Strings(providers)
	return providers
}

// GetProviderInfo returns config for a provider.
func (c *FluxClient) GetProviderInfo(provider string) *adapters.ProviderRegistryConfig {
	if c != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
	}
	return c.providerInfoLocked(provider)
}

func (c *FluxClient) providerInfoLocked(provider string) *adapters.ProviderRegistryConfig {
	if p, ok := adapters.CoreProviders[provider]; ok {
		return copyRegistryConfig(p)
	}
	if p, ok := adapters.OpenAICompatibleProviders[provider]; ok {
		return copyRegistryConfig(p)
	}
	if c != nil {
		if p, ok := c.customProviders[provider]; ok {
			return copyRegistryConfig(p)
		}
		if baseURL := c.baseURLs[provider]; baseURL != "" && validCustomProviderURL(baseURL) {
			return newCustomProviderInfo(provider, baseURL, "")
		}
	}
	return nil
}

func copyRegistryConfig(p adapters.ProviderRegistryConfig) *adapters.ProviderRegistryConfig {
	if p.Compat != nil {
		compat := *p.Compat
		p.Compat = &compat
	}
	return &p
}

func (c *FluxClient) getOrCreateProvider(providerName string) (core.Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.providers[providerName]; ok {
		return p, nil
	}
	if baseURL := c.baseURLs[providerName]; baseURL != "" && !validCustomProviderURL(baseURL) {
		return nil, fmt.Errorf("flux: invalid base URL for %s", providerName)
	}
	info := c.providerInfoLocked(providerName)
	if info == nil {
		return nil, fmt.Errorf("flux: unknown provider: %s", providerName)
	}

	apiKey := c.apiKeys[providerName]
	if apiKey == "" {
		if info.EnvKey != "" {
			apiKey = adapters.ResolveEnvSecret(info.EnvKey)
		}
	}
	baseURL := c.baseURLs[providerName]
	if baseURL == "" {
		baseURL = info.BaseURL
	}

	_, registeredCustom := c.customProviders[providerName]
	_, builtInCore := adapters.CoreProviders[providerName]
	_, builtInCompat := adapters.OpenAICompatibleProviders[providerName]
	custom := registeredCustom || !builtInCore && !builtInCompat
	if apiKey == "" && providerName != "ollama" && !(custom && info.EnvKey == "") {
		return nil, fmt.Errorf("flux: no API key for %s; set %s or call SetAPIKey()", providerName, info.EnvKey)
	}

	var p core.Provider
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
			return nil, fmt.Errorf("flux: vertex requires VERTEX_PROJECT_ID")
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
		if providerName == "longcat" {
			// LongCat speaks both the OpenAI and Anthropic wire protocols; the
			// dedicated client preserves both paths, matching the setup path.
			// Without this, the generic OpenAI client below would silently drop
			// the Anthropic route for Anthropic-configured longcat deployments.
			p = adapters.NewLongCatClient(apiKey, baseURL, config.DefaultLongCatAnthropicBaseURL, info.Compat)
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
