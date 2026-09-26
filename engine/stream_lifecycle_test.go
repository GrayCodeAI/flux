package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

var lifecycleRoute = Route{Provider: "mock", Model: "mock/model"}

// drainEngineStream reads until Next reports false, failing if it stalls.
func drainEngineStream(t *testing.T, stream *Stream) []Event {
	t.Helper()
	done := make(chan []Event, 1)
	go func() {
		var events []Event
		for stream.Next() {
			events = append(events, stream.Event())
		}
		done <- events
	}()
	select {
	case events := <-done:
		return events
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish")
		return nil
	}
}

func engineEventTypes(events []Event) []string {
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func streamOf(ctx context.Context, cancel context.CancelFunc, events ...core.FluxStreamEvent) *Stream {
	source := make(chan core.FluxStreamEvent, len(events))
	for _, event := range events {
		source <- event
	}
	close(source)
	return newStream(ctx, cancel, llm.NewStreamResult(source, "request-1", nil), lifecycleRoute)
}

func TestStreamProviderFailureWithLiveContextIsNotCancellation(t *testing.T) {
	tests := []struct {
		name  string
		event core.FluxStreamEvent
	}{
		{"upstream timeout error", core.FluxStreamEvent{
			Type: "error", Error: "context deadline exceeded (Client.Timeout exceeded while reading body)",
			ErrorInfo: &llm.StreamErrorInfo{Kind: llm.ErrKindTimeout, Retryable: true},
		}},
		{"upstream canceled error", core.FluxStreamEvent{
			Type: "error", Error: "context canceled",
			ErrorInfo: &llm.StreamErrorInfo{Kind: llm.ErrKindCanceled},
		}},
		{"upstream cancelled event", core.FluxStreamEvent{
			Type: "cancelled", Error: "context canceled",
			ErrorInfo: &llm.StreamErrorInfo{Kind: llm.ErrKindCanceled},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			stream := streamOf(ctx, cancel, tt.event)
			defer stream.Close()

			events := drainEngineStream(t, stream)
			for _, event := range events {
				if event.Type == EventCancelled {
					t.Fatalf("events = %v; a provider failure must not look like the caller's cancellation", engineEventTypes(events))
				}
			}
			err := stream.Err()
			if IsCode(err, ErrorCancelled) || !IsCode(err, ErrorProviderUnavailable) {
				t.Fatalf("error = %v, want provider_unavailable", err)
			}
			var engineErr *Error
			if !errors.As(err, &engineErr) || !engineErr.Retryable {
				t.Fatalf("error = %#v, want retryable", err)
			}
		})
	}
}

func TestStreamCallerCancellationReportsContextCause(t *testing.T) {
	tests := []struct {
		name     string
		ctx      func() (context.Context, context.CancelFunc)
		wantKind string
		wantErr  error
	}{
		{"cancel", func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}, llm.ErrKindCanceled, context.Canceled},
		{"deadline", func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}, llm.ErrKindTimeout, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			// The provider reports the consequence of the caller's
			// cancellation with a kind that does not match the context.
			stream := streamOf(ctx, cancel, core.FluxStreamEvent{
				Type: "error", Error: "stream read error",
				ErrorInfo: &llm.StreamErrorInfo{Kind: llm.ErrKindUnavailable, Retryable: true},
			})
			defer stream.Close()

			events := drainEngineStream(t, stream)
			if len(events) == 0 {
				t.Fatal("stream ended without events")
			}
			last := events[len(events)-1]
			if last.Type != EventCancelled || last.ErrorInfo == nil || last.ErrorInfo.Kind != tt.wantKind {
				t.Fatalf("events = %v, last = %+v, want cancelled terminal with kind %s", engineEventTypes(events), last, tt.wantKind)
			}
			if last.RequestID != "request-1" {
				t.Fatalf("request ID = %q, want request-1", last.RequestID)
			}
			err := stream.Err()
			if !IsCode(err, ErrorCancelled) || !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want cancelled caused by %v", err, tt.wantErr)
			}
		})
	}
}
