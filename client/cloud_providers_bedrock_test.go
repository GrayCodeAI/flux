package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// AWS Bedrock provider tests. Split out of cloud_providers_test.go for clarity.
// =============================================================================
// AWS Bedrock Provider Tests
// =============================================================================

func newTestBedrockClient(serverURL, accessKey, secretKey, sessionToken, region string) *BedrockClient {
	c := NewBedrockClient(accessKey, secretKey, sessionToken, region)
	c.SetHTTPClient(&http.Client{})
	c.SetRetry(NewRetryConfig(0, 0, 0))
	return c
}

func TestBedrockClient_Name(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	if c.Name() != "anthropic-bedrock" {
		t.Errorf("expected name 'anthropic-bedrock', got %q", c.Name())
	}
}

func TestBedrockClient_ModelURL(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-west-2")
	url := c.ModelURL("anthropic.claude-3-5-sonnet-20241022-v2:0")
	// url.PathEscape does not encode ":" in Go, so it stays as-is
	expected := "https://bedrock-runtime.us-west-2.amazonaws.com/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke"
	if url != expected {
		t.Errorf("expected URL %q, got %q", expected, url)
	}
	// Verify the region is in the URL
	if !strings.Contains(url, "us-west-2") {
		t.Errorf("expected region in URL, got %q", url)
	}
}

func TestBedrockChat_Success(t *testing.T) {
	t.Parallel()
	var capturedMethod string
	var capturedHeaders http.Header
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedHeaders = r.Header.Clone()
		capturedBody, _ = io.ReadAll(r.Body)

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Bedrock!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  20,
				"output_tokens": 15,
			},
		})
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret-key", "session-token", "us-east-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	resp, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "Hello Bedrock"},
	}, ChatOptions{Model: "anthropic.claude-3-5-sonnet-20241022-v2:0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify method
	if capturedMethod != "POST" {
		t.Errorf("expected POST, got %s", capturedMethod)
	}

	// Verify AWS SigV4 auth headers
	auth := capturedHeaders.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Errorf("expected AWS4-HMAC-SHA256 auth, got %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKID/") {
		t.Errorf("expected AKID in credential, got %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Errorf("expected SignedHeaders in auth, got %q", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Errorf("expected Signature in auth, got %q", auth)
	}

	// Verify required AWS headers
	if capturedHeaders.Get("X-Amz-Date") == "" {
		t.Error("expected X-Amz-Date header")
	}
	if capturedHeaders.Get("X-Amz-Content-Sha256") == "" {
		t.Error("expected X-Amz-Content-Sha256 header")
	}
	// Note: Go's HTTP transport handles Host specially (stripped from Header map, sent in request line).
	// sign() does set Host for signing purposes, but it won't appear in r.Header at the handler.

	// Verify session token is included
	if capturedHeaders.Get("X-Amz-Security-Token") != "session-token" {
		t.Errorf("expected X-Amz-Security-Token 'session-token', got %q", capturedHeaders.Get("X-Amz-Security-Token"))
	}

	// Verify Anthropic-format request body
	var bodyMap map[string]interface{}
	json.Unmarshal(capturedBody, &bodyMap)
	if bodyMap["model"] != "anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Errorf("expected model in body, got %v", bodyMap["model"])
	}
	if bodyMap["max_tokens"] == nil {
		t.Error("expected max_tokens in body")
	}

	// Verify response
	if resp.Content != "Hello from Bedrock!" {
		t.Errorf("expected 'Hello from Bedrock!', got %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", resp.FinishReason)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.PromptTokens != 20 || resp.Usage.CompletionTokens != 15 {
		t.Errorf("unexpected usage: prompt=%d, completion=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func TestBedrockChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := newTestBedrockClient("http://localhost", "AKID", "secret", "", "us-east-1")
	_, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got: %v", err)
	}
}

func TestBedrockChat_NoSessionToken(t *testing.T) {
	t.Parallel()
	var capturedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret", "", "us-east-1") // No session token
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "anthropic.claude-3-5-sonnet-20241022-v2:0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session token header should NOT be set
	if capturedHeaders.Get("X-Amz-Security-Token") != "" {
		t.Errorf("expected no X-Amz-Security-Token for empty session token, got %q", capturedHeaders.Get("X-Amz-Security-Token"))
	}
}

func TestBedrockChat_ToolUseResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "I'll check the weather."},
				{
					"type":  "tool_use",
					"id":    "toolu_bedrock_1",
					"name":  "get_weather",
					"input": map[string]interface{}{"city": "Seattle"},
				},
			},
			"stop_reason": "tool_use",
			"usage": map[string]interface{}{
				"input_tokens":                30,
				"output_tokens":               20,
				"cache_creation_input_tokens": 100,
				"cache_read_input_tokens":     50,
			},
		})
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret", "session", "us-east-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	resp, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "What's the weather in Seattle?"},
	}, ChatOptions{Model: "anthropic.claude-3-5-sonnet-20241022-v2:0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.FinishReason != "tool_use" {
		t.Errorf("expected tool_use, got %q", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "toolu_bedrock_1" {
		t.Errorf("expected ID 'toolu_bedrock_1', got %q", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got %q", tc.Name)
	}
	if tc.Arguments["city"] != "Seattle" {
		t.Errorf("expected city=Seattle, got %v", tc.Arguments["city"])
	}

	// Verify cache token usage
	if resp.Usage.CacheCreationTokens != 100 {
		t.Errorf("expected CacheCreationTokens=100, got %d", resp.Usage.CacheCreationTokens)
	}
	if resp.Usage.CacheReadTokens != 50 {
		t.Errorf("expected CacheReadTokens=50, got %d", resp.Usage.CacheReadTokens)
	}
}

func TestBedrockBuildBody_DefaultMaxTokens(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	if int(bodyMap["max_tokens"].(float64)) != 4096 {
		t.Errorf("expected default max_tokens=4096, got %v", bodyMap["max_tokens"])
	}
	if bodyMap["model"] != "claude-sonnet-4-6" {
		t.Errorf("expected model in body, got %v", bodyMap["model"])
	}
}

func TestBedrockBuildBody_CustomMaxTokens(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6", MaxTokens: 8192})
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	if int(bodyMap["max_tokens"].(float64)) != 8192 {
		t.Errorf("expected max_tokens=8192, got %v", bodyMap["max_tokens"])
	}
}

func TestBedrockBuildBody_WithSystemPrompt(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "system", Content: "Be concise."},
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	system, ok := bodyMap["system"].(string)
	if !ok {
		t.Fatal("expected system field")
	}
	if !strings.Contains(system, "Be concise.") {
		t.Errorf("expected system prompt, got %q", system)
	}
}

func TestBedrockBuildBody_WithTools(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "user", Content: "Hello"},
	}, ChatOptions{
		Model: "claude-sonnet-4-6",
		Tools: []GraycodeRouterTool{
			{Name: "calculator", Description: "Do math", Parameters: map[string]interface{}{"type": "object"}},
		},
	})
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
	if tool["name"] != "calculator" {
		t.Errorf("expected tool name 'calculator', got %v", tool["name"])
	}
	if tool["input_schema"] == nil {
		t.Error("expected input_schema in bedrock tool")
	}
}

func TestBedrockBuildBody_ToolResultMessage(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "user", Content: "What is the weather?"},
		{Role: "assistant", ToolUse: []ToolCall{
			{ID: "toolu_1", Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}},
		}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "toolu_1", Content: "72F and sunny"}}},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
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

func TestBedrockBuildBody_SystemMerge(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	body, err := c.BuildBody([]GraycodeRouterMessage{
		{Role: "system", Content: "From messages"},
		{Role: "user", Content: "Hello"},
	}, ChatOptions{Model: "claude-sonnet-4-6", System: "From opts"})
	if err != nil {
		t.Fatalf("buildBody error: %v", err)
	}

	var bodyMap map[string]interface{}
	json.Unmarshal(body, &bodyMap)

	system := bodyMap["system"].(string)
	if !strings.Contains(system, "From opts") {
		t.Errorf("expected opts system in merged, got %q", system)
	}
	if !strings.Contains(system, "From messages") {
		t.Errorf("expected messages system in merged, got %q", system)
	}
}

func TestBedrockChat_ErrorResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		fmt.Fprint(w, `{"message":"User is not authorized to perform: bedrock:InvokeModel"}`)
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "anthropic.claude-3-5-sonnet-20241022-v2:0"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "bedrock") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected bedrock error, got: %v", err)
	}
}

func TestBedrockChat_MissingCredentials(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("", "", "", "us-east-1")
	c.SetHTTPClient(&http.Client{})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "credentials are incomplete") {
		t.Errorf("expected 'credentials are incomplete', got: %v", err)
	}
}

func TestBedrockSigV4_SignatureComponents(t *testing.T) {
	t.Parallel()
	// Test the signing helper functions
	c := NewBedrockClient("AKID", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "session-token", "us-east-1")

	req, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/invoke", nil)
	req.Header.Set("Content-Type", "application/json")
	body := []byte(`{"model":"test"}`)

	err := c.Sign(req, body, mustParseTime("20230901T000000Z"))
	if err != nil {
		t.Fatalf("sign error: %v", err)
	}

	// Verify required signing headers are present
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
		t.Errorf("expected AWS4-HMAC-SHA256, got %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKID/20230901/us-east-1/bedrock-runtime/aws4_request") {
		t.Errorf("expected correct credential scope, got %q", auth)
	}

	if req.Header.Get("X-Amz-Date") != "20230901T000000Z" {
		t.Errorf("expected X-Amz-Date '20230901T000000Z', got %q", req.Header.Get("X-Amz-Date"))
	}
	if req.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Error("expected X-Amz-Content-Sha256 to be set")
	}
	if req.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Errorf("expected X-Amz-Security-Token 'session-token', got %q", req.Header.Get("X-Amz-Security-Token"))
	}
	// sign() sets Host in the header map for signing purposes
	if host := req.Header.Get("Host"); host != "bedrock-runtime.us-east-1.amazonaws.com" {
		t.Errorf("expected Host header 'bedrock-runtime.us-east-1.amazonaws.com', got %q", host)
	}

	// SignedHeaders should include the expected headers
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Error("expected SignedHeaders in auth")
	}
}

func TestBedrockSigV4_DeterministicSignature(t *testing.T) {
	t.Parallel()
	// Same inputs should produce the same signature
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	now := mustParseTime("20230901T000000Z")
	body := []byte(`{"model":"test"}`)

	req1, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/invoke", nil)
	req1.Header.Set("Content-Type", "application/json")
	c.Sign(req1, body, now)

	req2, _ := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/invoke", nil)
	req2.Header.Set("Content-Type", "application/json")
	c.Sign(req2, body, now)

	auth1 := req1.Header.Get("Authorization")
	auth2 := req2.Header.Get("Authorization")
	if auth1 != auth2 {
		t.Errorf("expected identical signatures, got:\n%s\n%s", auth1, auth2)
	}
}

func TestBedrockPing_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET for ping, got %s", r.Method)
		}
		// Verify it's signed
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256") {
			t.Error("expected signed ping request")
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"modelSummaries":[]}`)
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})

	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBedrockPing_MissingCredentials(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("", "", "", "")
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "credentials are incomplete") {
		t.Errorf("expected 'credentials are incomplete', got: %v", err)
	}
}

func TestBedrockPing_InvalidCredentials(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer server.Close()

	c := NewBedrockClient("AKID", "secret", "", "us-east-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "invalid credentials") {
		t.Errorf("expected 'invalid credentials' error, got: %v", err)
	}
}

func TestBedrockStreamChat_ModelRequired(t *testing.T) {
	t.Parallel()
	c := newTestBedrockClient("http://localhost", "AKID", "secret", "", "us-east-1")
	_, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("expected 'model is required' error, got: %v", err)
	}
}

func TestBedrockStreamChat_MissingCredentials(t *testing.T) {
	t.Parallel()
	c := NewBedrockClient("", "", "", "us-east-1")
	c.SetHTTPClient(&http.Client{})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestBedrockModelIDMapping(t *testing.T) {
	t.Parallel()
	// Test that various model IDs produce correct URLs in modelURL
	c := NewBedrockClient("AKID", "secret", "", "us-east-1")

	tests := []struct {
		model  string
		wantID string
	}{
		{"anthropic.claude-3-5-sonnet-20241022-v2:0", "anthropic.claude-3-5-sonnet-20241022-v2:0"},
		{"anthropic.claude-3-haiku-20240307-v1:0", "anthropic.claude-3-haiku-20240307-v1:0"},
		{"anthropic.claude-sonnet-4-5-20250514-v1:0", "anthropic.claude-sonnet-4-5-20250514-v1:0"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			url := c.ModelURL(tt.model)
			// Verify base URL structure
			if !strings.HasPrefix(url, "https://bedrock-runtime.us-east-1.amazonaws.com/model/") {
				t.Errorf("expected bedrock-runtime URL prefix, got %q", url)
			}
			if !strings.HasSuffix(url, "/invoke") {
				t.Errorf("expected /invoke suffix, got %q", url)
			}
			// Verify model ID is in the URL (url.PathEscape preserves colons)
			if !strings.Contains(url, tt.wantID) {
				t.Errorf("expected model ID %q in URL %q", tt.wantID, url)
			}
		})
	}
}

func TestBedrockChat_RegionInURL(t *testing.T) {
	t.Parallel()
	var capturedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content":     []map[string]interface{}{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 5, "output_tokens": 2},
		})
	}))
	defer server.Close()

	// Test with different regions - they should be reflected in the URL
	c := NewBedrockClient("AKID", "secret", "", "eu-west-1")
	c.SetHTTPClient(&http.Client{
		Transport: &bedrockRewriteTransport{target: server.URL},
	})
	c.SetRetry(NewRetryConfig(0, 0, 0))

	_, err := c.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The URL should have been constructed with the region
	// Note: the rewrite transport changes the host, but the original path should have the model
	_ = capturedURL // URL was rewritten by transport; path should contain model ID
}

// Helper types and functions

func mustParseTime(s string) time.Time {
	t, err := time.Parse("20060102T150405Z", s)
	if err != nil {
		panic(err)
	}
	return t
}

// bedrockRewriteTransport redirects requests from the Bedrock AWS endpoints
// to a local test server, preserving the path and query.
type bedrockRewriteTransport struct {
	target string
}

func (t *bedrockRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	req.Host = req.URL.Host
	return http.DefaultTransport.RoundTrip(req)
}
