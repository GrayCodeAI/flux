package catalog

import "testing"

func TestAnthropicNameToCanonical(t *testing.T) {
	t.Parallel()
	tests := []struct{ input, want string }{
		{"claude-sonnet-4-6-20250814", "claude-sonnet-4-6"},
		{"us.hawk.claude-opus-4-6-v1:0", "claude-opus-4-6"},
		{"claude-3-5-haiku-20241022", "claude-3-5-haiku"},
		{"claude-3-7-sonnet-20250219", "claude-3-7-sonnet"},
		{"claude-opus-4-5-20251101", "claude-opus-4-5"},
		{"claude-opus-4-1-20250805", "claude-opus-4-1"},
		{"gpt-4o", "gpt-4o"},
	}
	for _, tt := range tests {
		if got := AnthropicNameToCanonical(tt.input); got != tt.want {
			t.Errorf("AnthropicNameToCanonical(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetModelMarketingName(t *testing.T) {
	t.Parallel()
	tests := []struct{ input, want string }{
		{"claude-opus-4-6", "Opus 4.6"},
		{"claude-sonnet-4-6[1m]", "Sonnet 4.6 (1M context)"},
		{"claude-sonnet-4-6", "Sonnet 4.6"},
		{"claude-3-7-sonnet-20250219", "Sonnet 3.7"},
		{"claude-haiku-4-5-20251001", "Haiku 4.5"},
		{"gpt-4o", ""},
	}
	for _, tt := range tests {
		if got := GetModelMarketingName(tt.input); got != tt.want {
			t.Errorf("GetModelMarketingName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetProviderDefaultModel_AllProvidersEmptyWithoutCatalog(t *testing.T) {
	t.Parallel()
	// All providers return empty without a catalog (fully dynamic)
	allProviders := []string{
		"anthropic", "openai", "gemini", "grok", "bedrock", "kimi",
	}
	for _, provider := range allProviders {
		if got := GetProviderDefaultModel(provider, nil); got != "" {
			t.Fatalf("%s default should be empty without catalog, got %q", provider, got)
		}
	}
}

func TestGetModelDeprecationWarning(t *testing.T) {
	t.Parallel()
	warning := GetModelDeprecationWarning("claude-3-7-sonnet-20250219", "anthropic")
	if warning == "" {
		t.Error("expected deprecation warning for claude-3-7-sonnet on anthropic")
	}
	warning = GetModelDeprecationWarning("claude-sonnet-4-6", "anthropic")
	if warning != "" {
		t.Errorf("expected no warning for claude-sonnet-4-6, got %q", warning)
	}
}

func TestModelsForProvider(t *testing.T) {
	t.Parallel()
	cat := testModelCatalog()
	models := cat.Providers["anthropic"]
	if len(models) == 0 {
		t.Error("expected anthropic models in default catalog")
	}
	models = cat.Providers["nonexistent"]
	if len(models) != 0 {
		t.Error("expected no models for nonexistent provider")
	}
	var nilCat *ModelCatalog
	if nilCat != nil && len(nilCat.Providers["anthropic"]) > 0 {
		t.Error("expected no models for nil catalog")
	}
}
