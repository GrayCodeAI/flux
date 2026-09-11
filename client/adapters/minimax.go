package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// MiniMaxClient uses the OpenAI-compatible MiniMax endpoint.
type MiniMaxClient struct {
	openAI *OpenAIClient
}

// NewMiniMaxClient builds a MiniMax provider client.
// openAIBase is typically "https://api.minimax.io/v1".
func NewMiniMaxClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *MiniMaxClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("minimax"))
	return &MiniMaxClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *MiniMaxClient) Name() string { return "minimax" }

func (c *MiniMaxClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *MiniMaxClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *MiniMaxClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*MiniMaxClient)(nil)
