package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestNewGeminiOpenAIClient_OpenAI(t *testing.T) {
	t.Parallel()
	client := NewGeminiOpenAIClient("gemini-key", "https://generativelanguage.googleapis.com/v1beta/openai", &GeminiCompat)
	if client == nil || client.openAI == nil {
		t.Fatal("expected OpenAI client")
	}
	if client.Name() != "gemini" {
		t.Fatalf("Name = %q", client.Name())
	}
}

func TestGeminiOpenAIClient_ChatUsesOpenAIPath(t *testing.T) {
	t.Parallel()
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chat",
			"choices": []map[string]interface{}{
				{"message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer server.Close()

	client := NewGeminiOpenAIClient("key", server.URL, &GeminiCompat, core.WithRetry(core.NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "gemini-2.0-flash", MaxTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Content != "ok" {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.Contains(path, "/chat/completions") {
		t.Fatalf("path = %q, want chat/completions", path)
	}
}

func TestGeminiOpenAIClient_Ping(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := NewGeminiOpenAIClient("key", server.URL, &GeminiCompat, core.WithTimeout(2*time.Second))
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
