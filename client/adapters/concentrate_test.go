package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestNewConcentrateClient_HasBothProtocols(t *testing.T) {
	t.Parallel()
	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	if client == nil {
		t.Fatal("NewConcentrateClient returned nil")
	}
	if client.Name() != "concentrate" {
		t.Errorf("expected name 'concentrate', got %q", client.Name())
	}
	if client.router.OpenAI == nil {
		t.Fatal("expected OpenAI client")
	}
	if client.router.Anthropic == nil {
		t.Fatal("expected Anthropic client")
	}
}

func TestConcentrateClient_ChatOpenAIProtocol(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion",
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
		}), nil
	})

	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}

	// Non-anthropic model uses OpenAI protocol
	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-5", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("content = %q, want Hello!", resp.Content)
	}
}

func TestConcentrateClient_ChatAnthropicProtocol(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":          "msg_1",
			"type":        "message",
			"role":        "assistant",
			"content":     []map[string]string{{"type": "text", "text": "Hello from Anthropic!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 2},
		}), nil
	})

	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}

	// Anthropic model uses Messages API
	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-opus-5", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Anthropic!" {
		t.Errorf("content = %q, want Hello from Anthropic!", resp.Content)
	}
}

func TestConcentrateClient_StreamChatOpenAIProtocol(t *testing.T) {
	t.Parallel()
	// OpenAI streaming uses SSE format
	body := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Stream\"},\"index\":0}]}\n\ndata: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\" test\"},\"index\":0}]}\n\ndata: [DONE]\n\n"
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}

	result, err := client.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-5", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for event := range result.Events {
		if event.Type == "content" {
			content += event.Content
		}
	}
	if content != "Stream test" {
		t.Errorf("content = %q, want Stream test", content)
	}
}

func TestConcentrateClient_PingOpenAISuccess(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]string{"status": "ok"}), nil
	})

	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConcentrateClient_PingFallbackToAnthropic(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Fail OpenAI endpoint
		if req.URL.Path == "/v1/models" && !strings.Contains(req.URL.String(), "anthropic") {
			return jsonResponse(http.StatusServiceUnavailable, map[string]any{
				"error": map[string]string{"message": "service unavailable"},
			}), nil
		}
		return jsonResponse(http.StatusOK, map[string]string{"status": "ok"}), nil
	})

	client := NewConcentrateClient("cn-key", "https://api.concentrate.ai/v1", nil)
	client.router.OpenAI.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping fallback: %v", err)
	}
}
