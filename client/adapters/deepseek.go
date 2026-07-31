package adapters

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// DeepSeekClient uses OpenAI-compatible DeepSeek endpoints first,
// with optional Anthropic-compat fallback if the OpenAI endpoint is down.
type DeepSeekClient struct {
	router ProtocolRouter
	logger *slog.Logger
}

// NewDeepSeekClient builds a DeepSeek provider client.
// openAIBase is typically "https://api.deepseek.com/v1"
// anthropicBase is typically "https://api.deepseek.com/anthropic"
func NewDeepSeekClient(apiKey, openAIBase, anthropicBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *DeepSeekClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	anthropicBase = strings.TrimRight(strings.TrimSpace(anthropicBase), "/")
	dsOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("deepseek"))
	o := NewOpenAIClient(apiKey, openAIBase, compat, dsOpts...)
	var a *AnthropicClient
	if anthropicBase != "" {
		a = NewAnthropicClient(apiKey, anthropicBase, dsOpts...)
	}
	return &DeepSeekClient{
		router: ProtocolRouter{OpenAI: o, Anthropic: a},
		logger: slog.Default(),
	}
}

func (c *DeepSeekClient) Name() string { return "deepseek" }

func (c *DeepSeekClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.router.Chat(ctx, messages, opts, ChatProtocolCompletions, func(err error, _ *core.EyrieResponse) bool {
		if err != nil && c.router.Anthropic != nil && core.IsRetriableError(err) {
			c.logger.Info("DeepSeek: OpenAI endpoint failed; retrying via Anthropic compatibility", "error", err)
			return true
		}
		return false
	})
}

func (c *DeepSeekClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.router.StreamChat(ctx, messages, opts, ProtocolStreamConfig{
		Primary: ChatProtocolCompletions,
		FallbackOnError: func(err error) bool {
			if c.router.Anthropic != nil && core.IsRetriableError(err) {
				c.logger.Info("DeepSeek: OpenAI stream failed; retrying via Anthropic compatibility", "error", err)
				return true
			}
			return false
		},
	})
}

func (c *DeepSeekClient) Ping(ctx context.Context) error {
	if err := c.router.OpenAI.Ping(ctx); err == nil {
		return nil
	} else if c.router.Anthropic == nil || !core.IsRetriableError(err) {
		return err
	}
	return c.router.Anthropic.Ping(ctx)
}

var _ core.Provider = (*DeepSeekClient)(nil)
