package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
	"github.com/GrayCodeAI/graycode-router/llm"
)

const (
	// maxAzureRequestSize is the maximum request body size for Azure OpenAI (30 MB).
	maxAzureRequestSize = 30 * 1024 * 1024
)

type AzureClient struct {
	apiKey     string
	endpoint   string
	apiVersion string
	httpClient *http.Client
	retry      core.RetryConfig
	logger     *slog.Logger
	guardrails *core.Guardrails
}

var _ core.Provider = (*AzureClient)(nil)

func NewAzureClient(apiKey, endpoint, apiVersion string) *AzureClient {
	if apiVersion == "" {
		apiVersion = "2024-10-21"
	}
	return &AzureClient{
		apiKey:     apiKey,
		endpoint:   strings.TrimRight(endpoint, "/"),
		apiVersion: apiVersion,
		httpClient: core.NewPooledHTTPClient(core.DefaultTimeout),
		retry:      core.DefaultRetryConfig(),
		logger:     slog.Default(),
	}
}

func (c *AzureClient) Name() string { return "azure" }

func (c *AzureClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("User-Agent", core.UserAgent())
}

func (c *AzureClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, fmt.Errorf("graycode-router: model is required for azure")
	}

	reqBody := c.buildRequest(messages, opts, false)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure marshal request failed: %w", err)
	}
	if len(body) > maxAzureRequestSize {
		return nil, fmt.Errorf("graycode-router: request size %d bytes exceeds Azure limit of %d bytes", len(body), maxAzureRequestSize)
	}
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", c.endpoint, opts.Model, c.apiVersion)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure request creation failed: %w", err)
	}
	c.setHeaders(req)
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	c.logger.Debug("azure chat", "model", opts.Model, "endpoint", c.endpoint)

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("azure: close response body", "error", err)
		}
	}()

	requestID := resp.Header.Get("X-Request-Id")
	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("azure", "chat", resp.StatusCode, requestID, detail, readErr)
	}

	var or openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("graycode-router: azure decode failed: %w", err)
	}

	result := &core.GraycodeRouterResponse{FinishReason: "unknown", RequestID: requestID}
	if len(or.Choices) > 0 {
		ch := or.Choices[0]
		result.Content = ch.Message.Content
		result.FinishReason = ch.FinishReason
		for _, tc := range ch.Message.ToolCalls {
			var args map[string]interface{}
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			result.ToolCalls = append(result.ToolCalls, core.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
		}
	}
	if or.Usage != nil {
		result.Usage = &core.GraycodeRouterUsage{
			PromptTokens:     or.Usage.PromptTokens,
			CompletionTokens: or.Usage.CompletionTokens,
			TotalTokens:      or.Usage.TotalTokens,
		}
		if or.Usage.PromptTokensDetails != nil {
			result.Usage.CacheReadTokens = or.Usage.PromptTokensDetails.CachedTokens
		}
	}

	if err := core.ApplyGuardrails(ctx, result, c.guardrails); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *AzureClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, fmt.Errorf("graycode-router: model is required for azure")
	}

	reqBody := c.buildRequest(messages, opts, true)
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure stream marshal request failed: %w", err)
	}
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", c.endpoint, opts.Model, c.apiVersion)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure stream request creation failed: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	c.logger.Debug("azure stream", "model", opts.Model, "endpoint", c.endpoint)

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: azure stream request failed: %w", err)
	}

	requestID := resp.Header.Get("X-Request-Id")
	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		_ = resp.Body.Close()
		return nil, core.FormatAPIError("azure", "stream", resp.StatusCode, requestID, detail, readErr)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	sseEvents := core.ParseSSEStream(streamCtx, resp.Body, c.logger)
	events := core.ProcessOpenAIStream(streamCtx, sseEvents, c.logger)

	return llm.NewStreamResult(events, requestID, cancel), nil
}

func (c *AzureClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/openai/models?api-version=%s", c.endpoint, c.apiVersion)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graycode-router: azure ping failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("graycode-router: azure: invalid API key")
	}
	return nil
}

func (c *AzureClient) buildRequest(messages []core.GraycodeRouterMessage, opts core.ChatOptions, stream bool) openaiRequest {
	return buildRequestBase(messages, opts, stream, &AzureCompat)
}

// SetHTTPClient replaces the transport used for Azure requests.
func (c *AzureClient) SetHTTPClient(hc *http.Client) { c.httpClient = hc }

// SetRetry configures provider retry behavior.
func (c *AzureClient) SetRetry(rc core.RetryConfig) { c.retry = rc }

// APIVersion returns the API version.
func (c *AzureClient) APIVersion() string { return c.apiVersion }

// Endpoint returns the base endpoint.
func (c *AzureClient) Endpoint() string { return c.endpoint }
