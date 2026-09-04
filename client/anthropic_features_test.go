package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/adapters"
)

// Anthropic Ping, error handling, client config, and feature (thinking/tool-choice) tests. Split out of anthropic_test.go for clarity.
// --- Ping tests ---

func TestAnthropicPing_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models for ping, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "valid-key" {
			t.Errorf("expected valid-key, got %q", r.Header.Get("X-Api-Key"))
		}
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "msg_ping", "content": []map[string]interface{}{{"type": "text", "text": "hi"}},
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("valid-key", server.URL)
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestAnthropicPing_InvalidKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"type": "authentication_error", "message": "invalid x-api-key"},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("bad-key", server.URL)
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("expected 'invalid API key' error, got: %v", err)
	}
}

func TestAnthropicPing_NonAuthError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 500 is not treated as auth error by Ping
		w.WriteHeader(500)
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL)
	err := client.Ping(context.Background())
	// Non-401 errors should pass without error in current implementation
	if err != nil {
		t.Fatalf("expected no error for 500 (non-auth), got: %v", err)
	}
}

// --- Error handling tests ---

func TestAnthropicChat_Error401(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-401")
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "authentication_error",
				"message": "invalid x-api-key",
			},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("bad-key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "authentication_error") {
		t.Errorf("expected authentication_error in message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "req-401") {
		t.Errorf("expected request ID in error, got: %v", err)
	}
}

func TestAnthropicChat_Error429_Retry(t *testing.T) {
	t.Parallel()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(429)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{"type": "rate_limit_error", "message": "Too many requests"},
			})
			return
		}
		w.Header().Set("Request-Id", "req-retry-ok")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_retry",
			"content":     []map[string]interface{}{{"type": "text", "text": "finally!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 3},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient(
		"key", server.URL,
		WithRetry(NewRetryConfig(3, 1*time.Millisecond, 10*time.Millisecond, 429, 500, 502, 503)),
	)
	resp, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if resp.Content != "finally!" {
		t.Errorf("expected 'finally!', got %q", resp.Content)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestAnthropicChat_Error500_ExhaustedRetries(t *testing.T) {
	t.Parallel()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(500)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"type": "server_error", "message": "Internal error"},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient(
		"key", server.URL,
		WithRetry(NewRetryConfig(2, 1*time.Millisecond, 5*time.Millisecond, 500)),
	)
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("expected 'max retries' in error, got: %v", err)
	}
	// 1 initial + 2 retries = 3 attempts
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestAnthropicChat_ErrorInvalidJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-bad-json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("this is not json"))
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestAnthropicStreamChat_ErrorResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-stream-err")
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"type":    "invalid_request_error",
				"message": "messages: roles must alternate",
			},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "roles must alternate") {
		t.Errorf("expected roles must alternate error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "req-stream-err") {
		t.Errorf("expected request ID in error, got: %v", err)
	}
}

// --- Client configuration tests ---

func TestAnthropicClient_Name(t *testing.T) {
	t.Parallel()
	client := NewAnthropicClient("key", "")
	if client.Name() != "anthropic" {
		t.Errorf("expected 'anthropic', got %q", client.Name())
	}
}

func TestAnthropicClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	client := NewAnthropicClient("key", "")
	if client.BaseURL() != "https://api.anthropic.com" {
		t.Errorf("expected default base URL, got %q", client.BaseURL())
	}
}

func TestAnthropicClient_CustomBaseURL(t *testing.T) {
	t.Parallel()
	client := NewAnthropicClient("key", "https://custom.proxy.com")
	if client.BaseURL() != "https://custom.proxy.com" {
		t.Errorf("expected custom base URL, got %q", client.BaseURL())
	}
}

func TestAnthropicClient_WithOptions(t *testing.T) {
	t.Parallel()
	customHTTP := &http.Client{Timeout: 30 * time.Second}
	retryConfig := NewRetryConfig(5, 2*time.Second, 60*time.Second, 429)

	client := NewAnthropicClient(
		"key", "",
		WithHTTPClient(customHTTP),
		WithRetry(retryConfig),
	)
	if client.HTTPClient() != customHTTP {
		t.Error("expected custom HTTP client to be set")
	}
	if client.Retry().MaxRetries != 5 {
		t.Errorf("expected 5 max retries, got %d", client.Retry().MaxRetries)
	}
}

// --- parseImageString tests ---

func TestAnthropicParseImageString_Base64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input     string
		mediaType string
		data      string
		isBase64  bool
	}{
		{
			input:     "data:image/png;base64,iVBORw0KGgo=",
			mediaType: "image/png",
			data:      "iVBORw0KGgo=",
			isBase64:  true,
		},
		{
			input:     "data:image/jpeg;base64,/9j/4AAQSkZJRg==",
			mediaType: "image/jpeg",
			data:      "/9j/4AAQSkZJRg==",
			isBase64:  true,
		},
		{
			input:     "data:image/gif;base64,R0lGODlh",
			mediaType: "image/gif",
			data:      "R0lGODlh",
			isBase64:  true,
		},
		{
			input:     "data:image/webp;base64,UklGRl4=",
			mediaType: "image/webp",
			data:      "UklGRl4=",
			isBase64:  true,
		},
	}
	for _, tt := range tests {
		mediaType, data, isBase64 := parseImageString(tt.input)
		if mediaType != tt.mediaType {
			t.Errorf("parseImageString(%q): mediaType=%q, want %q", tt.input, mediaType, tt.mediaType)
		}
		if data != tt.data {
			t.Errorf("parseImageString(%q): data=%q, want %q", tt.input, data, tt.data)
		}
		if isBase64 != tt.isBase64 {
			t.Errorf("parseImageString(%q): isBase64=%v, want %v", tt.input, isBase64, tt.isBase64)
		}
	}
}

func TestAnthropicParseImageString_URL(t *testing.T) {
	t.Parallel()
	tests := []string{
		"https://example.com/image.png",
		"http://localhost:8080/pic.jpg",
		"https://cdn.example.com/path/to/image.webp?w=800",
	}
	for _, url := range tests {
		mediaType, data, isBase64 := parseImageString(url)
		if mediaType != "" {
			t.Errorf("parseImageString(%q): expected empty mediaType, got %q", url, mediaType)
		}
		if data != url {
			t.Errorf("parseImageString(%q): expected data=url, got %q", url, data)
		}
		if isBase64 {
			t.Errorf("parseImageString(%q): expected isBase64=false", url)
		}
	}
}

func TestAnthropicParseImageString_DataURIWithoutBase64(t *testing.T) {
	t.Parallel()
	// data: URI without ;base64, marker should be treated as URL
	input := "data:text/plain,Hello"
	_, data, isBase64 := parseImageString(input)
	if isBase64 {
		t.Error("expected isBase64=false for non-base64 data URI")
	}
	if data != input {
		t.Errorf("expected data to equal input, got %q", data)
	}
}

// --- Temperature tests ---

func TestAnthropicChat_WithTemperature(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Request-Id", "req-temp")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_temp",
			"content":     []map[string]interface{}{{"type": "text", "text": "warm"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	temp := 0.7
	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6", Temperature: &temp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", capturedBody["temperature"])
	}
}

// --- Request body verification tests ---

func TestAnthropicChat_RequestBodyStructure(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Request-Id", "req-body")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_body",
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "test message"},
	}, ChatOptions{Model: "claude-sonnet-4-6", MaxTokens: 2048})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody["model"] != "claude-sonnet-4-6" {
		t.Errorf("expected model in body, got %v", capturedBody["model"])
	}
	if int(capturedBody["max_tokens"].(float64)) != 2048 {
		t.Errorf("expected max_tokens=2048, got %v", capturedBody["max_tokens"])
	}
	msgs, ok := capturedBody["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message in body, got %v", capturedBody["messages"])
	}
}

// --- Conversation with tool round-trip ---

func TestAnthropicChat_FullToolRoundTrip(t *testing.T) {
	t.Parallel()
	callNum := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callNum++
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		w.Header().Set("Request-Id", fmt.Sprintf("req-rt-%d", callNum))

		if callNum == 1 {
			// First call: model wants to use a tool
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "msg_rt1",
				"content": []map[string]interface{}{
					{"type": "tool_use", "id": "toolu_rt", "name": "get_time", "input": map[string]interface{}{}},
				},
				"stop_reason": "tool_use",
				"usage":       map[string]int{"input_tokens": 20, "output_tokens": 15},
			})
		} else {
			// Second call: with tool result, model provides final answer
			// Verify the messages include tool result
			msgs := reqBody["messages"].([]interface{})
			if len(msgs) < 3 {
				t.Errorf("expected at least 3 messages in second call, got %d", len(msgs))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "msg_rt2",
				"content":     []map[string]interface{}{{"type": "text", "text": "It is 3pm."}},
				"stop_reason": "end_turn",
				"usage":       map[string]int{"input_tokens": 40, "output_tokens": 8},
			})
		}
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))

	// First call
	resp1, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "What time is it?"},
	}, ChatOptions{
		Model: "claude-sonnet-4-6",
		Tools: []GraycodeRouterTool{{Name: "get_time", Description: "Get current time", Parameters: map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if len(resp1.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp1.ToolCalls))
	}

	// Second call with tool result
	resp2, err := client.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "What time is it?"},
		{Role: "assistant", ToolUse: resp1.ToolCalls},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_rt", Content: "15:00 UTC"}}},
	}, ChatOptions{
		Model: "claude-sonnet-4-6",
		Tools: []GraycodeRouterTool{{Name: "get_time", Description: "Get current time", Parameters: map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if resp2.Content != "It is 3pm." {
		t.Errorf("expected final answer, got %q", resp2.Content)
	}
	if resp2.FinishReason != "end_turn" {
		t.Errorf("expected end_turn, got %s", resp2.FinishReason)
	}
}

// =============================================================================
// New feature tests
// =============================================================================

func TestResolveThinking_Modes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		opts     ChatOptions
		wantType string
		wantNil  bool
	}{
		{"adaptive", ChatOptions{ThinkingMode: "adaptive"}, "adaptive", false},
		{"disabled", ChatOptions{ThinkingMode: "disabled"}, "disabled", false},
		// Explicit "enabled" maps to adaptive (type:enabled+budget_tokens is
		// deprecated on Claude 4.6 and rejected on 4.7+).
		{"enabled with budget", ChatOptions{ThinkingMode: "enabled", ThinkingBudgetTokens: 10000}, "adaptive", false},
		{"enabled zero budget", ChatOptions{ThinkingMode: "enabled"}, "adaptive", false},
		{"legacy budget", ChatOptions{ThinkingBudgetTokens: 5000}, "enabled", false},
		{"legacy zero", ChatOptions{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveThinking(tt.opts)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil")
			}
			if got.Type != tt.wantType {
				t.Errorf("type = %q, want %q", got.Type, tt.wantType)
			}
		})
	}
}

func TestResolveThinking_Display(t *testing.T) {
	t.Parallel()
	// Display is honored on the legacy fixed-budget path (type:enabled with
	// budget_tokens), which is the only path still carrying a fixed budget.
	got := resolveThinking(ChatOptions{ThinkingBudgetTokens: 5000, ThinkingDisplay: "omitted"})
	if got == nil || got.Type != "enabled" || got.Display != "omitted" {
		t.Fatalf("expected enabled with display=omitted, got %+v", got)
	}
}

func TestResolveToolChoice(t *testing.T) {
	t.Parallel()
	if resolveToolChoice(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
	tc := resolveToolChoice(&ToolChoiceOption{Type: "tool", Name: "search", DisableParallelToolUse: true})
	if tc.Type != "tool" || tc.Name != "search" || !tc.DisableParallelToolUse {
		t.Fatalf("unexpected: %+v", tc)
	}
}

func TestResolveOutputConfig(t *testing.T) {
	t.Parallel()
	if resolveOutputConfig(ChatOptions{}) != nil {
		t.Fatal("expected nil for empty opts")
	}
	cfg := resolveOutputConfig(ChatOptions{OutputEffort: "high"})
	if cfg.Effort != "high" || cfg.Format != nil {
		t.Fatalf("unexpected: %+v", cfg)
	}
	cfg2 := resolveOutputConfig(ChatOptions{OutputSchema: `{"type":"object","properties":{"x":{"type":"string"}}}`})
	if cfg2.Format == nil || cfg2.Format.Type != "json_schema" {
		t.Fatalf("unexpected: %+v", cfg2)
	}
}

func TestAnthropicResponseFormatJSONSchema(t *testing.T) {
	c := NewAnthropicClient("test", "http://example.test")
	_, body, err := c.BuildAnthropicRequest(context.Background(), []GraycodeRouterMessage{{Role: "user", Content: "hi"}}, ChatOptions{
		Model: "claude-test",
		ResponseFormat: &ResponseFormat{
			Type:   "json_schema",
			Schema: `{"type":"object","properties":{"answer":{"type":"string"}}}`,
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"output_config":{"format":{"type":"json_schema"`) {
		t.Fatalf("structured output was not wired into Anthropic request: %s", body)
	}
}

func TestAnthropicResponseFormatRejectsSchemaLessJSON(t *testing.T) {
	c := NewAnthropicClient("test", "http://example.test")
	_, _, err := c.BuildAnthropicRequest(context.Background(), nil, ChatOptions{
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "use json_schema") {
		t.Fatalf("expected actionable json_object error, got %v", err)
	}
}

func TestAnthropicRequest_NewFields(t *testing.T) {
	t.Parallel()
	req := adapters.AnthropicRequest{
		Model:         "claude-sonnet-4-6",
		MaxTokens:     4096,
		TopP:          float64Ptr(0.9),
		TopK:          intPtr(50),
		StopSequences: []string{"STOP"},
		ToolChoice:    &anthropicToolChoice{Type: "any"},
		Thinking:      &anthropicThinking{Type: "adaptive"},
		Metadata:      &anthropicMetadata{UserID: "user-123"},
		ServiceTier:   "standard_only",
		OutputConfig:  &anthropicOutputConfig{Effort: "high"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{`"top_p":0.9`, `"top_k":50`, `"stop_sequences":["STOP"]`, `"tool_choice":{"type":"any"}`, `"thinking":{"type":"adaptive"}`, `"metadata":{"user_id":"user-123"}`, `"service_tier":"standard_only"`, `"output_config":{"effort":"high"}`} {
		if !contains(s, want) {
			t.Errorf("missing %q in JSON: %s", want, s)
		}
	}
}

func TestAnthropicChat_ThinkingBlocksInResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-think-1")
		_, _ = w.Write([]byte(`{"id":"msg_think","content":[{"type":"thinking","thinking":"Let me reason..."},{"type":"text","text":"The answer is 42."}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":20,"output_tokens_details":{"thinking_tokens":10}}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []GraycodeRouterMessage{{Role: "user", Content: "What is the answer?"}}, ChatOptions{
		Model:                "claude-sonnet-4-6",
		ThinkingMode:         "enabled",
		ThinkingBudgetTokens: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "The answer is 42." {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Thinking != "Let me reason..." {
		t.Errorf("thinking = %q", resp.Thinking)
	}
	if resp.Usage.ThinkingTokens != 10 {
		t.Errorf("thinking_tokens = %d", resp.Usage.ThinkingTokens)
	}
}

func TestAnthropicChat_RedactedThinkingSkipped(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"msg_rt","content":[{"type":"redacted_thinking","data":"encrypted"},{"type":"text","text":"Done."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Done." {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Thinking != "" {
		t.Errorf("thinking should be empty for redacted, got %q", resp.Thinking)
	}
}

func TestAnthropicRequest_WithToolChoice(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		tc, ok := body["tool_choice"].(map[string]interface{})
		if !ok {
			t.Errorf("expected tool_choice in request, got %v", body["tool_choice"])
			w.WriteHeader(400)
			return
		}
		if tc["type"] != "tool" || tc["name"] != "search" {
			t.Errorf("unexpected tool_choice: %v", tc)
		}
		_, _ = w.Write([]byte(`{"id":"msg","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{{Role: "user", Content: "search"}}, ChatOptions{
		Model:      "claude-sonnet-4-6",
		ToolChoice: &ToolChoiceOption{Type: "tool", Name: "search"},
		Tools:      []GraycodeRouterTool{{Name: "search", Description: "Search", Parameters: map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnthropicRequest_WithTopPAndStopSequences(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["top_p"] != 0.8 {
			t.Errorf("top_p = %v", body["top_p"])
		}
		stops, ok := body["stop_sequences"].([]interface{})
		if !ok || len(stops) != 1 || stops[0] != "END" {
			t.Errorf("stop_sequences = %v", body["stop_sequences"])
		}
		_, _ = w.Write([]byte(`{"id":"msg","content":[{"type":"text","text":"ok"}],"stop_reason":"stop_sequence","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []GraycodeRouterMessage{{Role: "user", Content: "Go"}}, ChatOptions{
		Model:         "claude-sonnet-4-6",
		TopP:          float64Ptr(0.8),
		StopSequences: []string{"END"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
