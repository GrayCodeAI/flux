package adapters

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// DynamicMu protects the OpenAICompatibleProviders map from concurrent access.
var DynamicMu sync.RWMutex

// registryFrozen prevents new provider registrations after first use.
var registryFrozen atomic.Bool

// DynamicProviderEnvVar is the opt-in env var that allows eyrie to
// auto-register an OpenAI-compatible provider from OPENAI_API_BASE /
// OPENAI_BASE_URL when an unknown provider name is requested.
const DynamicProviderEnvVar = "EYRIE_ALLOW_DYNAMIC_PROVIDERS"

// DynamicProviderEnabled reports whether callers may auto-register an
// OpenAI-compatible provider from OPENAI_API_BASE / OPENAI_BASE_URL when
// an unknown provider name is requested.
func DynamicProviderEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(DynamicProviderEnvVar)))
	return v == "1" || v == "true" || v == "yes"
}

// FreezeRegistry prevents further provider registrations.
func FreezeRegistry() {
	registryFrozen.Store(true)
}

// RegisterDynamicProvider adds a user-defined OpenAI-compatible provider at runtime.
func RegisterDynamicProvider(name, baseURL, envKey string) error {
	if registryFrozen.Load() {
		return fmt.Errorf("eyrie: provider registry is frozen; register providers before first use")
	}
	if baseURL == "" {
		return fmt.Errorf("eyrie: RegisterDynamicProvider: baseURL must not be empty")
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("eyrie: RegisterDynamicProvider: invalid baseURL %q (must be http/https with host)", baseURL)
	}
	DynamicMu.Lock()
	defer DynamicMu.Unlock()

	OpenAICompatibleProviders[name] = ProviderRegistryConfig{
		Name:              name,
		Type:              ProviderTypeOpenAICompatible,
		BaseURL:           baseURL,
		EnvKey:            envKey,
		SupportsStreaming: true,
		SupportsTools:     true,
		SupportsReasoning: false,
		Compat: &OpenAICompatConfig{
			MaxTokensField: "max_tokens",
		},
	}
	return nil
}

// OpenAIBaseFallbackURL returns the OPENAI_API_BASE or OPENAI_BASE_URL env var.
func OpenAIBaseFallbackURL() string {
	if u := os.Getenv("OPENAI_API_BASE"); u != "" {
		return u
	}
	if u := os.Getenv("OPENAI_BASE_URL"); u != "" {
		return u
	}
	return ""
}
