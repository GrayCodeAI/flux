package adapters

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
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
	client.anthropic.httpClient = &http.Client{Transport: transport}
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
	client.openai.httpClient = &http.Client{Transport: transport}
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

func TestOpenCodeGoClientQwenUsesAnthropicOnly(t *testing.T) {
	t.Parallel()
	var paths []string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if strings.HasSuffix(req.URL.Path, "/messages") {
			return jsonResponse(http.StatusUnauthorized, map[string]any{
				"error": map[string]string{"message": "Invalid API key"},
			}), nil
		}
		t.Fatalf("unexpected OpenAI path %q — no cross-protocol fallback", req.URL.Path)
		return nil, nil
	})

	client := NewOpenCodeGoClient("ocg-test-key", "https://opencode.example/zen/go/v1")
	client.anthropic.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.anthropic.httpClient = &http.Client{Transport: transport}
	client.openai.httpClient = &http.Client{Transport: transport}
	_, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "qwen3.7-max", MaxTokens: 256,
	})
	if err == nil {
		t.Fatal("expected Anthropic error without OpenAI fallback")
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/messages") {
		t.Fatalf("paths = %v, want single /messages call", paths)
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
	client.openai.httpClient = &http.Client{Transport: transport}
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

func TestOpenCodeGoClient_Name(t *testing.T) {
	t.Parallel()
	client := NewOpenCodeGoClient("key", "https://opencode.example/zen/go/v1")
	if client.Name() != "opencodego" {
		t.Errorf("Name() = %q, want opencodego", client.Name())
	}
}

func TestOpenCodeGoClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})
	client := NewOpenCodeGoClient("key", "https://opencode.example/zen/go/v1")
	client.openai.httpClient = &http.Client{Transport: transport}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenCodeGoClient_Ping_OpenAIOnly(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
	})
	client := NewOpenCodeGoClient("key", "https://openai.example/v1")
	client.openai.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	client.openai.httpClient = &http.Client{Transport: transport}
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error without Anthropic fallback")
	}
}

func TestNewOpenCodeGoClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	client := NewOpenCodeGoClient("key", "")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Name() != "opencodego" {
		t.Errorf("Name = %q, want opencodego", client.Name())
	}
}
