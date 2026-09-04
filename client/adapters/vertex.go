package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GrayCodeAI/graycode-router/client/core"
	"github.com/GrayCodeAI/graycode-router/llm"
)

// maxVertexRequestSize is the maximum request body size for Vertex AI (30 MB).
const maxVertexRequestSize = 30 * 1024 * 1024

type VertexClient struct {
	projectID  string
	region     string
	token      string
	httpClient *http.Client
	retry      core.RetryConfig
	logger     *slog.Logger
	guardrails *core.Guardrails
}

var _ core.Provider = (*VertexClient)(nil)

func NewVertexClient(projectID, region, token string) *VertexClient {
	return &VertexClient{
		projectID:  projectID,
		region:     region,
		token:      token,
		httpClient: core.NewPooledHTTPClient(core.DefaultTimeout),
		retry:      core.DefaultRetryConfig(),
		logger:     slog.Default().With("component", "vertex"),
	}
}

func (c *VertexClient) Name() string { return "anthropic-vertex" }

func (c *VertexClient) baseURL() string {
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models", c.region, c.projectID, c.region)
}

func (c *VertexClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, fmt.Errorf("graycode-router: model is required for vertex")
	}
	body, err := c.buildBody(messages, opts, false)
	if err != nil {
		return nil, err
	}

	// Check request size (30 MB Vertex limit)
	if len(body) > maxVertexRequestSize {
		return nil, fmt.Errorf("graycode-router: request size %d bytes exceeds Vertex limit of %d bytes", len(body), maxVertexRequestSize)
	}

	url := fmt.Sprintf("%s/%s:rawPredict", c.baseURL(), opts.Model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graycode-router: vertex request creation failed: %w", err)
	}
	c.setHeaders(req)
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: vertex request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("vertex: close response body", "error", err)
		}
	}()

	requestID := resp.Header.Get("X-Goog-Request-Id")

	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("vertex", "chat", resp.StatusCode, requestID, detail, readErr)
	}

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("graycode-router: vertex decode failed: %w", err)
	}

	graycodeRouterResp := ParseAnthropicResponse(ar, requestID, "")

	if err := core.ApplyGuardrails(ctx, graycodeRouterResp, c.guardrails); err != nil {
		return nil, err
	}

	return graycodeRouterResp, nil
}

func (c *VertexClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, fmt.Errorf("graycode-router: model is required for vertex")
	}
	body, err := c.buildBody(messages, opts, true)
	if err != nil {
		return nil, err
	}

	// Check request size (30 MB Vertex limit)
	if len(body) > maxVertexRequestSize {
		return nil, fmt.Errorf("graycode-router: request size %d bytes exceeds Vertex limit of %d bytes", len(body), maxVertexRequestSize)
	}

	url := fmt.Sprintf("%s/%s:streamRawPredict", c.baseURL(), opts.Model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graycode-router: vertex stream request failed: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	resp, err := core.DoWithRetry(ctx, c.httpClient, req, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: vertex stream request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		_ = resp.Body.Close()
		return nil, core.FormatAPIError("vertex", "stream", resp.StatusCode, resp.Header.Get("X-Goog-Request-Id"), detail, readErr)
	}

	requestID := resp.Header.Get("X-Goog-Request-Id")

	streamCtx, cancel := context.WithCancel(ctx)
	sseEvents := core.ParseSSEStream(streamCtx, resp.Body, c.logger)
	events := core.ProcessAnthropicStream(streamCtx, sseEvents, c.logger)

	return llm.NewStreamResult(events, requestID, cancel), nil
}

func (c *VertexClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models", c.region, c.projectID, c.region)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graycode-router: vertex ping failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("graycode-router: vertex: invalid credentials")
	}
	return nil
}

func (c *VertexClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", core.UserAgent())
}

func (c *VertexClient) buildBody(messages []core.GraycodeRouterMessage, opts core.ChatOptions, stream bool) ([]byte, error) {
	msgs, system := buildAnthropicMessages(messages)
	if opts.System != "" {
		if system != "" {
			system = opts.System + "\n\n" + system
		} else {
			system = opts.System
		}
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	thinking := resolveThinking(opts)

	reqBody := anthropicRequest{
		Model:         opts.Model,
		MaxTokens:     maxTokens,
		Messages:      msgs,
		System:        system,
		Temperature:   opts.Temperature,
		TopP:          opts.TopP,
		TopK:          opts.TopK,
		StopSequences: opts.StopSequences,
		Stream:        stream,
		Tools:         ConvertToAnthropicTools(opts.Tools),
		ToolChoice:    resolveToolChoice(opts.ToolChoice),
		Thinking:      thinking,
		Metadata:      resolveMetadata(opts),
		ServiceTier:   opts.ServiceTier,
		OutputConfig:  resolveOutputConfig(opts),
	}

	// Marshal and inject anthropic_version (Vertex-specific)
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	// Add anthropic_version field which is required for Vertex
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m["anthropic_version"] = "vertex-2023-10-16"
	return json.Marshal(m)
}

// SetHTTPClient replaces the transport used for Vertex requests.
func (c *VertexClient) SetHTTPClient(hc *http.Client) { c.httpClient = hc }

// SetRetry configures provider retry behavior.
func (c *VertexClient) SetRetry(rc core.RetryConfig) { c.retry = rc }

// HTTPClient returns the configured transport client.
func (c *VertexClient) HTTPClient() *http.Client { return c.httpClient }

// BaseURL returns the base URL for Vertex API requests.
func (c *VertexClient) BaseURL() string { return c.baseURL() }

// BuildBody constructs the request body for Vertex API calls.
func (c *VertexClient) BuildBody(messages []core.GraycodeRouterMessage, opts core.ChatOptions, stream bool) ([]byte, error) {
	return c.buildBody(messages, opts, stream)
}

// Region returns the configured Google Cloud region.
func (c *VertexClient) Region() string { return c.region }

// ProjectID returns the configured Google Cloud project identifier.
func (c *VertexClient) ProjectID() string { return c.projectID }
