package adapters

import (
	"context"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestMiMoClientChatFallsBackToAnthropicOnParamIncorrect(t *testing.T) {
	t.Parallel()
	anthropicCalls := 0
	openAITransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Fatalf("openai path = %q", req.URL.Path)
		}
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Param Incorrect"}}), nil
	})
	anthropicTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		anthropicCalls++
		if req.URL.Path != "/anthropic/v1/messages" {
			t.Fatalf("anthropic path = %q", req.URL.Path)
		}
		if req.Header.Get("api-key") != "tp-test-key" {
			t.Fatalf("missing MiMo api-key auth header")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id":          "msg_1",
			"type":        "message",
			"role":        "assistant",
			"content":     []map[string]string{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 1},
		}), nil
	})

	client := NewMiMoClient("tp-test-key", "https://openai.example/v1", "https://anthropic.example/anthropic", &XiaomiCompat, "xiaomi_mimo_token_plan")
	client.router.OpenAI.httpClient = &http.Client{Transport: openAITransport}
	client.router.Anthropic.httpClient = &http.Client{Transport: anthropicTransport}
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
	if anthropicCalls != 1 {
		t.Fatalf("anthropic calls = %d, want 1", anthropicCalls)
	}
}

func TestMiMoClientPreservesProtocolBaseURLs(t *testing.T) {
	t.Parallel()
	client := NewMiMoClient(
		"key",
		"https://openai.example/v1/",
		"https://anthropic.example/anthropic/",
		&XiaomiCompat,
		"xiaomi_mimo_token_plan",
	)
	if got := client.router.OpenAI.baseURL; got != "https://openai.example/v1" {
		t.Fatalf("OpenAI base URL = %q", got)
	}
	if client.router.Anthropic == nil {
		t.Fatal("Anthropic fallback client is nil")
	}
	if got := client.router.Anthropic.baseURL; got != "https://anthropic.example/anthropic" {
		t.Fatalf("Anthropic base URL = %q", got)
	}
}
