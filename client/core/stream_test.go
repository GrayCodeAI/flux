//nolint:errcheck
package core

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- ParseSSEStream tests ---

func TestSSEParseBasicEvents(t *testing.T) {
	t.Parallel()
	sseData := "event:message\ndata:hello world\n\nevent:done\ndata:bye\n\n"
	body := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()

	ch := ParseSSEStream(ctx, body, testLogger())

	var events []SSEEvent
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != "message" || events[0].Data != "hello world" {
		t.Errorf("event[0] = %+v, want event=message data=hello world", events[0])
	}
	if events[1].Event != "done" || events[1].Data != "bye" {
		t.Errorf("event[1] = %+v, want event=done data=bye", events[1])
	}
}

func TestSSEParseMultilineData(t *testing.T) {
	t.Parallel()
	sseData := "event:content\ndata:line one\ndata:line two\ndata:line three\n\n"
	body := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()

	ch := ParseSSEStream(ctx, body, testLogger())

	var events []SSEEvent
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "line one\nline two\nline three" {
		t.Errorf("multiline data = %q, want 'line one\\nline two\\nline three'", events[0].Data)
	}
}

func TestSSEParseEmptyEvents(t *testing.T) {
	t.Parallel()
	// Empty lines between events should not produce events with no data
	sseData := "\n\nevent:ping\ndata:pong\n\n\n\n"
	body := io.NopCloser(strings.NewReader(sseData))
	ctx := context.Background()

	ch := ParseSSEStream(ctx, body, testLogger())

	var events []SSEEvent
	for evt := range ch {
		events = append(events, evt)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (empty events skipped), got %d: %+v", len(events), events)
	}
	if events[0].Event != "ping" || events[0].Data != "pong" {
		t.Errorf("got %+v, want event=ping data=pong", events[0])
	}
}

func TestSSEParseContextCancellation(t *testing.T) {
	t.Parallel()
	// Create a body that will block until closed
	pr, pw := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	ch := ParseSSEStream(ctx, pr, testLogger())

	// Write one event
	_, _ = pw.Write([]byte("event:first\ndata:one\n\n"))
	evt := <-ch
	if evt.Event != "first" {
		t.Fatalf("expected first event, got %+v", evt)
	}

	// Cancel context and close writer to unblock the scanner
	cancel()
	pw.Close()

	// Channel should close promptly
	select {
	case _, ok := <-ch:
		if ok {
			// Might get one more event buffered, drain it
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after context cancellation")
	}
}

// --- ProcessAnthropicStream tests ---

func TestSSEAnthropicContentBlockDelta(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`}
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`}
	events <- SSEEvent{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`}
	events <- SSEEvent{Event: "message_stop", Data: `{"type":"message_stop"}`}
	close(events)

	ctx := context.Background()
	ch := ProcessAnthropicStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	// Should have: content "Hello", content " world", done
	contentCount := 0
	doneCount := 0
	for _, r := range results {
		switch r.Type {
		case "content":
			contentCount++
		case "done":
			doneCount++
			if r.StopReason != "end_turn" {
				t.Errorf("expected stop_reason=end_turn, got %q", r.StopReason)
			}
		}
	}
	if contentCount != 2 {
		t.Errorf("expected 2 content events, got %d", contentCount)
	}
	if doneCount != 1 {
		t.Errorf("expected 1 done event, got %d", doneCount)
	}
}

func TestSSEAnthropicToolUse(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc_123","name":"get_weather"}}`}
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\""}}`}
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"London\"}"}}`}
	events <- SSEEvent{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`}
	events <- SSEEvent{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`}
	events <- SSEEvent{Event: "message_stop", Data: `{"type":"message_stop"}`}
	close(events)

	ctx := context.Background()
	ch := ProcessAnthropicStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	// Find the tool_call event
	var toolCallEvt *EyrieStreamEvent
	for i := range results {
		if results[i].Type == "tool_call" {
			toolCallEvt = &results[i]
			break
		}
	}
	if toolCallEvt == nil {
		t.Fatal("expected a tool_call event")
	}
	if toolCallEvt.ToolCall.ID != "tc_123" {
		t.Errorf("tool call ID = %q, want tc_123", toolCallEvt.ToolCall.ID)
	}
	if toolCallEvt.ToolCall.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", toolCallEvt.ToolCall.Name)
	}
	city, _ := toolCallEvt.ToolCall.Arguments["city"].(string)
	if city != "London" {
		t.Errorf("tool call args[city] = %q, want London", city)
	}
}

func TestSSEAnthropicThinkingDelta(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`}
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","text":"Let me think..."}}`}
	events <- SSEEvent{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`}
	events <- SSEEvent{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`}
	events <- SSEEvent{Event: "message_stop", Data: `{"type":"message_stop"}`}
	close(events)

	ctx := context.Background()
	ch := ProcessAnthropicStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	thinkingFound := false
	for _, r := range results {
		if r.Type == "thinking" && r.Thinking == "Let me think..." {
			thinkingFound = true
		}
	}
	if !thinkingFound {
		t.Error("expected thinking event with 'Let me think...'")
	}
}

func TestSSEAnthropicThinkingTextDeltaHidden(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Event: "content_block_start", Data: `{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`}
	events <- SSEEvent{Event: "content_block_delta", Data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"private reasoning"}}`}
	events <- SSEEvent{Event: "content_block_stop", Data: `{"type":"content_block_stop","index":0}`}
	events <- SSEEvent{Event: "message_stop", Data: `{"type":"message_stop"}`}
	close(events)

	ctx := context.Background()
	ch := ProcessAnthropicStream(ctx, events, testLogger())

	var sawThinking, sawContent bool
	for evt := range ch {
		switch evt.Type {
		case "thinking":
			if evt.Thinking == "private reasoning" {
				sawThinking = true
			}
		case "content":
			sawContent = true
		}
	}
	if !sawThinking {
		t.Fatal("expected thinking event for text_delta inside thinking block")
	}
	if sawContent {
		t.Fatal("did not expect visible content from thinking block text_delta")
	}
}

func TestSSEAnthropicStopReason(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Event: "message_delta", Data: `{"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`}
	events <- SSEEvent{Event: "message_stop", Data: `{"type":"message_stop"}`}
	close(events)

	ctx := context.Background()
	ch := ProcessAnthropicStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	doneFound := false
	for _, r := range results {
		if r.Type == "done" {
			doneFound = true
			if r.StopReason != "max_tokens" {
				t.Errorf("stop reason = %q, want max_tokens", r.StopReason)
			}
		}
	}
	if !doneFound {
		t.Error("expected done event")
	}
}

// --- ProcessOpenAIStream tests ---

func TestSSEOpenAIChoicesDelta(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"Hi"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":" there"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	contentParts := []string{}
	for _, r := range results {
		if r.Type == "content" {
			contentParts = append(contentParts, r.Content)
		}
	}
	joined := strings.Join(contentParts, "")
	if joined != "Hi there" {
		t.Errorf("content = %q, want 'Hi there'", joined)
	}

	// Check done event
	doneFound := false
	for _, r := range results {
		if r.Type == "done" {
			doneFound = true
			if r.StopReason != "stop" {
				t.Errorf("stop reason = %q, want stop", r.StopReason)
			}
		}
	}
	if !doneFound {
		t.Error("expected done event")
	}
}

func TestSSEOpenAIToolCallsAccumulation(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	// First chunk: tool call starts
	events <- SSEEvent{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}`}
	// Second chunk: arguments continue
	events <- SSEEvent{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]},"finish_reason":null}]}`}
	// Third chunk: arguments finish
	events <- SSEEvent{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"hello\"}"}}]},"finish_reason":null}]}`}
	// Finish
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	var toolCallEvt *EyrieStreamEvent
	for i := range results {
		if results[i].Type == "tool_call" {
			toolCallEvt = &results[i]
			break
		}
	}
	if toolCallEvt == nil {
		t.Fatal("expected a tool_call event")
	}
	if toolCallEvt.ToolCall.ID != "call_abc" {
		t.Errorf("tool call ID = %q, want call_abc", toolCallEvt.ToolCall.ID)
	}
	if toolCallEvt.ToolCall.Name != "search" {
		t.Errorf("tool call name = %q, want search", toolCallEvt.ToolCall.Name)
	}
	query, _ := toolCallEvt.ToolCall.Arguments["query"].(string)
	if query != "hello" {
		t.Errorf("tool call args[query] = %q, want hello", query)
	}
}

func TestSSEOpenAIFinishReason(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"done"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"length"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	for _, r := range results {
		if r.Type == "done" {
			if r.StopReason != "length" {
				t.Errorf("stop reason = %q, want length", r.StopReason)
			}
			return
		}
	}
	t.Error("expected done event with finish_reason=length")
}

func TestSSEOpenAIUsage(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[],"usage":{"prompt_tokens":50,"completion_tokens":10,"total_tokens":60}}`}
	events <- SSEEvent{Data: `[DONE]`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []EyrieStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	usageFound := false
	for _, r := range results {
		if r.Type == "usage" && r.Usage != nil {
			usageFound = true
			if r.Usage.PromptTokens != 50 {
				t.Errorf("prompt tokens = %d, want 50", r.Usage.PromptTokens)
			}
			if r.Usage.CompletionTokens != 10 {
				t.Errorf("completion tokens = %d, want 10", r.Usage.CompletionTokens)
			}
			if r.Usage.TotalTokens != 60 {
				t.Errorf("total tokens = %d, want 60", r.Usage.TotalTokens)
			}
		}
	}
	if !usageFound {
		t.Error("expected usage event")
	}
}

// --- ParseInlineToolCalls tests ---

func TestSSEParseInlineToolCallsCanopywave(t *testing.T) {
	t.Parallel()
	text := `Here is my response.
<|tool_calls_section_begin|>
<|tool_call_begin|>
functions.get_weather:0
<|tool_call_argument_begin|>
{"city":"Tokyo","units":"celsius"}
<|tool_call_end|>
<|tool_calls_section_end|>`

	cleanText, toolCalls := ParseInlineToolCalls(text)

	if cleanText != "Here is my response." {
		t.Errorf("clean text = %q, want 'Here is my response.'", cleanText)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", toolCalls[0].Name)
	}
	city, _ := toolCalls[0].Arguments["city"].(string)
	if city != "Tokyo" {
		t.Errorf("args[city] = %q, want Tokyo", city)
	}
	units, _ := toolCalls[0].Arguments["units"].(string)
	if units != "celsius" {
		t.Errorf("args[units] = %q, want celsius", units)
	}
}

func TestSSEParseInlineToolCallsNoMarker(t *testing.T) {
	t.Parallel()
	text := "Just a normal response with no tool calls."
	cleanText, toolCalls := ParseInlineToolCalls(text)

	if cleanText != text {
		t.Errorf("clean text = %q, want original text", cleanText)
	}
	if len(toolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(toolCalls))
	}
}

func TestSSEParseInlineToolCallsMultiple(t *testing.T) {
	t.Parallel()
	text := `Thinking...
<|tool_calls_section_begin|>
<|tool_call_begin|>
functions.search:0
<|tool_call_argument_begin|>
{"query":"golang"}
<|tool_call_end|>
<|tool_call_begin|>
functions.read_file:1
<|tool_call_argument_begin|>
{"path":"/tmp/test.go"}
<|tool_call_end|>
<|tool_calls_section_end|>`

	cleanText, toolCalls := ParseInlineToolCalls(text)

	if cleanText != "Thinking..." {
		t.Errorf("clean text = %q, want 'Thinking...'", cleanText)
	}
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "search" {
		t.Errorf("first tool name = %q, want search", toolCalls[0].Name)
	}
	if toolCalls[1].Name != "read_file" {
		t.Errorf("second tool name = %q, want read_file", toolCalls[1].Name)
	}
	path, _ := toolCalls[1].Arguments["path"].(string)
	if path != "/tmp/test.go" {
		t.Errorf("second tool args[path] = %q, want /tmp/test.go", path)
	}
}
