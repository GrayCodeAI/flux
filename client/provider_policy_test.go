package client

import (
	"errors"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestApplyProviderChatDefaults(t *testing.T) {
	if opts := ApplyProviderChatDefaults("anthropic", ChatOptions{}); !opts.EnableCaching {
		t.Fatal("Anthropic caching default was not applied")
	}
	if opts := ApplyProviderChatDefaults("openai", ChatOptions{}); opts.EnableCaching {
		t.Fatal("non-Anthropic provider enabled caching")
	}
	enabled := true
	if opts := ApplyProviderChatDefaults("zai_payg", ChatOptions{GLMThinkingEnabled: &enabled}); opts.GLMThinkingEnabled == nil {
		t.Fatal("Z.AI thinking preference was removed")
	}
	if opts := ApplyProviderChatDefaults("openai", ChatOptions{GLMThinkingEnabled: &enabled}); opts.GLMThinkingEnabled != nil {
		t.Fatal("non-Z.AI thinking preference was retained")
	}
}

func TestIsContextOverflow(t *testing.T) {
	typed := &core.GraycodeRouterError{Provider: "openai", Message: "input exceeds the model's context window"}
	if !IsContextOverflow(typed) {
		t.Fatal("typed context error was not detected")
	}
	if !IsContextOverflow(errors.New("context_length_exceeded")) {
		t.Fatal("unstructured context error was not detected")
	}
	if IsContextOverflow(errors.New("unauthorized")) {
		t.Fatal("unrelated error was classified as context overflow")
	}
}
