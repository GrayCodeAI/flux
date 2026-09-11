package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// OpenGatewayClient uses the OpenAI-compatible OpenGateway endpoint.
type OpenGatewayClient struct {
	openAI *OpenAIClient
}

// NewOpenGatewayClient builds an OpenGateway provider client.
// openAIBase is typically "https://opengateway.gitlawb.com/v1".
func NewOpenGatewayClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *OpenGatewayClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("opengateway"))
	return &OpenGatewayClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *OpenGatewayClient) Name() string { return "opengateway" }

func (c *OpenGatewayClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *OpenGatewayClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *OpenGatewayClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*OpenGatewayClient)(nil)
