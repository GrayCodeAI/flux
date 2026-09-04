package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client/core"
	"github.com/GrayCodeAI/graycode-router/types"
)

func TestNewAzureClient(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("az-key", "https://my-azure.openai.azure.com", "2024-10-21")
	if c == nil {
		t.Fatal("NewAzureClient returned nil")
	}
	if c.apiKey != "az-key" {
		t.Errorf("apiKey = %q, want az-key", c.apiKey)
	}
	if c.apiVersion != "2024-10-21" {
		t.Errorf("apiVersion = %q, want 2024-10-21", c.apiVersion)
	}
	if !strings.HasSuffix(c.endpoint, "azure.com") {
		t.Errorf("unexpected endpoint: %q", c.endpoint)
	}
}

func TestNewAzureClient_DefaultAPIVersion(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "")
	if c.apiVersion != "2024-10-21" {
		t.Errorf("expected default api-version, got %q", c.apiVersion)
	}
}

func TestAzureClient_Name(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "")
	if c.Name() != "azure" {
		t.Errorf("Name() = %q, want azure", c.Name())
	}
}

func TestAzureClient_Chat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":      "chatcmpl-azure-1",
			"object":  "chat.completion",
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "Hello Azure!"}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
		}), nil
	})

	c := NewAzureClient("az-key", "https://example.openai.azure.com", "")
	c.httpClient = &http.Client{Transport: transport}

	resp, err := c.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello Azure!" {
		t.Errorf("content = %q, want Hello Azure!", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want stop", resp.FinishReason)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

func TestAzureClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("az-key", "https://example.openai.azure.com", "")
	_, err := c.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAzureClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"message": "Invalid API key"},
		}), nil
	})

	c := NewAzureClient("bad-key", "https://example.openai.azure.com", "")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}

	_, err := c.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestAzureClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sse := "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" Azure!\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(sse)),
		}, nil
	})

	c := NewAzureClient("az-key", "https://example.openai.azure.com", "")
	c.httpClient = &http.Client{Transport: transport}

	result, err := c.StreamChat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for event := range result.Events {
		if event.Type == "error" {
			t.Fatalf("unexpected error: %s", event.Error)
		}
		if event.Type == "content" {
			content += event.Content
		}
	}
	if content != "Hello Azure!" {
		t.Errorf("content = %q, want Hello Azure!", content)
	}
}

func TestAzureClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("az-key", "https://example.openai.azure.com", "")
	_, err := c.StreamChat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestAzureClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"data": []map[string]any{{"id": "gpt-4o", "status": "succeeded"}},
		}), nil
	})

	c := NewAzureClient("az-key", "https://example.openai.azure.com", "")
	c.httpClient = &http.Client{Transport: transport}

	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestAzureClient_Ping_AuthError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "401", "message": "Access denied"},
		}), nil
	})

	c := NewAzureClient("bad-key", "https://example.openai.azure.com", "")
	c.httpClient = &http.Client{Transport: transport}

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestAzureClient_APIVersionAndEndpoint(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "2025-01-01")
	if c.APIVersion() != "2025-01-01" {
		t.Errorf("APIVersion = %q", c.APIVersion())
	}
	if c.Endpoint() != "https://example.openai.azure.com" {
		t.Errorf("Endpoint = %q", c.Endpoint())
	}
}

func TestAzureClient_SetHTTPClientAndSetRetry(t *testing.T) {
	t.Parallel()
	c := NewAzureClient("key", "https://example.openai.azure.com", "")
	c2 := NewAzureClient("key", "https://example.openai.azure.com", "")

	c.SetHTTPClient(c2.httpClient)
	if c.httpClient != c2.httpClient {
		t.Error("SetHTTPClient did not replace client")
	}

	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 7}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 7 {
		t.Errorf("expected MaxRetries=7, got %d", c.retry.MaxRetries)
	}
}
