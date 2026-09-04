package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// BatchRequest represents a single request in a batch.
type BatchRequest struct {
	CustomID string                  `json:"custom_id"`
	Messages []GraycodeRouterMessage `json:"messages"`
	Options  ChatOptions             `json:"options"`
}

// BatchResponse represents a single response from a batch.
type BatchResponse struct {
	CustomID string                  `json:"custom_id"`
	Response *GraycodeRouterResponse `json:"response,omitempty"`
	Error    string                  `json:"error,omitempty"`
}

// BatchResult holds the overall batch operation result.
type BatchResult struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"` // "in_progress", "ended", "failed"
	Responses []BatchResponse `json:"responses,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// BatchClient handles Anthropic Message Batches API (50% cost discount).
type BatchClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewBatchClient creates a batch client for Anthropic's batch API.
func NewBatchClient(apiKey, baseURL string) *BatchClient {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &BatchClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: NewPooledHTTPClient(5 * time.Minute),
	}
}

// Submit sends a batch of requests. Returns the batch ID for polling.
func (bc *BatchClient) Submit(ctx context.Context, requests []BatchRequest) (string, error) {
	if len(requests) == 0 {
		return "", fmt.Errorf("graycode-router: batch requires at least one request")
	}

	type batchReqItem struct {
		CustomID string                 `json:"custom_id"`
		Params   map[string]interface{} `json:"params"`
	}

	items := make([]batchReqItem, len(requests))
	for i, r := range requests {
		maxTokens := r.Options.MaxTokens
		if maxTokens == 0 {
			maxTokens = 4096
		}
		msgs := make([]map[string]interface{}, 0, len(r.Messages))
		var system string
		for _, m := range r.Messages {
			if m.Role == "system" {
				system = m.Content
				continue
			}
			msgs = append(msgs, map[string]interface{}{"role": m.Role, "content": m.Content})
		}
		params := map[string]interface{}{
			"model":      r.Options.Model,
			"max_tokens": maxTokens,
			"messages":   msgs,
		}
		if system != "" {
			params["system"] = system
		}
		if r.Options.System != "" {
			params["system"] = r.Options.System
		}
		items[i] = batchReqItem{CustomID: r.CustomID, Params: params}
	}

	body, _ := json.Marshal(map[string]interface{}{"requests": items})

	req, err := http.NewRequestWithContext(ctx, "POST", bc.baseURL+"/v1/messages/batches", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", bc.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "message-batches-2024-09-24")

	// Retry on transient errors (429, 500, 502, 503).
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = bc.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("graycode-router: batch submit failed: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		break
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("batch: close response body", "error", err)
		}
	}()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("graycode-router: batch API error %d: %s", resp.StatusCode, string(errBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

// Poll checks the status of a batch. Returns the result when complete.
func (bc *BatchClient) Poll(ctx context.Context, batchID string) (*BatchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", bc.baseURL+"/v1/messages/batches/"+batchID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", bc.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "message-batches-2024-09-24")

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: batch poll failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("batch: close response body", "error", err)
		}
	}()

	var result BatchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
