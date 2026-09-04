package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// GeminiOpenAIClient uses the OpenAI-compatible Gemini endpoint
// (generativelanguage.googleapis.com openai compatibility layer).
// Distinct from the native GeminiClient which uses generateContent.
type GeminiOpenAIClient struct {
	openAI *OpenAIClient
}

// NewGeminiOpenAIClient builds a Gemini provider client over the
// OpenAI-compatible endpoint.
// openAIBase is typically "https://generativelanguage.googleapis.com/v1beta/openai".
func NewGeminiOpenAIClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *GeminiOpenAIClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("gemini"))
	return &GeminiOpenAIClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *GeminiOpenAIClient) Name() string { return "gemini" }

func (c *GeminiOpenAIClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *GeminiOpenAIClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *GeminiOpenAIClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*GeminiOpenAIClient)(nil)
