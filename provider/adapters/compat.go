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
	ThinkingFormat                   string `json:"thinking_format,omitempty"` // "openai", "zai", "qwen", "openrouter"
	// RequiresReasoningPassback instructs buildRequestBase to forward the
	// reasoning_content captured from a prior assistant turn (core.FluxMessage.Thinking)
	// back into the request's assistant messages. DeepSeek requires the assistant's
	// reasoning_content to be passed back whenever that turn performed a tool call,
	// otherwise the API returns HTTP 400. When no tool call happened the field is
	// ignored by the provider, so forwarding is always safe for compliant providers.
	RequiresReasoningPassback bool `json:"requires_reasoning_passback,omitempty"`
	// SupportsCacheRole enables Kimi/Moonshot context-cache injection: when
	// core.ChatOptions.KimiContextCacheID is non-empty, buildRequestBase prepends a
	// {"role":"cache","content":<id>} message per the MoonshotAI-Cookbook spec.
	SupportsCacheRole bool `json:"supports_cache_role,omitempty"`
	// OmitMaxTokens suppresses the max_tokens field so the provider applies
	// its own default. Useful for providers that pre-authorize the maximum
	// token cost (e.g. Agnes AI).
	OmitMaxTokens bool `json:"omit_max_tokens,omitempty"`
	// DefaultDisableThinking sets thinking to disabled when no thinking
	// preference is provided. Useful for providers that enable thinking
	// by default but don't support it in all configurations.
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
		ThinkingFormat: "openrouter", MaxTokensField: "max_tokens",
		SupportsUsageInStreaming: true,
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
	OpenGatewayCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
	}
	OllamaCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
	OpenCodeGoCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
		ThinkingFormat:           "openrouter",
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
		MaxTokensField:         "max_tokens",
		SupportsCacheRole:      true,
		ThinkingFormat:         "kimi",
		DefaultDisableThinking: true,
	}
	XiaomiCompat = OpenAICompatConfig{
		MaxTokensField:         "max_completion_tokens",
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
	// AgnesCompat: OpenAI-compatible; pre-authorizes max token cost, so omit max_tokens.
	AgnesCompat = OpenAICompatConfig{
		OmitMaxTokens:  true,
		ThinkingFormat: "agnes",
	}
	// LongCatCompat: OpenAI-compatible; enables thinking by default, so disable it.
	LongCatCompat = OpenAICompatConfig{
		MaxTokensField:         "max_tokens",
		ThinkingFormat:         "longcat",
		DefaultDisableThinking: true,
	}
	// MiniMaxCompat: OpenAI-compatible; enables thinking by default, so disable it.
	MiniMaxCompat = OpenAICompatConfig{
		MaxTokensField:         "max_tokens",
		ThinkingFormat:         "minimax",
		DefaultDisableThinking: true,
	}
	// DeepSeekCompat: OpenAI-compatible with usage in streaming.
	// DeepSeek requires the assistant's reasoning_content to be passed back whenever
	// that turn performed a tool call (HTTP 400 otherwise), so we forward it.
	// Enables thinking by default, so disable it.
	DeepSeekCompat = OpenAICompatConfig{
		MaxTokensField:            "max_tokens",
		SupportsUsageInStreaming:  true,
		RequiresReasoningPassback: true,
		ThinkingFormat:            "deepseek",
		DefaultDisableThinking:    true,
	}
	ConcentrateCompat = OpenAICompatConfig{
		MaxTokensField:           "max_tokens",
		SupportsUsageInStreaming: true,
	}
	StepFunCompat = OpenAICompatConfig{
		MaxTokensField: "max_tokens",
	}
)

func init() {
	compatible := map[string]*OpenAICompatConfig{
		"grok": &GrokCompat, "openrouter": &OpenRouterCompat, "gemini": &GeminiCompat,
		"zai_payg": &ZAICompat, "zai_coding": &ZAICompat,
		"canopywave": &CanopyWaveCompat, "poolside": &PoolsideCompat,
		"groq": &GroqCompat, "clinepass": &ClinePassCompat,
		"ollama": &OllamaCompat, "opencodego": &OpenCodeGoCompat,
		"kimi":        &KimiCompat,
		"xiaomi_mimo": &XiaomiCompat, "xiaomi_mimo_payg": &XiaomiCompat,
		"xiaomi_mimo_token_plan": &XiaomiCompat,
		"deepseek":               &DeepSeekCompat, "opengateway": &OpenGatewayCompat,
		"longcat":            &LongCatCompat,
		"minimax_token_plan": &MiniMaxCompat, "minimax_payg": &MiniMaxCompat,
		"stepfun": &StepFunCompat, "concentrate": &ConcentrateCompat,
		"agnes": &AgnesCompat,
	}
	for id, compat := range compatible {
		if provider, ok := OpenAICompatibleProviders[id]; ok {
			provider.Compat = compat
			OpenAICompatibleProviders[id] = provider
		}
	}
	core := map[string]*OpenAICompatConfig{
		"openai": &OpenAICompat, "azure": &AzureCompat,
		"bedrock": &BedrockCompat, "vertex": &VertexCompat,
	}
	for id, compat := range core {
		if provider, ok := CoreProviders[id]; ok {
			provider.Compat = compat
			CoreProviders[id] = provider
		}
	}
}
