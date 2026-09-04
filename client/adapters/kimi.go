package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// KimiClient uses the OpenAI-compatible Kimi (Moonshot) endpoint.
type KimiClient struct {
	openAI *OpenAIClient
}

// NewKimiClient builds a Kimi (Moonshot) provider client.
// openAIBase is typically "https://api.moonshot.ai/v1".
func NewKimiClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *KimiClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("kimi"))
	return &KimiClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *KimiClient) Name() string { return "kimi" }

func (c *KimiClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *KimiClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *KimiClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*KimiClient)(nil)
