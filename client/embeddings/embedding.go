package embeddings

import "github.com/GrayCodeAI/graycode-router/client/core"

// The embedding DTOs and the Embedder interface live in client/core because
// the protocol adapters implement Embedder. Aliased here so this package's
// API is unchanged.
type (
	// Embedder is the interface for creating embeddings.
	Embedder = core.Embedder
	// EmbeddingParams holds asymmetric params for indexing vs query.
	EmbeddingParams = core.EmbeddingParams
	// EmbeddingRequest represents an embedding API call.
	EmbeddingRequest = core.EmbeddingRequest
	// EmbeddingResponse holds embedding results.
	EmbeddingResponse = core.EmbeddingResponse
)
