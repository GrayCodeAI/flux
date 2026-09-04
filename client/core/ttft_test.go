package core

import (
	"context"
	"testing"
	"time"
)

// TestTTFTEventFiresBeforeContent verifies that a "ttft" event is emitted before
// the first "content" event and that it carries a non-negative TTFT value.
func TestTTFTEventFiresBeforeContent(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []GraycodeRouterStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	// Find the ttft event and the first content event.
	ttftIdx := -1
	firstContentIdx := -1
	for i, r := range results {
		if r.Type == "ttft" && ttftIdx < 0 {
			ttftIdx = i
		}
		if r.Type == "content" && firstContentIdx < 0 {
			firstContentIdx = i
		}
	}

	if ttftIdx < 0 {
		t.Fatal("expected a ttft event, got none")
	}
	if firstContentIdx < 0 {
		t.Fatal("expected at least one content event, got none")
	}
	if ttftIdx >= firstContentIdx {
		t.Errorf("ttft event at index %d should come before first content at index %d",
			ttftIdx, firstContentIdx)
	}

	ttftEvt := results[ttftIdx]
	if ttftEvt.TTFT < 0 {
		t.Errorf("ttft event TTFT = %d, want >= 0", ttftEvt.TTFT)
	}
}

// TestTTFTEventFiresOnToolCallDelta verifies that a "ttft" event is emitted when
// the first token is a tool-call argument delta (not a content delta).
func TestTTFTEventFiresOnToolCallDelta(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"fn","arguments":""}}]},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var results []GraycodeRouterStreamEvent
	for evt := range ch {
		results = append(results, evt)
	}

	ttftFound := false
	for _, r := range results {
		if r.Type == "ttft" {
			ttftFound = true
			if r.TTFT < 0 {
				t.Errorf("ttft event TTFT = %d, want >= 0", r.TTFT)
			}
		}
	}
	if !ttftFound {
		t.Error("expected a ttft event when stream has only tool-call deltas")
	}
}

// TestTTFTEventFiredExactlyOnce verifies only one ttft event is emitted per stream.
func TestTTFTEventFiredExactlyOnce(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	for i := 0; i < 5; i++ {
		events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"x"},"finish_reason":null}]}`}
	}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStream(ctx, events, testLogger())

	var ttftCount int
	for evt := range ch {
		if evt.Type == "ttft" {
			ttftCount++
		}
	}
	if ttftCount != 1 {
		t.Errorf("expected exactly 1 ttft event, got %d", ttftCount)
	}
}

// TestTTFTValue verifies the TTFT value is plausible (measured against wall clock).
func TestTTFTValue(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)

	// Use ProcessOpenAIStreamWithOpts with a known start time to test the value.
	start := time.Now().Add(-50 * time.Millisecond) // pretend 50ms elapsed already
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"hi"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ctx := context.Background()
	ch := ProcessOpenAIStreamWithOpts(ctx, events, testLogger(), DefaultRepeatDetector(), start)

	var ttftEvt *GraycodeRouterStreamEvent
	for evt := range ch {
		if evt.Type == "ttft" {
			e := evt
			ttftEvt = &e
		}
	}

	if ttftEvt == nil {
		t.Fatal("expected a ttft event")
	}
	if ttftEvt.TTFT < 50 {
		t.Errorf("TTFT = %d ms, want >= 50 ms (start was set 50ms in the past)", ttftEvt.TTFT)
	}
}
