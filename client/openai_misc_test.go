//nolint:errcheck
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// OpenAI Ping, compat-override, image-content, tool-result, and misc client tests. Split out of openai_test.go for clarity.
// --- TestOpenAIPing ---

func TestOpenAIPing_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("expected /models, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("unexpected auth: %s", auth)
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIPing_InvalidKey(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":{"message":"invalid key"}}`)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOpenAIPing_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	// 500 != 401, so Ping should succeed (it only checks for 401)
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error (500 should pass ping): %v", err)
	}
}

// --- TestOpenAI_CompatOverrides ---

func TestOpenAICompat_MaxTokensField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		compat     *OpenAICompatConfig
		wantKey    string
		notWantKey string
	}{
		{
			name:       "openai uses max_completion_tokens",
			compat:     &OpenAICompat,
			wantKey:    "max_completion_tokens",
			notWantKey: "max_tokens",
		},
		{
			name:       "grok uses max_tokens",
			compat:     &GrokCompat,
			wantKey:    "max_tokens",
			notWantKey: "max_completion_tokens",
		},
		{
			name:       "ollama uses max_tokens",
			compat:     &OllamaCompat,
			wantKey:    "max_tokens",
			notWantKey: "max_completion_tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var reqBody map[string]interface{}
				json.Unmarshal(body, &reqBody)

				if _, ok := reqBody[tt.wantKey]; !ok {
					t.Errorf("expected %s in request body", tt.wantKey)
				}
				if _, ok := reqBody[tt.notWantKey]; ok {
					t.Errorf("unexpected %s in request body", tt.notWantKey)
				}

				w.Header().Set("X-Request-Id", "req-compat")
				resp := map[string]interface{}{
					"id": "chatcmpl-compat",
					"choices": []map[string]interface{}{
						{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
					},
				}
				json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			c := newTestOpenAIClient(srv.URL, tt.compat)
			_, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOpenAICompat_StreamOptionsNotSentWhenUnsupported(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		if _, ok := reqBody["stream_options"]; ok {
			t.Error("stream_options should not be sent when SupportsUsageInStreaming is false")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-no-so")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"id\":\"x\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	// GrokCompat has SupportsUsageInStreaming=false
	c := newTestOpenAIClient(srv.URL, &GrokCompat)
	sr, err := c.StreamChat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sr.Close()
	// Drain events
	for range sr.Events {
	}
}

// --- TestOpenAI_ImageContent ---

func TestOpenAIChat_ImageContent_DataURI(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		msgs := reqBody["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		if len(content) != 2 {
			t.Fatalf("expected 2 content parts, got %d", len(content))
		}
		textPart := content[0].(map[string]interface{})
		if textPart["type"] != "text" || textPart["text"] != "Describe this image" {
			t.Errorf("unexpected text part: %v", textPart)
		}
		imgPart := content[1].(map[string]interface{})
		if imgPart["type"] != "image_url" {
			t.Errorf("expected image_url type, got %v", imgPart["type"])
		}
		imgURL := imgPart["image_url"].(map[string]interface{})
		if imgURL["url"] != "data:image/png;base64,iVBORw0KGgoAAAA" {
			t.Errorf("unexpected image url: %v", imgURL["url"])
		}

		w.Header().Set("X-Request-Id", "req-img")
		resp := map[string]interface{}{
			"id": "chatcmpl-img",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "A cat"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	msgs := []EyrieMessage{
		{Role: "user", Content: "Describe this image", Images: []string{"data:image/png;base64,iVBORw0KGgoAAAA"}},
	}
	resp, err := c.Chat(context.Background(), msgs, defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "A cat" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestOpenAIChat_ImageContent_HTTPUrl(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		msgs := reqBody["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		imgPart := content[0].(map[string]interface{})
		imgURL := imgPart["image_url"].(map[string]interface{})
		if imgURL["url"] != "https://example.com/image.png" {
			t.Errorf("unexpected image url: %v", imgURL["url"])
		}

		w.Header().Set("X-Request-Id", "req-img-url")
		resp := map[string]interface{}{
			"id": "chatcmpl-img2",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "An image"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	msgs := []EyrieMessage{
		{Role: "user", Images: []string{"https://example.com/image.png"}},
	}
	resp, err := c.Chat(context.Background(), msgs, defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "An image" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestOpenAIChat_ImageContent_RawBase64(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		msgs := reqBody["messages"].([]interface{})
		msg := msgs[0].(map[string]interface{})
		content := msg["content"].([]interface{})

		imgPart := content[0].(map[string]interface{})
		imgURL := imgPart["image_url"].(map[string]interface{})
		expected := "data:image/png;base64,AAAA"
		if imgURL["url"] != expected {
			t.Errorf("expected %q, got %v", expected, imgURL["url"])
		}

		w.Header().Set("X-Request-Id", "req-img-raw")
		resp := map[string]interface{}{
			"id": "chatcmpl-img3",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "raw"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	msgs := []EyrieMessage{
		{Role: "user", Images: []string{"AAAA"}},
	}
	resp, err := c.Chat(context.Background(), msgs, defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "raw" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// --- TestOpenAI_ToolResultMessages ---

func TestOpenAIChat_ToolResultMessage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		msgs := reqBody["messages"].([]interface{})
		if len(msgs) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(msgs))
		}

		// First: user message
		first := msgs[0].(map[string]interface{})
		if first["role"] != "user" {
			t.Errorf("expected user role, got %v", first["role"])
		}

		// Second: assistant with tool_calls
		second := msgs[1].(map[string]interface{})
		if second["role"] != "assistant" {
			t.Errorf("expected assistant role, got %v", second["role"])
		}
		tcs := second["tool_calls"].([]interface{})
		if len(tcs) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(tcs))
		}

		// Third: tool result
		third := msgs[2].(map[string]interface{})
		if third["role"] != "tool" {
			t.Errorf("expected tool role, got %v", third["role"])
		}
		if third["tool_call_id"] != "call_xyz" {
			t.Errorf("expected tool_call_id=call_xyz, got %v", third["tool_call_id"])
		}
		if third["content"] != "file contents here" {
			t.Errorf("unexpected tool result content: %v", third["content"])
		}

		w.Header().Set("X-Request-Id", "req-tr")
		resp := map[string]interface{}{
			"id": "chatcmpl-tr",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "Got it"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	msgs := []EyrieMessage{
		{Role: "user", Content: "Read main.go"},
		{Role: "assistant", ToolUse: []ToolCall{{ID: "call_xyz", Name: "read_file", Arguments: map[string]interface{}{"path": "main.go"}}}},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "call_xyz", Content: "file contents here"}}},
	}
	resp, err := c.Chat(context.Background(), msgs, defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Got it" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// --- TestOpenAI_Name ---

func TestOpenAIClient_Name(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "http://example.com", nil)
	if c.Name() != "openai" {
		t.Errorf("expected name=openai, got %s", c.Name())
	}
}

// --- TestOpenAI_DefaultBaseURL ---

func TestOpenAIClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	if c.BaseURL() != "https://api.openai.com/v1" {
		t.Errorf("expected default baseURL, got %s", c.BaseURL())
	}
}

// --- TestOpenAI_MaxTokensDefault ---

func TestOpenAIChat_MaxTokensDefault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		// Default compat is OpenAICompat which uses max_completion_tokens
		mct, ok := reqBody["max_completion_tokens"]
		if !ok {
			t.Fatal("expected max_completion_tokens in request")
		}
		if int(mct.(float64)) != 4096 {
			t.Errorf("expected default max_completion_tokens=4096, got %v", mct)
		}

		w.Header().Set("X-Request-Id", "req-mt")
		resp := map[string]interface{}{
			"id": "chatcmpl-mt",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, &OpenAICompat)
	_, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIChat_MaxTokensCustom(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		mct, ok := reqBody["max_completion_tokens"]
		if !ok {
			t.Fatal("expected max_completion_tokens")
		}
		if int(mct.(float64)) != 8192 {
			t.Errorf("expected max_completion_tokens=8192, got %v", mct)
		}

		w.Header().Set("X-Request-Id", "req-mt2")
		resp := map[string]interface{}{
			"id": "chatcmpl-mt2",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, &OpenAICompat)
	opts := ChatOptions{Model: "gpt-4o", MaxTokens: 8192}
	_, err := c.Chat(context.Background(), msgs(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func msgs() []EyrieMessage {
	return basicMessages()
}

// --- TestOpenAI_EmptyChoices ---

func TestOpenAIChat_EmptyChoices(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-empty")
		resp := map[string]interface{}{
			"id":      "chatcmpl-empty",
			"choices": []map[string]interface{}{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	resp, err := c.Chat(context.Background(), basicMessages(), defaultChatOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got %q", resp.Content)
	}
	if resp.FinishReason != "unknown" {
		t.Errorf("expected finish_reason=unknown for empty choices, got %s", resp.FinishReason)
	}
}

// --- TestOpenAI_Temperature ---

func TestOpenAIChat_Temperature(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		json.Unmarshal(body, &reqBody)

		temp, ok := reqBody["temperature"]
		if !ok {
			t.Fatal("expected temperature in request")
		}
		if temp.(float64) != 0.7 {
			t.Errorf("expected temperature=0.7, got %v", temp)
		}

		w.Header().Set("X-Request-Id", "req-temp")
		resp := map[string]interface{}{
			"id": "chatcmpl-temp",
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "ok"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL, nil)
	temp := 0.7
	opts := ChatOptions{Model: "gpt-4o", Temperature: &temp}
	_, err := c.Chat(context.Background(), basicMessages(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
