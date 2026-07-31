package adapters

// OpenAICompatConfig holds provider-specific compatibility flags
// that control how API requests are constructed for each provider.
type OpenAICompatConfig struct {
	SupportsStore                    bool   `json:"supports_store,omitempty"`
	SupportsDeveloperRole            bool   `json:"supports_developer_role,omitempty"`
	SupportsReasoningEffort          bool   `json:"supports_reasoning_effort,omitempty"`
	SupportsUsageInStreaming         bool   `json:"supports_usage_in_streaming,omitempty"`
	SupportsStrictMode               bool   `json:"supports_strict_mode,omitempty"`
	MaxTokensField                   string `json:"max_tokens_field,omitempty"` // "max_tokens" or "max_completion_tokens"
	RequiresToolResultName           bool   `json:"requires_tool_result_name,omitempty"`
	RequiresAssistantAfterToolResult bool   `json:"requires_assistant_after_tool_result,omitempty"`
	RequiresThinkingAsText           bool   `json:"requires_thinking_as_text,omitempty"`
	ThinkingFormat                   string `json:"thinking_format,omitempty"` // "zai","longcat","kimi","deepseek","xiaomi","minimax","agnes","qwen","openrouter"
	// StripReasoningFromInput instructs buildRequestBase to omit the reasoning_content
	// field from assistant messages. DeepSeek (and compatible providers) return HTTP 400
	// if reasoning_content appears in the input context of a multi-turn conversation.
	StripReasoningFromInput bool `json:"strip_reasoning_from_input,omitempty"`
	// SupportsCacheRole enables Kimi/Moonshot context-cache injection: when
	// core.ChatOptions.KimiContextCacheID is non-empty, buildRequestBase prepends a
	// {"role":"cache","content":<id>} message per the MoonshotAI-Cookbook spec.
	SupportsCacheRole bool `json:"supports_cache_role,omitempty"`
	// OmitMaxTokens instructs buildRequestBase to leave max_tokens unset instead
	// of sending the default 4096. Providers that pre-authorize the maximum token
	// cost (e.g. Agnes AI) can return insufficient_user_quota when that hold
	// exceeds the account balance; omitting max_tokens lets the provider apply its
	// own default, which avoids the oversized hold.
	OmitMaxTokens bool `json:"omit_max_tokens,omitempty"`
	// DefaultDisableThinking: when ThinkingEnabled is nil, emit an explicit
	// thinking={"type":"disabled"} for formats that use the thinking object.
	// LongCat enables thinking by default when the field is omitted; that often
	// burns the entire max_tokens budget on reasoning_content with no reply.
	DefaultDisableThinking bool `json:"default_disable_thinking,omitempty"`
}

// Per-provider compat configs.
var (
	OpenAICompat = OpenAICompatConfig{
		SupportsStore: true, SupportsDeveloperRole: true,
		SupportsReasoningEffort: true, SupportsUsageInStreaming: true,
		MaxTokensField: "max_completion_tokens",
	}
	GrokCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	OpenRouterCompat = OpenAICompatConfig{
		// OpenRouter: reasoning={enabled|effort} per official reasoning-tokens guide.
		ThinkingFormat:           "openrouter",
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
	}
	// AgnesCompat: Agnes AI is OpenAI-compatible only (chat completions). It does
	// not honor OpenAI-specific features like store/developer-role/reasoning,
	// so those are left disabled. It uses the standard max_tokens field.
	// Agnes pre-authorizes the maximum token cost of every request; sending the
	// default 4096 max_tokens makes that hold exceed the account balance and
	// triggers an insufficient_user_quota (403). OmitMaxTokens leaves max_tokens
	// unset so Agnes applies its own default, which keeps the hold small — this
	// matches the behavior of a bare curl to the Agnes API.
	// Thinking uses chat_template_kwargs.enable_thinking per Agnes OpenAI docs.
	AgnesCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		OmitMaxTokens:            true,
		ThinkingFormat:           "agnes",
	}
	// LongCatCompat: OpenAI-compatible only (https://api.longcat.chat/openai).
	// Official docs: max_tokens, thinking={"type":"enabled"|"disabled"}, tools.
	// DefaultDisableThinking is required: omitting thinking leaves LongCat's
	// server-side default (enabled), which commonly yields reasoning-only replies.
	LongCatCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		ThinkingFormat:           "longcat",
		DefaultDisableThinking:   true,
	}
	GeminiCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens", SupportsUsageInStreaming: true,
	}
	ZAICompat = OpenAICompatConfig{
		ThinkingFormat: "zai", MaxTokensField: "max_tokens",
		SupportsUsageInStreaming: true,
	}
	CanopyWaveCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	OllamaCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	OpenCodeGoCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		// OpenCode Go routes many models through OpenRouter-style reasoning.
		ThinkingFormat:          "openrouter",
		StripReasoningFromInput: true,
	}
	PoolsideCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	GroqCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	ClinePassCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	KimiCompat = OpenAICompatConfig{
		MaxTokensField:    "max_tokens",
		SupportsCacheRole: true,
		// Kimi K2.5/K2.6: thinking={"type":"enabled"|"disabled"} (defaults enabled).
		// https://platform.kimi.ai/docs/guide/use-kimi-k2-thinking-model
		ThinkingFormat:         "kimi",
		DefaultDisableThinking: true,
	}
	XiaomiCompat = OpenAICompatConfig{
		MaxTokensField: "max_completion_tokens",
		// Xiaomi MiMo: thinking={"type":"enabled"|"disabled"} (on by default for pro).
		// https://mimo.mi.com/docs/en-US/quick-start/usage-guide/text-generation/deep-thinking
		ThinkingFormat:         "xiaomi",
		DefaultDisableThinking: true,
	}
	AzureCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	BedrockCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	VertexCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	// DeepSeekCompat: OpenAI-compatible with usage in streaming.
	// The provider rejects reasoning_content in input messages with HTTP 400, so we strip it
	// for non-tool turns. Thinking mode: thinking={"type":...} (defaults enabled per
	// https://api-docs.deepseek.com/guides/thinking_mode).
	DeepSeekCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		StripReasoningFromInput:  true,
		ThinkingFormat:           "deepseek",
		DefaultDisableThinking:   true,
	}
	// ConcentrateCompat: OpenAI-compatible with usage in streaming.
	ConcentrateCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
	}
	// MiniMaxCompat: OpenAI chat completions at api.minimax.io/v1.
	// Official OpenAI path: thinking.type is "disabled" | "adaptive" (default on when omitted).
	// https://platform.minimax.io/docs/api-reference/text-openai-api
	MiniMaxCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		ThinkingFormat:           "minimax",
		DefaultDisableThinking:   true,
	}
)

func init() {
	// Attach compat configs to provider registry.
	// Acquire dynamicMu for consistency with the runtime lock protocol,
	// even though init() is single-threaded.
	DynamicMu.Lock()
	defer DynamicMu.Unlock()

	if p, ok := OpenAICompatibleProviders["agnes"]; ok {
		p.Compat = &AgnesCompat
		OpenAICompatibleProviders["agnes"] = p
	}
	if p, ok := OpenAICompatibleProviders["longcat"]; ok {
		p.Compat = &LongCatCompat
		OpenAICompatibleProviders["longcat"] = p
	}
	if p, ok := OpenAICompatibleProviders["grok"]; ok {
		p.Compat = &GrokCompat
		OpenAICompatibleProviders["grok"] = p
	}
	if p, ok := OpenAICompatibleProviders["openrouter"]; ok {
		p.Compat = &OpenRouterCompat
		OpenAICompatibleProviders["openrouter"] = p
	}
	if p, ok := OpenAICompatibleProviders["gemini"]; ok {
		p.Compat = &GeminiCompat
		OpenAICompatibleProviders["gemini"] = p
	}
	for _, id := range []string{"zai_payg", "zai_coding"} {
		if p, ok := OpenAICompatibleProviders[id]; ok {
			p.Compat = &ZAICompat
			OpenAICompatibleProviders[id] = p
		}
	}
	if p, ok := OpenAICompatibleProviders["canopywave"]; ok {
		p.Compat = &CanopyWaveCompat
		OpenAICompatibleProviders["canopywave"] = p
	}
	if p, ok := OpenAICompatibleProviders["poolside"]; ok {
		p.Compat = &PoolsideCompat
		OpenAICompatibleProviders["poolside"] = p
	}
	if p, ok := OpenAICompatibleProviders["groq"]; ok {
		p.Compat = &GroqCompat
		OpenAICompatibleProviders["groq"] = p
	}
	if p, ok := OpenAICompatibleProviders["clinepass"]; ok {
		p.Compat = &ClinePassCompat
		OpenAICompatibleProviders["clinepass"] = p
	}
	if p, ok := OpenAICompatibleProviders["ollama"]; ok {
		p.Compat = &OllamaCompat
		OpenAICompatibleProviders["ollama"] = p
	}
	if p, ok := OpenAICompatibleProviders["opencodego"]; ok {
		p.Compat = &OpenCodeGoCompat
		OpenAICompatibleProviders["opencodego"] = p
	}
	if p, ok := OpenAICompatibleProviders["kimi"]; ok {
		p.Compat = &KimiCompat
		OpenAICompatibleProviders["kimi"] = p
	}
	for _, id := range []string{"xiaomi_mimo", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan"} {
		if p, ok := OpenAICompatibleProviders[id]; ok {
			p.Compat = &XiaomiCompat
			OpenAICompatibleProviders[id] = p
		}
	}
	if p, ok := OpenAICompatibleProviders["deepseek"]; ok {
		p.Compat = &DeepSeekCompat
		OpenAICompatibleProviders["deepseek"] = p
	}
	if p, ok := OpenAICompatibleProviders["concentrate"]; ok {
		p.Compat = &ConcentrateCompat
		OpenAICompatibleProviders["concentrate"] = p
	}
	for _, id := range []string{"minimax_payg", "minimax_token_plan"} {
		if p, ok := OpenAICompatibleProviders[id]; ok {
			p.Compat = &MiniMaxCompat
			OpenAICompatibleProviders[id] = p
		}
	}
	if p, ok := CoreProviders["openai"]; ok {
		p.Compat = &OpenAICompat
		CoreProviders["openai"] = p
	}
	if p, ok := CoreProviders["azure"]; ok {
		p.Compat = &AzureCompat
		CoreProviders["azure"] = p
	}
	if p, ok := CoreProviders["bedrock"]; ok {
		p.Compat = &BedrockCompat
		CoreProviders["bedrock"] = p
	}
	if p, ok := CoreProviders["vertex"]; ok {
		p.Compat = &VertexCompat
		CoreProviders["vertex"] = p
	}
}
