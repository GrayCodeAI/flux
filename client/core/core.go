// Package core holds the provider contract and the data types shared by
// every layer of the graycode-router client: adapters, middleware, caching, embeddings,
// and the client facade itself.
//
// core is a leaf package — it must not import any other graycode-router/client
// subpackage. The conversation DTOs below are aliases to the canonical
// eagle/llm definitions; core re-exports them so subpackages
// share the contract without an import cycle through the facade. The public
// names remain available as aliases in github.com/GrayCodeAI/graycode-router/client,
// which is the API consumers should keep importing.
//
// See plans/client-package-decomposition.md for the migration plan.
package core

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/llm"
)

// Provider is the core interface for LLM providers.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Chat sends a non-streaming chat request.
	Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error)
	// StreamChat sends a streaming chat request.
	// The caller must call Close() on the returned StreamResult when done.
	StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error)
	// Ping checks connectivity and authentication.
	Ping(ctx context.Context) error
	// Name returns the provider name (e.g. "anthropic", "openai").
	Name() string
}

// GraycodeRouterConfig holds client configuration.
type GraycodeRouterConfig = llm.GraycodeRouterConfig

// ContentPart represents a piece of content in a multi-modal message.
// Use the helper types (TextPart, ImagePart, AudioPart) to construct these.
type ContentPart = llm.ContentPart

// ImageURLPart represents an image content part.
// URL can be an HTTP(S) URL or a data URI (data:image/png;base64,...).
type ImageURLPart = llm.ImageURLPart

// InputAudioPart represents an audio content part (base64 encoded).
type InputAudioPart = llm.InputAudioPart

// GraycodeRouterMessage represents a chat message.
// For simple text messages, set Content directly.
// For multi-modal messages (images, audio), use ContentParts.
// When ContentParts is non-empty, it takes precedence over Content and Images.
// The Images field is retained for backward compatibility.
type GraycodeRouterMessage = llm.GraycodeRouterMessage

// ToolResult represents the result of a tool execution.
type ToolResult = llm.ToolResult

// GraycodeRouterTool represents a tool definition.
type GraycodeRouterTool = llm.GraycodeRouterTool

// GraycodeRouterUsage tracks token usage.
type GraycodeRouterUsage = llm.GraycodeRouterUsage

// GraycodeRouterResponse is the response from a chat call.
type GraycodeRouterResponse = llm.GraycodeRouterResponse

// ToolCall represents a tool invocation.
type ToolCall = llm.ToolCall

// GraycodeRouterStreamEvent is a streaming event.
type GraycodeRouterStreamEvent = llm.GraycodeRouterStreamEvent

// StreamResult wraps a streaming response with cleanup.
// Callers must call Close() when done reading events, or cancel the context.
//
// StreamResult is aliased to the canonical contract type; its Close()
// method and canonical constructor (NewStreamResult) live in
// github.com/GrayCodeAI/graycode-router/llm.
type StreamResult = llm.StreamResult

// ResponseFormat specifies the desired output format for the model response.
type ResponseFormat = llm.ResponseFormat

// ChatOptions holds options for a chat request.
type ChatOptions = llm.ChatOptions

// ToolChoiceOption controls how the model uses tools (Anthropic).
type ToolChoiceOption = llm.ToolChoiceOption

// ContinuationConfig controls output continuation behavior.
type ContinuationConfig = llm.ContinuationConfig

// DefaultContinuationConfig returns sensible defaults.
func DefaultContinuationConfig() ContinuationConfig {
	return ContinuationConfig{MaxContinuations: 3, MaxTotalTokens: 32000}
}
