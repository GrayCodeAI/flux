package adapters

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"

	"github.com/GrayCodeAI/graycode-router/types"
)

// LongCatClient uses the OpenAI-compatible LongCat endpoint first, with
// Anthropic-compatible fallback on retriable errors. Both protocols are
// documented at https://longcat.chat/platform/docs/api/chat and
// https://longcat.chat/platform/docs/api/messages.
type LongCatClient struct {
	router ProtocolRouter
	logger *slog.Logger
}

// NewLongCatClient builds a LongCat dual-protocol client.
// openAIBase should be "https://api.longcat.chat/openai/v1".
// anthropicBase should be "https://api.longcat.chat/anthropic".
// The same apiKey is used for both sides.
func NewLongCatClient(apiKey, openAIBase, anthropicBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *LongCatClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	anthropicBase = strings.TrimRight(strings.TrimSpace(anthropicBase), "/")

	lcOpts := append([]core.ClientOption{core.WithProviderName("longcat")}, opts...)
	o := NewOpenAIClient(apiKey, openAIBase, compat, lcOpts...)

	var a *AnthropicClient
	if anthropicBase != "" {
		a = NewAnthropicClient(apiKey, anthropicBase, lcOpts...)
	}

	return &LongCatClient{
		router: ProtocolRouter{OpenAI: o, Anthropic: a},
		logger: slog.Default(),
	}
}

func (c *LongCatClient) Name() string {
	if c.router.OpenAI != nil {
		return c.router.OpenAI.Name()
	}
	return "longcat"
}

func (c *LongCatClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.router.Chat(ctx, messages, opts, ChatProtocolCompletions, func(err error, _ *core.GraycodeRouterResponse) bool {
		if err != nil && c.router.Anthropic != nil && longcatFallbackChatError(err) {
			c.logger.Info("LongCat: OpenAI endpoint failed; retrying via Anthropic compatibility",
				"error", err)
			return true
		}
		return false
	})
}

func (c *LongCatClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.router.StreamChat(ctx, messages, opts, ProtocolStreamConfig{
		Primary: ChatProtocolCompletions,
		FallbackOnError: func(err error) bool {
			if c.router.Anthropic != nil && longcatFallbackChatError(err) {
				c.logger.Info("LongCat: OpenAI stream failed; retrying via Anthropic compatibility",
					"error", err)
				return true
			}
			return false
		},
	})
}

func (c *LongCatClient) Ping(ctx context.Context) error {
	if err := c.router.OpenAI.Ping(ctx); err == nil {
		return nil
	} else if c.router.Anthropic == nil || !longcatRetryableChatError(err) {
		return err
	}
	return c.router.Anthropic.Ping(ctx)
}

func longcatFallbackChatError(err error) bool {
	if longcatRetryableChatError(err) {
		return true
	}
	if err == nil {
		return false
	}
	return oaCompatUnsupportedError(err)
}

func longcatRetryableChatError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if n := parseHTTPStatusFromError(msg); n > 0 {
		return n >= 500 || n == http.StatusUnauthorized || n == http.StatusForbidden
	}
	var graycodeRouterErr *core.GraycodeRouterError
	if errors.As(err, &graycodeRouterErr) {
		return graycodeRouterErr.IsRetriable()
	}
	return types.IsTransient(err)
}

var _ core.Provider = (*LongCatClient)(nil)
