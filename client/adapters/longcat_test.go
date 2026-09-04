package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestNewLongCatClient_Dual(t *testing.T) {
	t.Parallel()
	client := NewLongCatClient("lc-key", "https://api.longcat.chat/openai/v1", "https://api.longcat.chat/anthropic", &LongCatCompat)
	if client == nil || client.router.OpenAI == nil || client.router.Anthropic == nil {
		t.Fatalf("expected dual OpenAI + Anthropic clients, got %+v", client.router)
	}
	if client.Name() != "longcat" {
		t.Fatalf("Name = %q", client.Name())
	}
}

func TestLongCatClient_ChatUsesOpenAIPath(t *testing.T) {
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

	client := NewLongCatClient("key", server.URL, "", &LongCatCompat, core.WithRetry(core.NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "LongCat-2.0", MaxTokens: 16})
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

func TestLongCatClient_FallsBackToAnthropic(t *testing.T) {
	t.Parallel()
	var openAIHits, anthropicHits int32

	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&openAIHits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"upstream failure","type":"server_error","code":500}}`)
	}))
	defer openAI.Close()

	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&anthropicHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "msg_1",
			"type":        "message",
			"role":        "assistant",
			"content":     []map[string]string{{"type": "text", "text": "recovered via anthropic"}},
			"model":       "LongCat-2.0",
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 8},
		})
	}))
	defer anthropic.Close()

	client := NewLongCatClient("key", openAI.URL, anthropic.URL, &LongCatCompat, core.WithRetry(core.NewRetryConfig(0, 0, 0)))
	resp, err := client.Chat(context.Background(), []core.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "LongCat-2.0", MaxTokens: 16})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&openAIHits) == 0 || atomic.LoadInt32(&anthropicHits) == 0 {
		t.Fatalf("expected both protocols hit: openAI=%d anthropic=%d", openAIHits, anthropicHits)
	}
	if resp == nil || resp.Content != "recovered via anthropic" {
		t.Fatalf("resp = %+v, want anthropic content", resp)
	}
}

func TestLongCatClient_Ping(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	client := NewLongCatClient("key", server.URL, "", &LongCatCompat, core.WithTimeout(2*time.Second))
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
