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

func TestNewOllamaClient_OpenAI(t *testing.T) {
	t.Parallel()
	client := NewOllamaClient("", "http://localhost:11434/v1", &OllamaCompat)
	if client == nil || client.openAI == nil {
		t.Fatal("expected OpenAI client")
	}
	if client.Name() != "ollama" {
		t.Fatalf("Name = %q", client.Name())
	}
}

func TestOllamaClient_ChatUsesOpenAIPath(t *testing.T) {
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

	client := NewOllamaClient("", server.URL, &OllamaCompat, core.WithRetry(core.NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "llama3", MaxTokens: 16})
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

func TestOllamaClient_Ping(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := NewOllamaClient("", server.URL, &OllamaCompat, core.WithTimeout(2*time.Second))
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
