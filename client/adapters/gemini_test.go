package adapters

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewGeminiClient(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("AIza-test", "https://custom.example/v1beta")
	if c == nil {
		t.Fatal("NewGeminiClient returned nil")
	}
	if c.apiKey != "AIza-test" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != "https://custom.example/v1beta" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}

func TestNewGeminiClient_EmptyBaseURL(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("AIza-test", "")
	if c.baseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}

func TestGeminiClient_Name(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	if c.Name() != "gemini" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestGeminiClient_VertexDetection(t *testing.T) {
	t.Parallel()
	vertex := NewGeminiClient("token", "https://us-central1-aiplatform.googleapis.com/v1beta")
	nonVertex := NewGeminiClient("key", "https://generativelanguage.googleapis.com/v1beta")
	if !vertex.isVertex() {
		t.Error("expected Vertex detection for aiplatform URL")
	}
	if nonVertex.isVertex() {
		t.Error("expected non-Vertex for generativelanguage URL")
	}
}

func TestGeminiClient_Chat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello from Gemini!"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10,"totalTokenCount":15}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("AIza-key", "https://generativelanguage.googleapis.com/v1beta")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Gemini!" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("finishReason = %q", resp.FinishReason)
	}
}

func TestGeminiClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestGeminiClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "API key not valid"},
		}), nil
	})
	c := NewGeminiClient("bad-key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGeminiClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello \"}]}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"stream!\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":5,\"totalTokenCount\":7}}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
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

func TestGeminiClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

// Regression (audit E5): StreamChat captured X-Goog-Request-Id for error
// reporting but passed "" to NewStreamResult, dropping the provider request
// ID on successful streams.
func TestGeminiClient_StreamChat_PropagatesRequestID(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]},\"finishReason\":\"STOP\"}]}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":      []string{"text/event-stream"},
				"X-Goog-Request-Id": []string{"goog-req-42"},
			},
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	for range result.Events {
	}
	if result.RequestID != "goog-req-42" {
		t.Fatalf("stream request id = %q, want goog-req-42", result.RequestID)
	}
}

func TestGeminiClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"models": []map[string]any{}}), nil
	})
	c := NewGeminiClient("AIza-key", "")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestGeminiClient_Ping_AuthError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
	})
	c := NewGeminiClient("bad-key", "")
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestGeminiClient_HTTPClientAndRetry(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	if c.HTTPClient() == nil {
		t.Error("HTTPClient is nil")
	}
	rc := c.Retry()
	if rc.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d", rc.MaxRetries)
	}
}

func TestGeminiClient_ParseResponse(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	data, _ := json.Marshal(map[string]any{
		"candidates": []map[string]any{
			{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "Response"}}},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]int{
			"promptTokenCount": 10, "candidatesTokenCount": 20, "totalTokenCount": 30,
		},
	})
	resp, err := c.parseResponse(data, "req_id")
	if err != nil {
		t.Fatalf("parseResponse: %v", err)
	}
	if resp.Content != "Response" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q", resp.FinishReason)
	}
}

func TestGeminiClient_ParseResponse_NoCandidates(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	data, _ := json.Marshal(map[string]any{})
	_, err := c.parseResponse(data, "req_id")
	if err == nil {
		t.Fatal("expected error for no candidates")
	}
}

func TestGeminiClient_ParseResponse_Blocked(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "")
	data, _ := json.Marshal(map[string]any{
		"promptFeedback": map[string]string{"blockReason": "SAFETY", "blockReasonMessage": "Content blocked"},
	})
	_, err := c.parseResponse(data, "req_id")
	if err == nil {
		t.Fatal("expected error for blocked prompt")
	}
}

func TestMapGeminiFinishReason(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"STOP", "end_turn"},
		{"MAX_TOKENS", "max_tokens"},
		{"SAFETY", "content_filter"},
		{"OTHER", "OTHER"},
		{"", ""},
	}
	for _, tt := range tests {
		got := mapGeminiFinishReason(tt.in)
		if got != tt.want {
			t.Errorf("mapGeminiFinishReason(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGeminiSharedParserEnabled(t *testing.T) {
	t.Parallel()
	os.Unsetenv(geminiSharedParserEnvVar)
	if !geminiSharedParserEnabled() {
		t.Error("expected enabled by default")
	}
	os.Setenv(geminiSharedParserEnvVar, "0")
	if geminiSharedParserEnabled() {
		t.Error("expected disabled for '0'")
	}
	os.Setenv(geminiSharedParserEnvVar, "false")
	if geminiSharedParserEnabled() {
		t.Error("expected disabled for 'false'")
	}
	os.Setenv(geminiSharedParserEnvVar, "no")
	if geminiSharedParserEnabled() {
		t.Error("expected disabled for 'no'")
	}
	os.Unsetenv(geminiSharedParserEnvVar)
}

func TestProcessGeminiStream(t *testing.T) {
	t.Parallel()
	sseEvents := make(chan core.SSEEvent, 3)
	ctx := context.Background()
	events := ProcessGeminiStream(ctx, sseEvents, testLogger(t))
	sseEvents <- core.SSEEvent{Event: "data", Data: `{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`}
	sseEvents <- core.SSEEvent{Event: "data", Data: `{"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":5,"totalTokenCount":7}}`}
	close(sseEvents)

	var content string
	for evt := range events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}
	if content != "Hello world" {
		t.Errorf("content = %q", content)
	}
}

func TestProcessGeminiStream_ToolCall(t *testing.T) {
	t.Parallel()
	sseEvents := make(chan core.SSEEvent, 2)
	ctx := context.Background()
	events := ProcessGeminiStream(ctx, sseEvents, testLogger(t))
	sseEvents <- core.SSEEvent{Event: "data", Data: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"}}}]}}]}`}
	close(sseEvents)

	var toolCalls int
	for evt := range events {
		if evt.Type == "tool_call" && evt.ToolCall != nil {
			toolCalls++
			if evt.ToolCall.Name != "get_weather" {
				t.Errorf("tool name = %q", evt.ToolCall.Name)
			}
		}
	}
	if toolCalls != 1 {
		t.Errorf("tool calls = %d", toolCalls)
	}
}

func TestProcessGeminiStream_ToolCallWithUsage(t *testing.T) {
	t.Parallel()
	sseEvents := make(chan core.SSEEvent, 2)
	ctx := context.Background()
	events := ProcessGeminiStream(ctx, sseEvents, testLogger(t))
	sseEvents <- core.SSEEvent{Event: "data", Data: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"},"id":"call_1"}}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}}`}
	close(sseEvents)

	var toolCalls int
	var usage *core.EyrieUsage
	for evt := range events {
		if evt.Type == "tool_call" && evt.ToolCall != nil {
			toolCalls++
			if evt.ToolCall.ID != "call_1" {
				t.Errorf("tool call id = %q", evt.ToolCall.ID)
			}
		}
		if evt.Type == "done" && evt.Usage != nil {
			usage = evt.Usage
		}
	}
	if toolCalls != 1 {
		t.Errorf("tool calls = %d", toolCalls)
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestProcessGeminiStream_SSEError(t *testing.T) {
	t.Parallel()
	sseEvents := make(chan core.SSEEvent, 1)
	ctx := context.Background()
	events := ProcessGeminiStream(ctx, sseEvents, testLogger(t))
	sseEvents <- core.SSEEvent{Event: "error", Data: "parse error"}
	close(sseEvents)

	gotError := false
	for evt := range events {
		if evt.Type == "error" {
			gotError = true
		}
	}
	if !gotError {
		t.Error("expected error event")
	}
}

func TestGeminiClient_Chat_WithSystem(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["systemInstruction"] == nil {
			t.Error("expected systemInstruction in request body")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Hi"},
	}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %q", resp.Content)
	}
}

func TestGeminiClient_Chat_WithTools(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := body["tools"]; !ok {
			t.Error("expected tools in request body")
		}
		if _, ok := body["toolConfig"]; !ok {
			t.Error("expected toolConfig in request body")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"functionCall": map[string]any{"name": "get_weather", "args": map[string]string{"city": "NYC"}}}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Weather?"}}, core.ChatOptions{
		Model: "gemini-2.0-flash", MaxTokens: 256,
		Tools:      []core.EyrieTool{{Name: "get_weather", Description: "Get weather", Parameters: map[string]interface{}{"type": "object"}}},
		ToolChoice: &core.ToolChoiceOption{Type: "any"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestGeminiClient_Chat_WithResponseFormat(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gc, ok := body["generationConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("expected generationConfig")
		}
		if gc["responseMimeType"] != "application/json" {
			t.Errorf("responseMimeType = %v", gc["responseMimeType"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "json response"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "JSON please"}}, core.ChatOptions{
		Model: "gemini-2.0-flash", MaxTokens: 256,
		ResponseFormat: &core.ResponseFormat{Type: "json_schema", Schema: `{"type":"object"}`},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "json response" {
		t.Errorf("content = %q", resp.Content)
	}
}

func TestGeminiClient_Chat_WithTopLogProbs(t *testing.T) {
	t.Parallel()
	logprobs := 5
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gc, ok := body["generationConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("expected generationConfig")
		}
		if gc["logprobs"] != float64(5) {
			t.Errorf("logprobs = %v", gc["logprobs"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "logprobs done"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:       "gemini-2.0-flash",
		MaxTokens:   256,
		TopLogProbs: &logprobs,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_WithPenalties(t *testing.T) {
	t.Parallel()
	penalty := 0.5
	seed := 42
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gc, ok := body["generationConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("expected generationConfig")
		}
		if gc["presencePenalty"] != 0.5 {
			t.Errorf("presencePenalty = %v", gc["presencePenalty"])
		}
		if gc["seed"] != float64(42) {
			t.Errorf("seed = %v", gc["seed"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "gemini-2.0-flash", MaxTokens: 256,
		PresencePenalty: &penalty,
		Seed:            &seed,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_VertexAuth(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
		}
		if req.Header.Get("x-goog-api-key") != "" {
			t.Error("unexpected x-goog-api-key for Vertex")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "vertex"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("token", "https://us-central1-aiplatform.googleapis.com/v1beta")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "vertex" {
		t.Errorf("content = %q", resp.Content)
	}
}

func TestGeminiClient_Chat_ToolResults(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		contents := body["contents"].([]interface{})
		parts := contents[0].(map[string]interface{})["parts"].([]interface{})
		fr := parts[0].(map[string]interface{})["functionResponse"].(map[string]interface{})
		if fr["name"] != "get_weather" {
			t.Errorf("functionResponse name = %v", fr["name"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "done"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{
		{Role: "user", ToolResults: []core.ToolResult{{ToolUseID: "get_weather", Content: "72°F"}}},
	}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_StreamChat_LegacyParser(t *testing.T) {
	os.Setenv(geminiSharedParserEnvVar, "0")
	defer os.Unsetenv(geminiSharedParserEnvVar)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"legacy \"}]}}]}\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"stream\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1,\"totalTokenCount\":2}}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
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
	if content != "legacy stream" {
		t.Errorf("content = %q", content)
	}
}

func TestGeminiClient_StreamChat_Legacy_InvalidJSON(t *testing.T) {
	os.Setenv(geminiSharedParserEnvVar, "0")
	defer os.Unsetenv(geminiSharedParserEnvVar)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: not valid json\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var gotDone bool
	for evt := range result.Events {
		if evt.Type == "done" {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestGeminiClient_StreamChat_Legacy_NoCandidates(t *testing.T) {
	os.Setenv(geminiSharedParserEnvVar, "0")
	defer os.Unsetenv(geminiSharedParserEnvVar)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[]}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var gotDone bool
	for evt := range result.Events {
		if evt.Type == "done" {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestGeminiClient_StreamChat_Legacy_FunctionCall(t *testing.T) {
	os.Setenv(geminiSharedParserEnvVar, "0")
	defer os.Unsetenv(geminiSharedParserEnvVar)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"get_weather\",\"args\":{\"city\":\"NYC\"},\"id\":\"call_1\"}}]}}]}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var toolCalls int
	for evt := range result.Events {
		if evt.Type == "tool_call" && evt.ToolCall != nil {
			toolCalls++
			if evt.ToolCall.ID != "call_1" {
				t.Errorf("tool call id = %q", evt.ToolCall.ID)
			}
		}
	}
	if toolCalls != 1 {
		t.Errorf("tool calls = %d", toolCalls)
	}
}

func TestGeminiClient_StreamChat_Legacy_NoUsage(t *testing.T) {
	os.Setenv(geminiSharedParserEnvVar, "0")
	defer os.Unsetenv(geminiSharedParserEnvVar)

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]}}]}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var gotContent, gotDone bool
	for evt := range result.Events {
		if evt.Type == "content" {
			gotContent = true
		}
		if evt.Type == "done" {
			gotDone = true
		}
	}
	if !gotContent {
		t.Error("expected content event")
	}
	if !gotDone {
		t.Error("expected done event (from streamLoop fallback)")
	}
}

func TestGeminiClient_Chat_AssistantRole(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		contents := body["contents"].([]interface{})
		if len(contents) < 1 {
			t.Fatal("no contents")
		}
		msg := contents[0].(map[string]interface{})
		if msg["role"] != "model" {
			t.Errorf("role = %v", msg["role"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "assistant", Content: "Hello"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_UnknownRole(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		contents := body["contents"].([]interface{})
		if len(contents) < 1 {
			t.Fatal("no contents")
		}
		msg := contents[0].(map[string]interface{})
		if msg["role"] != "user" {
			t.Errorf("role = %v", msg["role"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "function", Content: "result"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_ContentParts(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		contents := body["contents"].([]interface{})
		msg := contents[0].(map[string]interface{})
		parts := msg["parts"].([]interface{})
		if len(parts) < 3 {
			t.Fatalf("expected 3 parts, got %d", len(parts))
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{
		Role: "user",
		ContentParts: []core.ContentPart{
			{Type: "text", Text: "hello"},
			{Type: "image_url", ImageURL: &core.ImageURLPart{URL: "https://example.com/img.png"}},
			{Type: "input_audio", InputAudio: &core.InputAudioPart{Data: "base64data", Format: "wav"}},
		},
	}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_LegacyImages(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		contents := body["contents"].([]interface{})
		msg := contents[0].(map[string]interface{})
		parts := msg["parts"].([]interface{})
		if len(parts) < 1 {
			t.Fatalf("expected parts")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{
		Role:   "user",
		Images: []string{"base64imagedata"},
	}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_ToolChoiceNone(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["toolConfig"] == nil {
			t.Fatal("expected toolConfig")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:      "gemini-2.0-flash",
		MaxTokens:  256,
		ToolChoice: &core.ToolChoiceOption{Type: "none"},
		Tools:      []core.EyrieTool{{Name: "test", Description: "a test tool"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestGeminiClient_Chat_PenaltiesOnly(t *testing.T) {
	t.Parallel()
	freq := 0.5
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		gc, ok := body["generationConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("expected generationConfig")
		}
		if gc["frequencyPenalty"] != 0.5 {
			t.Errorf("frequencyPenalty = %v", gc["frequencyPenalty"])
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"candidates":    []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "ok"}}}, "finishReason": "STOP"}},
			"usageMetadata": map[string]int{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		}), nil
	})
	c := NewGeminiClient("key", "")
	c.retry = core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}}
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model:            "gemini-2.0-flash",
		MaxTokens:        256,
		FrequencyPenalty: &freq,
		N:                ptrInt(2),
		LogProbs:         ptrBool(true),
		Seed:             ptrInt(42),
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func ptrInt(v int) *int    { return &v }
func ptrBool(v bool) *bool { return &v }

func TestProcessStreamChunk_Content(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent, 1)
	data := `{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`
	cont := c.processStreamChunk(context.Background(), data, events)
	if cont {
		t.Fatal("processStreamChunk returned true (done), expected false")
	}
	select {
	case evt := <-events:
		if evt.Type != "content" || evt.Content != "Hello" {
			t.Errorf("event = %+v", evt)
		}
	default:
		t.Fatal("expected content event")
	}
}

func TestProcessStreamChunk_ToolCall(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent, 1)
	data := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"},"id":"call_1"}}]}}]}`
	cont := c.processStreamChunk(context.Background(), data, events)
	if cont {
		t.Fatal("processStreamChunk returned true (done), expected false")
	}
	select {
	case evt := <-events:
		if evt.Type != "tool_call" || evt.ToolCall == nil || evt.ToolCall.Name != "get_weather" {
			t.Errorf("event = %+v", evt)
		}
	default:
		t.Fatal("expected tool_call event")
	}
}

func TestProcessStreamChunk_UsageDone(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent, 2)
	data := `{"candidates":[{"content":{"parts":[{"text":"Hi"}],"finishReason":"STOP"}}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":5,"totalTokenCount":7}}`
	cont := c.processStreamChunk(context.Background(), data, events)
	if !cont {
		t.Fatal("processStreamChunk returned false, expected true (done)")
	}
	var gotDone bool
	for i := 0; i < 2; i++ {
		select {
		case evt := <-events:
			if evt.Type == "done" {
				gotDone = true
			}
		default:
			return
		}
	}
	if !gotDone {
		t.Fatal("expected done event")
	}
}

func TestProcessStreamChunk_ContextCancelled(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := `{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`
	cont := c.processStreamChunk(ctx, data, events)
	if cont {
		t.Fatal("processStreamChunk returned true, expected false")
	}
}

func TestProcessStreamChunk_EmptyCandidates(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent, 1)
	data := `{"candidates":[]}`
	cont := c.processStreamChunk(context.Background(), data, events)
	if cont {
		t.Fatal("processStreamChunk returned true, expected false")
	}
	select {
	case <-events:
		t.Fatal("expected no events")
	default:
	}
}

func TestProcessStreamChunk_InvalidJSON(t *testing.T) {
	t.Parallel()
	c := NewGeminiClient("key", "https://gemini.example")
	events := make(chan core.EyrieStreamEvent, 1)
	cont := c.processStreamChunk(context.Background(), "not json", events)
	if cont {
		t.Fatal("processStreamChunk returned true, expected false")
	}
}
