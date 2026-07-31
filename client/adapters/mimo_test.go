package adapters

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestMiMoClient_OpenAIOnly(t *testing.T) {
	t.Parallel()
	client := NewMiMoClient("tp-test-key", "https://openai.example/v1/", &XiaomiCompat, "xiaomi_mimo_token_plan")
	if client == nil || client.openai == nil {
		t.Fatal("expected OpenAI client")
	}
	if got := client.openai.baseURL; got != "https://openai.example/v1" {
		t.Fatalf("OpenAI base URL = %q", got)
	}
	if client.ProviderID() != "xiaomi_mimo_token_plan" {
		t.Fatalf("ProviderID = %q", client.ProviderID())
	}
}

func TestMiMoClient_ChatUsesOpenAIAndMimoAuth(t *testing.T) {
	t.Parallel()
	var gotPath, gotAPIKey, gotAuth string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotAPIKey = req.Header.Get("api-key")
		gotAuth = req.Header.Get("Authorization")
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chat",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		}), nil
	})

	client := NewMiMoClient("tp-test-key", "https://openai.example/v1", &XiaomiCompat, "xiaomi_mimo_token_plan")
	client.openai.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.openai.httpClient = &http.Client{Transport: transport}

	response, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model:     "mimo-v2.5-pro",
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "ok" {
		t.Fatalf("content = %q, want ok", response.Content)
	}
	if !strings.HasSuffix(gotPath, "/chat/completions") {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAPIKey != "tp-test-key" && !strings.Contains(gotAuth, "tp-test-key") {
		t.Fatalf("missing MiMo auth headers: api-key=%q Authorization=%q", gotAPIKey, gotAuth)
	}
}

func TestMiMoClient_Name(t *testing.T) {
	t.Parallel()
	client := NewMiMoClient("key", "https://oai.example/v1", &XiaomiCompat, "providerA")
	if client.Name() != "providerA" {
		t.Errorf("Name() = %q, want providerA", client.Name())
	}
}

func TestMiMoClient_Name_NoOpenAI(t *testing.T) {
	t.Parallel()
	c := &MiMoClient{providerID: "bare-id"}
	if c.Name() != "bare-id" {
		t.Errorf("Name() = %q, want bare-id", c.Name())
	}
}

func TestMiMoClient_Ping_SuccessViaOpenAI(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"data": []map[string]any{}}), nil
	})
	client := NewMiMoClient("key", "https://oai.example/v1", &XiaomiCompat, "p")
	client.openai.httpClient = &http.Client{Transport: transport}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestMiMoClient_Chat_SurfacesOpenAIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Param Incorrect"}}), nil
	})
	client := NewMiMoClient("key", "https://oai.example/v1", &XiaomiCompat, "p")
	client.openai.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.openai.httpClient = &http.Client{Transport: transport}

	_, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "mimo"})
	if err == nil {
		t.Fatal("expected OpenAI error without Anthropic fallback")
	}
}

func TestMiMoClient_StreamChat_SurfacesOpenAIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Param Incorrect"}}), nil
	})
	client := NewMiMoClient("key", "https://oai.example/v1", &XiaomiCompat, "p")
	client.openai.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.openai.httpClient = &http.Client{Transport: transport}

	_, err := client.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "mimo"})
	if err == nil {
		t.Fatal("expected OpenAI error without Anthropic fallback")
	}
}

func TestMimoRetryableChatError_HTTPStatus(t *testing.T) {
	t.Parallel()
	if !MimoRetryableChatError(fmt.Errorf("HTTP 401")) {
		t.Error("expected 401 to be retryable")
	}
	if MimoRetryableChatError(fmt.Errorf("HTTP 400")) {
		t.Error("expected 400 not retryable")
	}
	if MimoRetryableChatError(fmt.Errorf("connection refused")) {
		t.Error("expected non-HTTP errors not retryable via status helper")
	}
}

func TestParseHTTPStatusFromError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		msg  string
		want int
	}{
		{"HTTP 404 Not Found", 404},
		{"status 500 internal", 500},
		{"error (403) forbidden", 403},
		{"no status here", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseHTTPStatusFromError(tt.msg)
		if got != tt.want {
			t.Errorf("parseHTTPStatusFromError(%q) = %d, want %d", tt.msg, got, tt.want)
		}
	}
}
