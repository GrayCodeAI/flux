//nolint:errcheck
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// OpenAI StreamChat tests. Split out of openai_test.go for clarity.
// --- TestOpenAIStreamChat ---

func TestOpenAIStreamChat_Success(t *testing.T) {
	t.Parallel()
	sseData := []string{
		`data: {"id":"chatcmpl-stream","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-stream","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-stream","choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-stream","choices":[{"delta":{},"finish_reason":"stop"}]}`,
		"",
		"data: [DONE]",
		"",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["stream"] != true {
			t.Error("expected stream=true in request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for _, line := range sseData {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	sr, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	if sr.RequestID != "req-stream" {
		t.Errorf("unexpected request_id: %s", sr.RequestID)
	}

	var content strings.Builder
	var gotDone bool
	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			content.WriteString(evt.Content)
		case "done":
			gotDone = true
			if evt.StopReason != "stop" {
				t.Errorf("expected stop_reason=stop, got %s", evt.StopReason)
			}
		case "error":
			t.Errorf("unexpected error event: %s", evt.Error)
		}
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", content.String())
	}
}

func TestOpenAIStreamChat_ToolCalls(t *testing.T) {
	t.Parallel()
	sseData := []string{
		`data: {"id":"chatcmpl-tc","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\""}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"main.go\"}"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-tc","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-stream-tc")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for _, line := range sseData {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	sr, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var toolCalls []ToolCall
	var gotDone bool
	for evt := range sr.Events {
		switch evt.Type {
		case "tool_call":
			if evt.ToolCall != nil {
				toolCalls = append(toolCalls, *evt.ToolCall)
			}
		case "done":
			gotDone = true
		case "error":
			t.Errorf("unexpected error event: %s", evt.Error)
		}
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	tc := toolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("unexpected tool call id: %s", tc.ID)
	}
	if tc.Name != "read_file" {
		t.Errorf("unexpected tool call name: %s", tc.Name)
	}
	if tc.Arguments["path"] != "main.go" {
		t.Errorf("unexpected arguments: %v", tc.Arguments)
	}
}

func TestOpenAIStreamChat_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	sseData := []string{
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"tool_a","arguments":""}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"tool_b","arguments":""}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"y\":2}"}}]},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-mtc","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"",
		"data: [DONE]",
		"",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-multi-tc")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for _, line := range sseData {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	sr, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var toolCalls []ToolCall
	for evt := range sr.Events {
		if evt.Type == "tool_call" && evt.ToolCall != nil {
			toolCalls = append(toolCalls, *evt.ToolCall)
		}
	}
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	names := []string{toolCalls[0].Name, toolCalls[1].Name}
	if !slices.Contains(names, "tool_a") || !slices.Contains(names, "tool_b") {
		t.Errorf("unexpected tool names: %s, %s", toolCalls[0].Name, toolCalls[1].Name)
	}
}

func TestOpenAIStreamChat_WithUsage(t *testing.T) {
	t.Parallel()
	sseData := []string{
		`data: {"id":"chatcmpl-u","choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl-u","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
		"",
		"data: [DONE]",
		"",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify stream_options.include_usage is set when compat supports it
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		so, ok := reqBody["stream_options"]
		if !ok {
			t.Error("expected stream_options in request")
		} else {
			soMap := so.(map[string]interface{})
			if soMap["include_usage"] != true {
				t.Error("expected include_usage=true")
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-usage")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		for _, line := range sseData {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	// Use OpenAICompat which supports usage in streaming
	c := newTestOpenAIClient(srv.URL, &OpenAICompat)
	sr, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var gotUsage bool
	for evt := range sr.Events {
		if evt.Type == "usage" && evt.Usage != nil {
			gotUsage = true
			if evt.Usage.PromptTokens != 5 || evt.Usage.CompletionTokens != 1 {
				t.Errorf("unexpected usage: %+v", evt.Usage)
			}
		}
	}
	if !gotUsage {
		t.Error("expected usage event")
	}
}

func TestOpenAIStreamChat_MissingModel(t *testing.T) {
	t.Parallel()
	c := newTestOpenAIClient("http://localhost", nil)
	_, err := c.StreamChat(context.Background(), basicMessages(), ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAIStreamChat_Error401(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-stream-401")
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"Invalid auth","type":"auth_error"}}`)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	_, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "Invalid auth") {
		t.Errorf("expected auth error, got: %v", err)
	}
}
