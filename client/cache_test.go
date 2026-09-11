package client

import (
	"encoding/json"
	"testing"
)

func TestBuildAnthropicCachedRequest_BasicMessages(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	req := buildAnthropicCachedRequest(messages, "claude-sonnet-4-20250514", 4096, nil, false, nil, nil, nil, nil, nil, nil)

	// System should be array with cache_control
	system, ok := req["system"].([]map[string]interface{})
	if !ok || len(system) != 1 {
		t.Fatal("expected system as array with one element")
	}
	if system[0]["cache_control"] == nil {
		t.Fatal("expected cache_control on system")
	}
	if system[0]["text"] != "You are helpful." {
		t.Fatal("system text mismatch")
	}

	// Messages: second-to-last (index 1, assistant) should have cache_control
	msgs := req["messages"].([]map[string]interface{})
	if len(msgs) != 3 { // 3 non-system messages
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Second to last message (index 1 = assistant "Hi there!") should be array with cache_control
	assistantContent, ok := msgs[1]["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected assistant content to be array after cache breakpoint")
	}
	if assistantContent[0]["cache_control"] == nil {
		t.Fatal("expected cache_control on second-to-last message")
	}
}

func TestBuildAnthropicCachedRequest_ToolUsePropagated(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "read file.go"},
		{Role: "assistant", Content: "", ToolUse: []ToolCall{
			{ID: "tc1", Name: "read", Arguments: map[string]interface{}{"path": "file.go"}},
		}},
		{Role: "user", Content: "", ToolResults: []ToolResult{{ToolUseID: "tc1", Content: "package main"}}},
		{Role: "user", Content: "now edit it"},
	}
	req := buildAnthropicCachedRequest(messages, "claude-sonnet-4-20250514", 4096, nil, false, nil, nil, nil, nil, nil, nil)

	msgs := req["messages"].([]map[string]interface{})
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Verify tool_use message (index 1) preserved
	assistantMsg := msgs[1]
	content, ok := assistantMsg["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected assistant tool_use as array content")
	}
	found := false
	for _, block := range content {
		if block["type"] == "tool_use" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tool_use block in assistant message")
	}

	// Verify tool_result message (index 2) is the cached one (second-to-last)
	toolResultMsg := msgs[2]
	trContent, ok := toolResultMsg["content"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected tool_result as array content")
	}
	if trContent[len(trContent)-1]["cache_control"] == nil {
		t.Fatal("expected cache_control on second-to-last message (tool_result)")
	}
}

func TestBuildAnthropicCachedRequest_ToolsAnnotated(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "hello"},
	}
	tools := []anthropicTool{
		{Name: "read", Description: "Read a file", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "write", Description: "Write a file", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "bash", Description: "Run command", InputSchema: map[string]interface{}{"type": "object"}},
	}
	req := buildAnthropicCachedRequest(messages, "claude-sonnet-4-20250514", 4096, nil, false, tools, nil, nil, nil, nil, nil)

	toolMaps, ok := req["tools"].([]map[string]interface{})
	if !ok || len(toolMaps) != 3 {
		t.Fatalf("expected 3 tools, got %v", req["tools"])
	}

	// Only the LAST tool should have cache_control
	if toolMaps[0]["cache_control"] != nil {
		t.Fatal("first tool should not have cache_control")
	}
	if toolMaps[1]["cache_control"] != nil {
		t.Fatal("second tool should not have cache_control")
	}
	if toolMaps[2]["cache_control"] == nil {
		t.Fatal("last tool must have cache_control")
	}
}

func TestCacheUsageParsing(t *testing.T) {
	t.Parallel()
	responseJSON := `{
		"id": "msg_123",
		"content": [{"type": "text", "text": "Hello!"}],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"cache_creation_input_tokens": 1000,
			"cache_read_input_tokens": 800
		}
	}`

	var ar anthropicResponse
	if err := json.Unmarshal([]byte(responseJSON), &ar); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if ar.Usage.CacheCreationInputTokens != 1000 {
		t.Fatalf("expected cache_creation=1000, got %d", ar.Usage.CacheCreationInputTokens)
	}
	if ar.Usage.CacheReadInputTokens != 800 {
		t.Fatalf("expected cache_read=800, got %d", ar.Usage.CacheReadInputTokens)
	}

	// Verify it propagates to EyrieUsage
	usage := &EyrieUsage{
		PromptTokens:        ar.Usage.InputTokens,
		CompletionTokens:    ar.Usage.OutputTokens,
		TotalTokens:         ar.Usage.InputTokens + ar.Usage.OutputTokens,
		CacheCreationTokens: ar.Usage.CacheCreationInputTokens,
		CacheReadTokens:     ar.Usage.CacheReadInputTokens,
	}
	if usage.CacheCreationTokens != 1000 || usage.CacheReadTokens != 800 {
		t.Fatal("cache tokens not propagated correctly")
	}
}

func TestBuildAnthropicCachedRequest_NoSystem(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Bye"},
	}
	req := buildAnthropicCachedRequest(messages, "claude-sonnet-4-20250514", 4096, nil, false, nil, nil, nil, nil, nil, nil)

	if _, ok := req["system"]; ok {
		t.Fatal("should not have system key when no system message")
	}
}

func TestBuildAnthropicCachedRequest_StreamFlag(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
	}
	req := buildAnthropicCachedRequest(messages, "claude-sonnet-4-20250514", 4096, nil, true, nil, nil, nil, nil, nil, nil)
	if req["stream"] != true {
		t.Fatal("expected stream=true")
	}
}
