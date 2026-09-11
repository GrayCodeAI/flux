//nolint:errcheck
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

// StreamChat tests live in openai_stream_test.go; Ping, compat, image,
// tool-result, and misc tests live in openai_misc_test.go.

// --- Helpers ---

func newTestOpenAIClient(url string, compat *OpenAICompatConfig) *OpenAIClient {
	rc := NewRetryConfig(0, 10*time.Millisecond, 50*time.Millisecond)
	return NewOpenAIClient("test-key", url, compat, WithRetry(rc))
}

func defaultChatOpts() ChatOptions {
	return ChatOptions{Model: "gpt-4o"}
}

func basicMessages() []EyrieMessage {
	return []EyrieMessage{{Role: "user", Content: "Hello"}}
}

// --- TestOpenAIChat ---

func TestOpenAIChat_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}

		// Verify request body
		var reqBody adapters.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if reqBody.Model != "gpt-4o" {
			t.Errorf("unexpected model: %s", reqBody.Model)
		}
		if reqBody.Stream {
			t.Error("expected stream to be false")
		}

		w.Header().Set("X-Request-Id", "req-123")
		json.NewEncoder(w).Encode(adapters.OpenAIResponse{
			ID: "chatcmpl-abc",
			Choices: []struct {
				Message struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content,omitempty"`
					ToolCalls        []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content,omitempty"`
						ToolCalls        []struct {
							ID       string `json:"id"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls,omitempty"`
					}{Content: "Hello! How can I help you?"},
					FinishReason: "stop",
				},
			},
			Usage: &struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				TotalTokens         int `json:"total_tokens"`
				PromptTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details,omitempty"`
			}{
				PromptTokens: 10, CompletionTokens: 8, TotalTokens: 18,
			},
		})
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	resp, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("unexpected finish_reason: %s", resp.FinishReason)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("unexpected request_id: %s", resp.RequestID)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 8 || resp.Usage.TotalTokens != 18 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOpenAIChat_MissingModel(t *testing.T) {
	t.Parallel()
	c := newTestOpenAIClient("http://localhost", nil)
	_, err := c.Chat(context.Background(), basicMessages(), ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAIChat_WithUsageCacheDetails(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-cached")
		resp := map[string]interface{}{
			"id": "chatcmpl-cached",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
				"prompt_tokens_details": map[string]interface{}{
					"cached_tokens": 50,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	resp, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens=50, got %d", resp.Usage.CacheReadTokens)
	}
}

// --- TestOpenAIChat_ToolCalls ---

func TestOpenAIChat_ToolCallsInResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools are sent in request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		tools, ok := reqBody["tools"].([]interface{})
		if !ok || len(tools) != 1 {
			t.Errorf("expected 1 tool in request, got %v", reqBody["tools"])
		}

		w.Header().Set("X-Request-Id", "req-tools")
		resp := map[string]interface{}{
			"id": "chatcmpl-tools",
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"location":"San Francisco","unit":"celsius"}`,
								},
							},
							{
								"id":   "call_def456",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_time",
									"arguments": `{"timezone":"PST"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens": 50, "completion_tokens": 30, "total_tokens": 80,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	opts := ChatOptions{
		Model: "gpt-4o",
		Tools: []EyrieTool{
			{
				Name:        "get_weather",
				Description: "Get the current weather",
				Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{"location": map[string]interface{}{"type": "string"}}},
			},
		},
	}
	resp, err := c.Chat(context.Background(), basicMessages(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %s", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("unexpected tool call id: %s", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("unexpected tool call name: %s", tc.Name)
	}
	if tc.Arguments["location"] != "San Francisco" {
		t.Errorf("unexpected arguments: %v", tc.Arguments)
	}
	tc2 := resp.ToolCalls[1]
	if tc2.Name != "get_time" {
		t.Errorf("unexpected second tool call name: %s", tc2.Name)
	}
}

// --- TestOpenAIChat_ResponseFormat ---

func TestOpenAIChat_ResponseFormatJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		rf, ok := reqBody["response_format"].(map[string]interface{})
		if !ok {
			t.Fatal("expected response_format in request")
		}
		if rf["type"] != "json_object" {
			t.Errorf("expected type=json_object, got %v", rf["type"])
		}

		w.Header().Set("X-Request-Id", "req-json")
		resp := map[string]interface{}{
			"id": "chatcmpl-json",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": `{"name":"test"}`}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	opts := ChatOptions{
		Model:          "gpt-4o",
		ResponseFormat: &ResponseFormat{Type: "json_object"},
	}
	resp, err := c.Chat(context.Background(), basicMessages(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"name":"test"}` {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestOpenAIChat_ResponseFormatJSONSchema(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		rf, ok := reqBody["response_format"].(map[string]interface{})
		if !ok {
			t.Fatal("expected response_format in request")
		}
		if rf["type"] != "json_schema" {
			t.Errorf("expected type=json_schema, got %v", rf["type"])
		}
		if _, ok := rf["json_schema"]; !ok {
			t.Error("expected json_schema field in response_format")
		}

		w.Header().Set("X-Request-Id", "req-schema")
		resp := map[string]interface{}{
			"id": "chatcmpl-schema",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": `{"result":42}`}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	opts := ChatOptions{
		Model: "gpt-4o",
		ResponseFormat: &ResponseFormat{
			Type:   "json_schema",
			Schema: `{"type":"object","properties":{"result":{"type":"integer"}}}`,
		},
	}
	resp, err := c.Chat(context.Background(), basicMessages(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"result":42}` {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// --- TestOpenAIChat_ErrorHandling ---

func TestOpenAIChat_Error401(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-401")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Incorrect API key provided",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	_, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "openai") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected openai error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Incorrect API key") {
		t.Errorf("expected error message about API key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "req-401") {
		t.Errorf("expected request_id in error, got: %v", err)
	}
}

func TestOpenAIChat_Error429(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-429")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit exceeded",
				"type":    "rate_limit_error",
			},
		})
	}))
	defer srv.Close()

	// Use no-retry config so 429 returns immediately as error
	c := newTestOpenAIClient(srv.URL, nil)
	_, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Errorf("expected rate limit error, got: %v", err)
	}
}

func TestOpenAIChat_Error500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-500")
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"Internal server error","type":"server_error"}}`)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	_, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "Internal server error") {
		t.Errorf("expected server error, got: %v", err)
	}
}

func TestOpenAIChat_ContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err := c.Chat(ctx, basicMessages(), defaultChatOpts())
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
