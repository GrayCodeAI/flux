package adapters

import (
	"context"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestPoolsideClient_Name(t *testing.T) {
	t.Parallel()
	c := NewPoolsideClient("psk", "https://poolside.example")
	if c.Name() != "poolside" {
		t.Errorf("Name() = %q, want poolside", c.Name())
	}
}

func TestPoolsideClient_Chat(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "Poolside!"}, "finish_reason": "stop"}},
		}), nil
	})

	c := NewPoolsideClient("psk", "https://poolside.example")
	c.openAI.httpClient = &http.Client{Transport: transport}

	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "poolside/laguna-m.1", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Poolside!" {
		t.Errorf("content = %q, want Poolside!", resp.Content)
	}
}

func TestPoolsideClient_Ping(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]string{"status": "ok"}), nil
	})

	c := NewPoolsideClient("psk", "https://poolside.example")
	c.openAI.httpClient = &http.Client{Transport: transport}

	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPoolsideClient_StreamChatContentful(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "Answer"}, "finish_reason": "stop"}},
		}), nil
	})

	c := NewPoolsideClient("psk", "https://poolside.example")
	c.openAI.httpClient = &http.Client{Transport: transport}

	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "poolside/laguna-m.1", MaxTokens: 256})
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
	if content != "Answer" {
		t.Errorf("content = %q, want Answer", content)
	}
}
