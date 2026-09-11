package runtime

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

func TestSupportsAnthropicCompactionSelection(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     bool
	}{
		{"anthropic", "claude-sonnet-4-6", true},
		{"openrouter", "claude-opus-4-6", true},
		{"anthropic", "claude-haiku-3", false},
		{"openai", "gpt-5", false},
	}
	for _, tt := range tests {
		if got := supportsAnthropicCompactionSelection(tt.provider, tt.model); got != tt.want {
			t.Errorf("supportsAnthropicCompactionSelection(%q, %q) = %v, want %v", tt.provider, tt.model, got, tt.want)
		}
	}
}

func TestAnthropicCompactionMessagesPreservesTools(t *testing.T) {
	messages, system := anthropicCompactionMessages([]client.EyrieMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "assistant", Content: "calling", ToolUse: []client.ToolCall{{ID: "tool-1", Name: "read"}}},
		{Role: "user", ToolResults: []client.ToolResult{{ToolUseID: "tool-1", Content: "result", IsError: true}}},
	})
	if system != "system prompt" {
		t.Fatalf("system = %q", system)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	assistant, ok := messages[0]["content"].([]map[string]any)
	if !ok || len(assistant) != 2 || assistant[1]["type"] != "tool_use" {
		t.Fatalf("assistant content = %#v", messages[0]["content"])
	}
	results, ok := messages[1]["content"].([]map[string]any)
	if !ok || len(results) != 1 || results[0]["is_error"] != true {
		t.Fatalf("tool results = %#v", messages[1]["content"])
	}
}
