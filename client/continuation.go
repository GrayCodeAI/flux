package client

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ContinuationConfig and DefaultContinuationConfig live in client/core;
// the client.* names remain available via aliases.go.

// ChatWithContinuation calls Chat and automatically continues if stop_reason is "max_tokens".
// It appends the partial response as an assistant message and retries, accumulating content.
// Returns the fully assembled response.
func ChatWithContinuation(ctx context.Context, p Provider, messages []GraycodeRouterMessage, opts ChatOptions, cfg ContinuationConfig) (*GraycodeRouterResponse, error) {
	if cfg.MaxContinuations <= 0 {
		cfg.MaxContinuations = 3
	}

	var accumulated strings.Builder
	var finalUsage *GraycodeRouterUsage
	var finalToolCalls []ToolCall
	msgs := make([]GraycodeRouterMessage, len(messages))
	copy(msgs, messages)

	for i := 0; i <= cfg.MaxContinuations; i++ {
		resp, err := p.Chat(ctx, msgs, opts)
		if err != nil {
			return nil, fmt.Errorf("graycode-router: continuation call %d failed: %w", i, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("graycode-router: continuation call %d returned nil response", i)
		}

		accumulated.WriteString(resp.Content)
		finalToolCalls = append(finalToolCalls, resp.ToolCalls...)

		// Merge usage (nil-safe)
		if resp.Usage != nil {
			if finalUsage == nil {
				finalUsage = &GraycodeRouterUsage{}
			}
			finalUsage.PromptTokens += resp.Usage.PromptTokens
			finalUsage.CompletionTokens += resp.Usage.CompletionTokens
			finalUsage.TotalTokens += resp.Usage.TotalTokens
		}

		// Check token cap
		if cfg.MaxTotalTokens > 0 && finalUsage != nil && finalUsage.CompletionTokens >= cfg.MaxTotalTokens {
			return &GraycodeRouterResponse{
				Content: accumulated.String(), FinishReason: "max_tokens",
				ToolCalls: finalToolCalls, Usage: finalUsage,
			}, nil
		}

		// If response ended with tool calls, don't continue — tool results needed
		if len(resp.ToolCalls) > 0 {
			return &GraycodeRouterResponse{
				Content: accumulated.String(), FinishReason: resp.FinishReason,
				ToolCalls: finalToolCalls, Usage: finalUsage, RequestID: resp.RequestID,
			}, nil
		}

		// Not max_tokens — we're done
		if resp.FinishReason != "max_tokens" {
			return &GraycodeRouterResponse{
				Content: accumulated.String(), FinishReason: resp.FinishReason,
				ToolCalls: finalToolCalls, Usage: finalUsage, RequestID: resp.RequestID,
			}, nil
		}

		// Hit max_tokens — append partial as assistant and continue
		if i < cfg.MaxContinuations {
			msgs = append(msgs,
				GraycodeRouterMessage{Role: "assistant", Content: accumulated.String()},
				GraycodeRouterMessage{Role: "user", Content: "Continue."})
		}
	}

	return &GraycodeRouterResponse{
		Content: accumulated.String(), FinishReason: "max_tokens",
		ToolCalls: finalToolCalls, Usage: finalUsage,
	}, nil
}

// StreamChatWithContinuation wraps StreamChat with automatic continuation when
// the response stops with "max_tokens" and contains only text (no tool calls).
// It returns a StreamResult whose Events channel transparently continues across
// multiple LLM calls, emitting a "continuation" event at each boundary.
//
// DEPRECATION NOTE: hawk's Session loop has its own max_tokens recovery
// (internal/engine/stream.go around the `recoveryCount` loop) that doesn't
// add a synthetic "Continue." user message, and the graycode-router conversation
// engine (graycode-router/conversation.Engine) has its own OutputGroupID-based
// engine-level continuation. The two engine-level paths produce cleaner
// conversation shapes (no synthetic user turns) and are the recommended
// pattern for new code. This client-level helper remains for
// backwards-compatibility with the embedded graycode-router HTTP server and
// non-hawk consumers; new code should implement continuation at the
// engine or call-site level instead.
//
// Will be removed in graycode-router v0.3.0. See graycode-router/CHANGELOG.md for the
// deprecation timeline.
func StreamChatWithContinuation(ctx context.Context, p Provider, messages []GraycodeRouterMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error) {
	if cfg.MaxContinuations <= 0 {
		cfg.MaxContinuations = 3
	}
	if cfg.MaxTotalTokens <= 0 {
		cfg.MaxTotalTokens = 32000
	}

	groupID := fmt.Sprintf("cont_%d", time.Now().UnixNano())
	outCh := make(chan GraycodeRouterStreamEvent, streamChannelBuffer)
	cancelCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(outCh)

		var accumulated strings.Builder
		var totalCompletionTokens int64
		var hadToolCalls bool
		msgs := make([]GraycodeRouterMessage, len(messages))
		copy(msgs, messages)

		for attempt := 0; attempt <= cfg.MaxContinuations; attempt++ {
			stream, err := p.StreamChat(cancelCtx, msgs, opts)
			if err != nil {
				emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "error", Error: err.Error()})
				return
			}

			var stopReason string
			for evt := range stream.Events {
				switch evt.Type {
				case "content":
					accumulated.WriteString(evt.Content)
					emit(cancelCtx, outCh, evt)
				case "tool_call":
					hadToolCalls = true
					emit(cancelCtx, outCh, evt)
				case "usage":
					if evt.Usage != nil {
						totalCompletionTokens += int64(evt.Usage.CompletionTokens)
					}
					emit(cancelCtx, outCh, evt)
				case "done":
					stopReason = evt.StopReason
				case "error":
					emit(cancelCtx, outCh, evt)
					// Warning-marked error events are non-fatal health
					// diagnostics emitted just before the terminal done;
					// keep consuming so that done event is observed.
					if evt.Warning == "" {
						return
					}
				default:
					emit(cancelCtx, outCh, evt)
				}
			}
			stream.Close()

			// Don't continue if: not max_tokens, had tool calls, or hit token cap
			if stopReason != "max_tokens" && stopReason != "length" {
				emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "done", StopReason: stopReason})
				return
			}
			if hadToolCalls {
				emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "done", StopReason: stopReason})
				return
			}
			if cfg.MaxTotalTokens > 0 && int(totalCompletionTokens) >= cfg.MaxTotalTokens {
				emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "done", StopReason: "max_tokens"})
				return
			}
			if attempt >= cfg.MaxContinuations {
				emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "done", StopReason: "max_tokens"})
				return
			}

			// Emit continuation boundary event
			emit(cancelCtx, outCh, GraycodeRouterStreamEvent{
				Type:       "continuation",
				Content:    groupID,
				StopReason: fmt.Sprintf("%d", attempt+1),
			})

			// Build continuation messages
			msgs = append(msgs,
				GraycodeRouterMessage{Role: "assistant", Content: accumulated.String()},
				GraycodeRouterMessage{Role: "user", Content: "Continue."})
		}

		emit(cancelCtx, outCh, GraycodeRouterStreamEvent{Type: "done", StopReason: "max_tokens"})
	}()

	return NewStreamResult(outCh, cancel), nil
}
