package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// OllamaClient uses the OpenAI-compatible local Ollama endpoint.
type OllamaClient struct {
	openAI *OpenAIClient
}

// NewOllamaClient builds an Ollama provider client.
// openAIBase is typically "http://localhost:11434/v1".
func NewOllamaClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *OllamaClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	opts = append(append([]core.ClientOption{}, opts...), core.WithProviderName("ollama"))
	return &OllamaClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, opts...),
	}
}

func (c *OllamaClient) Name() string { return "ollama" }

func (c *OllamaClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *OllamaClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *OllamaClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*OllamaClient)(nil)
