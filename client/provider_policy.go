package client

import (
	"errors"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// ApplyProviderChatDefaults applies provider policy that host applications
// should not need to encode themselves.
func ApplyProviderChatDefaults(provider string, opts ChatOptions) ChatOptions {
	if strings.EqualFold(strings.TrimSpace(provider), "anthropic") {
		opts.EnableCaching = true
	}
	opts = NormalizeThinkingOptions(opts)
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "zai_payg", "zai_coding", "agnes", "openrouter", "opencodego", "anthropic":
		// Keep ThinkingEnabled — wire encoding comes from each provider's ThinkingFormat
		// (or Anthropic resolveThinking).
	case "longcat", "kimi", "deepseek", "xiaomi_mimo", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan",
		"minimax_payg", "minimax_token_plan":
		// These providers enable thinking when the field is omitted. Default off so
		// simple chat does not burn max_tokens on reasoning_content alone.
		if EffectiveThinkingEnabled(opts) == nil {
			disabled := false
			opts.ThinkingEnabled = &disabled
			opts.GLMThinkingEnabled = &disabled
		}
	default:
		if !ProviderSupportsThinkingToggle(provider) {
			opts.ThinkingEnabled = nil
			opts.GLMThinkingEnabled = nil
		}
	}
	return opts
}

// IsContextOverflow reports whether an error means the request exceeded the
// selected model's context window.
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	var providerErr *core.EyrieError
	if errors.As(err, &providerErr) {
		message := strings.ToLower(providerErr.Message)
		if containsContextOverflowSignal(message) {
			return true
		}
	}
	return containsContextOverflowSignal(strings.ToLower(err.Error()))
}

func containsContextOverflowSignal(message string) bool {
	for _, signal := range []string{
		"input exceeds the model's context window",
		"context_length_exceeded",
		"context length",
		"maximum context",
		"too many tokens",
		"prompt is too long",
		"reduce the length",
	} {
		if strings.Contains(message, signal) {
			return true
		}
	}
	return false
}
