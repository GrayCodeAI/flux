package client

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
)

func TestOpenCodeGoUsesMessagesAPI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model string
		want  bool
	}{
		{"minimax-m2.5", true},
		{"opencodego/minimax-m2.5", true},
		{"qwen3.7-max", true},
		{"kimi-k2.5", false},
		{"glm-5", false},
		{"mimo-v2.5-pro", false},
	}
	for _, tc := range tests {
		if got := opencodego.UsesMessagesAPI(tc.model); got != tc.want {
			t.Errorf("UsesMessagesAPI(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestOpenCodeGoAnthropicBase(t *testing.T) {
	t.Parallel()
	if got := AnthropicBaseFromOpenAIV1("https://opencode.ai/zen/go/v1"); got != "https://opencode.ai/zen/go" {
		t.Fatalf("base = %q, want https://opencode.ai/zen/go", got)
	}
}
