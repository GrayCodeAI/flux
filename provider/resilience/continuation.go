package resilience

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

// ContinuationConfig lives in provider/core.

// ChatWithContinuation calls Chat and automatically continues if stop_reason is "max_tokens".
// It appends the partial response as an assistant message and retries, accumulating content.
// Returns the fully assembled response.
func ChatWithContinuation(ctx context.Context, p core.Provider, messages []core.FluxMessage, opts core.ChatOptions, cfg core.ContinuationConfig) (*core.FluxResponse, error) {
	if cfg.MaxContinuations <= 0 {
		cfg.MaxContinuations = 3
	}

	var accumulated strings.Builder
	var finalUsage *core.FluxUsage
	var finalToolCalls []core.ToolCall
	msgs := make([]core.FluxMessage, len(messages))
	copy(msgs, messages)

	for i := 0; i <= cfg.MaxContinuations; i++ {
		resp, err := p.Chat(ctx, msgs, opts)
		if err != nil {
			return nil, fmt.Errorf("flux: continuation call %d failed: %w", i, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("flux: continuation call %d returned nil response", i)
		}

		accumulated.WriteString(resp.Content)
		finalToolCalls = append(finalToolCalls, resp.ToolCalls...)

		// Merge usage (nil-safe)
		if resp.Usage != nil {
			if finalUsage == nil {
				finalUsage = &core.FluxUsage{}
			}
			finalUsage.PromptTokens += resp.Usage.PromptTokens
			finalUsage.CompletionTokens += resp.Usage.CompletionTokens
			finalUsage.TotalTokens += resp.Usage.TotalTokens
		}

		// Check token cap
		if cfg.MaxTotalTokens > 0 && finalUsage != nil && finalUsage.CompletionTokens >= cfg.MaxTotalTokens {
			return &core.FluxResponse{
				Content: accumulated.String(), FinishReason: "max_tokens",
				ToolCalls: finalToolCalls, Usage: finalUsage,
			}, nil
		}

		// If response ended with tool calls, don't continue — tool results needed
		if len(resp.ToolCalls) > 0 {
			return &core.FluxResponse{
				Content: accumulated.String(), FinishReason: resp.FinishReason,
				ToolCalls: finalToolCalls, Usage: finalUsage, RequestID: resp.RequestID,
			}, nil
		}

		// Not max_tokens — we're done
		if resp.FinishReason != "max_tokens" {
			return &core.FluxResponse{
				Content: accumulated.String(), FinishReason: resp.FinishReason,
				ToolCalls: finalToolCalls, Usage: finalUsage, RequestID: resp.RequestID,
			}, nil
		}

		// Hit max_tokens — append partial as assistant and continue
		if i < cfg.MaxContinuations {
			msgs = append(msgs,
				core.FluxMessage{Role: "assistant", Content: accumulated.String()},
				core.FluxMessage{Role: "user", Content: "Continue."})
		}
	}

	return &core.FluxResponse{
		Content: accumulated.String(), FinishReason: "max_tokens",
		ToolCalls: finalToolCalls, Usage: finalUsage,
	}, nil
}

// StreamChatWithContinuation wraps StreamChat with automatic continuation when
// the response stops with "max_tokens" and contains only text (no tool calls).
// It returns a core.StreamResult whose Events channel transparently continues across
// multiple LLM calls, emitting a "continuation" event at each boundary.
//
// DEPRECATION NOTE: rho's Session loop has its own max_tokens recovery
// (internal/engine/stream.go around the `recoveryCount` loop) that doesn't
// add a synthetic "Continue." user message, and the flux conversation
// engine (flux/conversation.Engine) has its own OutputGroupID-based
// engine-level continuation. The two engine-level paths produce cleaner
// conversation shapes (no synthetic user turns) and are the recommended
// pattern for new code. This client-level helper remains for
// backwards-compatibility with the embedded flux HTTP server and
// non-rho consumers; new code should implement continuation at the
// engine or call-site level instead.
//
// Will be removed in flux v0.3.0. See flux/CHANGELOG.md for the
// deprecation timeline.
func StreamChatWithContinuation(ctx context.Context, p core.Provider, messages []core.FluxMessage, opts core.ChatOptions, cfg core.ContinuationConfig) (*core.StreamResult, error) {
	if cfg.MaxContinuations <= 0 {
		cfg.MaxContinuations = 3
	}
	if cfg.MaxTotalTokens <= 0 {
		cfg.MaxTotalTokens = 32000
	}

	groupID := fmt.Sprintf("cont_%d", time.Now().UnixNano())
	outCh := make(chan core.FluxStreamEvent, core.StreamChannelBuffer)
	cancelCtx, cancel := context.WithCancel(ctx)

	go func() {
		defer close(outCh)

		var accumulated strings.Builder
		var totalCompletionTokens int64
		var hadToolCalls bool
		msgs := make([]core.FluxMessage, len(messages))
		copy(msgs, messages)

		for attempt := 0; attempt <= cfg.MaxContinuations; attempt++ {
			stream, err := p.StreamChat(cancelCtx, msgs, opts)
			if err != nil {
				core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "error", Error: err.Error()})
				return
			}

			var stopReason string
			for evt := range stream.Events {
				switch evt.Type {
				case "content":
					accumulated.WriteString(evt.Content)
					core.Emit(cancelCtx, outCh, evt)
				case "tool_call":
					hadToolCalls = true
					core.Emit(cancelCtx, outCh, evt)
				case "usage":
					if evt.Usage != nil {
						totalCompletionTokens += int64(evt.Usage.CompletionTokens)
					}
					core.Emit(cancelCtx, outCh, evt)
				case "done":
					stopReason = evt.StopReason
				case "error":
					core.Emit(cancelCtx, outCh, evt)
					// Warning-marked error events are non-fatal health
					// diagnostics emitted just before the terminal done;
					// keep consuming so that done event is observed.
					if evt.Warning == "" {
						return
					}
				default:
					core.Emit(cancelCtx, outCh, evt)
				}
			}
			stream.Close()

			// Don't continue if: not max_tokens, had tool calls, or hit token cap
			if stopReason != "max_tokens" && stopReason != "length" {
				core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "done", StopReason: stopReason})
				return
			}
			if hadToolCalls {
				core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "done", StopReason: stopReason})
				return
			}
			if cfg.MaxTotalTokens > 0 && int(totalCompletionTokens) >= cfg.MaxTotalTokens {
				core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "done", StopReason: "max_tokens"})
				return
			}
			if attempt >= cfg.MaxContinuations {
				core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "done", StopReason: "max_tokens"})
				return
			}

			// Emit continuation boundary event
			core.Emit(cancelCtx, outCh, core.FluxStreamEvent{
				Type:       "continuation",
				Content:    groupID,
				StopReason: fmt.Sprintf("%d", attempt+1),
			})

			// Build continuation messages
			msgs = append(msgs,
				core.FluxMessage{Role: "assistant", Content: accumulated.String()},
				core.FluxMessage{Role: "user", Content: "Continue."})
		}

		core.Emit(cancelCtx, outCh, core.FluxStreamEvent{Type: "done", StopReason: "max_tokens"})
	}()

	return llm.NewStreamResult(outCh, groupID, cancel), nil
}
