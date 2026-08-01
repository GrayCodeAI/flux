package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// CanopyWaveClient uses the OpenAI-compatible CanopyWave endpoint.
type CanopyWaveClient struct {
	openAI *OpenAIClient
}

// NewCanopyWaveClient builds a CanopyWave provider client.
// openAIBase is typically "https://inference.canopywave.io/v1".
func NewCanopyWaveClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *CanopyWaveClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("canopywave"))
	return &CanopyWaveClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *CanopyWaveClient) Name() string { return "canopywave" }

func (c *CanopyWaveClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *CanopyWaveClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *CanopyWaveClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*CanopyWaveClient)(nil)
