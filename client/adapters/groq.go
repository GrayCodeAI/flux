package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// GroqClient uses the OpenAI-compatible Groq endpoint.
type GroqClient struct {
	openAI *OpenAIClient
}

// NewGroqClient builds a Groq provider client.
// openAIBase is typically "https://api.groq.com/openai/v1".
func NewGroqClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *GroqClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("groq"))
	return &GroqClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *GroqClient) Name() string { return "groq" }

func (c *GroqClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *GroqClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *GroqClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*GroqClient)(nil)
