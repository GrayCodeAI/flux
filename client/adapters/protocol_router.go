package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// ChatProtocol selects which existing eyrie client handles a gateway request.
type ChatProtocol int

const (
	// ChatProtocolCompletions routes via OpenAIClient (POST /v1/chat/completions).
	ChatProtocolCompletions ChatProtocol = iota
	// ChatProtocolMessages routes via AnthropicClient (POST /v1/messages).
	ChatProtocolMessages
)

type streamOpener func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.StreamResult, error)

type chatOpener func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.EyrieResponse, error)

// protocolStreamFallback retries a reasoning-only /v1/messages stream via the
// alternate protocol. Non-streaming Chat is tried first because OpenCode Go
// MiniMax often returns answer text on chat/completions while the stream only
// exposes reasoning_content.
type protocolStreamFallback struct {
	stream streamOpener
	chat   chatOpener
}

// ProtocolChatFallback decides whether to retry via the alternate protocol
// after the primary attempt. resp is non-nil only when primary returned err=nil.
type ProtocolChatFallback func(primaryErr error, primaryResp *core.EyrieResponse) bool

// ProtocolRouter picks between OpenAIClient and AnthropicClient for gateways
// that expose both APIs (OpenCode Go, MiMo). All HTTP work stays in openai.go
// and anthropic.go; this type only orchestrates routing and fallback.
type ProtocolRouter struct {
	OpenAI    *OpenAIClient
	Anthropic *AnthropicClient
}

// ProtocolStreamConfig controls streaming across two protocols.
type ProtocolStreamConfig struct {
	Primary               ChatProtocol
	FallbackOnError       func(error) bool
	ReasoningOnlyFallback bool // retry alternate protocol when primary stream is reasoning-only
}

// Chat sends a request via the primary protocol, optionally falling back to the
// alternate protocol when fallback returns true.
func (r ProtocolRouter) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions, primary ChatProtocol, fallback ProtocolChatFallback) (*core.EyrieResponse, error) {
	primaryClient, fallbackClient := r.providers(primary)
	resp, err := primaryClient.Chat(ctx, messages, opts)
	if fallback == nil || !fallback(err, resp) {
		return resp, err
	}
	fallbackResp, fallbackErr := fallbackClient.Chat(ctx, messages, opts)
	if fallbackErr == nil && core.ResponseHasContent(fallbackResp) {
		return fallbackResp, nil
	}
	return resp, err
}

// StreamChat streams via the primary protocol. When FallbackOnError matches,
// the alternate protocol is tried immediately. When ReasoningOnlyFallback is set
// and the primary is /v1/messages, an empty reasoning-only stream retries via
// chat/completions.
func (r ProtocolRouter) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions, cfg ProtocolStreamConfig) (*core.StreamResult, error) {
	primaryClient, fallbackClient := r.providers(cfg.Primary)
	result, err := primaryClient.StreamChat(ctx, messages, opts)
	if err != nil {
		if cfg.FallbackOnError != nil && cfg.FallbackOnError(err) {
			return fallbackClient.StreamChat(ctx, messages, opts)
		}
		return result, err
	}
	if cfg.ReasoningOnlyFallback && cfg.Primary == ChatProtocolMessages {
		fallback := fallbackClient
		return newStreamWithReasoningFallback(ctx, messages, opts, result, protocolStreamFallback{
			chat: func(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
				return fallback.Chat(ctx, messages, opts)
			},
			stream: func(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
				return fallback.StreamChat(ctx, messages, opts)
			},
		}), nil
	}
	return result, nil
}

func (r ProtocolRouter) providers(primary ChatProtocol) (core.Provider, core.Provider) {
	if primary == ChatProtocolMessages {
		return r.Anthropic, r.OpenAI
	}
	return r.OpenAI, r.Anthropic
}

// AnthropicBaseFromOpenAIV1 strips a trailing /v1 from an OpenAI-compatible base URL.
func AnthropicBaseFromOpenAIV1(openAIBase string) string {
	base := strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	if strings.HasSuffix(base, "/v1") {
		return strings.TrimSuffix(base, "/v1")
	}
	return base
}

func streamResultFromChat(resp *core.EyrieResponse) *core.StreamResult {
	out := make(chan core.EyrieStreamEvent, core.StreamChannelBuffer)
	go func() {
		defer close(out)
		if resp == nil {
			return
		}
		if strings.TrimSpace(resp.Thinking) != "" {
			out <- core.EyrieStreamEvent{Type: "thinking", Thinking: resp.Thinking}
		}
		if strings.TrimSpace(resp.Content) != "" {
			out <- core.EyrieStreamEvent{Type: "content", Content: resp.Content}
		}
		for i := range resp.ToolCalls {
			tc := resp.ToolCalls[i]
			out <- core.EyrieStreamEvent{Type: "tool_call", ToolCall: &tc}
		}
		if resp.Usage != nil {
			out <- core.EyrieStreamEvent{Type: "usage", Usage: resp.Usage}
		}
		stop := resp.FinishReason
		if stop == "" {
			stop = "stop"
		}
		out <- core.EyrieStreamEvent{Type: "done", StopReason: stop}
	}()
	return llm.NewStreamResult(out, "", func() {})
}

func (f protocolStreamFallback) open(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	if f.chat != nil {
		resp, err := f.chat(ctx, messages, opts)
		if err == nil && core.ResponseHasContent(resp) {
			return streamResultFromChat(resp), nil
		}
	}
	if f.stream != nil {
		return f.stream(ctx, messages, opts)
	}
	return nil, fmt.Errorf("eyrie: protocol stream fallback is not configured")
}

func newStreamWithReasoningFallback(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions, primary *core.StreamResult, fallback protocolStreamFallback) *core.StreamResult {
	out := make(chan core.EyrieStreamEvent, core.StreamChannelBuffer)
	cancelCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(out)
		defer primary.Close()
		defer cancel()

		var (
			sawReasoning bool
			content      strings.Builder
			toolCalls    int
			buffered     []core.EyrieStreamEvent
			streamErr    bool
		)
		flush := func() {
			for _, ev := range buffered {
				select {
				case out <- ev:
				case <-cancelCtx.Done():
					return
				}
			}
			buffered = nil
		}
		for ev := range primary.Events {
			switch ev.Type {
			case "thinking":
				sawReasoning = true
			case "content":
				content.WriteString(ev.Content)
			case "tool_call":
				toolCalls++
			case "error":
				if !isReasoningOnlyStreamDiagnostic(ev.Error) {
					streamErr = true
				}
			}
			if strings.TrimSpace(content.String()) != "" || toolCalls > 0 {
				flush()
				select {
				case out <- ev:
				case <-cancelCtx.Done():
					return
				}
				continue
			}
			buffered = append(buffered, ev)
		}

		health := core.DetectResponseHealth(core.ResponseSignals{
			SawReasoning: sawReasoning,
			ContentLen:   len(strings.TrimSpace(content.String())),
			ToolCalls:    toolCalls,
			StreamEnded:  true,
			StreamErr:    streamErr,
		})
		if health != core.ResponseErrorOnlyReasoning {
			flush()
			return
		}

		fallbackResult, err := fallback.open(cancelCtx, messages, opts)
		if err != nil {
			flush()
			select {
			case out <- core.EyrieStreamEvent{Type: "error", Error: err.Error()}:
			case <-cancelCtx.Done():
			}
			return
		}
		defer fallbackResult.Close()
		for ev := range fallbackResult.Events {
			select {
			case out <- ev:
			case <-cancelCtx.Done():
				return
			}
		}
	}()
	return llm.NewStreamResult(out, "", cancel)
}

func isReasoningOnlyStreamDiagnostic(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "error_only_reasoning") ||
		strings.Contains(message, "reasoning tokens but no answer")
}
