package core

import "context"

// Embedder is the interface for creating embeddings. It lives in core (rather
// than client/embeddings) because protocol adapters implement it — keeping the
// "subpackages import core only" layering rule intact.
type Embedder interface {
	CreateEmbedding(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

// EmbeddingParams holds asymmetric params for indexing vs query.
type EmbeddingParams struct {
	Indexing map[string]string `json:"indexing,omitempty"`
	Query    map[string]string `json:"query,omitempty"`
}

// EmbeddingRequest represents an embedding API call.
type EmbeddingRequest struct {
	Model  string            `json:"model"`
	Input  []string          `json:"input"`
	Params map[string]string `json:"params,omitempty"` // indexing or query params
}

// EmbeddingResponse holds embedding results.
type EmbeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Usage      *EyrieUsage `json:"usage,omitempty"`
}
