package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// DeepSeekClient uses the OpenAI-compatible DeepSeek endpoint.
type DeepSeekClient struct {
	openAI *OpenAIClient
}

// NewDeepSeekClient builds a DeepSeek provider client.
// openAIBase is typically "https://api.deepseek.com"
func NewDeepSeekClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *DeepSeekClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	dsOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("deepseek"))
	return &DeepSeekClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, dsOpts...),
	}
}

func (c *DeepSeekClient) Name() string { return "deepseek" }

func (c *DeepSeekClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *DeepSeekClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *DeepSeekClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*DeepSeekClient)(nil)
