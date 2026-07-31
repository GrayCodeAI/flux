package adapters

import (
	"context"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestNewConcentrateResponsesClient(t *testing.T) {
	t.Parallel()
	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	if client == nil {
		t.Fatal("NewConcentrateResponsesClient returned nil")
	}
	if client.Name() != "concentrate" {
		t.Errorf("expected name 'concentrate', got %q", client.Name())
	}
}

func TestConcentrateResponsesClient_Chat(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":     "resp_123",
			"object": "response",
			"model":  "gpt-5",
			"output": []map[string]any{
				{
					"type":    "message",
					"role":    "assistant",
					"content": []map[string]any{{"type": "text", "text": "Hello from Responses API!"}},
				},
			},
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 20, "total_tokens": 30},
		}), nil
	})

	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-5", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Responses API!" {
		t.Errorf("content = %q, want Hello from Responses API!", resp.Content)
	}
	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 20 {
		t.Errorf("usage = %+v, want prompt=10, completion=20", resp.Usage)
	}
}

func TestConcentrateResponsesClient_ChatWithTools(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id":     "resp_456",
			"object": "response",
			"model":  "gpt-5",
			"output": []map[string]any{
				{
					"type":      "function_call",
					"id":        "call_1",
					"name":      "get_weather",
					"arguments": `{"city":"NYC"}`,
				},
			},
		}), nil
	})

	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "What's the weather?"}}, core.ChatOptions{Model: "gpt-5", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", resp.ToolCalls[0].Name)
	}
}

func TestConcentrateResponsesClient_Ping(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"data": []any{}}), nil
	})

	client := NewConcentrateResponsesClient("cn-key", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConcentrateResponsesClient_PingNoAuth(t *testing.T) {
	t.Parallel()
	// Ping should work without auth for Concentrate
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"data": []any{}}), nil
	})

	client := NewConcentrateResponsesClient("", "https://api.concentrate.ai/v1")
	client.httpClient = &http.Client{Transport: transport}

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping without auth: %v", err)
	}
}
