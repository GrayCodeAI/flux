package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/adapters"
)

// Vertex AI provider tests live in cloud_providers_vertex_test.go and
// AWS Bedrock provider tests live in cloud_providers_bedrock_test.go.

// =============================================================================
// Azure OpenAI Provider Tests
// =============================================================================

func newTestAzureClient(serverURL string) *AzureClient {
	c := NewAzureClient("test-api-key", serverURL, "2024-08-01-preview")
	c.SetHTTPClient(&http.Client{})
	c.SetRetry(NewRetryConfig(0, 0, 0))
	return c
}

func TestAzureClient_Name(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "")
	if c.Name() != "azure" {
		t.Errorf("expected name 'azure', got %q", c.Name())
	}
}

func TestAzureClient_DefaultAPIVersion(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "")
	if c.APIVersion() != "2024-10-21" {
		t.Errorf("expected default api-version '2024-10-21', got %q", c.APIVersion())
	}
}

func TestAzureClient_CustomAPIVersion(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "2024-10-01-preview")
	if c.APIVersion() != "2024-10-01-preview" {
		t.Errorf("expected custom api-version '2024-10-01-preview', got %q", c.APIVersion())
	}
}

func TestAzureClient_EndpointTrailingSlashStripped(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com/", "")
	if c.Endpoint() != "https://example.openai.azure.com" {
		t.Errorf("expected trailing slash stripped, got %q", c.Endpoint())
	}
}

func TestAzureChat_Success(t *testing.T) {
	t.Parallel()
	var capturedMethod, capturedPath, capturedQuery string
	var capturedHeaders http.Header
	var capturedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedHeaders = r.Header.Clone()

		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("X-Request-Id", "azure-req-001")
		_ = json.NewEncoder(w).Encode(adapters.OpenAIResponse{
			ID: "chatcmpl-azure-001",
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
					}{Content: "Hello from Azure!"},
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
			}{PromptTokens: 15, CompletionTokens: 10, TotalTokens: 25},
		})
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello Azure"},
	}, ChatOptions{Model: "gpt-4o-deployment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request method and path
	if capturedMethod != "POST" {
		t.Errorf("expected POST method, got %s", capturedMethod)
	}
	expectedPath := "/openai/deployments/gpt-4o-deployment/chat/completions"
	if capturedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, capturedPath)
	}
	if !strings.Contains(capturedQuery, "api-version=2024-08-01-preview") {
		t.Errorf("expected api-version in query, got %q", capturedQuery)
	}

	// Verify Azure-specific headers
	if capturedHeaders.Get("api-key") != "test-api-key" {
		t.Errorf("expected api-key header 'test-api-key', got %q", capturedHeaders.Get("api-key"))
	}
	if capturedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", capturedHeaders.Get("Content-Type"))
	}
	// Azure uses api-key, not Authorization Bearer
	if capturedHeaders.Get("Authorization") != "" {
		t.Errorf("azure should not send Authorization header, got %q", capturedHeaders.Get("Authorization"))
	}

	// Verify request body uses OpenAI format
	if capturedBody["model"] != "gpt-4o-deployment" {
		t.Errorf("expected model 'gpt-4o-deployment' in body, got %v", capturedBody["model"])
	}

	// Verify response
	if resp.Content != "Hello from Azure!" {
		t.Errorf("expected content 'Hello from Azure!', got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.FinishReason)
	}
	if resp.RequestID != "azure-req-001" {
		t.Errorf("expected request ID 'azure-req-001', got %q", resp.RequestID)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if resp.Usage.PromptTokens != 15 || resp.Usage.CompletionTokens != 10 {
		t.Errorf("unexpected usage: prompt=%d, completion=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func TestAzureChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := newTestAzureClient("http://localhost")
	_, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{}) // No model
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got: %v", err)
	}
}

func TestAzureChat_CacheReadTokens(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "azure-cache")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-cache",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "cached"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
				"prompt_tokens_details": map[string]interface{}{
					"cached_tokens": 50,
				},
			},
		})
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "cache test"},
	}, ChatOptions{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens=50, got %d", resp.Usage.CacheReadTokens)
	}
}

func TestAzureChat_ToolCallsInResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "azure-tc")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-tc",
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "Let me look that up.",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_azure_1",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "search_docs",
									"arguments": `{"query":"Azure pricing"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens": 40, "completion_tokens": 15, "total_tokens": 55,
			},
		})
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "What are Azure prices?"},
	}, ChatOptions{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_azure_1" {
		t.Errorf("expected tool call ID 'call_azure_1', got %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Name != "search_docs" {
		t.Errorf("expected tool name 'search_docs', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["query"] != "Azure pricing" {
		t.Errorf("expected query 'Azure pricing', got %v", resp.ToolCalls[0].Arguments["query"])
	}
}

func TestAzureChat_ErrorResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "azure-err-001")
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"code":"401","message":"Access denied due to invalid subscription key"}}`)
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	_, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "gpt-4o"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "azure") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected azure error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "azure-err-001") {
		t.Errorf("expected request ID in error, got: %v", err)
	}
}

func TestAzureChat_ToolsIncludedInRequest(t *testing.T) {
	t.Parallel()
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		w.Header().Set("X-Request-Id", "azure-tools")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-tools",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	_, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{
		Model: "gpt-4o",
		Tools: []EyrieTool{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]interface{}{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools, ok := capturedBody["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools in request body")
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]interface{})
	fn := tool["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %v", fn["name"])
	}
}

func TestAzureStreamChat_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the Accept header is set for SSE
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %q", r.Header.Get("Accept"))
		}
		// Verify api-key header
		if r.Header.Get("api-key") != "test-api-key" {
			t.Errorf("expected api-key header, got %q", r.Header.Get("api-key"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "azure-stream-001")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{\"content\":\"Azure \"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{\"content\":\"streaming works\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-s\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	sr, err := c.StreamChat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello Azure"},
	}, ChatOptions{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()

	if sr.RequestID != "azure-stream-001" {
		t.Errorf("expected request ID 'azure-stream-001', got %q", sr.RequestID)
	}

	var content strings.Builder
	var gotDone bool
	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			content.WriteString(evt.Content)
		case "done":
			gotDone = true
		case "error":
			t.Errorf("unexpected error event: %s", evt.Error)
		}
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if content.String() != "Azure streaming works" {
		t.Errorf("expected 'Azure streaming works', got %q", content.String())
	}
}

func TestAzureStreamChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := newTestAzureClient("http://localhost")
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

func TestAzurePing_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET for ping, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/openai/models") {
			t.Errorf("expected /openai/models in path, got %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "api-version=") {
			t.Error("expected api-version in ping query")
		}
		if r.Header.Get("api-key") != "test-api-key" {
			t.Errorf("expected api-key header on ping, got %q", r.Header.Get("api-key"))
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestAzurePing_InvalidKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("expected 'invalid API key' error, got: %v", err)
	}
}

func TestAzureChat_EmptyChoices(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "azure-empty")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-empty",
			"choices": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	c := newTestAzureClient(server.URL)
	resp, err := c.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got %q", resp.Content)
	}
	if resp.FinishReason != "unknown" {
		t.Errorf("expected finish_reason=unknown for empty choices, got %q", resp.FinishReason)
	}
}

// =============================================================================
// Cross-provider interface compliance tests
// =============================================================================

func TestAzureClient_ImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*AzureClient)(nil)
}

func TestVertexClient_ImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*VertexClient)(nil)
}

func TestBedrockClient_ImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*BedrockClient)(nil)
}

// =============================================================================
// parseAnthropicResponse helper tests
// =============================================================================

func TestParseAnthropicResponse_TextOnly(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	if err := json.Unmarshal([]byte(`{"content":[{"type":"text","text":"Hello!"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-123", "org-456")
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", resp.FinishReason)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected 'req-123', got %q", resp.RequestID)
	}
	if resp.OrganizationID != "org-456" {
		t.Errorf("expected 'org-456', got %q", resp.OrganizationID)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestParseAnthropicResponse_WithToolUse(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	if err := json.Unmarshal([]byte(`{"content":[{"type":"text","text":"Let me search."},{"type":"tool_use","id":"toolu_1","name":"search","input":{"q":"test"}}],"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":15,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}`), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-456", "")
	if resp.Content != "Let me search." {
		t.Errorf("expected text content, got %q", resp.Content)
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("expected tool_use, got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "toolu_1" {
		t.Errorf("expected tool ID 'toolu_1', got %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Name != "search" {
		t.Errorf("expected tool name 'search', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["q"] != "test" {
		t.Errorf("expected q=test, got %v", resp.ToolCalls[0].Arguments["q"])
	}
	if resp.Usage.CacheCreationTokens != 100 {
		t.Errorf("expected CacheCreationTokens=100, got %d", resp.Usage.CacheCreationTokens)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens=50, got %d", resp.Usage.CacheReadTokens)
	}
}

func TestParseAnthropicResponse_EmptyContent(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	if err := json.Unmarshal([]byte(`{"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":0}}`), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-empty", "")
	if resp.Content != "" {
		t.Errorf("expected empty content, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(resp.ToolCalls))
	}
}

// =============================================================================
// AWS SigV4 helper function tests
// =============================================================================

func TestSHA256Hex(t *testing.T) {
	t.Parallel()
	// SHA-256 of empty bytes
	hash := sha256Hex([]byte{})
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hash != expected {
		t.Errorf("expected %q, got %q", expected, hash)
	}

	// SHA-256 of known value
	hash = sha256Hex([]byte("hello"))
	expected = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if hash != expected {
		t.Errorf("expected %q, got %q", expected, hash)
	}
}

func TestAWSSigningKey(t *testing.T) {
	t.Parallel()
	// Verify signing key derivation doesn't panic and produces consistent results
	key1 := awsSigningKey("secret", "20230901", "us-east-1", "bedrock")
	key2 := awsSigningKey("secret", "20230901", "us-east-1", "bedrock")
	if len(key1) != 32 {
		t.Errorf("expected 32-byte signing key, got %d bytes", len(key1))
	}
	for i := range key1 {
		if key1[i] != key2[i] {
			t.Fatal("signing key should be deterministic")
		}
	}

	// Different secret should produce different key
	key3 := awsSigningKey("different", "20230901", "us-east-1", "bedrock")
	same := true
	for i := range key1 {
		if key1[i] != key3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different secrets should produce different signing keys")
	}
}

func TestCanonicalAWSHeaders(t *testing.T) {
	t.Parallel()
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Host", "bedrock-runtime.us-east-1.amazonaws.com")
	headers.Set("X-Amz-Date", "20230901T000000Z")

	canonical, signed := canonicalAWSHeaders(headers)

	// Headers should be sorted alphabetically
	if !strings.Contains(canonical, "content-type:application/json\n") {
		t.Errorf("expected content-type in canonical headers, got %q", canonical)
	}
	if !strings.Contains(canonical, "host:bedrock-runtime.us-east-1.amazonaws.com\n") {
		t.Errorf("expected host in canonical headers, got %q", canonical)
	}

	// Signed headers should be sorted and semicolon-separated
	expectedSigned := "content-type;host;x-amz-date"
	if signed != expectedSigned {
		t.Errorf("expected signed headers %q, got %q", expectedSigned, signed)
	}
}

func TestAWSCanonicalURI(t *testing.T) {
	t.Parallel()
	if awsCanonicalURI("") != "/" {
		t.Errorf("expected '/' for empty path, got %q", awsCanonicalURI(""))
	}
	if awsCanonicalURI("/model/test/invoke") != "/model/test/invoke" {
		t.Errorf("expected '/model/test/invoke', got %q", awsCanonicalURI("/model/test/invoke"))
	}
}
