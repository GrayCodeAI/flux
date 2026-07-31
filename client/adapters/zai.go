package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// ZAIClient uses the official OpenAI-compatible Z.AI surface only
// (paas/v4 or coding/paas/v4).
type ZAIClient struct {
	openai     *OpenAIClient
	providerID string
}

// NewZAIClient builds an OpenAI-compatible Z.AI client for a plan/region gateway.
// openAIBase should be the resolved general or coding paas base.
func NewZAIClient(apiKey, openAIBase string, compat *OpenAICompatConfig, providerID string, opts ...core.ClientOption) *ZAIClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	zaiOpts := append([]core.ClientOption{core.WithProviderName(providerID)}, opts...)
	return &ZAIClient{
		openai:     NewOpenAIClient(apiKey, openAIBase, compat, zaiOpts...),
		providerID: providerID,
	}
}

func (c *ZAIClient) Name() string {
	if c.openai != nil {
		return c.openai.Name()
	}
	return c.providerID
}

func (c *ZAIClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openai.Chat(ctx, messages, opts)
}

func (c *ZAIClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openai.StreamChat(ctx, messages, opts)
}

func (c *ZAIClient) Ping(ctx context.Context) error {
	return c.openai.Ping(ctx)
}

var _ core.Provider = (*ZAIClient)(nil)
