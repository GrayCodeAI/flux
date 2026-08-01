package client

import (
	"testing"
)

// ---------------------------------------------------------------------------
// 1. Provider compatibility matrix
// ---------------------------------------------------------------------------

func TestCompatMatrixCoreProvidersHaveCompat(t *testing.T) {
	t.Parallel()
	// Every core provider must have a non-nil Compat config after init().
	for _, name := range []string{"openai", "azure", "bedrock", "vertex"} {
		p, ok := CoreProviders[name]
		if !ok {
			t.Fatalf("CoreProviders missing %q", name)
		}
		if p.Compat == nil {
			t.Errorf("CoreProviders[%q].Compat is nil", name)
		}
	}
}

func TestCompatMatrixOpenAICompatibleProvidersHaveCompat(t *testing.T) {
	t.Parallel()
	// Every OpenAI-compatible provider must have a non-nil Compat config after init().
	expected := []string{
		"grok", "openrouter", "gemini", "zai_payg", "zai_coding",
		"canopywave", "ollama", "opencodego", "kimi", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan",
	}
	for _, name := range expected {
		p, ok := OpenAICompatibleProviders[name]
		if !ok {
			t.Fatalf("OpenAICompatibleProviders missing %q", name)
		}
		if p.Compat == nil {
			t.Errorf("OpenAICompatibleProviders[%q].Compat is nil", name)
		}
	}
}

func TestCompatMatrixMaxTokensFieldValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider string
		field    string
		compat   *OpenAICompatConfig
	}{
		// Core providers
		{"openai", "max_completion_tokens", &OpenAICompat},
		{"azure", "max_tokens", &AzureCompat},
		{"bedrock", "max_tokens", &BedrockCompat},
		{"vertex", "max_tokens", &VertexCompat},
		// OpenAI-compatible
		{"grok", "max_tokens", &GrokCompat},
		{"openrouter", "max_tokens", &OpenRouterCompat},
		{"gemini", "max_tokens", &GeminiCompat},
		{"zai_payg", "max_tokens", &ZAICompat},
		{"zai_coding", "max_tokens", &ZAICompat},
		{"canopywave", "max_tokens", &CanopyWaveCompat},
		{"ollama", "max_tokens", &OllamaCompat},
		{"opencodego", "max_tokens", &OpenCodeGoCompat},
		{"kimi", "max_tokens", &KimiCompat},
		{"xiaomi_mimo_payg", "max_completion_tokens", &XiaomiCompat},
		{"xiaomi_mimo_token_plan", "max_completion_tokens", &XiaomiCompat},
	}
	for _, tc := range tests {
		if tc.compat.MaxTokensField != tc.field {
			t.Errorf("%s MaxTokensField = %q, want %q", tc.provider, tc.compat.MaxTokensField, tc.field)
		}
	}
}

func TestCompatMatrixOpenAIUniqueCapabilities(t *testing.T) {
	t.Parallel()
	// OpenAI is the only provider with SupportsStore and SupportsDeveloperRole.
	if !OpenAICompat.SupportsStore {
		t.Error("OpenAI should support store")
	}
	if !OpenAICompat.SupportsDeveloperRole {
		t.Error("OpenAI should support developer role")
	}
	if !OpenAICompat.SupportsReasoningEffort {
		t.Error("OpenAI should support reasoning effort")
	}

	// Check that other providers do NOT have these.
	others := []*OpenAICompatConfig{
		&GrokCompat, &OpenRouterCompat, &GeminiCompat,
		&ZAICompat, &CanopyWaveCompat, &OllamaCompat,
		&OpenCodeGoCompat, &KimiCompat, &XiaomiCompat,
		&AzureCompat, &BedrockCompat, &VertexCompat,
	}
	for _, c := range others {
		if c.SupportsStore {
			t.Error("non-OpenAI provider should not support store")
		}
		if c.SupportsDeveloperRole {
			t.Error("non-OpenAI provider should not support developer role")
		}
	}
}

func TestCompatMatrixThinkingFormatValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		compat *OpenAICompatConfig
		format string
	}{
		{&OpenRouterCompat, "openrouter"},
		{&ZAICompat, "zai"},
		{&OpenCodeGoCompat, "openrouter"},
		{&AgnesCompat, "agnes"},
		{&LongCatCompat, "longcat"},
		{&KimiCompat, "kimi"},
		{&DeepSeekCompat, "deepseek"},
		{&XiaomiCompat, "xiaomi"},
		{&MiniMaxCompat, "minimax"},
	}
	for _, tc := range tests {
		if tc.compat.ThinkingFormat != tc.format {
			t.Errorf("ThinkingFormat = %q, want %q", tc.compat.ThinkingFormat, tc.format)
		}
	}
	for _, c := range []*OpenAICompatConfig{&LongCatCompat, &KimiCompat, &DeepSeekCompat, &XiaomiCompat, &MiniMaxCompat} {
		if !c.DefaultDisableThinking {
			t.Fatalf("%q DefaultDisableThinking should be true", c.ThinkingFormat)
		}
	}

	// Providers without explicit thinking format should have empty string.
	noFormat := []*OpenAICompatConfig{
		&OpenAICompat, &GrokCompat, &GeminiCompat,
		&CanopyWaveCompat, &OllamaCompat,
		&AzureCompat, &BedrockCompat, &VertexCompat,
	}
	for _, c := range noFormat {
		if c.ThinkingFormat != "" {
			t.Errorf("unexpected ThinkingFormat %q", c.ThinkingFormat)
		}
	}
}

func TestCompatMatrixUsageInStreaming(t *testing.T) {
	t.Parallel()
	// Providers that report usage in streaming.
	supportsUsage := []*OpenAICompatConfig{
		&OpenAICompat, &OpenRouterCompat, &GeminiCompat, &ZAICompat, &OpenCodeGoCompat,
	}
	for _, c := range supportsUsage {
		if !c.SupportsUsageInStreaming {
			t.Errorf("expected SupportsUsageInStreaming=true")
		}
	}

	// Providers that do NOT report usage in streaming.
	noUsage := []*OpenAICompatConfig{
		&GrokCompat, &CanopyWaveCompat, &OllamaCompat,
		&KimiCompat, &XiaomiCompat,
		&AzureCompat, &BedrockCompat, &VertexCompat,
	}
	for _, c := range noUsage {
		if c.SupportsUsageInStreaming {
			t.Errorf("expected SupportsUsageInStreaming=false")
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Model support checking (DeprecationChecker)
// ---------------------------------------------------------------------------

func TestCompatDeprecatedModels(t *testing.T) {
	t.Parallel()
	dc := NewDeprecationChecker()

	tests := []struct {
		model       string
		deprecated  bool
		replacement string
	}{
		{"claude-3-opus-20240229", true, "claude-opus-4-6"},
		{"claude-3-sonnet-20240229", true, "claude-sonnet-4-6"},
		{"claude-3-haiku-20240307", true, "claude-haiku-4-5-20251001"},
		{"gpt-4-turbo", true, "gpt-4.1"},
		{"gpt-3.5-turbo", true, "gpt-4.1-mini"},
	}
	for _, tc := range tests {
		info := dc.Check(tc.model)
		if info == nil {
			t.Errorf("Check(%q) = nil, want deprecated", tc.model)
			continue
		}
		if info.Replacement != tc.replacement {
			t.Errorf("Check(%q).Replacement = %q, want %q", tc.model, info.Replacement, tc.replacement)
		}
		if info.Message == "" {
			t.Errorf("Check(%q).Message is empty", tc.model)
		}
	}
}

func TestCompatCurrentModelsNotDeprecated(t *testing.T) {
	t.Parallel()
	dc := NewDeprecationChecker()

	current := []string{
		"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5-20251001",
		"gpt-4.1", "gpt-4.1-mini", "gpt-4o",
	}
	for _, model := range current {
		if info := dc.Check(model); info != nil {
			t.Errorf("Check(%q) should be nil for current model, got %+v", model, info)
		}
	}
}

func TestCompatDeprecationCheckerNilSafety(t *testing.T) {
	t.Parallel()
	dc := NewDeprecationChecker()
	// Check a non-existent model.
	if info := dc.Check("nonexistent-model-xyz"); info != nil {
		t.Errorf("expected nil for unknown model, got %+v", info)
	}
}

// ---------------------------------------------------------------------------
// 3. Feature availability per provider
// ---------------------------------------------------------------------------

// The tests in this section read or write the package-level cachedCatalog
// (directly, or transitively via Get/Supports) and run sequentially (no
// t.Parallel()): mutating shared state via save/restore races under the
// parallel test runner. See also client/features_test.go.

func TestCompatFeatureMatrixAllProviders(t *testing.T) {
	// With no catalog loaded, all providers return zero-value FeatureSet
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()
	cachedCatalog = nil

	pf := NewProviderFeatures()

	for _, provider := range []string{"anthropic", "openai", "gemini", "ollama", "openrouter", "grok"} {
		got := pf.Get(provider)
		if got.ToolUse {
			t.Errorf("%s: zero-value should have ToolUse=false", provider)
		}
		if got.Streaming {
			t.Errorf("%s: zero-value should have Streaming=false", provider)
		}
		if got.MaxContext != 0 {
			t.Errorf("%s: zero-value MaxContext = %d, want 0", provider, got.MaxContext)
		}
	}
}

func TestCompatSupportsAllFeatureAliases(t *testing.T) {
	pf := NewProviderFeatures()

	// Verify that feature name aliases resolve identically.
	aliasGroups := [][]string{
		{"tools", "tool_use"},
		{"caching", "cache"},
		{"json", "json_mode"},
	}
	for _, group := range aliasGroups {
		for _, provider := range []string{"anthropic", "openai", "gemini", "ollama"} {
			first := pf.Supports(provider, group[0])
			for _, alias := range group[1:] {
				if pf.Supports(provider, alias) != first {
					t.Errorf("alias mismatch for %s: Supports(%q,%q)=%v but Supports(%q,%q)=%v",
						provider, provider, group[0], first, provider, alias, !first)
				}
			}
		}
	}
}

func TestCompatSupportsUnknownFeatureReturnsFalse(t *testing.T) {
	pf := NewProviderFeatures()
	if pf.Supports("anthropic", "nonexistent_feature_xyz") {
		t.Error("unknown feature should return false")
	}
}

func TestCompatSupportsCaseInsensitiveFeatureNames(t *testing.T) {
	pf := NewProviderFeatures()
	// "Thinking" vs "thinking" should resolve the same.
	if pf.Supports("anthropic", "Thinking") != pf.Supports("anthropic", "thinking") {
		t.Error("feature name lookup should be case-insensitive")
	}
}
