package provider

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/GrayCodeAI/flux/provider/adapters"
)

// RegisterCustomProvider adds an OpenAI-compatible endpoint to this client
// only. Re-registration replaces the endpoint and invalidates its cached
// transport. Other clients are unaffected.
func (c *FluxClient) RegisterCustomProvider(name, baseURL, envKey string) error {
	if c == nil {
		return fmt.Errorf("flux: client is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("flux: custom provider name is required without whitespace")
	}
	if _, exists := adapters.CoreProviders[name]; exists {
		return fmt.Errorf("flux: custom provider %q collides with built-in provider", name)
	}
	if _, exists := adapters.OpenAICompatibleProviders[name]; exists {
		return fmt.Errorf("flux: custom provider %q collides with built-in provider", name)
	}
	if !validCustomProviderURL(baseURL) {
		return fmt.Errorf("flux: invalid custom provider base URL")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.customProviders == nil {
		c.customProviders = make(map[string]adapters.ProviderRegistryConfig)
	}
	if c.baseURLs == nil {
		c.baseURLs = make(map[string]string)
	}
	c.customProviders[name] = *newCustomProviderInfo(name, baseURL, strings.TrimSpace(envKey))
	c.baseURLs[name] = baseURL
	delete(c.providers, name)
	return nil
}

func newCustomProviderInfo(name, baseURL, envKey string) *adapters.ProviderRegistryConfig {
	return &adapters.ProviderRegistryConfig{
		Name: name, Type: adapters.ProviderTypeOpenAICompatible,
		BaseURL: baseURL, EnvKey: envKey,
		SupportsStreaming: true, SupportsTools: true,
		Compat: &adapters.OpenAICompatConfig{MaxTokensField: "max_tokens"},
	}
}

func validCustomProviderURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
