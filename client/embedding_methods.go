package client

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/eyrie/client/adapters"
)

// Compile-time check that *adapters.OpenAIClient implements embeddings.Embedder.
var _ Embedder = (*adapters.OpenAIClient)(nil)

// CreateEmbedding sends an embedding request to the specified (or default) provider.
func (c *EyrieClient) CreateEmbedding(ctx context.Context, req EmbeddingRequest, provider string) (*EmbeddingResponse, error) {
	if provider == "" {
		provider = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return nil, err
	}
	embedder, ok := p.(Embedder)
	if !ok {
		return nil, fmt.Errorf("eyrie: provider %s does not support embeddings", provider)
	}
	return embedder.CreateEmbedding(ctx, req)
}
