package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// Compile-time check that OpenAIClient implements core.Embedder.
var _ core.Embedder = (*OpenAIClient)(nil)

// openaiEmbeddingResponse is the wire format for OpenAI-compatible embedding APIs.
type openaiEmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type OpenAIEmbeddingData = openaiEmbeddingData

type openaiEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type OpenAIEmbeddingUsage = openaiEmbeddingUsage

type openaiEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []openaiEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  openaiEmbeddingUsage  `json:"usage"`
}

type OpenAIEmbeddingResponse = openaiEmbeddingResponse

// CreateEmbedding sends an embedding request to the OpenAI-compatible API endpoint.
func (c *OpenAIClient) CreateEmbedding(ctx context.Context, req core.EmbeddingRequest) (*core.EmbeddingResponse, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("eyrie: model is required for %s embeddings", c.providerName)
	}

	bodyMap := map[string]interface{}{
		"model": req.Model,
		"input": req.Input,
	}
	for k, v := range req.Params {
		bodyMap[k] = v
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("eyrie: failed to marshal embedding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("eyrie: failed to create embedding request: %w", err)
	}
	c.setHeaders(httpReq)
	httpReq.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	c.logger.Debug("openai embedding", "provider", c.providerName, "model", req.Model)

	resp, err := core.DoWithRetry(ctx, c.httpClient, httpReq, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("eyrie: %s embedding request failed: %w", c.providerName, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("embedding: close response body", "error", err)
		}
	}()

	if resp.StatusCode != 200 {
		requestID := resp.Header.Get("X-Request-Id")
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError(c.providerName+" embedding", "embedding", resp.StatusCode, requestID, detail, readErr)
	}

	var or openaiEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("eyrie: failed to decode %s embedding response: %w", c.providerName, err)
	}

	result := &core.EmbeddingResponse{
		Model: or.Model,
	}
	if len(or.Data) > 0 {
		result.Embeddings = make([][]float32, len(or.Data))
		for _, d := range or.Data {
			vec := make([]float32, len(d.Embedding))
			for j, v := range d.Embedding {
				vec[j] = float32(v)
			}
			result.Embeddings[d.Index] = vec
		}
	}
	if or.Usage.PromptTokens > 0 || or.Usage.TotalTokens > 0 {
		result.Usage = &core.EyrieUsage{
			PromptTokens: or.Usage.PromptTokens,
			TotalTokens:  or.Usage.TotalTokens,
		}
	}

	return result, nil
}
