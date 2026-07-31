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
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// ConcentrateResponsesClient uses the Concentrate Responses API (the production-ready
// API per Concentrate docs). This is the recommended adapter for Concentrate AI.
//
// The Responses API uses /v1/responses endpoint with a unified format that works
// across all models (OpenAI, Anthropic, Google, etc.).
type ConcentrateResponsesClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewConcentrateResponsesClient builds a Concentrate AI provider client using the
// Responses API. baseURL is typically "https://api.concentrate.ai/v1".
func NewConcentrateResponsesClient(apiKey, baseURL string, opts ...core.ClientOption) *ConcentrateResponsesClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	c := &ConcentrateResponsesClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt.Apply(c)
	}
	return c
}

func (c *ConcentrateResponsesClient) Name() string { return "concentrate" }

// responsesRequest is the request body for the Concentrate Responses API.
type responsesRequest struct {
	Input              interface{}          `json:"input"`
	Model              string               `json:"model"`
	Instructions       string               `json:"instructions,omitempty"`
	MaxOutputTokens    int                  `json:"max_output_tokens,omitempty"`
	Temperature        *float64             `json:"temperature,omitempty"`
	TopP               *float64             `json:"top_p,omitempty"`
	Stream             bool                 `json:"stream,omitempty"`
	Reasoning          *responsesReasoning  `json:"reasoning,omitempty"`
	Tools              []responsesTool      `json:"tools,omitempty"`
	ToolChoice         interface{}          `json:"tool_choice,omitempty"`
	ParallelToolCalls  bool                 `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string               `json:"previous_response_id,omitempty"`
	PromptCacheKey     string               `json:"prompt_cache_key,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
	Text               *responsesTextConfig `json:"text,omitempty"`
}

type responsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type responsesTool struct {
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description,omitempty"`
		Parameters  map[string]interface{} `json:"parameters,omitempty"`
	} `json:"function,omitempty"`
}

type responsesTextConfig struct {
	Format    string `json:"format,omitempty"`
	Verbosity string `json:"verbosity,omitempty"`
}

// responsesResponse is the response from the Concentrate Responses API.
type responsesResponse struct {
	ID     string          `json:"id"`
	Object string          `json:"object"`
	Model  string          `json:"model"`
	Output []outputItem    `json:"output"`
	Usage  *responsesUsage `json:"usage,omitempty"`
}

type outputItem struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Status    string          `json:"status,omitempty"`
	Role      string          `json:"role,omitempty"`
	Content   []outputContent `json:"content,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	Name      string          `json:"name,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
}

type outputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Chat implements core.Provider using the Responses API.
func (c *ConcentrateResponsesClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	req := c.buildRequest(messages, opts, false)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("concentrate: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("concentrate: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("concentrate: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("concentrate: request failed (%d): %s", resp.StatusCode, string(body))
	}

	var apiResp responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("concentrate: decode response: %w", err)
	}

	return c.toEyrieResponse(apiResp), nil
}

// StreamChat implements core.Provider using the Responses API with SSE streaming.
func (c *ConcentrateResponsesClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	req := c.buildRequest(messages, opts, true)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("concentrate: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("concentrate: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("concentrate: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("concentrate: stream request failed (%d): %s", resp.StatusCode, string(body))
	}

	return c.handleStream(ctx, resp), nil
}

// Ping checks the health of the Concentrate API.
func (c *ConcentrateResponsesClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return fmt.Errorf("concentrate: create ping request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("concentrate: ping failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("concentrate: ping failed (%d)", resp.StatusCode)
	}
	return nil
}

func (c *ConcentrateResponsesClient) buildRequest(messages []core.EyrieMessage, opts core.ChatOptions, stream bool) responsesRequest {
	req := responsesRequest{
		Model:             opts.Model,
		Input:             c.messagesToInput(messages),
		MaxOutputTokens:   opts.MaxTokens,
		Stream:            stream,
		ParallelToolCalls: true,
	}

	if opts.System != "" {
		req.Instructions = opts.System
	}

	if opts.Temperature != nil {
		req.Temperature = opts.Temperature
	}

	if opts.TopP != nil {
		req.TopP = opts.TopP
	}

	// Map reasoning effort
	if opts.ReasoningEffort != "" {
		req.Reasoning = &responsesReasoning{
			Effort: opts.ReasoningEffort,
		}
	}

	// Map tools
	if len(opts.Tools) > 0 {
		req.Tools = make([]responsesTool, 0, len(opts.Tools))
		for _, tool := range opts.Tools {
			t := responsesTool{
				Type: "function",
			}
			t.Function.Name = tool.Name
			t.Function.Description = tool.Description
			t.Function.Parameters = tool.Parameters
			req.Tools = append(req.Tools, t)
		}
	}

	// Map text format for structured output
	if opts.ResponseFormat != nil && opts.ResponseFormat.Type == "json_schema" {
		req.Text = &responsesTextConfig{
			Format: "json_schema",
		}
	}

	return req
}

func (c *ConcentrateResponsesClient) messagesToInput(messages []core.EyrieMessage) []map[string]interface{} {
	input := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		item := map[string]interface{}{
			"role": msg.Role,
		}
		if msg.Content != "" {
			item["content"] = msg.Content
		}
		input = append(input, item)
	}
	return input
}

func (c *ConcentrateResponsesClient) toEyrieResponse(resp responsesResponse) *core.EyrieResponse {
	eyrieResp := &core.EyrieResponse{
		Content: c.extractOutputText(resp.Output),
	}

	if resp.Usage != nil {
		eyrieResp.Usage = &core.EyrieUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	// Extract tool calls
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			var args map[string]interface{}
			if item.Arguments != "" {
				_ = json.Unmarshal([]byte(item.Arguments), &args)
			}
			eyrieResp.ToolCalls = append(eyrieResp.ToolCalls, core.ToolCall{
				ID:        item.ID,
				Name:      item.Name,
				Arguments: args,
			})
		}
	}

	return eyrieResp
}

func (c *ConcentrateResponsesClient) extractOutputText(output []outputItem) string {
	var text strings.Builder
	for _, item := range output {
		if item.Type == "message" {
			for _, content := range item.Content {
				if content.Type == "text" {
					text.WriteString(content.Text)
				}
			}
		}
	}
	return text.String()
}

// streamEvent represents a single SSE event from the Responses API.
type streamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber int             `json:"sequence_number,omitempty"`
	Delta          string          `json:"delta,omitempty"`
	Item           json.RawMessage `json:"item,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
	OutputIndex    int             `json:"output_index,omitempty"`
	ContentIndex   int             `json:"content_index,omitempty"`
}

func (c *ConcentrateResponsesClient) handleStream(ctx context.Context, resp *http.Response) *core.StreamResult {
	events := make(chan core.EyrieStreamEvent, core.StreamChannelBuffer)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		reader := newSSEReader(resp.Body)
		for {
			event, err := reader.Read()
			if err != nil {
				if err != io.EOF {
					events <- core.EyrieStreamEvent{Type: "error", Error: err.Error()}
				}
				return
			}

			switch event.Type {
			case "response.output_text.delta":
				if event.Delta != "" {
					events <- core.EyrieStreamEvent{Type: "content", Content: event.Delta}
				}
			case "response.reasoning_text.delta":
				if event.Delta != "" {
					events <- core.EyrieStreamEvent{Type: "thinking", Thinking: event.Delta}
				}
			case "response.function_call_arguments.delta":
				if event.Delta != "" {
					events <- core.EyrieStreamEvent{Type: "tool_call_delta", Content: event.Delta}
				}
			case "response.completed":
				var r responsesResponse
				if err := json.Unmarshal(event.Response, &r); err == nil && r.Usage != nil {
					events <- core.EyrieStreamEvent{Type: "usage", Usage: &core.EyrieUsage{
						PromptTokens:     r.Usage.InputTokens,
						CompletionTokens: r.Usage.OutputTokens,
						TotalTokens:      r.Usage.TotalTokens,
					}}
				}
				events <- core.EyrieStreamEvent{Type: "done", StopReason: "stop"}
				return
			case "response.failed":
				events <- core.EyrieStreamEvent{Type: "error", Error: "response failed"}
				return
			case "error":
				events <- core.EyrieStreamEvent{Type: "error", Error: "stream error"}
				return
			}
		}
	}()

	return llm.NewStreamResult(events, "", func() {})
}

// sseReader reads SSE events from a stream.
type sseReader struct {
	buf    []byte
	reader io.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{reader: r}
}

func (s *sseReader) Read() (streamEvent, error) {
	var event streamEvent
	var data strings.Builder

	buf := make([]byte, 4096)
	for {
		n, err := s.reader.Read(buf)
		if err != nil {
			return event, err
		}
		s.buf = append(s.buf, buf[:n]...)

		// Process complete SSE events (separated by double newline)
		for {
			idx := bytes.Index(s.buf, []byte("\n\n"))
			if idx == -1 {
				break
			}

			block := string(s.buf[:idx])
			s.buf = s.buf[idx+2:]

			lines := strings.Split(block, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					data.WriteString(strings.TrimPrefix(line, "data: "))
				}
			}

			if data.Len() > 0 {
				if err := json.Unmarshal([]byte(data.String()), &event); err != nil {
					return event, fmt.Errorf("concentrate: unmarshal stream event: %w", err)
				}
				return event, nil
			}
		}
	}
}

// Ensure ConcentrateResponsesClient implements core.Provider.
var _ core.Provider = (*ConcentrateResponsesClient)(nil)

// Ensure ConcentrateResponsesClient implements Configurable for ClientOption.Apply.
var _ core.Configurable = (*ConcentrateResponsesClient)(nil)

// SetTimeout implements core.Configurable.
func (c *ConcentrateResponsesClient) SetTimeout(d time.Duration) {
	c.httpClient.Timeout = d
}

// SetHTTPClient implements core.Configurable.
func (c *ConcentrateResponsesClient) SetHTTPClient(hc *http.Client) {
	c.httpClient = hc
}

// SetRetry implements core.Configurable.
func (c *ConcentrateResponsesClient) SetRetry(rc core.RetryConfig) {
	// Retries handled at HTTP client level
}

// SetLogger implements core.Configurable.
func (c *ConcentrateResponsesClient) SetLogger(l *slog.Logger) {
	c.logger = l
}

// SetGuardrails implements core.Configurable.
func (c *ConcentrateResponsesClient) SetGuardrails(g *core.Guardrails) {
	// Guardrails configured at API key level per Concentrate docs
}

// SetProviderName implements core.Configurable.
func (c *ConcentrateResponsesClient) SetProviderName(string) {
	// Provider name fixed
}

// SetMimoAuth implements core.Configurable.
func (c *ConcentrateResponsesClient) SetMimoAuth() {
	// Not applicable
}

// HTTPClient implements core.Configurable.
func (c *ConcentrateResponsesClient) HTTPClient() *http.Client {
	return c.httpClient
}

// Retry implements core.Configurable.
func (c *ConcentrateResponsesClient) Retry() core.RetryConfig {
	return core.RetryConfig{}
}

// Logger implements core.Configurable.
func (c *ConcentrateResponsesClient) Logger() *slog.Logger {
	return c.logger
}

// Guardrails implements core.Configurable.
func (c *ConcentrateResponsesClient) Guardrails() *core.Guardrails {
	return nil
}

// DefaultModel implements core.Configurable.
func (c *ConcentrateResponsesClient) DefaultModel() string {
	return ""
}

// DefaultMaxTokens implements core.Configurable.
func (c *ConcentrateResponsesClient) DefaultMaxTokens() int {
	return 0
}

// DefaultTemperature implements core.Configurable.
func (c *ConcentrateResponsesClient) DefaultTemperature() *float64 {
	return nil
}

// Version implements core.Configurable.
func (c *ConcentrateResponsesClient) Version() string {
	return "v1"
}

// SetDefaultModel implements core.Configurable.
func (c *ConcentrateResponsesClient) SetDefaultModel(model string) {
	// Not applicable
}

// SetDefaultMaxTokens implements core.Configurable.
func (c *ConcentrateResponsesClient) SetDefaultMaxTokens(n int) {
	// Not applicable
}

// SetDefaultTemperature implements core.Configurable.
func (c *ConcentrateResponsesClient) SetDefaultTemperature(t float64) {
	// Not applicable
}

// BaseURL implements core.Configurable.
func (c *ConcentrateResponsesClient) BaseURL() string {
	return c.baseURL
}

// SetBaseURL implements core.Configurable.
func (c *ConcentrateResponsesClient) SetBaseURL(url string) {
	c.baseURL = strings.TrimRight(strings.TrimSpace(url), "/")
}

// SetAPIKey implements core.Configurable.
func (c *ConcentrateResponsesClient) SetAPIKey(key string) {
	c.apiKey = key
}
