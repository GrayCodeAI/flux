package client

import (
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// ProviderThinkingFormat is the wire encoding for extended thinking / reasoning.
// Each provider that supports a host-controlled toggle has its own format; the
// host preference is always the generic ChatOptions.ThinkingEnabled (with
// deprecated GLMThinkingEnabled as a Z.AI-era alias).
//
// Formats (from official provider docs):
//
//	zai / longcat / kimi / deepseek / xiaomi → thinking={"type":"enabled"|"disabled"}
//	minimax                                    → thinking={"type":"adaptive"|"disabled"}
//	agnes                                      → chat_template_kwargs.enable_thinking
//	qwen                                       → enable_thinking (top-level)
//	openrouter                                 → reasoning={enabled: true|false}
const (
	ThinkingFormatNone       = ""
	ThinkingFormatZAI        = "zai"
	ThinkingFormatLongCat    = "longcat"
	ThinkingFormatKimi       = "kimi"
	ThinkingFormatDeepSeek   = "deepseek"
	ThinkingFormatXiaomi     = "xiaomi"
	ThinkingFormatMiniMax    = "minimax" // MiniMax OpenAI: thinking={"type":"adaptive"|"disabled"}
	ThinkingFormatAgnes      = "agnes"
	ThinkingFormatQwen       = "qwen"
	ThinkingFormatOpenRouter = "openrouter"
)

// ProviderSupportsThinkingToggle reports whether the provider honors the generic
// ThinkingEnabled host preference on the wire.
func ProviderSupportsThinkingToggle(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "zai_payg", "zai_coding",
		"longcat",
		"agnes",
		"kimi",
		"deepseek",
		"xiaomi_mimo", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan",
		"minimax_payg", "minimax_token_plan",
		"openrouter",
		"opencodego",
		"anthropic":
		return true
	default:
		return false
	}
}

// EffectiveThinkingEnabled returns the host thinking preference.
// ThinkingEnabled is the standard field; GLMThinkingEnabled is accepted as a
// deprecated alias so older Z.AI call sites keep working.
func EffectiveThinkingEnabled(opts core.ChatOptions) *bool {
	if opts.ThinkingEnabled != nil {
		return opts.ThinkingEnabled
	}
	return opts.GLMThinkingEnabled
}

// NormalizeThinkingOptions copies ThinkingEnabled ↔ GLMThinkingEnabled so both
// fields stay consistent for adapters and older hosts.
func NormalizeThinkingOptions(opts core.ChatOptions) core.ChatOptions {
	switch {
	case opts.ThinkingEnabled != nil && opts.GLMThinkingEnabled == nil:
		opts.GLMThinkingEnabled = opts.ThinkingEnabled
	case opts.ThinkingEnabled == nil && opts.GLMThinkingEnabled != nil:
		opts.ThinkingEnabled = opts.GLMThinkingEnabled
	}
	return opts
}
