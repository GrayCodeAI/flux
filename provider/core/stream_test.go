//nolint:errcheck
package core

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/flux/llm"
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

func TestSSEParseFlushesUnterminatedFinalEvent(t *testing.T) {
	t.Parallel()
	body := io.NopCloser(strings.NewReader("data: [DONE]"))
	ch := ParseSSEStream(context.Background(), body, testLogger())

	var events []SSEEvent
	for event := range ch {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Data != "[DONE]" {
		t.Fatalf("events = %+v, want one [DONE] event", events)
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

func TestCoordinateStreamResultTerminalContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		events      []FluxStreamEvent
		wantTypes   []string
		wantErrKind string
	}{
		{
			name:        "truncated",
			events:      []FluxStreamEvent{{Type: "content", Content: "partial"}},
			wantTypes:   []string{"content", "error"},
			wantErrKind: llm.ErrKindTruncated,
		},
		{
			name: "fatal before done",
			events: []FluxStreamEvent{
				{Type: "error", Error: "connection reset"},
				{Type: "done"},
			},
			wantTypes: []string{"error"},
		},
		{
			name: "terminal before late usage",
			events: []FluxStreamEvent{
				{Type: "done", StopReason: "stop"},
				{Type: "usage", Usage: &FluxUsage{TotalTokens: 1}},
			},
			wantTypes: []string{"done"},
		},
		{
			name: "warning before done",
			events: []FluxStreamEvent{
				{Type: "error", Error: "empty response", Warning: "empty response"},
				{Type: "done", StopReason: "stop"},
			},
			wantTypes: []string{"error", "done"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events := make(chan FluxStreamEvent, len(tt.events))
			for _, event := range tt.events {
				events <- event
			}
			close(events)

			var closes atomic.Int32
			source := llm.NewStreamResult(events, "request-1", func() { closes.Add(1) })
			result := CoordinateStreamResult(context.Background(), source)
			var got []FluxStreamEvent
			for event := range result.Events {
				got = append(got, event)
			}
			result.Close()

			if len(got) != len(tt.wantTypes) {
				t.Fatalf("events = %+v, want types %v", got, tt.wantTypes)
			}
			for i, wantType := range tt.wantTypes {
				if got[i].Type != wantType {
					t.Fatalf("event[%d].Type = %q, want %q", i, got[i].Type, wantType)
				}
				if got[i].RequestID != "request-1" {
					t.Fatalf("event[%d].RequestID = %q, want request-1", i, got[i].RequestID)
				}
			}
			if tt.wantErrKind != "" {
				last := got[len(got)-1]
				if last.ErrorInfo == nil || last.ErrorInfo.Kind != tt.wantErrKind || !last.ErrorInfo.Retryable {
					t.Fatalf("terminal error info = %+v, want retryable kind %q", last.ErrorInfo, tt.wantErrKind)
				}
			}
			if count := closes.Load(); count != 1 {
				t.Fatalf("source close count = %d, want 1", count)
			}
		})
	}
}

func TestTransformStreamResultCloseUnblocksForwarder(t *testing.T) {
	t.Parallel()

	events := make(chan FluxStreamEvent)
	streamCtx, cancel := context.WithCancel(context.Background())
	producerDone := make(chan struct{})
	go func() {
		defer close(events)
		defer close(producerDone)
		<-streamCtx.Done()
	}()

	var closes atomic.Int32
	source := llm.NewStreamResult(events, "request-2", func() {
		closes.Add(1)
		cancel()
	})
	result := TransformStreamResult(context.Background(), source, func(_ context.Context, event FluxStreamEvent) (FluxStreamEvent, error) {
		return event, nil
	})
	result.Close()

	select {
	case <-producerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("producer did not stop after stream close")
	}
	select {
	case _, ok := <-result.Events:
		if ok {
			t.Fatal("unexpected event after close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transformed event channel did not close")
	}
	result.Close()
	if got := closes.Load(); got != 1 {
		t.Fatalf("source close count = %d, want 1", got)
	}
}

func TestUsageDeltaHandlesCumulativeAndSplitUsage(t *testing.T) {
	t.Parallel()

	t.Run("cumulative", func(t *testing.T) {
		var state *FluxUsage
		var prompt, completion int
		for _, usage := range []*FluxUsage{
			{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
			{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		} {
			delta := UsageDelta(state, usage)
			state = MergeUsage(state, usage)
			if delta != nil {
				prompt += delta.PromptTokens
				completion += delta.CompletionTokens
			}
		}
		if prompt != 1 || completion != 2 {
			t.Fatalf("prompt=%d completion=%d, want 1/2", prompt, completion)
		}
	})

	t.Run("split then aggregate", func(t *testing.T) {
		var state *FluxUsage
		var prompt, completion int
		for _, usage := range []*FluxUsage{
			{PromptTokens: 10},
			{CompletionTokens: 5},
			{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		} {
			delta := UsageDelta(state, usage)
			state = MergeUsage(state, usage)
			if delta != nil {
				prompt += delta.PromptTokens
				completion += delta.CompletionTokens
			}
		}
		if prompt != 10 || completion != 5 {
			t.Fatalf("prompt=%d completion=%d, want 10/5", prompt, completion)
		}
	})

	t.Run("reverse split then aggregate", func(t *testing.T) {
		var state *FluxUsage
		var prompt, completion int
		for _, usage := range []*FluxUsage{
			{CompletionTokens: 5},
			{PromptTokens: 10},
			{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		} {
			delta := UsageDelta(state, usage)
			state = MergeUsage(state, usage)
			if delta != nil {
				prompt += delta.PromptTokens
				completion += delta.CompletionTokens
			}
		}
		if prompt != 10 || completion != 5 {
			t.Fatalf("prompt=%d completion=%d, want 10/5", prompt, completion)
		}
	})

	t.Run("duplicate aggregate", func(t *testing.T) {
		usage := &FluxUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}
		state := MergeUsage(nil, usage)
		if delta := UsageDelta(state, usage); delta != nil {
			t.Fatalf("delta = %+v, want nil", delta)
		}
	})
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

	var results []FluxStreamEvent
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

	var results []FluxStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	// Find the tool_call event
	var toolCallEvt *FluxStreamEvent
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

	var results []FluxStreamEvent
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

	var results []FluxStreamEvent
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

	var results []FluxStreamEvent
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

	var results []FluxStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	var toolCallEvt *FluxStreamEvent
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

	var results []FluxStreamEvent
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

	var results []FluxStreamEvent
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

func TestSSEOpenAIChannelCloseDoesNotSynthesizeDone(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 1)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"partial"},"finish_reason":null}]}`}
	close(events)

	var results []FluxStreamEvent
	for event := range ProcessOpenAIStream(context.Background(), events, testLogger()) {
		results = append(results, event)
	}
	if len(results) == 0 {
		t.Fatal("expected content event")
	}
	for _, event := range results {
		if event.Type == "done" {
			t.Fatalf("unexpected done event: %+v", event)
		}
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
