package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestNewAnthropicClient(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://custom.proxy.com")
	if c == nil {
		t.Fatal("NewAnthropicClient returned nil")
	}
	if c.apiKey != "sk-ant-key" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != "https://custom.proxy.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.version != "2023-06-01" {
		t.Errorf("version = %q", c.version)
	}
}

func TestNewAnthropicClient_EmptyBaseURL(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "")
	if c.baseURL != "https://api.anthropic.com" {
		t.Errorf("baseURL = %q, want https://api.anthropic.com", c.baseURL)
	}
}

func TestAnthropicClient_Name(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	if c.Name() != "anthropic" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestAnthropicClient_Chat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "Hello from Anthropic!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Anthropic!" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("finishReason = %q", resp.FinishReason)
	}
}

func TestAnthropicClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAnthropicClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "permission denied"},
		}), nil
	})
	c := NewAnthropicClient("bad-key", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error for forbidden")
	}
}

func TestAnthropicClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello stream!\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewAnthropicClient("key", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var content string
	for evt := range result.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}
	if content != "Hello stream!" {
		t.Errorf("content = %q", content)
	}
}

func TestAnthropicClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAnthropicClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})
	c := NewAnthropicClient("key", "https://api.anthropic.com")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAnthropicClient_Ping_AuthError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
	})
	c := NewAnthropicClient("bad-key", "")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestAnthropicClient_CountTokens_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"input_tokens": 42}), nil
	})
	c := NewAnthropicClient("key", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.CountTokens(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Count me"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("CountTokens: %v", err)
	}
	if result.InputTokens != 42 {
		t.Errorf("InputTokens = %d", result.InputTokens)
	}
}

func TestAnthropicClient_CountTokens_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	_, err := c.CountTokens(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAnthropicClient_CountTokens_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{
			"error": map[string]string{"message": "invalid model"},
		}), nil
	})
	c := NewAnthropicClient("key", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.CountTokens(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-invalid"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnthropicClient_SetHTTPClientAndRetry(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	c2 := NewAnthropicClient("key2", "")
	c.SetHTTPClient(c2.httpClient)
	if c.httpClient != c2.httpClient {
		t.Error("SetHTTPClient did not replace client")
	}
	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 5}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", c.retry.MaxRetries)
	}
}

func TestConvertToAnthropicTools(t *testing.T) {
	t.Parallel()
	result := ConvertToAnthropicTools([]core.EyrieTool{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]interface{}{"type": "object"}},
	})
	if len(result) != 1 {
		t.Fatalf("len = %d", len(result))
	}
	if result[0].Name != "get_weather" {
		t.Errorf("Name = %q", result[0].Name)
	}
	if result[0].Description != "Get weather" {
		t.Errorf("Description = %q", result[0].Description)
	}
}

func TestConvertToAnthropicTools_Empty(t *testing.T) {
	t.Parallel()
	result := ConvertToAnthropicTools(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseAnthropicResponse(t *testing.T) {
	t.Parallel()
	ar := anthropicResponse{
		ID: "msg_1",
		Content: []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text,omitempty"`
			Thinking  string          `json:"thinking,omitempty"`
			Signature string          `json:"signature,omitempty"`
			Data      string          `json:"data,omitempty"`
			ID        string          `json:"id,omitempty"`
			Name      string          `json:"name,omitempty"`
			Input     json.RawMessage `json:"input,omitempty"`
		}{
			{Type: "text", Text: "Hello"},
			{Type: "thinking", Thinking: "deep thoughts"},
			{Type: "redacted_thinking", Data: "sensitive"},
		},
		StopReason: "end_turn",
		Usage: struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			OutputTokensDetails      struct {
				ThinkingTokens int `json:"thinking_tokens"`
			} `json:"output_tokens_details"`
		}{
			InputTokens: 10, OutputTokens: 20,
			CacheCreationInputTokens: 2, CacheReadInputTokens: 3,
		},
	}
	ar.Usage.OutputTokensDetails.ThinkingTokens = 5

	resp := ParseAnthropicResponse(ar, "req_1", "org_1")
	if resp.Content != "Hello" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Thinking != "deep thoughts" {
		t.Errorf("Thinking = %q", resp.Thinking)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
	if resp.RequestID != "req_1" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
	if resp.OrganizationID != "org_1" {
		t.Errorf("OrganizationID = %q", resp.OrganizationID)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 20 {
		t.Errorf("usage tokens: %+v", resp.Usage)
	}
	if resp.Usage.ThinkingTokens != 5 {
		t.Errorf("ThinkingTokens = %d", resp.Usage.ThinkingTokens)
	}
	if resp.Usage.CacheCreationTokens != 2 {
		t.Errorf("CacheCreationTokens = %d", resp.Usage.CacheCreationTokens)
	}
	if resp.Usage.CacheReadTokens != 3 {
		t.Errorf("CacheReadTokens = %d", resp.Usage.CacheReadTokens)
	}
}

func TestParseAnthropicResponse_ToolUse(t *testing.T) {
	t.Parallel()
	input, _ := json.Marshal(map[string]string{"city": "NYC"})
	ar := anthropicResponse{
		Content: []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text,omitempty"`
			Thinking  string          `json:"thinking,omitempty"`
			Signature string          `json:"signature,omitempty"`
			Data      string          `json:"data,omitempty"`
			ID        string          `json:"id,omitempty"`
			Name      string          `json:"name,omitempty"`
			Input     json.RawMessage `json:"input,omitempty"`
		}{
			{Type: "text", Text: "Let me check"},
			{Type: "tool_use", ID: "tc1", Name: "get_weather", Input: input},
		},
		StopReason: "tool_use",
	}
	resp := ParseAnthropicResponse(ar, "req_2", "")
	if resp.Content != "Let me check" {
		t.Errorf("Content = %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "tc1" {
		t.Errorf("ToolCall.ID = %q", resp.ToolCalls[0].ID)
	}
}

func TestBuildAnthropicMessages_Basic(t *testing.T) {
	t.Parallel()
	msgs, system := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	})
	if system != "Be helpful" {
		t.Errorf("system = %q", system)
	}
	if len(msgs) != 2 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	if msgs[0]["role"] != "user" {
		t.Errorf("msg[0] role = %v", msgs[0]["role"])
	}
	if msgs[0]["content"] != "Hello" {
		t.Errorf("msg[0] content = %v", msgs[0]["content"])
	}
}

func TestBuildAnthropicMessages_ToolUse(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "assistant", Content: "Let me check", ToolUse: []core.ToolCall{{ID: "tu1", Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}}}},
	})
	if len(msgs) != 1 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[1]["type"] != "tool_use" {
		t.Errorf("block type = %v", content[1]["type"])
	}
}

func TestBuildAnthropicMessages_ToolResults(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "user", ToolResults: []core.ToolResult{{ToolUseID: "tu1", Content: "72°F"}}},
	})
	if len(msgs) != 1 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[0]["type"] != "tool_result" {
		t.Errorf("block type = %v", content[0]["type"])
	}
	if content[0]["tool_use_id"] != "tu1" {
		t.Errorf("tool_use_id = %v", content[0]["tool_use_id"])
	}
}

func TestBuildAnthropicMessages_ToolResultWithError(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "user", ToolResults: []core.ToolResult{{ToolUseID: "tu1", Content: "error!", IsError: true}}},
	})
	content := msgs[0]["content"].([]map[string]interface{})
	if content[0]["is_error"] != true {
		t.Errorf("expected is_error=true")
	}
}

func TestBuildAnthropicMessages_ContentParts(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "user", ContentParts: []core.ContentPart{
			{Type: "text", Text: "Describe this image"},
			{Type: "image_url", ImageURL: &core.ImageURLPart{URL: "data:image/png;base64,iVBORw0KGgo="}},
		}},
	})
	if len(msgs) != 1 {
		t.Fatalf("msgs = %d", len(msgs))
	}
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[1]["type"] != "image" {
		t.Errorf("block type = %v", content[1]["type"])
	}
}

func TestBuildAnthropicMessages_ContentParts_Audio(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "user", ContentParts: []core.ContentPart{
			{Type: "input_audio", InputAudio: &core.InputAudioPart{Data: "base64data", Format: "mp3"}},
		}},
	})
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 1 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[0]["type"] != "audio" {
		t.Errorf("block type = %v", content[0]["type"])
	}
}

func TestBuildAnthropicMessages_LegacyImages(t *testing.T) {
	t.Parallel()
	msgs, _ := BuildAnthropicMessages([]core.EyrieMessage{
		{Role: "user", Content: "Check", Images: []string{"data:image/jpeg;base64,/9j/4AAQ=="}},
	})
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[1]["type"] != "image" {
		t.Errorf("block type = %v", content[1]["type"])
	}
}

func TestResolveThinking(t *testing.T) {
	t.Parallel()
	on := true
	off := false
	tests := []struct {
		name   string
		mode   string
		budget int
		enable *bool
		want   *AnthropicThinking
	}{
		{"adaptive", "adaptive", 0, nil, &AnthropicThinking{Type: "adaptive"}},
		{"disabled", "disabled", 0, nil, &AnthropicThinking{Type: "disabled"}},
		{"enabled", "enabled", 1000, nil, &AnthropicThinking{Type: "adaptive"}},
		{"budget_legacy", "", 500, nil, &AnthropicThinking{Type: "enabled", BudgetTokens: 500}},
		{"unset", "", 0, nil, nil},
		{"thinking_enabled_true", "", 0, &on, &AnthropicThinking{Type: "adaptive"}},
		{"thinking_enabled_false", "", 0, &off, &AnthropicThinking{Type: "disabled"}},
		{"mode_wins_over_toggle", "disabled", 0, &on, &AnthropicThinking{Type: "disabled"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveThinking(core.ChatOptions{
				ThinkingMode: tt.mode, ThinkingBudgetTokens: tt.budget, ThinkingEnabled: tt.enable,
			})
			if tt.want == nil {
				if got != nil {
					t.Errorf("ResolveThinking = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ResolveThinking = nil, want %+v", *tt.want)
				return
			}
			if got.Type != tt.want.Type || got.BudgetTokens != tt.want.BudgetTokens {
				t.Errorf("ResolveThinking = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}

func TestResolveToolChoice(t *testing.T) {
	t.Parallel()
	if ResolveToolChoice(nil) != nil {
		t.Error("expected nil for nil input")
	}
	tc := ResolveToolChoice(&core.ToolChoiceOption{Type: "tool", Name: "get_weather", DisableParallelToolUse: true})
	if tc == nil {
		t.Fatal("expected non-nil")
	}
	if tc.Type != "tool" || tc.Name != "get_weather" || !tc.DisableParallelToolUse {
		t.Errorf("ToolChoice = %+v", *tc)
	}
}

func TestResolveOutputConfig(t *testing.T) {
	t.Parallel()
	if ResolveOutputConfig(core.ChatOptions{}) != nil {
		t.Error("expected nil for empty opts")
	}
	cfg := ResolveOutputConfig(core.ChatOptions{OutputEffort: "high"})
	if cfg == nil || cfg.Effort != "high" {
		t.Errorf("OutputConfig = %+v", cfg)
	}
}

func TestAudioFormatToMediaType(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"wav", "audio/wav"},
		{"mp3", "audio/mpeg"},
		{"flac", "audio/flac"},
		{"ogg", "audio/ogg"},
		{"aac", "audio/aac"},
		{"webm", "audio/webm"},
		{"audio/wav", "audio/wav"},
		{"unknown", "audio/unknown"},
	}
	for _, tt := range tests {
		got := AudioFormatToMediaType(tt.in)
		if got != tt.want {
			t.Errorf("AudioFormatToMediaType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestThinkingForBudget(t *testing.T) {
	t.Parallel()
	if ThinkingForBudget(0) != nil {
		t.Error("expected nil for budget 0")
	}
	tb := ThinkingForBudget(5000)
	if tb == nil || tb.Type != "enabled" || tb.BudgetTokens != 5000 {
		t.Errorf("ThinkingForBudget(5000) = %+v", *tb)
	}
}

func TestBuildAnthropicRequest_ResponseFormat(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	_, _, err := c.BuildAnthropicRequest(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_object"},
	}, false)
	if err == nil {
		t.Fatal("expected error for json_object without schema")
	}
}

func TestBuildAnthropicRequest_ResponseFormatJSONSchema(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	req, _, err := c.BuildAnthropicRequest(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: `{"type":"object"}`},
	}, false)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
}

func TestBuildAnthropicRequest_Headers(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	req, _, err := c.BuildAnthropicRequest(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "claude-sonnet-4-20250514", MaxTokens: 256,
	}, false)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	if req.Header.Get("X-Api-Key") != "sk-ant-key" {
		t.Errorf("X-Api-Key = %q", req.Header.Get("X-Api-Key"))
	}
	if req.Header.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("Anthropic-Version = %q", req.Header.Get("Anthropic-Version"))
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
}

func TestAnthropicClient_MimoAuthRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if req.Header.Get("api-key") != "tp-key" {
				t.Errorf("expected api-key on first call, got %q", req.Header.Get("api-key"))
			}
			return jsonResponse(http.StatusUnauthorized, map[string]any{"error": "invalid"}), nil
		}
		if req.Header.Get("Authorization") != "Bearer tp-key" {
			t.Errorf("expected Bearer on retry, got %q", req.Header.Get("Authorization"))
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "retried"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 1},
		}), nil
	})
	c := NewAnthropicClient("tp-key", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.SetMimoAuth()
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "retried" {
		t.Errorf("content = %q", resp.Content)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestAnthropicClient_Chat_DefaultMaxTokens(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["max_tokens"] != float64(4096) {
			t.Errorf("max_tokens = %v, expected 4096", body["max_tokens"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestAnthropicClient_Chat_ResponseFormatJSONSchema(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["output_config"] == nil {
			t.Error("expected output_config in request")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: `{"type":"object","properties":{"name":{"type":"string"}}}`},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestAnthropicClient_Chat_ResponseFormatEmptySchema(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: ""},
	})
	if err == nil {
		t.Fatal("expected error for empty schema")
	}
}

func TestAnthropicClient_Chat_ResponseFormatInvalidSchema(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: "not valid json"},
	})
	if err == nil {
		t.Fatal("expected error for invalid schema")
	}
}

func TestAnthropicClient_Chat_ResponseFormatJSONObject(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "json_object"},
	})
	if err == nil {
		t.Fatal("expected error for json_object without schema")
	}
}

func TestAnthropicClient_Chat_ResponseFormatUnknown(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		ResponseFormat: &core.ResponseFormat{Type: "unknown_type"},
	})
	if err == nil {
		t.Fatal("expected error for unknown response format")
	}
}

func TestAnthropicClient_Chat_SystemPrompt(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		sys, ok := body["system"].(string)
		if !ok {
			t.Fatalf("system is not a string: %T", body["system"])
		}
		if !strings.Contains(sys, "custom system") {
			t.Errorf("system = %q, expected to contain 'custom system'", sys)
		}
		if !strings.Contains(sys, "from message") {
			t.Errorf("system = %q, expected to contain 'from message'", sys)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{
		{Role: "system", Content: "from message"},
		{Role: "user", Content: "Hi"},
	}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256, System: "custom system"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestAnthropicClient_Chat_EnableCaching(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["max_tokens"] != float64(256) {
			t.Errorf("max_tokens = %v", body["max_tokens"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:         "claude-sonnet-4-20250514",
		MaxTokens:     256,
		EnableCaching: true,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestAnthropicClient_Chat_MetadataUserID(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		meta, ok := body["metadata"].(map[string]interface{})
		if !ok {
			t.Fatal("expected metadata in request")
		}
		if meta["user_id"] != "user-123" {
			t.Errorf("user_id = %v", meta["user_id"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_ant_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})
	c := NewAnthropicClient("sk-ant-key", "https://api.anthropic.com")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      256,
		MetadataUserID: "user-123",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestBuildAnthropicCachedRequest_Basic(t *testing.T) {
	t.Parallel()
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{{Role: "user", Content: "Hello"}},
		"claude-3", 256, nil, false, nil, nil, nil, nil, nil, nil,
	)
	if req["model"] != "claude-3" {
		t.Errorf("model = %v", req["model"])
	}
	if req["max_tokens"] != 256 {
		t.Errorf("max_tokens = %v", req["max_tokens"])
	}
	if req["stream"] != false {
		t.Errorf("stream = %v", req["stream"])
	}
}

func TestBuildAnthropicCachedRequest_WithSystem(t *testing.T) {
	t.Parallel()
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{{Role: "system", Content: "You are helpful"}},
		"claude-3", 256, nil, false, nil, nil, nil, nil, nil, nil,
	)
	sys, ok := req["system"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected system as array of maps")
	}
	if len(sys) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(sys))
	}
	if sys[0]["cache_control"] == nil {
		t.Error("expected cache_control on system")
	}
}

func TestBuildAnthropicCachedRequest_WithTools(t *testing.T) {
	t.Parallel()
	tools := []AnthropicTool{{Name: "test", Description: "a test", InputSchema: map[string]interface{}{"type": "object"}}}
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{{Role: "user", Content: "Hello"}},
		"claude-3", 256, nil, false, tools, nil, nil, nil, nil, nil,
	)
	toolMaps, ok := req["tools"].([]map[string]interface{})
	if !ok {
		t.Fatal("expected tools as array of maps")
	}
	if len(toolMaps) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolMaps))
	}
	if toolMaps[0]["cache_control"] == nil {
		t.Error("expected cache_control on last tool")
	}
}

func TestBuildAnthropicCachedRequest_WithAllOptions(t *testing.T) {
	t.Parallel()
	temp := 0.7
	thinking := &AnthropicThinking{Type: "enabled", BudgetTokens: 1024}
	toolChoice := &AnthropicToolChoice{Type: "auto"}
	topP := 0.9
	topK := 5
	stopSeqs := []string{"\n\n"}
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{{Role: "user", Content: "Hello"}},
		"claude-3", 256, &temp, false, nil, thinking, toolChoice, &topP, &topK, stopSeqs,
	)
	if req["temperature"] != 0.7 {
		t.Errorf("temperature = %v", req["temperature"])
	}
	if req["thinking"] == nil {
		t.Error("expected thinking")
	}
	if req["tool_choice"] == nil {
		t.Error("expected tool_choice")
	}
	if req["top_p"] != 0.9 {
		t.Errorf("top_p = %v", req["top_p"])
	}
	if req["top_k"] != 5 {
		t.Errorf("top_k = %v", req["top_k"])
	}
	if req["stop_sequences"] == nil {
		t.Error("expected stop_sequences")
	}
}

func TestBuildAnthropicCachedRequest_ApplyCacheBreakpoint(t *testing.T) {
	t.Parallel()
	// Test with string content
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{
			{Role: "user", Content: "First"},
			{Role: "user", Content: "Second"},
		},
		"claude-3", 256, nil, false, nil, nil, nil, nil, nil, nil,
	)
	msgs := req["messages"].([]map[string]interface{})
	if len(msgs) < 2 {
		t.Fatal("expected at least 2 messages")
	}
	// Second-to-last message should have cache_control
	firstMsg := msgs[0]
	content := firstMsg["content"]
	contentArr, ok := content.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content as array, got %T", content)
	}
	if contentArr[0]["cache_control"] == nil {
		t.Error("expected cache_control on second-to-last message")
	}
}

func TestBuildAnthropicCachedRequest_ApplyCacheBreakpoint_Maps(t *testing.T) {
	t.Parallel()
	// Use ContentParts to trigger the []map[string]interface{} content path
	req := BuildAnthropicCachedRequest(
		[]core.EyrieMessage{
			{Role: "user", Content: "First", ContentParts: []core.ContentPart{{Type: "text", Text: "First text"}}},
			{Role: "user", Content: "Second"},
		},
		"claude-3", 256, nil, false, nil, nil, nil, nil, nil, nil,
	)
	msgs := req["messages"].([]map[string]interface{})
	if len(msgs) < 2 {
		t.Fatal("expected at least 2 messages")
	}
	// Second-to-last message should have cache_control on its content
	firstMsg := msgs[0]
	content := firstMsg["content"]
	contentArr, ok := content.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content as array, got %T", content)
	}
	if contentArr[0]["cache_control"] == nil {
		t.Error("expected cache_control on content array element")
	}
}
