package client

import (
	"testing"
)

func TestAnthropicBaseFromOpenAIV1(t *testing.T) {
	t.Parallel()
	if got := AnthropicBaseFromOpenAIV1("https://example.com/zen/go/v1"); got != "https://example.com/zen/go" {
		t.Fatalf("got %q", got)
	}
	if got := AnthropicBaseFromOpenAIV1("https://example.com/zen/go"); got != "https://example.com/zen/go" {
		t.Fatalf("got %q", got)
	}
}
