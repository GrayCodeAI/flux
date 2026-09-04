//nolint:errcheck
package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// slogDiscard returns a *slog.Logger that discards all output. Used
// by tests that exercise parse/process functions directly.
func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// geminiSSEServer builds a mock HTTP server that streams Gemini-format
// SSE responses. The handler writes each frame as `data: <json>\n\n`
// to the response body and flushes so the client sees chunks as they
// are produced. The frames are written in order from the given slice.
func geminiSSEServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-goog-api-key", "ok") // not used; just for the test header
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("httptest.ResponseWriter is not an http.Flusher")
			return
		}

		for _, frame := range frames {
			// SSE: each event is "data: <payload>\n\n". An empty
			// line terminates the event.
			if _, err := fmt.Fprintf(w, "data: %s\n\n", frame); err != nil {
				return
			}
			flusher.Flush()
			// Give the client time to consume each frame so we
			// don't race the parser.
			time.Sleep(2 * time.Millisecond)
		}
		// Keep connection open briefly so the client sees the
		// end-of-stream via io.EOF rather than a close.
		time.Sleep(20 * time.Millisecond)
	}))
}

// drainGeminiStream consumes the full event stream with a deadline and
// returns the collected events. Used by all the streaming tests.
func drainGeminiStream(t *testing.T, sr *StreamResult, timeout time.Duration) []GraycodeRouterStreamEvent {
	t.Helper()
	var out []GraycodeRouterStreamEvent
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("drainGeminiStream: timed out after %v (got %d events)", timeout, len(out))
		case evt, ok := <-sr.Events:
			if !ok {
				return out
			}
			out = append(out, evt)
		}
	}
}

// TestGemini_Stream_SharedParser_Text: a simple text response is
// emitted as content events followed by a "done" with usage.
func TestGemini_Stream_SharedParser_Text(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	frames := []string{
		`{"candidates":[{"content":{"parts":[{"text":"Hello "}],"role":"model"},"finishReason":""}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"world!"}],"role":"model"},"finishReason":""}]}`,
		`{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}`,
	}
	srv := geminiSSEServer(t, frames)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hi"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	events := drainGeminiStream(t, sr, 5*time.Second)
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d: %+v", len(events), events)
	}

	// First two events should be content.
	if events[0].Type != "content" || events[0].Content != "Hello " {
		t.Errorf("events[0] = %+v, want content \"Hello \"", events[0])
	}
	if events[1].Type != "content" || events[1].Content != "world!" {
		t.Errorf("events[1] = %+v, want content \"world!\"", events[1])
	}

	// Final event should be "done" with usage.
	last := events[len(events)-1]
	if last.Type != "done" {
		t.Errorf("last event type = %q, want \"done\"", last.Type)
	}
	if last.Usage == nil {
		t.Fatal("done event missing Usage")
	}
	if last.Usage.PromptTokens != 5 {
		t.Errorf("Usage.PromptTokens = %d, want 5", last.Usage.PromptTokens)
	}
	if last.Usage.CompletionTokens != 3 {
		t.Errorf("Usage.CompletionTokens = %d, want 3", last.Usage.CompletionTokens)
	}
}

// TestGemini_Stream_SharedParser_ToolCall: a function-call chunk
// emits a tool_call event with the right Name/Args/ID.
func TestGemini_Stream_SharedParser_ToolCall(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	frames := []string{
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call-1","name":"get_weather","args":{"city":"SF"}}}],"role":"model"},"finishReason":""}]}`,
		`{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"totalTokenCount":20}}`,
	}
	srv := geminiSSEServer(t, frames)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "weather?"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	events := drainGeminiStream(t, sr, 5*time.Second)

	var toolEvts []GraycodeRouterStreamEvent
	var doneEvt *GraycodeRouterStreamEvent
	for i := range events {
		if events[i].Type == "tool_call" {
			toolEvts = append(toolEvts, events[i])
		}
		if events[i].Type == "done" {
			doneEvt = &events[i]
		}
	}
	if len(toolEvts) != 1 {
		t.Fatalf("expected 1 tool_call event, got %d: %+v", len(toolEvts), events)
	}
	tc := toolEvts[0].ToolCall
	if tc == nil {
		t.Fatal("tool_call event missing ToolCall payload")
	}
	if tc.ID != "call-1" {
		t.Errorf("ToolCall.ID = %q, want call-1", tc.ID)
	}
	if tc.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want get_weather", tc.Name)
	}
	if got, _ := tc.Arguments["city"].(string); got != "SF" {
		t.Errorf("ToolCall.Arguments[city] = %v, want \"SF\"", tc.Arguments["city"])
	}
	if doneEvt == nil {
		t.Fatal("missing done event")
	}
	if doneEvt.Usage == nil || doneEvt.Usage.PromptTokens != 12 {
		t.Errorf("done.Usage.PromptTokens = %v, want 12", doneEvt.Usage)
	}
}

// TestGemini_Stream_SharedParser_DoneWithUsage: usage in the final
// chunk produces a single "done" event carrying both StopReason
// and Usage.
func TestGemini_Stream_SharedParser_DoneWithUsage(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	frames := []string{
		`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":""}]}`,
		`{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":13,"totalTokenCount":20}}`,
	}
	srv := geminiSSEServer(t, frames)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "go"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()
	events := drainGeminiStream(t, sr, 5*time.Second)

	var doneEvt *GraycodeRouterStreamEvent
	for i := range events {
		if events[i].Type == "done" {
			doneEvt = &events[i]
		}
	}
	if doneEvt == nil {
		t.Fatal("missing done event")
	}
	if doneEvt.StopReason != "max_tokens" {
		t.Errorf("StopReason = %q, want \"max_tokens\"", doneEvt.StopReason)
	}
	if doneEvt.Usage == nil {
		t.Fatal("done event missing Usage (original Gemini emits usage in done)")
	}
	if doneEvt.Usage.TotalTokens != 20 {
		t.Errorf("TotalTokens = %d, want 20", doneEvt.Usage.TotalTokens)
	}
}

// TestGemini_Stream_SharedParser_EmptyStream: a server that returns
// 200 with no body should still emit a "done" event so consumers
// don't hang.
func TestGemini_Stream_SharedParser_EmptyStream(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "x"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()
	events := drainGeminiStream(t, sr, 3*time.Second)

	if len(events) != 1 {
		t.Fatalf("expected 1 event (done), got %d: %+v", len(events), events)
	}
	if events[0].Type != "done" {
		t.Errorf("event type = %q, want \"done\"", events[0].Type)
	}
}

// TestGemini_Stream_SharedParser_ContextCancel: cancelling the
// context stops the stream and the consumer sees the channel close.
func TestGemini_Stream_SharedParser_ContextCancel(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	// Server writes one event then blocks indefinitely.
	gotFrame := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":""}]}`)
		flusher.Flush()
		gotFrame <- struct{}{}
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	sr, err := c.StreamChat(ctx, []GraycodeRouterMessage{
		{Role: "user", Content: "x"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	// Wait for the first event to confirm the stream is open.
	select {
	case <-sr.Events:
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive first event")
	}
	select {
	case <-gotFrame:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not flush first frame")
	}
	// Cancel the context. The stream should close.
	cancel()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-sr.Events:
			if !ok {
				return // channel closed cleanly
			}
		case <-deadline.C:
			t.Fatal("stream did not close within 3s of cancel")
		}
	}
}

// TestGemini_Stream_SharedParser_LargeEvent: a single SSE event
// larger than the initial 64KB scanner buffer (but well under
// the 2MB max) is parsed correctly.
func TestGemini_Stream_SharedParser_LargeEvent(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	// Build a single 200KB text payload.
	bigText := strings.Repeat("x", 200*1024)
	frame := `{"candidates":[{"content":{"parts":[{"text":"` + bigText + `"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":50000,"totalTokenCount":50001}}`
	srv := geminiSSEServer(t, []string{frame})
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "x"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()
	events := drainGeminiStream(t, sr, 10*time.Second)

	var contentEvt *GraycodeRouterStreamEvent
	for i := range events {
		if events[i].Type == "content" {
			contentEvt = &events[i]
			break
		}
	}
	if contentEvt == nil {
		t.Fatal("no content event in 200KB response")
	}
	if len(contentEvt.Content) != len(bigText) {
		t.Errorf("content length = %d, want %d", len(contentEvt.Content), len(bigText))
	}
}

// TestGemini_Stream_SharedParser_FeatureFlag: when
// GRAYCODE_ROUTER_GEMINI_SHARED_PARSER=0, the old streamLoop path is used.
func TestGemini_Stream_SharedParser_FeatureFlag(t *testing.T) {
	frames := []string{
		`{"candidates":[{"content":{"parts":[{"text":"shared-on"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
	}
	srv := geminiSSEServer(t, frames)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)

	t.Run("shared-on-by-default", func(t *testing.T) {
		_ = os.Unsetenv(geminiSharedParserEnvVar)
		sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
			{Role: "user", Content: "x"},
		}, ChatOptions{Model: "gemini-test"})
		if err != nil {
			t.Fatalf("StreamChat: %v", err)
		}
		defer sr.Close()
		events := drainGeminiStream(t, sr, 5*time.Second)
		if len(events) < 2 {
			t.Fatalf("expected at least 2 events, got %d", len(events))
		}
		if events[0].Type != "content" || events[0].Content != "shared-on" {
			t.Errorf("events[0] = %+v", events[0])
		}
	})

	t.Run("opt-out-with-0", func(t *testing.T) {
		t.Setenv(geminiSharedParserEnvVar, "0")
		sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
			{Role: "user", Content: "x"},
		}, ChatOptions{Model: "gemini-test"})
		if err != nil {
			t.Fatalf("StreamChat: %v", err)
		}
		defer sr.Close()
		events := drainGeminiStream(t, sr, 5*time.Second)
		if len(events) < 2 {
			t.Fatalf("expected at least 2 events, got %d", len(events))
		}
		if events[0].Type != "content" || events[0].Content != "shared-on" {
			t.Errorf("events[0] = %+v (opt-out path)", events[0])
		}
	})

	t.Run("opt-out-with-false", func(t *testing.T) {
		t.Setenv(geminiSharedParserEnvVar, "false")
		sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
			{Role: "user", Content: "x"},
		}, ChatOptions{Model: "gemini-test"})
		if err != nil {
			t.Fatalf("StreamChat: %v", err)
		}
		defer sr.Close()
		_ = drainGeminiStream(t, sr, 5*time.Second)
	})

	t.Run("explicit-1-stays-on", func(t *testing.T) {
		t.Setenv(geminiSharedParserEnvVar, "1")
		sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
			{Role: "user", Content: "x"},
		}, ChatOptions{Model: "gemini-test"})
		if err != nil {
			t.Fatalf("StreamChat: %v", err)
		}
		defer sr.Close()
		events := drainGeminiStream(t, sr, 5*time.Second)
		if events[0].Type != "content" {
			t.Errorf("events[0].Type = %q, want content", events[0].Type)
		}
	})
}

// TestProcessGeminiStream_PreservesDoneWithUsage: directly test
// processGeminiStream (the new function) to verify the done-with-usage
// semantic without going through the HTTP layer.
func TestProcessGeminiStream_PreservesDoneWithUsage(t *testing.T) {
	usageFrame := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}}`
	sseCh := make(chan SSEEvent, 2)
	sseCh <- SSEEvent{Data: usageFrame}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := processGeminiStream(ctx, sseCh, slogDiscard())
	close(sseCh)

	events := collectGraycodeRouterStreamEvents(t, out, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].Type != "done" {
		t.Errorf("event type = %q, want done", events[0].Type)
	}
	if events[0].Usage == nil {
		t.Fatal("Usage missing from done event")
	}
	if events[0].Usage.TotalTokens != 3 {
		t.Errorf("TotalTokens = %d, want 3", events[0].Usage.TotalTokens)
	}
}

// TestProcessGeminiStream_DoneWithoutUsage: a finish reason
// without usage emits a "done" with StopReason but no Usage.
func TestProcessGeminiStream_DoneWithoutUsage(t *testing.T) {
	frames := []SSEEvent{
		{Data: `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":""}]}`},
		{Data: `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}]}`},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseCh := make(chan SSEEvent, len(frames))
	for _, f := range frames {
		sseCh <- f
	}
	close(sseCh)

	out := processGeminiStream(ctx, sseCh, slogDiscard())
	events := collectGraycodeRouterStreamEvents(t, out, 2*time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != "content" {
		t.Errorf("events[0].Type = %q, want content", events[0].Type)
	}
	if events[1].Type != "done" {
		t.Errorf("events[1].Type = %q, want done", events[1].Type)
	}
	if events[1].Usage != nil {
		t.Errorf("Usage = %+v, want nil (finish-reason-only done)", events[1].Usage)
	}
	if events[1].StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", events[1].StopReason)
	}
}

// TestProcessGeminiStream_EmptyStream_EmitsDone: when the SSE
// channel closes without a finish reason, a bare "done" event is
// emitted (matches the original streamLoop's if !doneSent fallback).
func TestProcessGeminiStream_EmptyStream_EmitsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseCh := make(chan SSEEvent)
	close(sseCh)

	out := processGeminiStream(ctx, sseCh, slogDiscard())
	events := collectGraycodeRouterStreamEvents(t, out, 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].Type != "done" {
		t.Errorf("event type = %q, want done", events[0].Type)
	}
}

// collectGraycodeRouterStreamEvents reads events until the channel closes
// or the deadline fires. Used by the processGeminiStream direct tests.
func collectGraycodeRouterStreamEvents(t *testing.T, ch <-chan GraycodeRouterStreamEvent, timeout time.Duration) []GraycodeRouterStreamEvent {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var out []GraycodeRouterStreamEvent
	for {
		select {
		case <-deadline.C:
			return out
		case evt, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, evt)
		}
	}
}

// TestGemini_Stream_SharedParser_PreservesClientState
func TestGemini_Stream_SharedParser_PreservesClientState(t *testing.T) {
	t.Setenv(geminiSharedParserEnvVar, "1")
	srv := geminiSSEServer(t, []string{
		`{"candidates":[{"content":{"parts":[{"text":"x"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
	})
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	if c.HTTPClient() == nil {
		t.Fatal("NewGeminiClient did not initialize httpClient")
	}
	if c.Retry().MaxRetries == 0 {
		t.Fatal("retry config not initialized")
	}

	sr, err := c.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "x"},
	}, ChatOptions{Model: "gemini-test"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()
	_ = drainGeminiStream(t, sr, 5*time.Second)
	if c.HTTPClient() == nil {
		t.Error("httpClient was clobbered by StreamChat")
	}
}
