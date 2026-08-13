package config

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/credentials"
)

// OpenAICompatibleRuntimeMode identifies the runtime mode.
type OpenAICompatibleRuntimeMode = string

// OpenAICompatibleApiKeySource identifies where the API key came from.
type OpenAICompatibleApiKeySource = string

// ResolvedOpenAICompatibleRuntime holds the resolved runtime config.
type ResolvedOpenAICompatibleRuntime struct {
	Mode         string                  `json:"mode"`
	Request      ResolvedProviderRequest `json:"request"`
	APIKey       string                  `json:"-"`
	APIKeySource string                  `json:"api_key_source"`
}

// IsOpenAICompatibleRuntimeEnabled checks if any provider API key is set.
func IsOpenAICompatibleRuntimeEnabled() bool {
	// Derive detection keys from the registered runtime profiles so adding a
	// provider cannot silently leave readiness checks out of sync.
	for _, profile := range RuntimeProviderProfiles {
		for _, key := range profile.DetectionEnv {
			if envValue(key) != "" {
				return true
			}
		}
	}
	return envValue("OLLAMA_BASE_URL") != ""
}

func envValue(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	// Bound the keyring lookup to prevent indefinite stalls when the OS
	// keychain is unresponsive. The keyring itself has a 30s timeout,
	// but envValue is called many times during provider construction.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Always check the credential store first for ALL keys.
	if v := credentials.LookupSecret(ctx, key); v != "" {
		return v
	}
	// Fall back to process environment only when the credential store has
	// nothing for this key.
	return strings.TrimSpace(os.Getenv(key))
}

func asTrimmedEnv(key string) string {
	return envValue(key)
}

func firstEnvValue(keys []string) string {
	for _, k := range keys {
		if v := asTrimmedEnv(k); v != "" {
			return v
		}
	}
	return ""
}

func resolveRuntimeProvider() RuntimeProviderProfile {
	for _, key := range OpenAICompatibleRuntimeProfileOrder {
		profile := OpenAICompatibleRuntimeProfiles[key]
		for _, envKey := range profile.DetectionEnv {
			if asTrimmedEnv(envKey) != "" {
				return profile
			}
		}
	}
	if v := envValue("OLLAMA_BASE_URL"); v != "" {
		base := OpenAIRuntimeProfile
		base.DefaultBaseURL = v
		if base.DefaultBaseURL == "" {
			base.DefaultBaseURL = OllamaDefaultBaseURL
		}
		base.ModelEnv = []string{"OLLAMA_MODEL"}
		base.BaseURLEnv = []string{"OLLAMA_BASE_URL"}
		base.APIKeys = nil
		return base
	}
	return OpenAIRuntimeProfile
}

func resolveProviderAPIKey(profile RuntimeProviderProfile) (string, string) {
	for _, k := range profile.APIKeys {
		if v := asTrimmedEnv(k.Env); v != "" {
			return v, k.Source
		}
	}
	return "", "none"
}

// ResolveOpenAICompatibleRuntime resolves the full OpenAI-compatible runtime config.
func ResolveOpenAICompatibleRuntime(model, baseURL, fallbackModel string) ResolvedOpenAICompatibleRuntime {
	provider := resolveRuntimeProvider()
	runtimeModel := model
	if runtimeModel == "" {
		runtimeModel = firstEnvValue(provider.ModelEnv)
	}
	runtimeBaseURL := baseURL
	if runtimeBaseURL == "" {
		runtimeBaseURL = firstEnvValue(provider.BaseURLEnv)
	}
	if runtimeBaseURL == "" {
		runtimeBaseURL = provider.DefaultBaseURL
	}

	fb := fallbackModel
	if fb == "" {
		fb = firstEnvValue(provider.ModelEnv)
	}
	if fb == "" {
		fb = provider.DefaultModel
	}

	request := ResolveProviderRequest(runtimeModel, runtimeBaseURL, fb)
	apiKey, apiKeySource := resolveProviderAPIKey(provider)

	return ResolvedOpenAICompatibleRuntime{
		Mode:         provider.Mode,
		Request:      request,
		APIKey:       apiKey,
		APIKeySource: apiKeySource,
	}
}
