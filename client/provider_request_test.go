//nolint:errcheck
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// equalJSONIgnoringStream decodes two JSON request bodies and returns
// true if they're equal modulo the top-level "stream" field. Used to
// verify Chat and StreamChat produce byte-identical request bodies
// (modulo the stream flag).
func equalJSONIgnoringStream(a, b []byte) bool {
	var ma, mb map[string]interface{}
	if err := json.Unmarshal(a, &ma); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &mb); err != nil {
		return false
	}
	delete(ma, "stream")
	delete(mb, "stream")
	return reflect.DeepEqual(ma, mb)
}

// captureAnthropicRequest returns a handler that captures the request
// body and Accept header, then dispatches to the chat or stream
// response based on Accept.
func captureAnthropicRequest(chatBody, streamBody *[]byte, streamAccept *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Accept") == "text/event-stream" {
			*streamBody = body
			*streamAccept = r.Header.Get("Accept")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\n", `{"type":"message_stop"}`)
		} else {
			*chatBody = body
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"msg_1","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`)
		}
	}
}

// TestAnthropic_ChatVsStream_SameBody: the request bodies produced
// by Chat and StreamChat must be byte-identical except for the
// "stream" field. A drift here means the streaming and non-streaming
// paths are inconsistent (e.g., a tool_use field only present in
// streaming, or a max_tokens default that's different).
func TestAnthropic_ChatVsStream_SameBody(t *testing.T) {
	t.Parallel()
	var chatBody, streamBody []byte
	var streamAccept string
	srv := httptest.NewServer(captureAnthropicRequest(&chatBody, &streamBody, &streamAccept))
	defer srv.Close()

	c := NewAnthropicClient("test-key", srv.URL)
	messages := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	temp := 0.7
	topP := 0.9
	topK := 40
	opts := ChatOptions{
		Model:         "claude-test",
		MaxTokens:     1024,
		Temperature:   &temp,
		System:        "You are a helpful assistant.",
		TopP:          &topP,
		TopK:          &topK,
		StopSequences: []string{"STOP"},
	}

	// Make Chat call.
	if _, err := c.Chat(context.Background(), messages, opts); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// Make StreamChat call.
	sr, err := c.StreamChat(context.Background(), messages, opts)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range sr.Events {
	}
	sr.Close()

	if !equalJSONIgnoringStream(chatBody, streamBody) {
		t.Errorf("Chat and StreamChat bodies differ (modulo stream).\nChat: %s\nStream: %s", chatBody, streamBody)
	}
	if streamAccept != "text/event-stream" {
		t.Errorf("StreamChat Accept = %q, want text/event-stream", streamAccept)
	}
	// Verify stream:true was in the stream body and absent (or false) in the chat body.
	var sBody, cBody map[string]interface{}
	_ = json.Unmarshal(streamBody, &sBody)
	_ = json.Unmarshal(chatBody, &cBody)
	if v, _ := sBody["stream"].(bool); !v {
		t.Errorf("stream body should have stream=true, got %v", sBody["stream"])
	}
	if v, ok := cBody["stream"]; ok {
		// Either false or absent is acceptable; if present it must be false.
		if vb, _ := v.(bool); vb {
			t.Errorf("chat body should NOT have stream=true, got %v", v)
		}
	}
}

// TestAnthropic_BuildRequest_ModelRequired: an empty Model returns
// a clear error from the helper.
func TestAnthropic_BuildRequest_ModelRequired(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("test-key", "http://localhost:0")
	_, _, err := c.BuildAnthropicRequest(
		context.Background(),
		[]GraycodeRouterMessage{{Role: "user", Content: "hi"}},
		ChatOptions{}, // no Model
		false,
	)
	if err == nil {
		t.Fatal("expected error for missing Model, got nil")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("err = %q, want 'model is required'", err.Error())
	}
}

// TestAnthropic_BuildRequest_StreamSetsAccept: when stream is true,
// the helper sets the Accept header. When false, it does not.
func TestAnthropic_BuildRequest_StreamSetsAccept(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("test-key", "http://localhost:0")
	messages := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	opts := ChatOptions{Model: "claude-test"}

	chatReq, _, err := c.BuildAnthropicRequest(context.Background(), messages, opts, false)
	if err != nil {
		t.Fatalf("buildAnthropicRequest(chat): %v", err)
	}
	if got := chatReq.Header.Get("Accept"); got == "text/event-stream" {
		t.Errorf("Chat request has Accept=text/event-stream; should not")
	}

	streamReq, _, err := c.BuildAnthropicRequest(context.Background(), messages, opts, true)
	if err != nil {
		t.Fatalf("buildAnthropicRequest(stream): %v", err)
	}
	if got := streamReq.Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("StreamChat request Accept = %q, want text/event-stream", got)
	}
}

// TestAnthropic_BuildRequest_GetBody: GetBody must return a fresh
// reader over the same body (needed for the MiMo 401 retry path).
func TestAnthropic_BuildRequest_GetBody(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("test-key", "http://localhost:0")
	req, body, err := c.BuildAnthropicRequest(
		context.Background(),
		[]GraycodeRouterMessage{{Role: "user", Content: "hi"}},
		ChatOptions{Model: "claude-test"},
		false,
	)
	if err != nil {
		t.Fatalf("buildAnthropicRequest: %v", err)
	}
	if req.GetBody == nil {
		t.Fatal("GetBody is nil")
	}
	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	defer rc.Close()
	reRead, _ := io.ReadAll(rc)
	if !bytes.Equal(reRead, body) {
		t.Errorf("GetBody() returned different bytes than the original body")
	}
}

// TestAnthropic_BuildRequest_SystemPromptMerging: when both
// opts.System and message-level system messages are present, they
// are merged (System prefix + messages). Verify this for both
// Chat and Stream paths.
func TestAnthropic_BuildRequest_SystemPromptMerging(t *testing.T) {
	t.Parallel()
	var chatBody, streamBody []byte
	var streamAccept string
	srv := httptest.NewServer(captureAnthropicRequest(&chatBody, &streamBody, &streamAccept))
	defer srv.Close()

	c := NewAnthropicClient("test-key", srv.URL)
	messages := []GraycodeRouterMessage{
		{Role: "system", Content: "from-message"},
		{Role: "user", Content: "hi"},
	}
	opts := ChatOptions{
		Model:  "claude-test",
		System: "from-opts",
	}

	if _, err := c.Chat(context.Background(), messages, opts); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	sr, err := c.StreamChat(context.Background(), messages, opts)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range sr.Events {
	}
	sr.Close()

	// Both bodies should have the merged system prompt.
	for name, body := range map[string][]byte{"chat": chatBody, "stream": streamBody} {
		var p map[string]interface{}
		if err := json.Unmarshal(body, &p); err != nil {
			t.Errorf("%s: unmarshal: %v", name, err)
			continue
		}
		sys, ok := p["system"].(string)
		if !ok {
			t.Errorf("%s: system field is not a string: %T", name, p["system"])
			continue
		}
		// The merge prepends opts.System before any message-level
		// system content (buildAnthropicMessages produces the
		// "from-message" body, then opts.System is prepended).
		want := "from-opts\n\nfrom-message"
		if sys != want {
			t.Errorf("%s: system = %q, want %q", name, sys, want)
		}
	}
}

// TestAnthropic_BuildRequest_SizeLimit: bodies over 32 MB are
// rejected with a clear error.
func TestAnthropic_BuildRequest_SizeLimit(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("test-key", "http://localhost:0")
	// Build a single huge message that pushes the body over 32 MB.
	big := strings.Repeat("x", 33*1024*1024) // 33 MB
	messages := []GraycodeRouterMessage{{Role: "user", Content: big}}
	_, _, err := c.BuildAnthropicRequest(context.Background(), messages,
		ChatOptions{Model: "claude-test"}, false)
	if err == nil {
		t.Fatal("expected size-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds Anthropic limit") {
		t.Errorf("err = %q, want 'exceeds Anthropic limit'", err.Error())
	}
}

// captureOpenAIRequest returns a handler that captures the request
// body and Accept header for the OpenAI Chat Completions endpoint,
// then dispatches to the chat or stream response.
func captureOpenAIRequest(chatBody, streamBody *[]byte, streamAccept *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Accept") == "text/event-stream" {
			*streamBody = body
			*streamAccept = r.Header.Get("Accept")
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\n", `{"choices":[]}`)
		} else {
			*chatBody = body
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
		}
	}
}

// TestOpenAI_ChatVsStream_SameBody: same byte-equality assertion for
// the OpenAI Chat Completions path.
func TestOpenAI_ChatVsStream_SameBody(t *testing.T) {
	t.Parallel()
	var chatBody, streamBody []byte
	var streamAccept string
	srv := httptest.NewServer(captureOpenAIRequest(&chatBody, &streamBody, &streamAccept))
	defer srv.Close()

	c := NewOpenAIClient("test-key", srv.URL, &OpenAICompatConfig{})
	messages := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	temp := 0.7
	topP := 0.9
	opts := ChatOptions{
		Model:       "gpt-test",
		MaxTokens:   1024,
		Temperature: &temp,
		TopP:        &topP,
	}

	if _, err := c.Chat(context.Background(), messages, opts); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	sr, err := c.StreamChat(context.Background(), messages, opts)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range sr.Events {
	}
	sr.Close()

	if !equalJSONIgnoringStream(chatBody, streamBody) {
		t.Errorf("OpenAI Chat and StreamChat bodies differ (modulo stream).\nChat: %s\nStream: %s", chatBody, streamBody)
	}
	if streamAccept != "text/event-stream" {
		t.Errorf("StreamChat Accept = %q, want text/event-stream", streamAccept)
	}
}

// TestOpenAI_BuildRequest_StreamSetsAccept: OpenAI StreamChat sets
// the Accept: text/event-stream header; Chat does not.
func TestOpenAI_BuildRequest_StreamSetsAccept(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("test-key", "http://localhost:0", &OpenAICompatConfig{})
	messages := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	opts := ChatOptions{Model: "gpt-test"}

	chatReq, _, err := c.BuildOpenAIRequest(context.Background(), messages, opts, false)
	if err != nil {
		t.Fatalf("buildOpenAIRequest(chat): %v", err)
	}
	if got := chatReq.Header.Get("Accept"); got == "text/event-stream" {
		t.Errorf("Chat request has Accept=text/event-stream; should not")
	}

	streamReq, _, err := c.BuildOpenAIRequest(context.Background(), messages, opts, true)
	if err != nil {
		t.Fatalf("buildOpenAIRequest(stream): %v", err)
	}
	if got := streamReq.Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("StreamChat request Accept = %q, want text/event-stream", got)
	}
}
