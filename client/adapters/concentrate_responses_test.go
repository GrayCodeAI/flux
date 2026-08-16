package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestNewConcentrateResponsesClient(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1/")
	if client == nil {
		t.Fatal("NewConcentrateResponsesClient returned nil")
	}
	if client.Name() != "concentrate" {
		t.Errorf("provider name = %q, want concentrate", client.Name())
	}
	if client.BaseURL() != "https://api.concentrate.ai/v1" {
		t.Errorf("base URL = %q", client.BaseURL())
	}
}

func TestNewConcentrateResponsesClient_UsesSharedPooledHTTPClient(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	if client.httpClient.Timeout != core.DefaultTimeout {
		t.Errorf("timeout = %v, want default %v", client.httpClient.Timeout, core.DefaultTimeout)
	}
	if client.httpClient.Transport != core.NewPooledHTTPClient(0).Transport {
		t.Error("client does not use the shared pooled transport")
	}
	if client.Retry().MaxRetries != core.DefaultRetryConfig().MaxRetries {
		t.Error("client does not default to the shared retry config")
	}
}

func TestConcentrateResponsesClient_ChatUsesResponsesContract(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/responses" {
			t.Fatalf("request = %s %s", req.Method, req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer cn-key" {
			t.Fatalf("Authorization = %q", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools := body["tools"].([]interface{})
		tool := tools[0].(map[string]interface{})
		if tool["name"] != "read_file" || tool["type"] != "function" {
			t.Fatalf("tool = %#v", tool)
		}
		if _, legacy := tool["function"]; legacy {
			t.Fatalf("Responses API tool contains Chat Completions wrapper: %#v", tool)
		}

		return jsonResponse(http.StatusOK, map[string]any{
			"id":     "resp_123",
			"object": "response",
			"status": "completed",
			"model":  "gpt-5",
			"output": []map[string]any{{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{{
					"type": "output_text",
					"text": "Hello from Responses API!",
				}},
			}},
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 20, "total_tokens": 30},
		}), nil
	})

	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}
	resp, err := client.Chat(
		context.Background(),
		[]core.EyrieMessage{{Role: "user", Content: "Hi"}},
		core.ChatOptions{
			Model: "gpt-5",
			Tools: []core.EyrieTool{{
				Name: "read_file", Description: "Read a file",
				Parameters: map[string]interface{}{"type": "object"},
			}},
		},
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Responses API!" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.RequestID != "resp_123" || resp.FinishReason != "stop" {
		t.Errorf("response metadata = %+v", resp)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 20 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestConcentrateResponsesClient_BuildRequestPreservesToolTurn(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	req, err := client.buildRequest([]core.EyrieMessage{
		{Role: "user", Content: "Read main.go"},
		{Role: "assistant", ToolUse: []core.ToolCall{{
			ID: "call_1", Name: "read_file", Arguments: map[string]interface{}{"path": "main.go"},
		}}},
		{Role: "user", ToolResults: []core.ToolResult{{
			ToolUseID: "call_1", Content: "package main", IsError: true,
		}}},
	}, core.ChatOptions{
		Model: "gpt-5",
		Tools: []core.EyrieTool{{
			Name: "read_file", Parameters: map[string]interface{}{"type": "object"},
		}},
		ToolChoice: &core.ToolChoiceOption{Type: "tool", Name: "read_file", DisableParallelToolUse: true},
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	input := req.Input.([]map[string]interface{})
	if got := input[1]["type"]; got != "function_call" {
		t.Fatalf("assistant tool item type = %v", got)
	}
	if got := input[1]["call_id"]; got != "call_1" {
		t.Fatalf("assistant call_id = %v", got)
	}
	if got := input[2]["type"]; got != "function_call_output" {
		t.Fatalf("tool result type = %v", got)
	}
	if got := input[2]["is_error"]; got != true {
		t.Fatalf("tool result is_error = %v", got)
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Fatal("parallel_tool_calls should be explicitly false")
	}
	choice := req.ToolChoice.(map[string]interface{})
	if choice["type"] != "function" || choice["name"] != "read_file" {
		t.Fatalf("tool_choice = %#v", choice)
	}
}

func TestConcentrateResponsesClient_StructuredOutputUsesTextFormat(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	req, err := client.buildRequest(nil, core.ChatOptions{
		Model: "gpt-5",
		ResponseFormat: &core.ResponseFormat{
			Type:   "json_schema",
			Schema: `{"type":"object","properties":{"answer":{"type":"string"}}}`,
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if req.Text == nil || req.Text.Format["type"] != "json_schema" {
		t.Fatalf("text format = %#v", req.Text)
	}
	if req.Text.Format["name"] != "hawk_response" {
		t.Fatalf("schema name = %#v", req.Text.Format["name"])
	}
	if _, ok := req.Text.Format["schema"].(map[string]interface{}); !ok {
		t.Fatalf("schema = %#v", req.Text.Format["schema"])
	}
}

func TestConcentrateResponsesClient_InvalidSchemaFailsBeforeRequest(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	_, err := client.buildRequest(nil, core.ChatOptions{
		Model:          "gpt-5",
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: "not-json"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "invalid response schema") {
		t.Fatalf("error = %v", err)
	}
}

func TestConcentrateResponsesClient_ChatToolCallUsesCallID(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":     "resp_456",
			"status": "completed",
			"output": []map[string]any{{
				"type":      "function_call",
				"id":        "item_1",
				"call_id":   "call_1",
				"name":      "get_weather",
				"arguments": `{"city":"NYC"}`,
			}},
		}), nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	resp, err := client.Chat(context.Background(), nil, core.ChatOptions{Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %q", resp.FinishReason)
	}
}

func TestConcentrateResponsesClient_StreamTextUsageAndDone(t *testing.T) {
	t.Parallel()
	stream := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\r\n")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(stream)),
		}, nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	result, err := client.StreamChat(context.Background(), nil, core.ChatOptions{Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	var got []core.EyrieStreamEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-result.Events:
			if !ok {
				if len(got) != 3 {
					t.Fatalf("events = %+v", got)
				}
				if got[0].Type != "content" || got[0].Content != "Hello" {
					t.Fatalf("content event = %+v", got[0])
				}
				if got[1].Type != "usage" || got[1].Usage.TotalTokens != 3 {
					t.Fatalf("usage event = %+v", got[1])
				}
				if got[2].Type != "done" || got[2].StopReason != "stop" {
					t.Fatalf("done event = %+v", got[2])
				}
				return
			}
			got = append(got, event)
		case <-timeout:
			t.Fatal("timed out waiting for stream")
		}
	}
}

func TestConcentrateResponsesClient_StreamToolCall(t *testing.T) {
	t.Parallel()
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_7","name":"read_file"}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"path\":"}`,
		``,
		`data: {"type":"response.function_call_arguments.done","output_index":0,"call_id":"call_7","name":"read_file","arguments":"{\"path\":\"main.go\"}"}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		``,
	}, "\n")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(stream)), Header: http.Header{}}, nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	result, err := client.StreamChat(context.Background(), nil, core.ChatOptions{Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	var got []core.EyrieStreamEvent
	for event := range result.Events {
		got = append(got, event)
	}
	if len(got) != 2 || got[0].Type != "tool_call" || got[0].ToolCall == nil {
		t.Fatalf("events = %+v", got)
	}
	if got[0].ToolCall.ID != "call_7" || got[0].ToolCall.Name != "read_file" || got[0].ToolCall.Arguments["path"] != "main.go" {
		t.Fatalf("tool call = %+v", got[0].ToolCall)
	}
	if got[1].Type != "done" || got[1].StopReason != "tool_calls" {
		t.Fatalf("done = %+v", got[1])
	}
}

func TestConcentrateResponsesClient_PingDoesNotRequireAuth(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/models" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("public models request has Authorization = %q", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{"data": []any{}}), nil
	})
	client := NewConcentrateResponsesClient("", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConcentrateResponsesClient_ChatRetriesOn500ThenSucceeds(t *testing.T) {
	t.Parallel()
	attempts := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return jsonResponse(http.StatusInternalServerError, map[string]any{
				"error": map[string]string{"message": "upstream exploded"},
			}), nil
		}
		// The retried request must still carry the full body (GetBody path).
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode retried body: %v", err)
		}
		if body["model"] != "gpt-5" {
			t.Fatalf("retried body = %#v", body)
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id":     "resp_retry",
			"status": "completed",
			"output": []map[string]any{{
				"type": "message", "role": "assistant",
				"content": []map[string]any{{"type": "output_text", "text": "ok"}},
			}},
		}), nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}
	client.SetRetry(core.NewRetryConfig(2, time.Millisecond, 2*time.Millisecond, 500))

	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-5"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2 (one 500 + one success)", attempts)
	}
}

func TestConcentrateResponsesClient_ChatErrorIsStructuredEyrieError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		resp := jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "invalid_api_key", "message": "bad key"},
		})
		resp.Header.Set("X-Request-Id", "req_abc")
		return resp, nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}
	client.SetRetry(core.RetryConfig{}) // no retries: classify the terminal error

	_, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-5"})
	if err == nil {
		t.Fatal("expected error")
	}
	var eyrieErr *core.EyrieError
	if !errors.As(err, &eyrieErr) {
		t.Fatalf("error is %T, want *core.EyrieError (%v)", err, err)
	}
	if eyrieErr.Provider != "concentrate" || eyrieErr.Op != "chat" {
		t.Fatalf("provider/op = %s/%s", eyrieErr.Provider, eyrieErr.Op)
	}
	if eyrieErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", eyrieErr.StatusCode)
	}
	if !eyrieErr.IsAuthError() {
		t.Error("IsAuthError() = false, want true")
	}
	if eyrieErr.IsRetriable() {
		t.Error("IsRetriable() = true for 401, want false")
	}
	if eyrieErr.RequestID != "req_abc" {
		t.Errorf("request id = %q, want req_abc", eyrieErr.RequestID)
	}
	if !strings.Contains(eyrieErr.Message, "bad key") {
		t.Errorf("message = %q, want it to carry the provider detail", eyrieErr.Message)
	}
}

func TestConcentrateResponsesClient_StreamErrorIsStructuredEyrieError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		resp := jsonResponse(http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{"type": "rate_limit_error", "message": "slow down"},
		})
		resp.Header.Set("X-Request-Id", "req_429")
		return resp, nil
	})
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}
	client.SetRetry(core.RetryConfig{}) // no retries: classify the terminal error

	_, err := client.StreamChat(context.Background(), nil, core.ChatOptions{Model: "gpt-5"})
	if err == nil {
		t.Fatal("expected error")
	}
	var eyrieErr *core.EyrieError
	if !errors.As(err, &eyrieErr) {
		t.Fatalf("error is %T, want *core.EyrieError (%v)", err, err)
	}
	if eyrieErr.Op != "stream" {
		t.Fatalf("op = %s, want stream", eyrieErr.Op)
	}
	if eyrieErr.StatusCode != http.StatusTooManyRequests || !eyrieErr.IsRateLimited() || !eyrieErr.IsRetriable() {
		t.Fatalf("status = %d (rate-limited=%v, retriable=%v), want 429/true/true",
			eyrieErr.StatusCode, eyrieErr.IsRateLimited(), eyrieErr.IsRetriable())
	}
	if eyrieErr.RequestID != "req_429" {
		t.Errorf("request id = %q, want req_429", eyrieErr.RequestID)
	}
}

// The adapter must not impose a short whole-response timeout: a stream whose
// wall time exceeds the previous hard-coded 120s-class wiring still delivers
// every event plus the terminal done. Gaps are kept small so the test stays
// fast; the default pooled client (core.DefaultTimeout) is exercised as-is.
func TestConcentrateResponsesClient_StreamSurvivesSlowServer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req_slow")
		flusher := w.(http.Flusher)
		send := func(payload string) {
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		send(`{"type":"response.output_text.delta","delta":"slow"}`)
		time.Sleep(150 * time.Millisecond)
		send(`{"type":"response.output_text.delta","delta":" but steady"}`)
		time.Sleep(150 * time.Millisecond)
		send(`{"type":"response.completed","response":{"status":"completed"}}`)
	}))
	defer server.Close()

	client := NewConcentrateResponsesClient("cn-key", server.URL)
	result, err := client.StreamChat(context.Background(), nil, core.ChatOptions{Model: "gpt-5"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content strings.Builder
	sawDone := false
	for evt := range result.Events {
		switch evt.Type {
		case "content":
			content.WriteString(evt.Content)
		case "done":
			sawDone = true
		}
	}
	if got := content.String(); got != "slow but steady" {
		t.Fatalf("content = %q", got)
	}
	if !sawDone {
		t.Fatal("stream never delivered the terminal done event")
	}
	if result.RequestID != "req_slow" {
		t.Fatalf("stream request id = %q, want req_slow", result.RequestID)
	}
}

func TestNormalizeToolParamsDoesNotMutateCallerMap(t *testing.T) {
	t.Parallel()
	original := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"path": map[string]interface{}{"type": "string"}},
	}
	normalized := normalizeToolParams(original)
	if _, has := original["additionalProperties"]; has {
		t.Fatal("caller's parameter map was mutated in place")
	}
	if got := normalized["additionalProperties"]; got != false {
		t.Fatalf("normalized additionalProperties = %#v, want false", got)
	}
	if _, ok := normalized["properties"].(map[string]interface{}); !ok {
		t.Fatalf("normalized properties = %#v, want the original nested schema", normalized["properties"])
	}

	explicit := map[string]interface{}{"type": "object", "additionalProperties": true}
	if got := normalizeToolParams(explicit); got["additionalProperties"] != true {
		t.Fatalf("explicit additionalProperties = %#v, want preserved true", got["additionalProperties"])
	}
}
