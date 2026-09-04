// Package llm is the canonical provider port contract for the hawk ecosystem.
//
// It is the single source of truth for the conversation DTOs and the Provider
// interface that hawk (product face) and graycode-router (provider engine) speak across
// their boundary. Both sides alias to these types, so there is exactly one
// definition of each DTO and no per-call conversion.
//
// hawk owns the product vocabulary (hence names like GraycodeRouterMessage); graycode-router
// implements the port. graycode-router's internal transport types stay graycode-router-scoped and
// never appear here.
package llm

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/tools"
)

// GraycodeRouterConfig holds client configuration.
type GraycodeRouterConfig struct {
	Provider   string `json:"provider,omitempty"`
	APIKey     string `json:"-"`
	BaseURL    string `json:"base_url,omitempty"`
	Model      string `json:"model,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

// ContentPart is a provider-neutral multimodal message part.
type ContentPart struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	ImageURL   *ImageURLPart   `json:"image_url,omitempty"`
	InputAudio *InputAudioPart `json:"input_audio,omitempty"`
}

// ImageURLPart describes an image URL or data URI.
type ImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// InputAudioPart describes base64-encoded audio content.
type InputAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// GraycodeRouterMessage is the provider-neutral conversation message shape.
type GraycodeRouterMessage struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	Thinking     string        `json:"thinking,omitempty"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	Images       []string      `json:"images,omitempty"`
	ToolUse      []ToolCall    `json:"tool_use,omitempty"`
	ToolResults  []ToolResult  `json:"tool_results,omitempty"`
}

// ToolCall is a tool invocation. Aliased to tools.ToolCall so the ecosystem
// has a single ToolCall/ToolResult vocabulary (see tools/tool.go).
type ToolCall = tools.ToolCall

// ToolResult is a tool execution result. Aliased to tools.ToolResult.
type ToolResult = tools.ToolResult

// GraycodeRouterTool is a tool definition.
type GraycodeRouterTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ResponseFormat specifies the desired output format for a model response.
type ResponseFormat struct {
	Type   string `json:"type"`
	Schema string `json:"schema,omitempty"`
}

// ToolChoiceOption controls how the model uses tools.
type ToolChoiceOption struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// ChatOptions holds request options for an engine chat call.
type ChatOptions struct {
	Provider             string               `json:"provider,omitempty"`
	Model                string               `json:"model,omitempty"`
	Temperature          *float64             `json:"temperature,omitempty"`
	MaxTokens            int                  `json:"max_tokens,omitempty"`
	Stream               bool                 `json:"stream,omitempty"`
	Tools                []GraycodeRouterTool `json:"tools,omitempty"`
	System               string               `json:"system,omitempty"`
	EnableCaching        bool                 `json:"enable_caching,omitempty"`
	ResponseFormat       *ResponseFormat      `json:"response_format,omitempty"`
	ReasoningEffort      string               `json:"reasoning_effort,omitempty"`
	ThinkingBudgetTokens int                  `json:"thinking_budget_tokens,omitempty"`
	ThinkingMode         string               `json:"thinking_mode,omitempty"`
	ThinkingDisplay      string               `json:"thinking_display,omitempty"`
	ThinkingEnabled      *bool                `json:"thinking_enabled,omitempty"`
	// GLMThinkingEnabled is a deprecated alias of ThinkingEnabled kept for
	// older Z.AI call sites. New code should set ThinkingEnabled. Adapters
	// accept either field; wire encoding is selected per provider via
	// OpenAICompatConfig.ThinkingFormat ("zai", "longcat", "agnes", …).
	GLMThinkingEnabled *bool             `json:"glm_thinking_enabled,omitempty"`
	VirtualKeyID       string            `json:"virtual_key_id,omitempty"`
	KimiContextCacheID string            `json:"kimi_context_cache_id,omitempty"`
	KimiCacheResetTTL  bool              `json:"kimi_cache_reset_ttl,omitempty"`
	TopP               *float64          `json:"top_p,omitempty"`
	TopK               *int              `json:"top_k,omitempty"`
	StopSequences      []string          `json:"stop_sequences,omitempty"`
	ToolChoice         *ToolChoiceOption `json:"tool_choice,omitempty"`
	MetadataUserID     string            `json:"metadata_user_id,omitempty"`
	ServiceTier        string            `json:"service_tier,omitempty"`
	OutputEffort       string            `json:"output_effort,omitempty"`
	OutputSchema       string            `json:"output_schema,omitempty"`
	PresencePenalty    *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty   *float64          `json:"frequency_penalty,omitempty"`
	N                  *int              `json:"n,omitempty"`
	LogProbs           *bool             `json:"logprobs,omitempty"`
	TopLogProbs        *int              `json:"top_logprobs,omitempty"`
	Seed               *int              `json:"seed,omitempty"`
	Store              *bool             `json:"store,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Modalities         []string          `json:"modalities,omitempty"`
	AudioConfig        string            `json:"audio_config,omitempty"`
	Prediction         string            `json:"prediction,omitempty"`
	WebSearchOptions   string            `json:"web_search_options,omitempty"`
	PromptCacheOptions *PromptCacheOpts  `json:"prompt_cache_options,omitempty"`
	CacheControl       *CacheControl     `json:"cache_control,omitempty"`
	// Responses API extensions (Concentrate)
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Include            []string            `json:"include,omitempty"`
	Routing            *RoutingConfig      `json:"routing,omitempty"`
	MaxToolCalls       *int                `json:"max_tool_calls,omitempty"`
	ContextManagement  []ContextManagement `json:"context_management,omitempty"`
	SafetyIdentifier   *string             `json:"safety_identifier,omitempty"`
	Background         *bool               `json:"background,omitempty"`
	Truncation         *string             `json:"truncation,omitempty"`
	Conversation       interface{}         `json:"conversation,omitempty"`
}

type PromptCacheOpts struct {
	Mode string `json:"mode,omitempty"`
	TTL  string `json:"ttl,omitempty"`
}

type CacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type RoutingConfig struct {
	Model    interface{} `json:"model,omitempty"`
	Strategy string      `json:"strategy,omitempty"` // "min" or "max"
	Metric   string      `json:"metric,omitempty"`   // "cost", "latency", "quality"
}

type ContextManagement struct {
	Type string `json:"type"` // "compact_20260112", "clear_thinking_20251015", etc.
}

// ContinuationConfig controls output continuation behavior.
type ContinuationConfig struct {
	MaxContinuations int
	MaxTotalTokens   int
}

// GraycodeRouterUsage tracks token usage.
type GraycodeRouterUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
	ThinkingTokens      int `json:"thinking_tokens,omitempty"`
}

// ResolvedRoute is the concrete provider/model route selected by the engine.
type ResolvedRoute struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	DeploymentRouting bool   `json:"deployment_routing,omitempty"`
}

// GraycodeRouterResponse is the chat response DTO.
type GraycodeRouterResponse struct {
	Content        string               `json:"content"`
	Thinking       string               `json:"thinking,omitempty"`
	Usage          *GraycodeRouterUsage `json:"usage,omitempty"`
	ToolCalls      []ToolCall           `json:"tool_calls,omitempty"`
	FinishReason   string               `json:"finish_reason"`
	RequestID      string               `json:"request_id,omitempty"`
	OrganizationID string               `json:"organization_id,omitempty"`
	Route          *ResolvedRoute       `json:"route,omitempty"`
}

// GraycodeRouterStreamEvent is a streaming event.
type GraycodeRouterStreamEvent struct {
	Type       string               `json:"type"`
	Content    string               `json:"content,omitempty"`
	ToolCall   *ToolCall            `json:"tool_call,omitempty"`
	Thinking   string               `json:"thinking,omitempty"`
	Error      string               `json:"error,omitempty"`
	Warning    string               `json:"warning,omitempty"`
	RequestID  string               `json:"request_id,omitempty"`
	Usage      *GraycodeRouterUsage `json:"usage,omitempty"`
	StopReason string               `json:"stop_reason,omitempty"`
	// TTFT and TTFTms both carry time-to-first-token in milliseconds but ride
	// different events: the dedicated "ttft" event populates TTFT, while the
	// terminal "done" event populates TTFTms. The engine normalizes the two
	// into a single value (preferring TTFTms, falling back to TTFT). Both are
	// retained for wire compatibility with existing producers/consumers.
	TTFTms int            `json:"ttft_ms,omitempty"`
	TTFT   int            `json:"ttft,omitempty"`
	Route  *ResolvedRoute `json:"route,omitempty"`
}

// StreamResult wraps a streaming response with cleanup. Callers must call Close()
// when done reading events, or cancel the context.
type StreamResult struct {
	Events    <-chan GraycodeRouterStreamEvent
	RequestID string
	cancel    context.CancelFunc
}

// NewStreamResult constructs a stream result. The cancel function is optional
// and must be idempotent.
func NewStreamResult(events <-chan GraycodeRouterStreamEvent, requestID string, cancel context.CancelFunc) *StreamResult {
	return &StreamResult{Events: events, RequestID: requestID, cancel: cancel}
}

// Close stops the stream and releases resources.
func (sr *StreamResult) Close() {
	if sr != nil && sr.cancel != nil {
		sr.cancel()
	}
}
