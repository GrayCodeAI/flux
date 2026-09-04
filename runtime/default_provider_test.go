package runtime

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
)

func TestDefaultModelProviderFilter_FromProviderConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cfg    *config.ProviderConfig
		expect string
	}{
		{
			name:   "anthropic active",
			cfg:    &config.ProviderConfig{ActiveProvider: "anthropic", AnthropicAPIKey: "sk-test"},
			expect: "anthropic",
		},
		{
			name:   "openai active",
			cfg:    &config.ProviderConfig{ActiveProvider: "openai", OpenAIAPIKey: "sk-test"},
			expect: "openai",
		},
		{
			name:   "empty config",
			cfg:    &config.ProviderConfig{},
			expect: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := config.DefaultProviderFromConfig(tt.cfg)
			if catalog.CanonicalProviderID(p) != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, p)
			}
		})
	}
}

func TestDefaultModelProviderFilter_LoadDoesNotPanic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = DefaultModelProviderFilter(ctx)
}

func TestDefaultModelProviderFilter_WithEmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	_ = DefaultModelProviderFilter(context.Background())
}
