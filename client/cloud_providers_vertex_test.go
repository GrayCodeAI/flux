package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Google Vertex AI provider tests. Split out of cloud_providers_test.go for clarity.
// =============================================================================
// Google Vertex AI Provider Tests
// =============================================================================

func newTestVertexClient(serverURL, projectID, region, token string) *VertexClient {
	c := NewVertexClient(projectID, region, token)
	c.SetHTTPClient(&http.Client{})
	c.SetRetry(NewRetryConfig(0, 0, 0))
	return c
}

func TestVertexClient_Name(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("my-project", "us-central1", "test-token")
	if c.Name() != "anthropic-vertex" {
		t.Errorf("expected name 'anthropic-vertex', got %q", c.Name())
	}
}

func TestVertexClient_BaseURL(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("my-project", "us-east1", "token")
	expected := "https://us-east1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east1/publishers/anthropic/models"
	if c.BaseURL() != expected {
		t.Errorf("expected baseURL %q, got %q", expected, c.BaseURL())
	}
}

func TestVertexChat_Success(t *testing.T) {
	t.Parallel()
	var capturedMethod, capturedPath string
	var capturedHeaders http.Header
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedHeaders = r.Header.Clone()

		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Vertex!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  20,
				"output_tokens": 15,
			},
		})
	}))
	defer server.Close()

	c := newTestVertexClient(server.URL, "my-project", "us-central1", "test-bearer-token")

	// Override the baseURL by constructing with server URL host
	// We need to set the URL directly since BaseURL() is computed from projectID/region
	// For testing, we'll monkey-patch the httpClient to redirect to our test server
	originalDo := c.HTTPClient()
	c.SetHTTPClient(&http.Client{
		Transport: &redirectTransport{target: server.URL},
	})
	defer func() { c.SetHTTPClient(originalDo) }()

	// We can't easily redirect because the Vertex client constructs the full URL.
	// Instead, use a test that verifies the body and headers by running against
	// a server that acts as the Vertex endpoint. Since BaseURL() is computed,
	// we test the BuildBody method and headers separately.
	// For an integration-level test, we use a custom approach.

	// Let's test by verifying the buildBody output and header behavior separately.
	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello Vertex"},
	}, ChatOptions{Model: "claude-sonnet-4-6"}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}

	// Verify Anthropic-specific fields
	if bodyMap["anthropic_version"] != "vertex-2023-10-16" {
		t.Errorf("expected anthropic_version 'vertex-2023-10-16', got %v", bodyMap["anthropic_version"])
	}
	if bodyMap["model"] != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %v", bodyMap["model"])
	}
	// stream field is omitted when false (omitempty), which is fine for the API
	if bodyMap["stream"] != nil && bodyMap["stream"] != false {
		t.Errorf("expected stream=false or absent, got %v", bodyMap["stream"])
	}
	maxTok, ok := bodyMap["max_tokens"].(float64)
	if !ok || int(maxTok) != 4096 {
		t.Errorf("expected default max_tokens=4096, got %v", bodyMap["max_tokens"])
	}

	// Suppress unused warnings
	_ = capturedMethod
	_ = capturedPath
	_ = capturedHeaders
}

func TestVertexChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := newTestVertexClient("http://localhost", "proj", "us-central1", "token")
	_, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got: %v", err)
	}
}

func TestVertexBuildBody_WithSystemPrompt(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	system, ok := bodyMap["system"].(string)
	if !ok {
		t.Fatal("expected system field in body")
	}
	if !strings.Contains(system, "You are a helpful assistant.") {
		t.Errorf("expected system prompt in body, got %q", system)
	}
}

func TestVertexBuildBody_SystemMerge(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "system", Content: "From messages"},
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6", System: "From opts"}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	system := bodyMap["system"].(string)
	if !strings.Contains(system, "From opts") {
		t.Errorf("expected opts system, got %q", system)
	}
	if !strings.Contains(system, "From messages") {
		t.Errorf("expected messages system, got %q", system)
	}
}

func TestVertexBuildBody_CustomMaxTokens(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6", MaxTokens: 8192}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	if int(bodyMap["max_tokens"].(float64)) != 8192 {
		t.Errorf("expected max_tokens=8192, got %v", bodyMap["max_tokens"])
	}
}

func TestVertexBuildBody_WithTemperature(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")
	temp := 0.5

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6", Temperature: &temp}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	if bodyMap["temperature"].(float64) != 0.5 {
		t.Errorf("expected temperature=0.5, got %v", bodyMap["temperature"])
	}
}

func TestVertexBuildBody_WithTools(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{
		Model: "claude-sonnet-4-6",
		Tools: []EyrieTool{
			{Name: "search", Description: "Search the web", Parameters: map[string]interface{}{"type": "object"}},
		},
	}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	tools, ok := bodyMap["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools in body")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]interface{})
	if tool["name"] != "search" {
		t.Errorf("expected tool name 'search', got %v", tool["name"])
	}
	if tool["input_schema"] == nil {
		t.Error("expected input_schema in vertex tool")
	}
}

func TestVertexBuildBody_StreamFlag(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"}, true)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	if bodyMap["stream"] != true {
		t.Errorf("expected stream=true, got %v", bodyMap["stream"])
	}
}

func TestVertexBuildBody_ToolResultMessage(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "What is the weather?"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "toolu_1", Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}},
		}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_1", Content: "72F and sunny"}}},
	}, ChatOptions{Model: "claude-sonnet-4-6"}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	msgs := bodyMap["messages"].([]interface{})
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
}

func TestVertexBuildBody_VertexVersionField(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")

	body, err := c.BuildBody([]EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"}, false)
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	// The anthropic_version must be vertex-specific
	version, ok := bodyMap["anthropic_version"].(string)
	if !ok {
		t.Fatal("expected anthropic_version field")
	}
	if version != "vertex-2023-10-16" {
		t.Errorf("expected 'vertex-2023-10-16', got %q", version)
	}
}

func TestVertexChat_SuccessWithFullResponse(t *testing.T) {
	t.Parallel()
	// Test by creating a mock server and using a custom transport
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Bearer auth
		if r.Header.Get("Authorization") != "Bearer vert-token" {
			t.Errorf("expected Bearer vert-token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		// Verify it's POST to rawPredict path
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, ":rawPredict") {
			t.Errorf("expected :rawPredict in path, got %s", r.URL.Path)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Vertex AI!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  25,
				"output_tokens": 10,
			},
		})
	}))
	defer server.Close()

	c := NewVertexClient("test-project", "us-central1", "vert-token")
	c.SetHTTPClient(server.Client())
	c.SetRetry(NewRetryConfig(0, 0, 0))

	// We need to override the region/project so the URL hits our test server
	// The baseURL() method constructs the URL from region/projectID.
	// For testing, we create a custom transport that rewrites the URL.
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})

	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Vertex AI!" {
		t.Errorf("expected 'Hello from Vertex AI!', got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", resp.FinishReason)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.PromptTokens != 25 {
		t.Errorf("expected 25 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.TotalTokens != 35 {
		t.Errorf("expected 35 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestVertexChat_ToolUseResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Let me search for that."},
				{
					"type":  "tool_use",
					"id":    "toolu_vert_1",
					"name":  "web_search",
					"input": map[string]interface{}{"query": "vertex ai pricing"},
				},
			},
			"stop_reason": "tool_use",
			"usage": map[string]interface{}{
				"input_tokens":  30,
				"output_tokens": 20,
			},
		})
	}))
	defer server.Close()

	c := NewVertexClient("proj", "us-central1", "token")
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Search for vertex pricing"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Let me search for that." {
		t.Errorf("expected text content, got %q", resp.Content)
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("expected tool_use finish reason, got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_vert_1" {
		t.Errorf("expected tool ID 'toolu_vert_1', got %q", tc.ID)
	}
	if tc.Name != "web_search" {
		t.Errorf("expected tool name 'web_search', got %q", tc.Name)
	}
	if tc.Arguments["query"] != "vertex ai pricing" {
		t.Errorf("expected query 'vertex ai pricing', got %v", tc.Arguments["query"])
	}
}

func TestVertexChat_ErrorResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"error":{"code":403,"message":"Permission denied"}}`)
	}))
	defer server.Close()

	c := NewVertexClient("proj", "us-central1", "token")
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "vertex") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected vertex error, got: %v", err)
	}
}

func TestVertexStreamChat_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify stream path uses streamRawPredict
		if !strings.Contains(r.URL.Path, ":streamRawPredict") {
			t.Errorf("expected :streamRawPredict in path, got %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Vertex \"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"streaming\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	c := NewVertexClient("proj", "us-central1", "token")
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	sr, err := c.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	var content strings.Builder
	var gotDone bool
	var stopReason string
	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			content.WriteString(evt.Content)
		case "done":
			gotDone = true
			stopReason = evt.StopReason
		}
	}
	if content.String() != "Vertex streaming" {
		t.Errorf("expected 'Vertex streaming', got %q", content.String())
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if stopReason != "end_turn" {
		t.Errorf("expected stop_reason=end_turn, got %q", stopReason)
	}
}

func TestVertexStreamChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "token")
	_, err := c.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got: %v", err)
	}
}

func TestVertexPing_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET for ping, got %s", r.Method)
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	c := NewVertexClient("proj", "us-central1", "token")
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})

	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestVertexPing_InvalidCredentials(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer server.Close()

	c := NewVertexClient("proj", "us-central1", "token")
	c.SetHTTPClient(&http.Client{
		Transport: &vertexRewriteTransport{target: server.URL, originalHost: "us-central1-aiplatform.googleapis.com"},
	})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("expected 'invalid credentials' error, got: %v", err)
	}
}

// vertexRewriteTransport rewrites requests from the Vertex base URL to a test server.
type vertexRewriteTransport struct {
	target       string
	originalHost string
}

func (t *vertexRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.Host, t.originalHost) || strings.Contains(req.URL.Host, t.originalHost) {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(t.target, "http://")
		req.Host = req.URL.Host
	}
	return http.DefaultTransport.RoundTrip(req)
}

// redirectTransport redirects all requests to a target server.
type redirectTransport struct {
	target string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	req.Host = req.URL.Host
	return http.DefaultTransport.RoundTrip(req)
}
