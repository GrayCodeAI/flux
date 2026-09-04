package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// GrokClient uses the OpenAI-compatible xAI (Grok) endpoint.
type GrokClient struct {
	openAI *OpenAIClient
}

// NewGrokClient builds an xAI (Grok) provider client.
// openAIBase is typically "https://api.x.ai/v1".
func NewGrokClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *GrokClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("grok"))
	return &GrokClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *GrokClient) Name() string { return "grok" }

func (c *GrokClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *GrokClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *GrokClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*GrokClient)(nil)
