package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// PoolsideClient uses Poolside's OpenAI-compatible transport and retries a
// reasoning-only stream once through the non-streaming endpoint. Laguna models
// can spend an entire streamed turn in reasoning while still returning answer
// content from a complete non-streaming request.
type PoolsideClient struct {
	openAI *OpenAIClient
}

func NewPoolsideClient(apiKey, baseURL string, opts ...core.ClientOption) *PoolsideClient {
	poolsideOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("poolside"))
	return &PoolsideClient{openAI: NewOpenAIClient(apiKey, strings.TrimRight(baseURL, "/"), &PoolsideCompat, poolsideOpts...)}
}

func (c *PoolsideClient) Name() string { return "poolside" }

func (c *PoolsideClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *PoolsideClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	primary, err := c.openAI.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if core.ResponseHasContent(primary) || len(primary.ToolCalls) > 0 {
		return streamResultFromChat(primary), nil
	}
	recovered, err := c.reasoningOnlyFallbackChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	return streamResultFromChat(recovered), nil
}

func (c *PoolsideClient) reasoningOnlyFallbackChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	// A Laguna stream can exhaust itself in reasoning when Hawk's large tool
	// catalog is attached. Preserve tools on the primary request, but make the
	// one-shot recovery text-only so the model emits its final answer.
	opts.Tools = nil
	opts.ToolChoice = nil
	if opts.MaxTokens < 512 {
		opts.MaxTokens = 512
	}
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *PoolsideClient) Ping(ctx context.Context) error { return c.openAI.Ping(ctx) }

var _ core.Provider = (*PoolsideClient)(nil)
