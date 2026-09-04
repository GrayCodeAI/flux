package embeddings

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestDefaultEmbeddingParamsCohere(t *testing.T) {
	t.Parallel()
	tests := []string{"cohere-embed-v3", "embed-v2", "embed-english-v3.0"}
	for _, model := range tests {
		p := DefaultEmbeddingParams(model)
		if p.Indexing["input_type"] != "search_document" {
			t.Errorf("model %q: Indexing[input_type] = %q, want %q", model, p.Indexing["input_type"], "search_document")
		}
		if p.Query["input_type"] != "search_query" {
			t.Errorf("model %q: Query[input_type] = %q, want %q", model, p.Query["input_type"], "search_query")
		}
	}
}

func TestDefaultEmbeddingParamsVoyage(t *testing.T) {
	t.Parallel()
	tests := []string{"voyage-2", "voyage-large-2", "voyage-code-2"}
	for _, model := range tests {
		p := DefaultEmbeddingParams(model)
		if p.Indexing["input_type"] != "document" {
			t.Errorf("model %q: Indexing[input_type] = %q, want %q", model, p.Indexing["input_type"], "document")
		}
		if p.Query["input_type"] != "query" {
			t.Errorf("model %q: Query[input_type] = %q, want %q", model, p.Query["input_type"], "query")
		}
	}
}

func TestDefaultEmbeddingParamsNomic(t *testing.T) {
	t.Parallel()
	p := DefaultEmbeddingParams("nomic-embed-text-v1")
	if len(p.Indexing) != 0 {
		t.Errorf("Nomic Indexing should be empty, got %v", p.Indexing)
	}
	if p.Query["prompt_name"] != "query" {
		t.Errorf("Nomic Query[prompt_name] = %q, want %q", p.Query["prompt_name"], "query")
	}
}

func TestDefaultEmbeddingParamsGemini(t *testing.T) {
	t.Parallel()
	tests := []string{"gemini-embedding-001", "text-embedding-004"}
	for _, model := range tests {
		p := DefaultEmbeddingParams(model)
		if p.Indexing["task_type"] != "RETRIEVAL_DOCUMENT" {
			t.Errorf("model %q: Indexing[task_type] = %q, want %q", model, p.Indexing["task_type"], "RETRIEVAL_DOCUMENT")
		}
		if p.Query["task_type"] != "RETRIEVAL_QUERY" {
			t.Errorf("model %q: Query[task_type] = %q, want %q", model, p.Query["task_type"], "RETRIEVAL_QUERY")
		}
	}
}

func TestDefaultEmbeddingParamsUnknown(t *testing.T) {
	t.Parallel()
	p := DefaultEmbeddingParams("some-unknown-model")
	if len(p.Indexing) != 0 || len(p.Query) != 0 {
		t.Errorf("unknown model should return empty params, got Indexing=%v Query=%v", p.Indexing, p.Query)
	}
}

func TestDefaultEmbeddingParamsCaseInsensitive(t *testing.T) {
	t.Parallel()
	p := DefaultEmbeddingParams("COHERE-EMBED-V3")
	if p.Indexing["input_type"] != "search_document" {
		t.Errorf("case-insensitive lookup failed: Indexing[input_type] = %q", p.Indexing["input_type"])
	}
}

func TestEmbeddingRequestStruct(t *testing.T) {
	t.Parallel()
	req := EmbeddingRequest{
		Model:  "test",
		Input:  []string{"a", "b"},
		Params: map[string]string{"key": "value"},
	}
	if req.Model != "test" {
		t.Errorf("Model = %q, want %q", req.Model, "test")
	}
	if len(req.Input) != 2 {
		t.Errorf("Input length = %d, want 2", len(req.Input))
	}
	if req.Params["key"] != "value" {
		t.Errorf("Params[key] = %q, want %q", req.Params["key"], "value")
	}
}

func TestEmbeddingResponseStruct(t *testing.T) {
	t.Parallel()
	resp := &EmbeddingResponse{
		Embeddings: [][]float32{{0.1, 0.2}},
		Model:      "test",
		Usage:      &core.GraycodeRouterUsage{PromptTokens: 5, TotalTokens: 5},
	}
	if resp.Model != "test" {
		t.Errorf("Model = %q, want %q", resp.Model, "test")
	}
	if len(resp.Embeddings) != 1 {
		t.Errorf("Embeddings length = %d, want 1", len(resp.Embeddings))
	}
	if resp.Usage.TotalTokens != 5 {
		t.Errorf("Usage.TotalTokens = %d, want 5", resp.Usage.TotalTokens)
	}
}

func TestEmbeddingParamsStruct(t *testing.T) {
	t.Parallel()
	p := EmbeddingParams{
		Indexing: map[string]string{"task": "document"},
		Query:    map[string]string{"task": "query"},
	}
	if p.Indexing["task"] != "document" {
		t.Errorf("Indexing[task] = %q, want %q", p.Indexing["task"], "document")
	}
	if p.Query["task"] != "query" {
		t.Errorf("Query[task] = %q, want %q", p.Query["task"], "query")
	}
}
