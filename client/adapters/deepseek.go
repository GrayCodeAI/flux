package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// DeepSeekClient uses the official OpenAI-compatible DeepSeek surface only
// (https://api.deepseek.com).
type DeepSeekClient struct {
	openai *OpenAIClient
}

// NewDeepSeekClient builds an OpenAI-compatible DeepSeek client.
// openAIBase is typically "https://api.deepseek.com/v1".
func NewDeepSeekClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *DeepSeekClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	dsOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("deepseek"))
	return &DeepSeekClient{openai: NewOpenAIClient(apiKey, openAIBase, compat, dsOpts...)}
}

func (c *DeepSeekClient) Name() string { return "deepseek" }

func (c *DeepSeekClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openai.Chat(ctx, messages, opts)
}

func (c *DeepSeekClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openai.StreamChat(ctx, messages, opts)
}

func (c *DeepSeekClient) Ping(ctx context.Context) error {
	return c.openai.Ping(ctx)
}

var _ core.Provider = (*DeepSeekClient)(nil)
