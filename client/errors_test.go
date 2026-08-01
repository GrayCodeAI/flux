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
)

func TestAnthropicClient401(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-401")
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"type": "authentication_error", "message": "invalid api key"},
		})
	}))
	defer server.Close()

	ac := NewAnthropicClient("bad-key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "req-401") {
		t.Errorf("expected request ID in error, got: %v", err)
	}
}

func TestAnthropicClient429WithRetry(t *testing.T) {
	t.Parallel()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			fmt.Fprint(w, `{"error":{"type":"rate_limit","message":"too many requests"}}`)
			return
		}
		w.Header().Set("Request-Id", "req-success")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "msg_ok", "content": []map[string]interface{}{{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	ac := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(3, time.Millisecond, 10*time.Millisecond, 429, 500)))
	resp, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("expected OK, got %s", resp.Content)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestAnthropicClient500(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"type":"server_error","message":"internal server error"}}`)
	}))
	defer server.Close()

	ac := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(1, time.Millisecond, time.Millisecond, 500)))
	_, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("expected max retries error, got: %v", err)
	}
}

func TestAnthropicClientTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer server.Close()

	ac := NewAnthropicClient("key", server.URL, WithTimeout(50*time.Millisecond), WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAnthropicClientMalformedJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `{invalid json`)
	}))
	defer server.Close()

	ac := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestAnthropicClientContextCancelled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ac := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	_, err := ac.Chat(ctx, []EyrieMessage{
		{Role: "user", Content: "Hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestEyrieErrorStructure(t *testing.T) {
	t.Parallel()
	err := &EyrieError{
		Provider:   "anthropic",
		Op:         "chat",
		StatusCode: 429,
		RequestID:  "req-123",
		Message:    "rate limited",
	}
	if !err.IsRetriable() {
		t.Error("429 should be retriable")
	}
	if !err.IsRateLimited() {
		t.Error("429 should be rate limited")
	}
	if err.IsAuthError() {
		t.Error("429 should not be auth error")
	}

	authErr := &EyrieError{StatusCode: 401}
	if !authErr.IsAuthError() {
		t.Error("401 should be auth error")
	}
	if authErr.IsRetriable() {
		t.Error("401 should not be retriable")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "anthropic") || !strings.Contains(errStr, "429") || !strings.Contains(errStr, "req-123") {
		t.Errorf("error string missing expected fields: %s", errStr)
	}
}

func TestAnthropicToolCallParsing(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   "msg_123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "I'll help you."},
				{"type": "tool_use", "id": "tool_1", "name": "read_file", "input": json.RawMessage(`{"path":"/tmp/test.go"}`)},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 20},
		})
	}))
	defer server.Close()

	ac := NewAnthropicClient("key", server.URL, WithRetry(NewRetryConfig(0, 0, 0)))
	resp, err := ac.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Read file"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "I'll help you." {
		t.Errorf("expected text content, got: %s", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "read_file" {
		t.Errorf("expected read_file, got %s", tc.Name)
	}
	if tc.ID != "tool_1" {
		t.Errorf("expected tool_1, got %s", tc.ID)
	}
	if tc.Arguments["path"] != "/tmp/test.go" {
		t.Errorf("expected /tmp/test.go, got %v", tc.Arguments["path"])
	}
}

func TestStreamParsingEdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("empty events ignored", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("\n\n")) // empty event
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()

		oc := NewOpenAIClient("key", server.URL, &OpenAICompat)
		sr, err := oc.StreamChat(context.Background(), []EyrieMessage{
			{Role: "user", Content: "Hi"},
		}, ChatOptions{Model: "gpt-4o"})
		if err != nil {
			t.Fatal(err)
		}
		defer sr.Close()
		var content string
		for evt := range sr.Events {
			if evt.Type == "content" {
				content += evt.Content
			}
		}
		if content != "ok" {
			t.Errorf("expected 'ok', got %q", content)
		}
	})

	t.Run("multiline data field", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"line1\"}}]}\n\n"))
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"line2\"}}]}\n\n"))
			w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()

		oc := NewOpenAIClient("key", server.URL, &OpenAICompat)
		sr, err := oc.StreamChat(context.Background(), []EyrieMessage{
			{Role: "user", Content: "Hi"},
		}, ChatOptions{Model: "gpt-4o"})
		if err != nil {
			t.Fatal(err)
		}
		defer sr.Close()
		var content string
		for evt := range sr.Events {
			if evt.Type == "content" {
				content += evt.Content
			}
		}
		if content != "line1line2" {
			t.Errorf("expected 'line1line2', got %q", content)
		}
	})
}
