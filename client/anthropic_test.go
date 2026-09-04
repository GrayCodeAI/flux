package client

import (
	"testing"
)

// Chat/StreamChat tests live in anthropic_chat_test.go; Ping, error,
// client-config, and feature tests live in anthropic_features_test.go.

// --- buildAnthropicMessages tests ---

func TestAnthropicBuildMessages_TextOnly(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "How are you?"},
	}
	result, system := buildAnthropicMessages(msgs)
	if system != "You are helpful." {
		t.Errorf("expected system to be extracted, got %q", system)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (system excluded), got %d", len(result))
	}
	if result[0]["role"] != "user" {
		t.Errorf("expected first message role=user, got %v", result[0]["role"])
	}
	if result[0]["content"] != "Hello" {
		t.Errorf("expected content=Hello, got %v", result[0]["content"])
	}
	if result[1]["role"] != "assistant" {
		t.Errorf("expected second message role=assistant, got %v", result[1]["role"])
	}
	if result[2]["content"] != "How are you?" {
		t.Errorf("expected last content, got %v", result[2]["content"])
	}
}

func TestAnthropicBuildMessages_ToolUse(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "assistant", Content: "Let me check.", ToolUse: []ToolCall{
			{ID: "call_1", Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}},
		}},
	}
	result, _ := buildAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content to be []map, got %T", result[0]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks (text + tool_use), got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "Let me check." {
		t.Errorf("expected text block, got %v", content[0])
	}
	if content[1]["type"] != "tool_use" {
		t.Errorf("expected tool_use block, got %v", content[1])
	}
	if content[1]["id"] != "call_1" {
		t.Errorf("expected id=call_1, got %v", content[1]["id"])
	}
	if content[1]["name"] != "get_weather" {
		t.Errorf("expected name=get_weather, got %v", content[1]["name"])
	}
}

func TestAnthropicBuildMessages_ToolUseNoText(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "call_2", Name: "read_file", Arguments: map[string]interface{}{"path": "/tmp/x"}},
		}},
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	// No text block, only tool_use
	if len(content) != 1 {
		t.Fatalf("expected 1 content block (tool_use only), got %d", len(content))
	}
	if content[0]["type"] != "tool_use" {
		t.Errorf("expected tool_use, got %v", content[0]["type"])
	}
}

func TestAnthropicBuildMessages_ToolResult(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", ToolResults: []ToolResult{{
			ToolUseID: "call_1",
			Content:   "Temperature: 72F",
			IsError:   false,
		}}},
	}
	result, _ := buildAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content to be []map, got %T", result[0]["content"])
	}
	if content[0]["type"] != "tool_result" {
		t.Errorf("expected tool_result type, got %v", content[0]["type"])
	}
	if content[0]["tool_use_id"] != "call_1" {
		t.Errorf("expected tool_use_id=call_1, got %v", content[0]["tool_use_id"])
	}
	if content[0]["content"] != "Temperature: 72F" {
		t.Errorf("expected tool content, got %v", content[0]["content"])
	}
	// is_error should NOT be present for non-error results
	if _, exists := content[0]["is_error"]; exists {
		t.Errorf("is_error should not be set for non-error result")
	}
}

func TestAnthropicBuildMessages_ToolResultError(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", ToolResults: []ToolResult{{
			ToolUseID: "call_err",
			Content:   "connection refused",
			IsError:   true,
		}}},
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if content[0]["is_error"] != true {
		t.Errorf("expected is_error=true, got %v", content[0]["is_error"])
	}
}

func TestAnthropicBuildMessages_ImageBase64(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Content: "What is this?", Images: []string{
			"data:image/png;base64,iVBORw0KGgoAAAANS",
		}},
	}
	result, _ := buildAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected multi-part content, got %T", result[0]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks (text + image), got %d", len(content))
	}
	if content[0]["type"] != "text" {
		t.Errorf("expected text block first, got %v", content[0]["type"])
	}
	if content[1]["type"] != "image" {
		t.Errorf("expected image block, got %v", content[1]["type"])
	}
	source, ok := content[1]["source"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected source map, got %T", content[1]["source"])
	}
	if source["type"] != "base64" {
		t.Errorf("expected base64 source type, got %v", source["type"])
	}
	if source["media_type"] != "image/png" {
		t.Errorf("expected media_type=image/png, got %v", source["media_type"])
	}
	if source["data"] != "iVBORw0KGgoAAAANS" {
		t.Errorf("expected base64 data, got %v", source["data"])
	}
}

func TestAnthropicBuildMessages_ImageURL(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Content: "Describe this", Images: []string{
			"https://example.com/image.jpg",
		}},
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(content))
	}
	source := content[1]["source"].(map[string]interface{})
	if source["type"] != "url" {
		t.Errorf("expected url source type, got %v", source["type"])
	}
	if source["url"] != "https://example.com/image.jpg" {
		t.Errorf("expected URL, got %v", source["url"])
	}
}

func TestAnthropicBuildMessages_ImageNoText(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Images: []string{"https://example.com/pic.png"}},
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	// Only 1 block (image), no text block since Content is empty
	if len(content) != 1 {
		t.Fatalf("expected 1 block (image only), got %d", len(content))
	}
	if content[0]["type"] != "image" {
		t.Errorf("expected image type, got %v", content[0]["type"])
	}
}

func TestAnthropicBuildMessages_MultipleImages(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Content: "Compare these", Images: []string{
			"data:image/jpeg;base64,/9j/4AAQ",
			"https://example.com/other.png",
		}},
	}
	result, _ := buildAnthropicMessages(msgs)
	content := result[0]["content"].([]map[string]interface{})
	// text + 2 images = 3 blocks
	if len(content) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(content))
	}
	// First image is base64
	src1 := content[1]["source"].(map[string]interface{})
	if src1["type"] != "base64" {
		t.Errorf("first image should be base64, got %v", src1["type"])
	}
	if src1["media_type"] != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %v", src1["media_type"])
	}
	// Second image is URL
	src2 := content[2]["source"].(map[string]interface{})
	if src2["type"] != "url" {
		t.Errorf("second image should be url, got %v", src2["type"])
	}
}

func TestAnthropicBuildMessages_NoSystem(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Content: "Hello"},
	}
	result, system := buildAnthropicMessages(msgs)
	if system != "" {
		t.Errorf("expected empty system, got %q", system)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestAnthropicBuildMessages_EmptyInput(t *testing.T) {
	t.Parallel()
	result, system := buildAnthropicMessages(nil)
	if system != "" {
		t.Errorf("expected empty system, got %q", system)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

// --- convertToAnthropicTools tests ---

func TestAnthropicConvertTools(t *testing.T) {
	t.Parallel()
	tools := []GraycodeRouterTool{
		{
			Name:        "get_weather",
			Description: "Get weather for a city",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []interface{}{"city"},
			},
		},
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				},
			},
		},
	}

	result := convertToAnthropicTools(tools)
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
	if result[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", result[0].Name)
	}
	if result[0].Description != "Get weather for a city" {
		t.Errorf("expected description, got %s", result[0].Description)
	}
	if result[0].InputSchema["type"] != "object" {
		t.Errorf("expected type=object in input_schema, got %v", result[0].InputSchema["type"])
	}
	if result[1].Name != "read_file" {
		t.Errorf("expected read_file, got %s", result[1].Name)
	}
}

func TestAnthropicConvertTools_Empty(t *testing.T) {
	t.Parallel()
	result := convertToAnthropicTools(nil)
	if result != nil {
		t.Errorf("expected nil for empty tools, got %v", result)
	}
	result = convertToAnthropicTools([]GraycodeRouterTool{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

// Helpers
func float64Ptr(f float64) *float64 { return &f }
func intPtr(i int) *int             { return &i }
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
