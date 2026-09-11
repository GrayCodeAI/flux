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

	"github.com/GrayCodeAI/eyrie/client/adapters"
)

// AnthropicClient Chat and StreamChat tests. Split out of anthropic_test.go for clarity.
// --- AnthropicClient.Chat() tests ---

func TestAnthropicChat_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "sk-test-123" {
			t.Errorf("expected API key header, got %q", r.Header.Get("X-Api-Key"))
		}
		if r.Header.Get("Anthropic-Version") != "2023-06-01" {
			t.Errorf("expected version header, got %q", r.Header.Get("Anthropic-Version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content-type, got %q", r.Header.Get("Content-Type"))
		}

		// Decode request body
		var req adapters.AnthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "claude-sonnet-4-6" {
			t.Errorf("expected model claude-sonnet-4-6, got %s", req.Model)
		}
		if req.MaxTokens != 1024 {
			t.Errorf("expected max_tokens 1024, got %d", req.MaxTokens)
		}
		if req.System != "Be helpful" {
			t.Errorf("expected system prompt, got %q", req.System)
		}

		w.Header().Set("Request-Id", "req-abc-123")
		_, _ = w.Write([]byte(`{"id":"msg_001","content":[{"type":"text","text":"Hello! How can I help?"}],"stop_reason":"end_turn","usage":{"input_tokens":25,"output_tokens":12}}`))
	}))
	defer server.Close()

	client := NewAnthropicClient(
		"sk-test-123", server.URL,
		WithRetry(NewRetryConfig(0, 0, 0)),
	)

	resp, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi there"},
	}, ChatOptions{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		System:    "Be helpful",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello! How can I help?" {
		t.Errorf("expected response text, got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected end_turn, got %s", resp.FinishReason)
	}
	if resp.RequestID != "req-abc-123" {
		t.Errorf("expected request ID, got %s", resp.RequestID)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if resp.Usage.PromptTokens != 25 {
		t.Errorf("expected 25 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 12 {
		t.Errorf("expected 12 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 37 {
		t.Errorf("expected 37 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestAnthropicChat_WithToolCallResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-tool-1")
		// Return a response with tool_use blocks
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   "msg_002",
			"type": "message",
			"content": []map[string]interface{}{
				{"type": "text", "text": "I'll check the weather."},
				{
					"type":  "tool_use",
					"id":    "toolu_01",
					"name":  "get_weather",
					"input": map[string]interface{}{"city": "San Francisco"},
				},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]int{"input_tokens": 50, "output_tokens": 30},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient(
		"sk-test", server.URL,
		WithRetry(NewRetryConfig(0, 0, 0)),
	)
	resp, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "What is the weather in SF?"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "I'll check the weather." {
		t.Errorf("expected text content, got %q", resp.Content)
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("expected tool_use finish reason, got %s", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_01" {
		t.Errorf("expected tool call ID toolu_01, got %s", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", tc.Name)
	}
	if tc.Arguments["city"] != "San Francisco" {
		t.Errorf("expected city=San Francisco, got %v", tc.Arguments["city"])
	}
}

func TestAnthropicChat_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-multi")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   "msg_003",
			"type": "message",
			"content": []map[string]interface{}{
				{
					"type":  "tool_use",
					"id":    "toolu_a",
					"name":  "read_file",
					"input": map[string]interface{}{"path": "/etc/hosts"},
				},
				{
					"type":  "tool_use",
					"id":    "toolu_b",
					"name":  "list_dir",
					"input": map[string]interface{}{"path": "/tmp"},
				},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]int{"input_tokens": 20, "output_tokens": 40},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "read /etc/hosts and list /tmp"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected read_file, got %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[1].Name != "list_dir" {
		t.Errorf("expected list_dir, got %s", resp.ToolCalls[1].Name)
	}
}

func TestAnthropicChat_DefaultMaxTokens(t *testing.T) {
	t.Parallel()
	var capturedBody adapters.AnthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Request-Id", "req-default")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_004",
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"}) // MaxTokens not set
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBody.MaxTokens != 4096 {
		t.Errorf("expected default max_tokens=4096, got %d", capturedBody.MaxTokens)
	}
}

func TestAnthropicChat_ModelRequired(t *testing.T) {
	t.Parallel()
	client := NewAnthropicClient("key", "http://localhost")
	_, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{}) // No model set
	if err == nil {
		t.Fatal("expected error when model is empty")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected model required error, got: %v", err)
	}
}

func TestAnthropicChat_SystemMerge(t *testing.T) {
	t.Parallel()
	var capturedBody adapters.AnthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Request-Id", "req-sys")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_005",
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "system", Content: "From messages"},
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6", System: "From opts"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// System from opts + system from message should be merged
	if !strings.Contains(capturedBody.System, "From opts") {
		t.Errorf("expected opts system in merged system, got %q", capturedBody.System)
	}
	if !strings.Contains(capturedBody.System, "From messages") {
		t.Errorf("expected message system in merged system, got %q", capturedBody.System)
	}
}

func TestAnthropicChat_WithTools(t *testing.T) {
	t.Parallel()
	var capturedBody adapters.AnthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		w.Header().Set("Request-Id", "req-tools")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_006",
			"content":     []map[string]interface{}{{"type": "text", "text": "done"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 30, "output_tokens": 5},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		Model: "claude-sonnet-4-6",
		Tools: []EyrieTool{
			{Name: "calculator", Description: "Math", Parameters: map[string]interface{}{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedBody.Tools) != 1 {
		t.Fatalf("expected 1 tool in request, got %d", len(capturedBody.Tools))
	}
	if capturedBody.Tools[0].Name != "calculator" {
		t.Errorf("expected tool name calculator, got %s", capturedBody.Tools[0].Name)
	}
}

func TestAnthropicChat_CacheUsage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-cache")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_007",
			"content":     []map[string]interface{}{{"type": "text", "text": "cached!"}},
			"stop_reason": "end_turn",
			"usage": map[string]int{
				"input_tokens":                10,
				"output_tokens":               5,
				"cache_creation_input_tokens": 100,
				"cache_read_input_tokens":     50,
			},
		})
	}))
	defer server.Close()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.CacheCreationTokens != 100 {
		t.Errorf("expected 100 cache creation tokens, got %d", resp.Usage.CacheCreationTokens)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected 50 cache read tokens, got %d", resp.Usage.CacheReadTokens)
	}
}

// --- StreamChat tests ---

func TestAnthropicStreamChat_TextContent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Request-Id", "req-stream-1")
		flusher, _ := w.(http.Flusher)

		// message_start with usage
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":15,\"output_tokens\":0}}}\n\n")
		flusher.Flush()

		// content_block_start
		_, _ = fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()

		// text deltas
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n")
		flusher.Flush()

		// content_block_stop
		_, _ = fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		flusher.Flush()

		// message_delta with stop_reason
		_, _ = fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":8}}\n\n")
		flusher.Flush()

		// message_stop
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	sr, err := client.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	if sr.RequestID != "req-stream-1" {
		t.Errorf("expected request ID req-stream-1, got %s", sr.RequestID)
	}

	var content string
	var gotDone bool
	var gotUsage bool
	var stopReason string
	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			content += evt.Content
		case "done":
			gotDone = true
			stopReason = evt.StopReason
		case "usage":
			gotUsage = true
		}
	}
	if content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", content)
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if stopReason != "end_turn" {
		t.Errorf("expected end_turn stop reason, got %q", stopReason)
	}
	if !gotUsage {
		t.Error("expected usage event")
	}
}

func TestAnthropicStreamChat_ToolUse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Request-Id", "req-stream-tool")
		flusher, _ := w.(http.Flusher)

		// message_start
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":20,\"output_tokens\":0}}}\n\n")
		flusher.Flush()

		// Text block
		_, _ = fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Let me check.\"}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		flusher.Flush()

		// Tool use block
		_, _ = fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_stream1\",\"name\":\"get_weather\"}}\n\n")
		flusher.Flush()

		// Tool input deltas (streamed JSON)
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\"\"}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\": \\\"NYC\\\"}\"}}\n\n")
		flusher.Flush()

		// Tool block stop
		_, _ = fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n")
		flusher.Flush()

		// message_delta
		_, _ = fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":25}}\n\n")
		flusher.Flush()

		// message_stop
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := NewAnthropicClient("sk-test", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	sr, err := client.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Weather in NYC?"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var content string
	var toolCalls []ToolCall
	var stopReason string
	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			content += evt.Content
		case "tool_call":
			if evt.ToolCall != nil {
				toolCalls = append(toolCalls, *evt.ToolCall)
			}
		case "done":
			stopReason = evt.StopReason
		}
	}
	if content != "Let me check." {
		t.Errorf("expected 'Let me check.', got %q", content)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "toolu_stream1" {
		t.Errorf("expected tool ID toolu_stream1, got %s", toolCalls[0].ID)
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", toolCalls[0].Name)
	}
	if toolCalls[0].Arguments["city"] != "NYC" {
		t.Errorf("expected city=NYC, got %v", toolCalls[0].Arguments["city"])
	}
	if stopReason != "tool_use" {
		t.Errorf("expected tool_use stop reason, got %q", stopReason)
	}
}

func TestAnthropicStreamChat_ModelRequired(t *testing.T) {
	t.Parallel()
	client := NewAnthropicClient("key", "http://localhost")
	_, err := client.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected error when model is empty")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected model required error, got: %v", err)
	}
}

func TestAnthropicStreamChat_ContextCancel(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Request-Id", "req-cancel")
		flusher, _ := w.(http.Flusher)

		// Send a few events then hang
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n")
		flusher.Flush()

		// Hang to simulate slow response
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	client := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	sr, err := client.StreamChat(ctx, []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var content string
	for evt := range sr.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}
	// Should have received partial content before cancellation
	if content != "partial" {
		t.Errorf("expected 'partial', got %q", content)
	}
}
