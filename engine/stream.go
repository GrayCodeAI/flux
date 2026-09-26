package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

// Stream is a normalized, pull-based event stream. Next must not be called
// concurrently. Close is idempotent and may be called from another goroutine.
type Stream struct {
	ctx    context.Context
	cancel context.CancelFunc
	source *core.StreamResult
	route  Route
	events chan Event
	closed chan struct{}

	mu      sync.Mutex
	current Event
	err     error
	once    sync.Once
}

func newStream(ctx context.Context, cancel context.CancelFunc, source *core.StreamResult, route Route) *Stream {
	if ctx == nil {
		ctx = context.Background()
	}
	s := &Stream{
		ctx: ctx, cancel: cancel, source: source, route: route,
		events: make(chan Event, 32), closed: make(chan struct{}),
	}
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
		if s.closed != nil {
			close(s.closed)
		}
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
	if !s.emit(Event{Type: EventRouteSelected, Route: cloneRoute(&s.route)}) {
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			if s.isClosed() {
				return
			}
			err := s.ctx.Err()
			s.setError(classify("stream", s.route, err))
			s.emitCancellation(err)
			return
		case event, ok := <-s.source.Events:
			if !ok {
				if s.ctx.Err() == nil {
					s.setError(classify("stream", s.route, core.ErrStreamTruncated))
				} else if !s.isClosed() {
					err := s.ctx.Err()
					s.setError(classify("stream", s.route, err))
					s.emitCancellation(err)
				}
				return
			}
			if event.RequestID == "" && s.source != nil {
				event.RequestID = s.source.RequestID
			}
			normalized, err := normalizeEvent(event)
			if err != nil {
				s.setError(classify("stream", s.route, err))
				return
			}
			if normalized.Type == EventCancelled {
				s.setError(classify("stream", s.route, streamContextError(normalized)))
				if !s.emit(normalized) {
					return
				}
				return
			}
			if !s.emit(normalized) {
				if !s.isClosed() && s.ctx.Err() != nil {
					err := s.ctx.Err()
					s.setError(classify("stream", s.route, err))
					s.emitCancellation(err)
				}
				return
			}
			if normalized.Type == EventDone {
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

func (s *Stream) emitCancellation(err error) {
	if err == nil || s.isClosed() {
		return
	}
	kind := llm.ErrKindCanceled
	if errors.Is(err, context.DeadlineExceeded) {
		kind = llm.ErrKindTimeout
	}
	event := Event{
		Type:      EventCancelled,
		Error:     err.Error(),
		ErrorInfo: &llm.StreamErrorInfo{Kind: kind},
		Route:     cloneRoute(&s.route),
	}
	select {
	case s.events <- event:
	case <-s.closed:
	}
}

func (s *Stream) isClosed() bool {
	if s.closed == nil {
		return false
	}
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *Stream) setError(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func normalizeEvent(event core.FluxStreamEvent) (Event, error) {
	out := Event{
		Content: event.Content, Thinking: event.Thinking, RequestID: event.RequestID,
		Error: event.Error, Warning: event.Warning, ErrorInfo: cloneErrorInfo(event.ErrorInfo),
		Usage: fromClientUsage(event.Usage), StopReason: event.StopReason,
		TTFTms: event.TTFTms, TTFT: event.TTFT, Route: cloneRoute(event.Route),
		ProviderBlock: cloneProviderBlock(event.ProviderBlock),
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
	case "cancelled", "canceled":
		out.Type = EventCancelled
		if out.ErrorInfo == nil {
			out.ErrorInfo = &llm.StreamErrorInfo{Kind: llm.ErrKindCanceled}
		}
	case "error":
		if event.Warning != "" {
			// Non-fatal health diagnostic (e.g. a reasoning-only response):
			// client/core marks these with Warning so they can be surfaced
			// without terminating the stream — the terminal done/usage event
			// follows. Forward as a warning event; do not set Err()/stop.
			return Event{
				Type: EventWarning, RequestID: event.RequestID, Error: event.Error,
				ErrorInfo: cloneErrorInfo(event.ErrorInfo), Warning: event.Warning,
				Route: cloneRoute(event.Route),
			}, nil
		}
		if event.ErrorInfo != nil {
			switch event.ErrorInfo.Kind {
			case llm.ErrKindCanceled:
				out.Type = EventCancelled
				return out, nil
			case llm.ErrKindTimeout:
				out.Type = EventCancelled
				return out, nil
			case llm.ErrKindTruncated:
				return Event{}, core.ErrStreamTruncated
			}
		}
		return Event{}, &Error{Code: ErrorProviderUnavailable, Operation: "stream", Message: event.Error}
	default:
		out.Type = event.Type
	}
	if event.ToolCall != nil {
		out.ToolCall = cloneToolCall(event.ToolCall)
	}
	return out, nil
}

func cloneErrorInfo(info *llm.StreamErrorInfo) *llm.StreamErrorInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	return &cloned
}

func cloneProviderBlock(block *llm.ProviderBlock) *llm.ProviderBlock {
	if block == nil {
		return nil
	}
	cloned := *block
	cloned.Data = append([]byte(nil), block.Data...)
	return &cloned
}

func cloneToolCall(call *llm.ToolCall) *llm.ToolCall {
	if call == nil {
		return nil
	}
	cloned := *call
	if call.RawArguments != nil {
		cloned.RawArguments = append([]byte(nil), call.RawArguments...)
	}
	if call.ProviderMetadata != nil {
		cloned.ProviderMetadata = make(map[string]json.RawMessage, len(call.ProviderMetadata))
		for key, value := range call.ProviderMetadata {
			cloned.ProviderMetadata[key] = append(json.RawMessage(nil), value...)
		}
	}
	return &cloned
}

func streamContextError(event Event) error {
	if event.ErrorInfo != nil && event.ErrorInfo.Kind == llm.ErrKindTimeout {
		return context.DeadlineExceeded
	}
	return context.Canceled
}
