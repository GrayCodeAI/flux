package adapters

import (
	"context"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestMiMoClientChatUsesOpenAIEndpoint(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		if req.Header.Get("api-key") != "tp-test-key" {
			t.Fatalf("missing MiMo api-key auth header")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chat",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		}), nil
	})

	client := NewMiMoClient("tp-test-key", "https://openai.example/v1", &XiaomiCompat, "xiaomi_mimo_token_plan")
	client.openAI.httpClient = &http.Client{Transport: transport}
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
}

func TestMiMoClientNoAnthropicFallback(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Param Incorrect"}}), nil
	})

	client := NewMiMoClient("tp-test-key", "https://openai.example/v1", &XiaomiCompat, "xiaomi_mimo_token_plan")
	client.openAI.httpClient = &http.Client{Transport: transport}
	_, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "mimo-v2.5-pro",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMiMoClientPreservesOpenAIBaseURL(t *testing.T) {
	t.Parallel()
	client := NewMiMoClient(
		"key",
		"https://openai.example/v1/",
		&XiaomiCompat,
		"xiaomi_mimo_token_plan",
	)
	if got := client.openAI.baseURL; got != "https://openai.example/v1" {
		t.Fatalf("OpenAI base URL = %q", got)
	}
}
