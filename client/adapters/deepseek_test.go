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

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestNewDeepSeekClient_OpenAIOnly(t *testing.T) {
	t.Parallel()
	client := NewDeepSeekClient("ds-key", "https://api.deepseek.com/v1", &DeepSeekCompat)
	if client == nil || client.openai == nil {
		t.Fatal("expected OpenAI client")
	}
	if client.Name() != "deepseek" {
		t.Fatalf("Name = %q", client.Name())
	}
}

func TestDeepSeekClient_ChatUsesOpenAIPath(t *testing.T) {
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

	client := NewDeepSeekClient("key", server.URL, &DeepSeekCompat, core.WithRetry(core.NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "deepseek-v4-flash", MaxTokens: 16})
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

func TestDeepSeekClient_Ping(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := NewDeepSeekClient("key", server.URL, &DeepSeekCompat, core.WithTimeout(2*time.Second))
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
