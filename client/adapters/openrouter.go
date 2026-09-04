package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// OpenRouterClient uses the OpenAI-compatible OpenRouter endpoint.
type OpenRouterClient struct {
	openAI *OpenAIClient
}

// NewOpenRouterClient builds an OpenRouter provider client.
// openAIBase is typically "https://openrouter.ai/api/v1".
func NewOpenRouterClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *OpenRouterClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("openrouter"))
	return &OpenRouterClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *OpenRouterClient) Name() string { return "openrouter" }

func (c *OpenRouterClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *OpenRouterClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *OpenRouterClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*OpenRouterClient)(nil)
