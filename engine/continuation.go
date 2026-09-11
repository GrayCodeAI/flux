package engine

import (
	"context"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

// streamWithContinuation implements continuation at the stable engine layer.
// It deliberately does not depend on client.StreamChatWithContinuation, which
// is a deprecated compatibility helper scheduled for removal in Eyrie v0.3.
func streamWithContinuation(ctx context.Context, provider client.Provider, messages []client.EyrieMessage, opts client.ChatOptions, limits Limits) (*client.StreamResult, error) {
	maxContinuations := limits.MaxContinuations
	if maxContinuations <= 0 {
		maxContinuations = client.DefaultContinuationConfig().MaxContinuations
	}
	maxTotalTokens := limits.MaxTotalOutputTokens
	if maxTotalTokens <= 0 {
		maxTotalTokens = client.DefaultContinuationConfig().MaxTotalTokens
	}
	streamCtx, cancel := context.WithCancel(ctx)
	first, err := provider.StreamChat(streamCtx, messages, opts)
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan client.EyrieStreamEvent, 64)
	go func() {
		defer close(out)
		defer cancel()
		current := first
		requestID := first.RequestID
		msgs := append([]client.EyrieMessage(nil), messages...)
		totalOutput := 0

		for attempt := 0; ; attempt++ {
			var segment strings.Builder
			hadToolCall := false
			var terminal client.EyrieStreamEvent
			for event := range current.Events {
				switch event.Type {
				case "content":
					segment.WriteString(event.Content)
				case "tool_call":
					hadToolCall = true
				case "usage":
					if event.Usage != nil {
						totalOutput += event.Usage.CompletionTokens
					}
				case "done":
					terminal = event
					continue
				}
				if !emitEngineEvent(streamCtx, out, event) {
					current.Close()
					return
				}
			}
			current.Close()

			needsContinuation := terminal.StopReason == "max_tokens" || terminal.StopReason == "length"
			if !needsContinuation || hadToolCall || totalOutput >= maxTotalTokens || attempt >= maxContinuations {
				if terminal.Type == "" {
					terminal = client.EyrieStreamEvent{Type: "done", StopReason: terminal.StopReason, RequestID: requestID}
				}
				_ = emitEngineEvent(streamCtx, out, terminal)
				return
			}

			if !emitEngineEvent(streamCtx, out, client.EyrieStreamEvent{
				Type: "continuation", Content: requestID, StopReason: strconv.Itoa(attempt + 1),
			}) {
				return
			}
			msgs = append(
				msgs,
				client.EyrieMessage{Role: "assistant", Content: segment.String()},
				client.EyrieMessage{Role: "user", Content: "Continue."},
			)
			next, err := provider.StreamChat(streamCtx, msgs, opts)
			if err != nil {
				_ = emitEngineEvent(streamCtx, out, client.EyrieStreamEvent{Type: "error", Error: err.Error(), RequestID: requestID})
				return
			}
			current = next
			if next.RequestID != "" {
				requestID = next.RequestID
			}
		}
	}()
	return client.NewStreamResultWithRequestID(out, first.RequestID, cancel), nil
}

func emitEngineEvent(ctx context.Context, out chan<- client.EyrieStreamEvent, event client.EyrieStreamEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
