package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"

	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
)

// OpenCodeGoClient routes OpenCode Go models through OpenAIClient and
// AnthropicClient per opencode.ai/docs/go, with cross-protocol fallback when
// the primary path returns no answer text (e.g. reasoning-only MiniMax streams).
type OpenCodeGoClient struct {
	router ProtocolRouter
}

// NewOpenCodeGoClient builds an OpenCode Go provider client.
func NewOpenCodeGoClient(apiKey, baseURL string, opts ...core.ClientOption) *OpenCodeGoClient {
	openBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if openBase == "" {
		openBase = opencodego.DefaultBaseURL
	}
	ocgOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("opencodego"))
	return &OpenCodeGoClient{router: ProtocolRouter{
		OpenAI:    NewOpenAIClient(apiKey, openBase, &OpenCodeGoCompat, ocgOpts...),
		Anthropic: NewAnthropicClient(apiKey, AnthropicBaseFromOpenAIV1(openBase), ocgOpts...),
	}}
}

func (c *OpenCodeGoClient) Name() string { return "opencodego" }

func (c *OpenCodeGoClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	opts.Model = opencodego.NativeModelID(opts.Model)
	if opencodego.UsesMessagesAPI(opts.Model) {
		return c.router.Chat(ctx, messages, opts, ChatProtocolMessages, openCodeGoMessagesFallback)
	}
	return c.router.OpenAI.Chat(ctx, messages, opts)
}

func (c *OpenCodeGoClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	opts.Model = opencodego.NativeModelID(opts.Model)
	if opencodego.UsesMessagesAPI(opts.Model) {
		return c.router.StreamChat(ctx, messages, opts, ProtocolStreamConfig{
			Primary:               ChatProtocolMessages,
			ReasoningOnlyFallback: true,
		})
	}
	return c.router.OpenAI.StreamChat(ctx, messages, opts)
}

func (c *OpenCodeGoClient) Ping(ctx context.Context) error {
	if err := c.router.OpenAI.Ping(ctx); err == nil {
		return nil
	}
	return c.router.Anthropic.Ping(ctx)
}

func openCodeGoMessagesFallback(primaryErr error, primaryResp *core.EyrieResponse) bool {
	if primaryErr != nil {
		return oaCompatUnsupportedError(primaryErr)
	}
	return !core.ResponseHasContent(primaryResp)
}

func oaCompatUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status=401") ||
		strings.Contains(msg, "http 401") ||
		strings.Contains(msg, "oa-compat") ||
		strings.Contains(msg, "not supported")
}

// OACompatUnsupportedError reports whether an error indicates OA-compat is unsupported.
func OACompatUnsupportedError(err error) bool { return oaCompatUnsupportedError(err) }

var _ core.Provider = (*OpenCodeGoClient)(nil)
