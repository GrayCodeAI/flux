package tools

import "encoding/json"

// ToolCall represents a provider-neutral tool invocation contract.
type ToolCall struct {
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	// RawArguments is the exact JSON argument document the provider produced.
	// Arguments is derived from it; when the provider emitted malformed JSON,
	// Arguments is nil and RawArguments preserves the bytes so a host can
	// repair or surface them instead of executing a tool with empty input.
	RawArguments json.RawMessage `json:"raw_arguments,omitempty"`
	// ProviderMetadata carries opaque per-call provider state that must be
	// echoed back verbatim on the next turn (for example a Gemini
	// thoughtSignature). Hosts must not interpret or modify it.
	ProviderMetadata map[string]json.RawMessage `json:"provider_metadata,omitempty"`
}

// ToolResult represents a provider-neutral tool execution result contract.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}
