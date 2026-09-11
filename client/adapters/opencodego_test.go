package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestOpenCodeGoClientRoutesMiniMaxToAnthropic(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotAuth = req.Header.Get("X-Api-Key")
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "Hello!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 2},
		}), nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}
	response, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "minimax-m2.5", MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "Hello!" {
		t.Fatalf("content = %q, want Hello!", response.Content)
	}
	if !strings.HasSuffix(gotPath, "/v1/messages") {
		t.Fatalf("path = %q, want suffix /v1/messages", gotPath)
	}
	if gotAuth != "ocg-test-key" {
		t.Fatalf("X-Api-Key = %q, want ocg-test-key", gotAuth)
	}
}

func TestOpenCodeGoClientRoutesKimiToOpenAI(t *testing.T) {
	t.Parallel()
	var gotPath string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hi there!"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
		}), nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	response, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "kimi-k2.5", MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "Hi there!" {
		t.Fatalf("content = %q, want Hi there!", response.Content)
	}
	if !strings.HasSuffix(gotPath, "/chat/completions") {
		t.Fatalf("path = %q, want suffix /chat/completions", gotPath)
	}
}

func TestOpenCodeGoClientQwen401FallsBackToOpenAI(t *testing.T) {
	t.Parallel()
	var paths []string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, "/messages") {
			return jsonResponse(http.StatusUnauthorized, map[string]any{
				"error": map[string]string{"message": "Invalid API key"},
			}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "OK"}, "finish_reason": "stop"},
			},
		}), nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	response, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "qwen3.7-max", MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "OK" {
		t.Fatalf("content = %q, want OK; paths=%v", response.Content, paths)
	}
}

func TestOpenCodeGoClientMessagesEmptyFallsBackToOpenAI(t *testing.T) {
	t.Parallel()
	var paths []string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, "/messages") {
			return jsonResponse(http.StatusOK, map[string]any{
				"id": "msg_1", "type": "message", "role": "assistant",
				"content":     []map[string]string{{"type": "thinking", "thinking": "hmm"}},
				"stop_reason": "end_turn",
			}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"},
			},
		}), nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	response, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "minimax-m3", MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "Hello!" {
		t.Fatalf("content = %q, want Hello!; paths=%v", response.Content, paths)
	}
	if len(paths) < 2 {
		t.Fatalf("expected anthropic then openai, got %v", paths)
	}
}

func TestOpenCodeGoClientNormalizesModelID(t *testing.T) {
	t.Parallel()
	var gotModel string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/chat/completions") {
			var body struct {
				Model string `json:"model"`
			}
			if err := jsonDecodeRequest(req, &body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			gotModel = body.Model
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hi"}, "finish_reason": "stop"},
			},
		}), nil
	})
	client := NewOpenCodeGoClient("key", "https://opencode.example/zen/go/v1")
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	_, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "opencode-go/kimi-k2.6", MaxTokens: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "kimi-k2.6" {
		t.Fatalf("model = %q, want kimi-k2.6", gotModel)
	}
}

func TestOpenCodeGoClientStreamMiniMaxReasoningOnlyFallsBackToChat(t *testing.T) {
	t.Parallel()
	var paths []string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, "/messages") {
			body := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"text\":\"hmm\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}
		if strings.HasSuffix(req.URL.Path, "/chat/completions") && req.Header.Get("Accept") == "text/event-stream" {
			t.Fatal("stream fallback should not run before non-streaming chat fallback")
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"},
			},
		}), nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.router.Anthropic.httpClient = &http.Client{Transport: transport}
	client.router.OpenAI.httpClient = &http.Client{Transport: transport}
	result, err := client.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hello how are you?"}}, core.ChatOptions{
		Model: "minimax-m3", MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for event := range result.Events {
		switch event.Type {
		case "thinking":
			t.Fatal("reasoning-only primary stream must not leak thinking before chat fallback")
		case "content":
			content += event.Content
		case "error":
			t.Fatalf("unexpected stream error: %s", event.Error)
		}
	}
	if content != "Hello!" {
		t.Fatalf("content = %q, want Hello!; paths=%v", content, paths)
	}
	if len(paths) < 2 {
		t.Fatalf("expected /messages then /chat/completions, got %v", paths)
	}
}
