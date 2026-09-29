package engine

import (
	"context"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

// streamWithContinuation implements continuation at the stable engine layer.
// It owns continuation at the engine boundary so provider adapters stay
// focused on one request/response exchange.
func streamWithContinuation(ctx context.Context, provider core.Provider, messages []core.FluxMessage, opts core.ChatOptions, limits Limits) (*core.StreamResult, error) {
	maxContinuations := limits.MaxContinuations
	if maxContinuations <= 0 {
		maxContinuations = core.DefaultContinuationConfig().MaxContinuations
	}
	maxTotalTokens := limits.MaxTotalOutputTokens
	if maxTotalTokens <= 0 {
		maxTotalTokens = core.DefaultContinuationConfig().MaxTotalTokens
	}
	streamCtx, cancel := context.WithCancel(ctx)
	first, err := provider.StreamChat(streamCtx, messages, opts)
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan core.FluxStreamEvent, 64)
	go func() {
		defer close(out)
		defer cancel()
		current := first
		requestID := first.RequestID
		msgs := append([]core.FluxMessage(nil), messages...)
		totalOutput := 0

		for attempt := 0; ; attempt++ {
			var segment strings.Builder
			hadToolCall := false
			var terminal core.FluxStreamEvent
		segmentLoop:
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
					break segmentLoop
				case "error":
					if event.Warning == "" {
						_ = emitEngineEvent(streamCtx, out, event)
						current.Close()
						return
					}
				}
				if !emitEngineEvent(streamCtx, out, event) {
					current.Close()
					return
				}
			}
			current.Close()
			if terminal.Type == "" {
				return
			}

			needsContinuation := terminal.StopReason == "max_tokens" || terminal.StopReason == "length"
			if !needsContinuation || hadToolCall || totalOutput >= maxTotalTokens || attempt >= maxContinuations {
				_ = emitEngineEvent(streamCtx, out, terminal)
				return
			}

			if !emitEngineEvent(streamCtx, out, core.FluxStreamEvent{
				Type: "continuation", Content: requestID, StopReason: strconv.Itoa(attempt + 1),
			}) {
				return
			}
			msgs = append(
				msgs,
				core.FluxMessage{Role: "assistant", Content: segment.String()},
				core.FluxMessage{Role: "user", Content: "Continue."},
			)
			next, err := provider.StreamChat(streamCtx, msgs, opts)
			if err != nil {
				_ = emitEngineEvent(streamCtx, out, core.FluxStreamEvent{Type: "error", Error: err.Error(), RequestID: requestID})
				return
			}
			current = next
			if next.RequestID != "" {
				requestID = next.RequestID
			}
		}
	}()
	return llm.NewStreamResult(out, first.RequestID, cancel), nil
}

func emitEngineEvent(ctx context.Context, out chan<- core.FluxStreamEvent, event core.FluxStreamEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
