//nolint:errcheck
package config

import (
	"context"
	"os"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func clearRuntimeProviderCredentials(t *testing.T) {
	t.Helper()

	seen := make(map[string]struct{})
	for _, profile := range OpenAICompatibleRuntimeProfiles {
		for _, key := range profile.DetectionEnv {
			seen[key] = struct{}{}
		}
		for _, key := range profile.APIKeys {
			seen[key.Env] = struct{}{}
		}
	}
	seen["OLLAMA_BASE_URL"] = struct{}{}
	for key := range seen {
		t.Setenv(key, "")
	}
}

func TestRuntimeProfileFields(t *testing.T) {
	t.Parallel()
	profiles := map[string]RuntimeProviderProfile{
		"anthropic":  AnthropicRuntimeProfile,
		"openai":     OpenAIRuntimeProfile,
		"grok":       GrokRuntimeProfile,
		"gemini":     GeminiRuntimeProfile,
		"openrouter": OpenRouterRuntimeProfile,
		"fireworks":  FireworksRuntimeProfile,
		"canopywave": CanopyWaveRuntimeProfile,
		"deepseek":   DeepSeekRuntimeProfile,
		"zai_payg":   ZAIPaygRuntimeProfile,
		"opencodego": OpenCodeGoRuntimeProfile,
	}

	for name, profile := range profiles {
		if profile.Mode == "" {
			t.Errorf("profile %q has empty Mode", name)
		}
		if profile.DefaultBaseURL == "" {
			t.Errorf("profile %q has empty DefaultBaseURL", name)
		}
		// DefaultModel is now fully dynamic — no hardcoded defaults
		if len(profile.DetectionEnv) == 0 {
			t.Errorf("profile %q has empty DetectionEnv", name)
		}
		if len(profile.ModelEnv) == 0 {
			t.Errorf("profile %q has empty ModelEnv", name)
		}
		if len(profile.BaseURLEnv) == 0 {
			t.Errorf("profile %q has empty BaseURLEnv", name)
		}
	}
}

func TestRuntimeProfileAPIKeys(t *testing.T) {
	t.Parallel()
	// All profiles except ollama should have API keys
	profiles := map[string]RuntimeProviderProfile{
		"anthropic":  AnthropicRuntimeProfile,
		"openai":     OpenAIRuntimeProfile,
		"grok":       GrokRuntimeProfile,
		"gemini":     GeminiRuntimeProfile,
		"openrouter": OpenRouterRuntimeProfile,
		"fireworks":  FireworksRuntimeProfile,
		"canopywave": CanopyWaveRuntimeProfile,
		"deepseek":   DeepSeekRuntimeProfile,
		"zai_payg":   ZAIPaygRuntimeProfile,
		"opencodego": OpenCodeGoRuntimeProfile,
	}

	for name, profile := range profiles {
		if len(profile.APIKeys) == 0 {
			t.Errorf("profile %q should have API keys defined", name)
		}
		for i, key := range profile.APIKeys {
			if key.Env == "" {
				t.Errorf("profile %q APIKeys[%d] has empty Env", name, i)
			}
			if key.Source == "" {
				t.Errorf("profile %q APIKeys[%d] has empty Source", name, i)
			}
		}
	}
}

func TestModelEnvKeysCorrectForEachProvider(t *testing.T) {
	t.Parallel()
	expected := map[string]string{
		ProviderAnthropic:  "ANTHROPIC_MODEL",
		ProviderOpenAI:     "OPENAI_MODEL",
		ProviderCanopyWave: "CANOPYWAVE_MODEL",
		ProviderDeepSeek:   "DEEPSEEK_MODEL",
		ProviderOpenRouter: "OPENROUTER_MODEL",
		ProviderFireworks:  "FIREWORKS_MODEL",
		ProviderGrok:       "XAI_MODEL",
		ProviderGemini:     "GEMINI_MODEL",
		ProviderOllama:     "OLLAMA_MODEL",
		ProviderOpenCodeGo: "OPENCODEGO_MODEL",
	}

	for provider, expectedKey := range expected {
		keys, ok := ProviderModelEnvKeys[provider]
		if !ok {
			t.Errorf("ProviderModelEnvKeys missing provider %q", provider)
			continue
		}
		if len(keys) == 0 {
			t.Errorf("ProviderModelEnvKeys[%q] is empty", provider)
			continue
		}
		if keys[0] != expectedKey {
			t.Errorf("ProviderModelEnvKeys[%q][0] = %q, want %q", provider, keys[0], expectedKey)
		}
	}
}

func TestProviderModelEnvKeys_AllProvidersPresent(t *testing.T) {
	t.Parallel()
	allProviders := []string{
		ProviderAnthropic, ProviderOpenAI, ProviderCanopyWave, ProviderDeepSeek,
		ProviderOpenRouter, ProviderGrok, ProviderGemini,
		ProviderFireworks,
		ProviderOllama, ProviderOpenCodeGo,
	}

	for _, provider := range allProviders {
		if _, ok := ProviderModelEnvKeys[provider]; !ok {
			t.Errorf("ProviderModelEnvKeys missing provider %q", provider)
		}
	}
}

func TestResolveOpenAICompatibleRuntime_WithEnv(t *testing.T) {
	clearRuntimeProviderCredentials(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "sk-test-key-1234567890")

	result := ResolveOpenAICompatibleRuntime("gpt-4o", "", "")
	if result.Mode != "openai" {
		t.Errorf("expected mode 'openai', got %q", result.Mode)
	}
	if result.Request.ResolvedModel != "gpt-4o" {
		t.Errorf("expected resolved model 'gpt-4o', got %q", result.Request.ResolvedModel)
	}
	if result.APIKey != "sk-test-key-1234567890" {
		t.Error("resolved API key did not match the isolated test credential")
	}
	if result.APIKeySource != "openai" {
		t.Errorf("expected API key source 'openai', got %q", result.APIKeySource)
	}
}

func TestResolveOpenAICompatibleRuntime_GrokProvider(t *testing.T) {
	clearRuntimeProviderCredentials(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv("XAI_API_KEY"), "xai-test-key-1234567890")

	result := ResolveOpenAICompatibleRuntime("", "", "")
	if result.Mode != "grok" {
		t.Errorf("expected mode 'grok', got %q", result.Mode)
	}
	if result.APIKey != "xai-test-key-1234567890" {
		t.Error("resolved API key did not match the isolated xAI test credential")
	}
	if result.APIKeySource != "grok" {
		t.Errorf("expected source 'grok', got %q", result.APIKeySource)
	}
}

func TestResolveOpenAICompatibleRuntime_FallbackModel(t *testing.T) {
	clearRuntimeProviderCredentials(t)
	clearKeys := []string{
		"OPENROUTER_API_KEY", "XAI_API_KEY", "GEMINI_API_KEY",
		"ANTHROPIC_API_KEY", "CANOPYWAVE_API_KEY", "DEEPSEEK_API_KEY", "ZAI_API_KEY", "OPENAI_API_KEY",
		"OPENCODEGO_API_KEY", "OLLAMA_BASE_URL",
		"MOONSHOT_API_KEY", "XIAOMI_MIMO_PAYG_API_KEY", "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
		"OPENAI_MODEL", "OPENAI_BASE_URL", "OPENAI_API_BASE",
	}
	for _, k := range clearKeys {
		os.Unsetenv(k)
	}

	// No model specified, should use fallback
	result := ResolveOpenAICompatibleRuntime("", "", "my-fallback-model")
	if result.Request.ResolvedModel != "my-fallback-model" {
		t.Errorf("expected fallback model 'my-fallback-model', got %q", result.Request.ResolvedModel)
	}
}

func TestResolveOpenAICompatibleRuntime_NoKeys(t *testing.T) {
	clearRuntimeProviderCredentials(t)
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	result := ResolveOpenAICompatibleRuntime("", "", "")
	if result.APIKey != "" {
		t.Error("expected an empty API key when no provider credential is configured")
	}
	if result.APIKeySource != "none" {
		t.Errorf("expected source 'none', got %q", result.APIKeySource)
	}
}

func TestOpenAICompatibleRuntimeProfileOrder(t *testing.T) {
	t.Parallel()
	if len(OpenAICompatibleRuntimeProfileOrder) == 0 {
		t.Fatal("runtime profile order should not be empty")
	}
	// Verify all entries in the order have corresponding profiles
	for _, key := range OpenAICompatibleRuntimeProfileOrder {
		if _, ok := OpenAICompatibleRuntimeProfiles[key]; !ok {
			t.Errorf("profile order contains %q but no matching profile exists", key)
		}
	}
}

func TestOpenAICompatibleRuntimeProfiles_Complete(t *testing.T) {
	t.Parallel()
	// Every profile in the map should have valid structure
	for key, profile := range OpenAICompatibleRuntimeProfiles {
		if profile.Mode == "" {
			t.Errorf("profile %q has empty Mode", key)
		}
		if profile.DefaultBaseURL == "" && key != "xiaomi_mimo_token_plan" {
			t.Errorf("profile %q has empty DefaultBaseURL", key)
		}
		// DefaultModel is now fully dynamic — no hardcoded defaults
	}
}
