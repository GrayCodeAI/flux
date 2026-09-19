package provider

import "testing"

func TestDerivedProviderRegistryDedicatedTransports(t *testing.T) {
	want := map[string]ProviderType{
		"anthropic": ProviderTypeAnthropic,
		"openai":    ProviderTypeOpenAI,
		"azure":     ProviderTypeAzure,
		"bedrock":   ProviderTypeBedrock,
		"vertex":    ProviderTypeVertex,
	}
	if len(CoreProviders) != len(want) {
		t.Fatalf("CoreProviders has %d entries, want %d: %v", len(CoreProviders), len(want), CoreProviders)
	}
	for provider, providerType := range want {
		got, ok := CoreProviders[provider]
		if !ok {
			t.Fatalf("CoreProviders missing %q", provider)
		}
		if got.Type != providerType {
			t.Fatalf("CoreProviders[%q].Type = %q, want %q", provider, got.Type, providerType)
		}
	}
}

func TestDerivedProviderRegistryRuntimeOverrides(t *testing.T) {
	tests := []struct {
		provider string
		baseURL  string
		envKey   string
	}{
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "GEMINI_API_KEY"},
		{"clinepass", "https://api.cline.bot/api/v1", "CLINE_API_KEY"},
		{"ollama", "http://localhost:11434/v1", "OLLAMA_API_KEY"},
	}
	client := Client(nil)
	for _, tt := range tests {
		info := client.GetProviderInfo(tt.provider)
		if info == nil {
			t.Fatalf("GetProviderInfo(%q) returned nil", tt.provider)
		}
		if info.BaseURL != tt.baseURL {
			t.Errorf("GetProviderInfo(%q).BaseURL = %q, want %q", tt.provider, info.BaseURL, tt.baseURL)
		}
		if info.EnvKey != tt.envKey {
			t.Errorf("GetProviderInfo(%q).EnvKey = %q, want %q", tt.provider, info.EnvKey, tt.envKey)
		}
	}
}
