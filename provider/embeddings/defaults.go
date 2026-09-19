package embeddings

import "strings"

// DefaultEmbeddingParams returns known-good asymmetric params for common embedding models.
func DefaultEmbeddingParams(model string) EmbeddingParams {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "cohere"), strings.Contains(m, "embed-v"), strings.Contains(m, "embed-english"):
		return EmbeddingParams{
			Indexing: map[string]string{"input_type": "search_document"},
			Query:    map[string]string{"input_type": "search_query"},
		}
	case strings.Contains(m, "voyage"):
		return EmbeddingParams{
			Indexing: map[string]string{"input_type": "document"},
			Query:    map[string]string{"input_type": "query"},
		}
	case strings.Contains(m, "nomic"):
		return EmbeddingParams{
			Indexing: map[string]string{},
			Query:    map[string]string{"prompt_name": "query"},
		}
	case strings.Contains(m, "gemini"), strings.Contains(m, "text-embedding"):
		return EmbeddingParams{
			Indexing: map[string]string{"task_type": "RETRIEVAL_DOCUMENT"},
			Query:    map[string]string{"task_type": "RETRIEVAL_QUERY"},
		}
	default:
		return EmbeddingParams{}
	}
}
