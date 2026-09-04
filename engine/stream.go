package engine

import (
	"context"
	"sync"

	"github.com/GrayCodeAI/graycode-router/client"
)

// Stream is a normalized, pull-based event stream. Next must not be called
// concurrently. Close is idempotent and may be called from another goroutine.
type Stream struct {
	ctx    context.Context
	cancel context.CancelFunc
	source *client.StreamResult
	route  Route
	events chan Event

	mu      sync.Mutex
	current Event
	err     error
	once    sync.Once
}

func newStream(ctx context.Context, cancel context.CancelFunc, source *client.StreamResult, route Route) *Stream {
	s := &Stream{ctx: ctx, cancel: cancel, source: source, route: route, events: make(chan Event, 32)}
	go s.forward()
	return s
}

// Next advances to the next event.
func (s *Stream) Next() bool {
	if s == nil {
		return false
	}
	event, ok := <-s.events
	if !ok {
		return false
	}
	s.mu.Lock()
	s.current = event
	s.mu.Unlock()
	return true
}

// Event returns the most recent event produced by Next.
func (s *Stream) Event() Event {
	if s == nil {
		return Event{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Err returns the terminal stream error, if any.
func (s *Stream) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close cancels generation and releases provider resources.
func (s *Stream) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.source != nil {
			s.source.Close()
		}
	})
	return nil
}

func (s *Stream) forward() {
	defer close(s.events)
	defer s.Close()
	if !s.emit(Event{Type: EventRouteSelected, Route: &s.route}) {
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			s.setError(classify("stream", s.route, s.ctx.Err()))
			return
		case event, ok := <-s.source.Events:
			if !ok {
				return
			}
			normalized, err := normalizeEvent(event)
			if err != nil {
				s.setError(classify("stream", s.route, err))
				return
			}
			if !s.emit(normalized) {
				return
			}
		}
	}
}

func (s *Stream) emit(event Event) bool {
	select {
	case s.events <- event:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Stream) setError(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func normalizeEvent(event client.GraycodeRouterStreamEvent) (Event, error) {
	out := Event{
		Content: event.Content, Thinking: event.Thinking, RequestID: event.RequestID,
		Usage: fromClientUsage(event.Usage), StopReason: event.StopReason,
		TTFTms: event.TTFTms,
	}
	if out.TTFTms == 0 {
		out.TTFTms = event.TTFT
	}
	switch event.Type {
	case "content":
		out.Type = EventContentDelta
	case "thinking":
		out.Type = EventThinkingDelta
	case "tool_call":
		out.Type = EventToolCallDone
	case "tool_input_delta":
		out.Type = EventToolCallDelta
	case "done":
		// Usage remains attached to done for backward-friendly single-event
		// accounting; future providers may also emit EventUsage separately.
		out.Type = EventDone
	case "ttft":
		out.Type = EventTTFT
	case "continuation":
		out.Type = EventContinuation
	case "error":
		if event.Warning != "" {
			// Non-fatal health diagnostic (e.g. a reasoning-only response):
			// client/core marks these with Warning so they can be surfaced
			// without terminating the stream — the terminal done/usage event
			// follows. Forward as a warning event; do not set Err()/stop.
			return Event{Type: EventWarning, Warning: event.Warning}, nil
		}
		return Event{}, &Error{Code: ErrorProviderUnavailable, Operation: "stream", Message: event.Error}
	default:
		out.Type = event.Type
	}
	if event.ToolCall != nil {
		out.ToolCall = &ToolCall{ID: event.ToolCall.ID, Name: event.ToolCall.Name, Arguments: event.ToolCall.Arguments}
	}
	return out, nil
}
