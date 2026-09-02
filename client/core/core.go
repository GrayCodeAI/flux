// Package core holds the provider contract and the data types shared by
// every layer of the eyrie client: adapters, middleware, caching, embeddings,
// and the client facade itself.
//
// core is a leaf package — it must not import any other eyrie/client
// subpackage. The conversation DTOs below are aliases to the canonical
// eagle/llm definitions; core re-exports them so subpackages
// share the contract without an import cycle through the facade. The public
// names remain available as aliases in github.com/GrayCodeAI/eyrie/client,
// which is the API consumers should keep importing.
//
// See plans/client-package-decomposition.md for the migration plan.
package core

import (
	"context"

	"github.com/GrayCodeAI/eyrie/llm"
)

// Provider is the core interface for LLM providers.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Chat sends a non-streaming chat request.
	Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error)
	// StreamChat sends a streaming chat request.
	// The caller must call Close() on the returned StreamResult when done.
	StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error)
	// Ping checks connectivity and authentication.
	Ping(ctx context.Context) error
	// Name returns the provider name (e.g. "anthropic", "openai").
	Name() string
}

// EyrieConfig holds client configuration.
type EyrieConfig = llm.EyrieConfig

// ContentPart represents a piece of content in a multi-modal message.
// Use the helper types (TextPart, ImagePart, AudioPart) to construct these.
type ContentPart = llm.ContentPart

// ImageURLPart represents an image content part.
// URL can be an HTTP(S) URL or a data URI (data:image/png;base64,...).
type ImageURLPart = llm.ImageURLPart

// InputAudioPart represents an audio content part (base64 encoded).
type InputAudioPart = llm.InputAudioPart

// EyrieMessage represents a chat message.
// For simple text messages, set Content directly.
// For multi-modal messages (images, audio), use ContentParts.
// When ContentParts is non-empty, it takes precedence over Content and Images.
// The Images field is retained for backward compatibility.
type EyrieMessage = llm.EyrieMessage

// ToolResult represents the result of a tool execution.
type ToolResult = llm.ToolResult

// EyrieTool represents a tool definition.
type EyrieTool = llm.EyrieTool

// EyrieUsage tracks token usage.
type EyrieUsage = llm.EyrieUsage

// EyrieResponse is the response from a chat call.
type EyrieResponse = llm.EyrieResponse

// ToolCall represents a tool invocation.
type ToolCall = llm.ToolCall

// EyrieStreamEvent is a streaming event.
type EyrieStreamEvent = llm.EyrieStreamEvent

// StreamResult wraps a streaming response with cleanup.
// Callers must call Close() when done reading events, or cancel the context.
//
// StreamResult is aliased to the canonical contract type; its Close()
// method and canonical constructor (NewStreamResult) live in
// github.com/GrayCodeAI/eyrie/llm.
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
