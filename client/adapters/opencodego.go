package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
	"github.com/GrayCodeAI/eyrie/client/core"
)

// OpenCodeGoClient routes each model to exactly one protocol:
//   - OpenAI /v1/chat/completions when the model has an OpenAI-compatible endpoint
//   - Anthropic /v1/messages when the model is Anthropic-only on OpenCode Go
//
// Never falls back across protocols for the same request (official Go docs list
// one endpoint per model).
type OpenCodeGoClient struct {
	openai    *OpenAIClient
	anthropic *AnthropicClient
}

// NewOpenCodeGoClient builds an OpenCode Go provider client.
func NewOpenCodeGoClient(apiKey, baseURL string, opts ...core.ClientOption) *OpenCodeGoClient {
	openBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if openBase == "" {
		openBase = opencodego.DefaultBaseURL
	}
	ocgOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("opencodego"))
	return &OpenCodeGoClient{
		openai:    NewOpenAIClient(apiKey, openBase, &OpenCodeGoCompat, ocgOpts...),
		anthropic: NewAnthropicClient(apiKey, AnthropicBaseFromOpenAIV1(openBase), ocgOpts...),
	}
}

func (c *OpenCodeGoClient) Name() string { return "opencodego" }

func (c *OpenCodeGoClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	opts.Model = opencodego.NativeModelID(opts.Model)
	if opencodego.UsesMessagesAPI(opts.Model) {
		return c.anthropic.Chat(ctx, messages, opts)
	}
	return c.openai.Chat(ctx, messages, opts)
}

func (c *OpenCodeGoClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	opts.Model = opencodego.NativeModelID(opts.Model)
	if opencodego.UsesMessagesAPI(opts.Model) {
		return c.anthropic.StreamChat(ctx, messages, opts)
	}
	return c.openai.StreamChat(ctx, messages, opts)
}

func (c *OpenCodeGoClient) Ping(ctx context.Context) error {
	// Health-check the OpenAI gateway surface; Anthropic-only models share the same host.
	return c.openai.Ping(ctx)
}

var _ core.Provider = (*OpenCodeGoClient)(nil)
