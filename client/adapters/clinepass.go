package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// ClinePassClient uses the OpenAI-compatible ClinePass endpoint.
type ClinePassClient struct {
	openAI *OpenAIClient
}

// NewClinePassClient builds a ClinePass provider client.
// openAIBase is typically "https://api.cline.bot/api/v1".
func NewClinePassClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *ClinePassClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("clinepass"))
	return &ClinePassClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *ClinePassClient) Name() string { return "clinepass" }

func (c *ClinePassClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *ClinePassClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *ClinePassClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*ClinePassClient)(nil)
