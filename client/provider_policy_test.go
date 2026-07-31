package client

import (
	"errors"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestApplyProviderChatDefaults(t *testing.T) {
	if opts := ApplyProviderChatDefaults("anthropic", ChatOptions{}); !opts.EnableCaching {
		t.Fatal("Anthropic caching default was not applied")
	}
	if opts := ApplyProviderChatDefaults("openai", ChatOptions{}); opts.EnableCaching {
		t.Fatal("non-Anthropic provider enabled caching")
	}
	enabled := true
	if opts := ApplyProviderChatDefaults("zai_payg", ChatOptions{ThinkingEnabled: &enabled}); opts.ThinkingEnabled == nil {
		t.Fatal("Z.AI thinking preference was removed")
	}
	if opts := ApplyProviderChatDefaults("longcat", ChatOptions{ThinkingEnabled: &enabled}); opts.ThinkingEnabled == nil || !*opts.ThinkingEnabled {
		t.Fatal("LongCat explicit thinking preference was removed")
	}
	if opts := ApplyProviderChatDefaults("longcat", ChatOptions{}); opts.ThinkingEnabled == nil || *opts.ThinkingEnabled {
		t.Fatal("LongCat should default ThinkingEnabled=false when unset")
	}
	if opts := ApplyProviderChatDefaults("kimi", ChatOptions{}); opts.ThinkingEnabled == nil || *opts.ThinkingEnabled {
		t.Fatal("Kimi should default ThinkingEnabled=false when unset")
	}
	if opts := ApplyProviderChatDefaults("deepseek", ChatOptions{}); opts.ThinkingEnabled == nil || *opts.ThinkingEnabled {
		t.Fatal("DeepSeek should default ThinkingEnabled=false when unset")
	}
	if opts := ApplyProviderChatDefaults("agnes", ChatOptions{GLMThinkingEnabled: &enabled}); opts.ThinkingEnabled == nil {
		t.Fatal("Agnes should normalize deprecated GLMThinkingEnabled alias")
	}
	if opts := ApplyProviderChatDefaults("openrouter", ChatOptions{ThinkingEnabled: &enabled}); opts.ThinkingEnabled == nil {
		t.Fatal("OpenRouter should keep ThinkingEnabled")
	}
	if opts := ApplyProviderChatDefaults("anthropic", ChatOptions{ThinkingEnabled: &enabled}); opts.ThinkingEnabled == nil {
		t.Fatal("Anthropic should keep ThinkingEnabled")
	}
	if opts := ApplyProviderChatDefaults("openai", ChatOptions{ThinkingEnabled: &enabled}); opts.ThinkingEnabled != nil || opts.GLMThinkingEnabled != nil {
		t.Fatal("non-thinking provider thinking preference was retained")
	}
}

func TestIsContextOverflow(t *testing.T) {
	typed := &core.EyrieError{Provider: "openai", Message: "input exceeds the model's context window"}
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
