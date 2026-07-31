package adapters

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/concentrate"
	"github.com/GrayCodeAI/eyrie/client/core"
)

// ConcentrateClient routes Concentrate AI models through OpenAIClient and
// AnthropicClient, selecting protocol per model based on the owned_by field.
// Concentrate exposes both /v1/chat/completions and /v1/messages endpoints.
type ConcentrateClient struct {
	router ProtocolRouter
	logger *slog.Logger
}

// NewConcentrateClient builds a Concentrate AI provider client.
// baseURL is typically "https://api.concentrate.ai/v1".
func NewConcentrateClient(apiKey, baseURL string, compat *OpenAICompatConfig, opts ...core.ClientOption) *ConcentrateClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	cOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("concentrate"))
	return &ConcentrateClient{
		router: ProtocolRouter{
			OpenAI:    NewOpenAIClient(apiKey, baseURL, compat, cOpts...),
			Anthropic: NewAnthropicClient(apiKey, AnthropicBaseFromOpenAIV1(baseURL), cOpts...),
		},
		logger: slog.Default(),
	}
}

func (c *ConcentrateClient) Name() string { return "concentrate" }

func (c *ConcentrateClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	if concentrate.UsesMessagesAPI(opts.Model) {
		return c.router.Chat(ctx, messages, opts, ChatProtocolMessages, nil)
	}
	return c.router.OpenAI.Chat(ctx, messages, opts)
}

func (c *ConcentrateClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	if concentrate.UsesMessagesAPI(opts.Model) {
		return c.router.StreamChat(ctx, messages, opts, ProtocolStreamConfig{
			Primary: ChatProtocolMessages,
		})
	}
	return c.router.OpenAI.StreamChat(ctx, messages, opts)
}

func (c *ConcentrateClient) Ping(ctx context.Context) error {
	if err := c.router.OpenAI.Ping(ctx); err == nil {
		return nil
	}
	return c.router.Anthropic.Ping(ctx)
}

var _ core.Provider = (*ConcentrateClient)(nil)
