package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// AgnesClient uses the OpenAI-compatible Agnes AI endpoint.
// Official docs expose an OpenAI-compatible API only.
type AgnesClient struct {
	openAI *OpenAIClient
}

// NewAgnesClient builds an Agnes AI provider client.
// openAIBase is typically "https://apihub.agnes-ai.com/v1".
func NewAgnesClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *AgnesClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	agnOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("agnes"))
	return &AgnesClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, agnOpts...),
	}
}

func (c *AgnesClient) Name() string { return "agnes" }

func (c *AgnesClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *AgnesClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *AgnesClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*AgnesClient)(nil)
