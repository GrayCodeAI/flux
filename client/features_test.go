package client

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// Tests below that read or write the package-level cachedCatalog run
// sequentially (no t.Parallel()): they mutate shared state via save/restore,
// which races under the parallel test runner.

func TestFeatureDefaultProviders_NoCatalog(t *testing.T) {
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()
	cachedCatalog = nil

	pf := NewProviderFeatures()

	// Without catalog, Get returns zero-value FeatureSet
	anthropic := pf.Get("anthropic")
	if anthropic.ToolUse {
		t.Error("zero-value should have ToolUse=false")
	}
	if anthropic.Streaming {
		t.Error("zero-value should have Streaming=false")
	}
	if anthropic.MaxContext != 0 {
		t.Errorf("zero-value MaxContext = %d, want 0", anthropic.MaxContext)
	}
	if anthropic.Thinking {
		t.Error("zero-value should have Thinking=false")
	}
}

func TestFeatureSupportsFeatureChecks_NoCatalog(t *testing.T) {
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()
	cachedCatalog = nil

	pf := NewProviderFeatures()

	// Without catalog, all features return false
	tests := []struct {
		provider string
		feature  string
		want     bool
	}{
		{"anthropic", "tool_use", false},
		{"anthropic", "streaming", false},
		{"anthropic", "thinking", false},
		{"openai", "tool_use", false},
		{"openai", "streaming", false},
	}

	for _, tc := range tests {
		got := pf.Supports(tc.provider, tc.feature)
		if got != tc.want {
			t.Errorf("Supports(%q, %q) = %v, want %v", tc.provider, tc.feature, got, tc.want)
		}
	}
}

func TestFeatureUnknownProviderDefaults(t *testing.T) {
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()
	cachedCatalog = nil

	pf := NewProviderFeatures()

	// Without catalog, unknown provider returns zero-value FeatureSet
	unknown := pf.Get("some-unknown-provider")
	if unknown.ToolUse {
		t.Error("unknown provider should return zero-value ToolUse=false")
	}
	if unknown.Streaming {
		t.Error("unknown provider should return zero-value Streaming=false")
	}
	if unknown.MaxContext != 0 {
		t.Errorf("unknown provider MaxContext = %d, want 0", unknown.MaxContext)
	}
}

func TestFeatureCaseInsensitiveProvider(t *testing.T) {
	pf := NewProviderFeatures()

	// Provider lookup should be case-insensitive
	upper := pf.Get("Anthropic")
	lower := pf.Get("anthropic")
	if upper.MaxContext != lower.MaxContext {
		t.Errorf("case-insensitive lookup failed: %d != %d", upper.MaxContext, lower.MaxContext)
	}
}

func TestFeatureDeprecationChecker(t *testing.T) {
	t.Parallel()
	dc := NewDeprecationChecker()

	// Check deprecated models
	info := dc.Check("claude-3-opus-20240229")
	if info == nil {
		t.Fatal("claude-3-opus-20240229 should be deprecated")
	}
	if info.Replacement != "claude-opus-4-6" {
		t.Errorf("replacement = %q, want claude-opus-4-6", info.Replacement)
	}

	// Non-deprecated model
	info = dc.Check("claude-sonnet-4-6")
	if info != nil {
		t.Errorf("claude-sonnet-4-6 should not be deprecated, got %+v", info)
	}
}

func TestFeatureSetFromCatalog_OverridesHardcoded(t *testing.T) {
	// Save and restore the global cachedCatalog
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()

	// Inject a mock catalog with per-model capabilities
	cachedCatalog = &catalog.CompiledCatalog{
		OfferingsByDeployment: map[string][]catalog.ModelOffering{
			"anthropic-direct": {
				{
					CanonicalModelID: "anthropic/claude-haiku-4-5",
					NativeModelID:    "claude-haiku-4-5-20251001",
					DeploymentID:     "anthropic-direct",
					Capabilities: catalog.CapabilitySet{
						ExplicitThinkingBudget: catalog.CapabilitySupported,
						AdaptiveThinking:       catalog.CapabilitySupported,
						FunctionCalling:        catalog.CapabilitySupported,
						ImageInput:             catalog.CapabilitySupported,
						MaxInputTokens:         200000,
						MaxOutputTokens:        64000,
					},
				},
				{
					CanonicalModelID: "anthropic/claude-opus-4-8",
					NativeModelID:    "claude-opus-4-8",
					DeploymentID:     "anthropic-direct",
					Capabilities: catalog.CapabilitySet{
						ExplicitThinkingBudget: catalog.CapabilitySupported,
						AdaptiveThinking:       catalog.CapabilitySupported,
						FunctionCalling:        catalog.CapabilitySupported,
						ImageInput:             catalog.CapabilitySupported,
						Effort:                 catalog.CapabilitySupported,
						StructuredOutput:       catalog.CapabilitySupported,
						CodeExecution:          catalog.CapabilitySupported,
						MaxInputTokens:         1000000,
						MaxOutputTokens:        128000,
					},
				},
			},
		},
		OfferingsByCanonicalModel: map[string][]catalog.ModelOffering{
			"anthropic/claude-haiku-4-5": {
				{
					CanonicalModelID: "anthropic/claude-haiku-4-5",
					NativeModelID:    "claude-haiku-4-5-20251001",
					DeploymentID:     "anthropic-direct",
					Capabilities: catalog.CapabilitySet{
						ExplicitThinkingBudget: catalog.CapabilitySupported,
						MaxInputTokens:         200000,
						MaxOutputTokens:        64000,
					},
				},
			},
		},
	}

	pf := NewProviderFeatures()

	// Should get catalog-backed values for haiku (200K context)
	haiku := pf.Get("claude-haiku-4-5-20251001")
	if haiku.MaxContext != 200000 {
		t.Errorf("haiku MaxContext = %d, want 200000 (from catalog)", haiku.MaxContext)
	}
	if haiku.MaxOutput != 64000 {
		t.Errorf("haiku MaxOutput = %d, want 64000", haiku.MaxOutput)
	}
	if !haiku.Thinking {
		t.Error("haiku should support thinking (from catalog)")
	}

	// Should get catalog-backed values for opus (1M context, effort, etc.)
	opus := pf.Get("claude-opus-4-8")
	if opus.MaxContext != 1000000 {
		t.Errorf("opus MaxContext = %d, want 1000000", opus.MaxContext)
	}
	if !opus.Effort {
		t.Error("opus should support effort (from catalog)")
	}
	if !opus.StructuredOutput {
		t.Error("opus should support structured output (from catalog)")
	}
	if !opus.CodeExecution {
		t.Error("opus should support code execution (from catalog)")
	}
}

func TestFeatureSetFromCatalog_FallsBackWhenNil(t *testing.T) {
	orig := cachedCatalog
	defer func() { cachedCatalog = orig }()
	cachedCatalog = nil

	pf := NewProviderFeatures()
	// Should get zero-value when catalog is nil
	anthropic := pf.Get("anthropic")
	if anthropic.MaxContext != 0 {
		t.Errorf("anthropic MaxContext = %d, want 0 (no catalog)", anthropic.MaxContext)
	}
	if anthropic.ToolUse {
		t.Error("no catalog should have ToolUse=false")
	}
}

func TestFeatureSetFromCapabilities(t *testing.T) {
	t.Parallel()
	caps := catalog.CapabilitySet{
		ExplicitThinkingBudget: catalog.CapabilitySupported,
		AdaptiveThinking:       catalog.CapabilitySupported,
		FunctionCalling:        catalog.CapabilitySupported,
		ImageInput:             catalog.CapabilitySupported,
		Effort:                 catalog.CapabilitySupported,
		StructuredOutput:       catalog.CapabilitySupported,
		CodeExecution:          catalog.CapabilitySupported,
		Citations:              catalog.CapabilitySupported,
		PDFInput:               catalog.CapabilitySupported,
		MaxInputTokens:         1000000,
		MaxOutputTokens:        128000,
	}
	fs := featureSetFromCapabilities(caps)
	if !fs.Thinking {
		t.Error("expected thinking")
	}
	if !fs.AdaptiveThinking {
		t.Error("expected adaptive thinking")
	}
	if !fs.ToolUse {
		t.Error("expected tool use")
	}
	if !fs.Images {
		t.Error("expected images")
	}
	if !fs.Effort {
		t.Error("expected effort")
	}
	if !fs.StructuredOutput {
		t.Error("expected structured output")
	}
	if !fs.CodeExecution {
		t.Error("expected code execution")
	}
	if !fs.Citations {
		t.Error("expected citations")
	}
	if !fs.PDFInput {
		t.Error("expected pdf input")
	}
	if fs.MaxContext != 1000000 {
		t.Errorf("MaxContext = %d", fs.MaxContext)
	}
	if fs.MaxOutput != 128000 {
		t.Errorf("MaxOutput = %d", fs.MaxOutput)
	}
}
