package adapters

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestCreateEmbedding_Validation(t *testing.T) {
	t.Parallel()
	client := &OpenAIClient{providerName: "test"}
	_, err := client.CreateEmbedding(context.Background(), core.EmbeddingRequest{Model: "", Input: []string{"hello"}})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestCreateEmbedding_Success(t *testing.T) {
	t.Parallel()
	var gotPath, gotMethod string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotMethod = req.Method
		return jsonResponse(http.StatusOK, map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1},
		}), nil
	})

	client := &OpenAIClient{
		providerName: "openai",
		baseURL:      "https://api.openai.com/v1",
		httpClient:   &http.Client{Transport: transport},
		logger:       testLogger(t),
	}

	resp, err := client.CreateEmbedding(context.Background(), core.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello world"},
	})
	if err != nil {
		t.Fatalf("CreateEmbedding: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/embeddings") {
		t.Errorf("path = %q, want suffix /embeddings", gotPath)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if resp.Model != "text-embedding-3-small" {
		t.Errorf("model = %q, want text-embedding-3-small", resp.Model)
	}
	if len(resp.Embeddings) != 1 || len(resp.Embeddings[0]) != 3 {
		t.Fatalf("expected 1 embedding of dim 3, got %d embeddings of dim %d", len(resp.Embeddings), len(resp.Embeddings[0]))
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 1 {
		t.Errorf("expected prompt_tokens=1, got %+v", resp.Usage)
	}
}

func TestCreateEmbedding_MultipleInputs(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "index": 0, "embedding": []float64{1.0, 0.0}},
				{"object": "embedding", "index": 1, "embedding": []float64{0.0, 1.0}},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]int{"prompt_tokens": 2, "total_tokens": 2},
		}), nil
	})

	client := &OpenAIClient{
		providerName: "openai",
		baseURL:      "https://api.openai.com/v1",
		httpClient:   &http.Client{Transport: transport},
		logger:       testLogger(t),
	}

	resp, err := client.CreateEmbedding(context.Background(), core.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("CreateEmbedding: %v", err)
	}
	if len(resp.Embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(resp.Embeddings))
	}
	if resp.Embeddings[0][0] != 1.0 || resp.Embeddings[1][1] != 1.0 {
		t.Errorf("unexpected embeddings: %v", resp.Embeddings)
	}
}

func TestCreateEmbedding_WithExtraParams(t *testing.T) {
	t.Parallel()
	var sentBody struct {
		Model      string   `json:"model"`
		Input      []string `json:"input"`
		Dimensions string   `json:"dimensions"`
	}
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		jsonDecodeRequest(req, &sentBody)
		return jsonResponse(http.StatusOK, map[string]any{
			"object": "list",
			"data":   []map[string]any{{"object": "embedding", "index": 0, "embedding": []float64{0.5}}},
			"model":  "text-embedding-3-small",
			"usage":  map[string]int{"prompt_tokens": 1, "total_tokens": 1},
		}), nil
	})

	client := &OpenAIClient{
		providerName: "openai",
		baseURL:      "https://api.openai.com/v1",
		httpClient:   &http.Client{Transport: transport},
		logger:       testLogger(t),
	}

	_, err := client.CreateEmbedding(context.Background(), core.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
		Params: map[string]string{
			"dimensions": "256",
		},
	})
	if err != nil {
		t.Fatalf("CreateEmbedding: %v", err)
	}
	if sentBody.Dimensions != "256" {
		t.Errorf("expected dimensions=256, got %q", sentBody.Dimensions)
	}
}

func TestCreateEmbedding_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"message": "Invalid API key"},
		}), nil
	})

	client := &OpenAIClient{
		providerName: "openai",
		baseURL:      "https://api.openai.com/v1",
		httpClient:   &http.Client{Transport: transport},
		logger:       testLogger(t),
	}

	_, err := client.CreateEmbedding(context.Background(), core.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"hello"},
	})
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}
