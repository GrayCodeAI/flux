package adapters

import (
	"bufio"
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
	"github.com/GrayCodeAI/eyrie/llm"
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
	retry      core.RetryConfig
	logger     *slog.Logger
}

// NewConcentrateResponsesClient builds a Concentrate AI provider client using the
// Responses API. baseURL is typically "https://api.concentrate.ai/v1".
func NewConcentrateResponsesClient(apiKey, baseURL string, opts ...core.ClientOption) *ConcentrateResponsesClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	c := &ConcentrateResponsesClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: core.NewPooledHTTPClient(core.DefaultTimeout),
		retry:      core.DefaultRetryConfig(),
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
	ParallelToolCalls  *bool                `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string               `json:"previous_response_id,omitempty"`
	PromptCacheKey     string               `json:"prompt_cache_key,omitempty"`
	Metadata           map[string]string    `json:"metadata,omitempty"`
	Text               *responsesTextConfig `json:"text,omitempty"`
	Store              *bool                `json:"store,omitempty"`
	// Extended parameters from Concentrate Responses API
	Include            []string            `json:"include,omitempty"`
	Routing            *routingConfig      `json:"routing,omitempty"`
	MaxToolCalls       *int                `json:"max_tool_calls,omitempty"`
	ContextManagement  []contextManagement `json:"context_management,omitempty"`
	SafetyIdentifier   *string             `json:"safety_identifier,omitempty"`
	Background         *bool               `json:"background,omitempty"`
	Truncation         *string             `json:"truncation,omitempty"`
	Conversation       interface{}         `json:"conversation,omitempty"`
	ServiceTier        string              `json:"service_tier,omitempty"`
	PromptCacheOptions *promptCacheOptions `json:"prompt_cache_options,omitempty"`
	CacheControl       *cacheControl       `json:"cache_control,omitempty"`
}

type promptCacheOptions struct {
	Mode string `json:"mode,omitempty"` // "implicit" or "explicit"
	TTL  string `json:"ttl,omitempty"`  // "5m", "30m", "1h"
}

type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
	TTL  string `json:"ttl,omitempty"`
}

type routingConfig struct {
	Model    interface{} `json:"model,omitempty"`
	Strategy string      `json:"strategy,omitempty"`
	Metric   string      `json:"metric,omitempty"`
}

type contextManagement struct {
	Type string `json:"type"`
}

type responsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type responsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
	Strict      bool                   `json:"strict,omitempty"`
}

type responsesTextConfig struct {
	Format map[string]interface{} `json:"format"`
}

// responsesResponse is the response from the Concentrate Responses API.
type responsesResponse struct {
	ID                string                     `json:"id"`
	Object            string                     `json:"object"`
	Status            string                     `json:"status"`
	Model             string                     `json:"model"`
	Output            []outputItem               `json:"output"`
	Usage             *responsesUsage            `json:"usage,omitempty"`
	Error             *responsesError            `json:"error,omitempty"`
	IncompleteDetails *responsesIncompleteDetail `json:"incomplete_details,omitempty"`
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

type responsesError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type responsesIncompleteDetail struct {
	Reason string `json:"reason,omitempty"`
}

// Chat implements core.Provider using the Responses API.
func (c *ConcentrateResponsesClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	req, err := c.buildRequest(messages, opts, false)
	if err != nil {
		return nil, err
	}

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
	httpReq.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	resp, err := core.DoWithRetry(ctx, c.httpClient, httpReq, c.retry, c.logger)
	if err != nil {
		return nil, fmt.Errorf("concentrate: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := resp.Header.Get("X-Request-Id")

	if resp.StatusCode != http.StatusOK {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("concentrate", "chat", resp.StatusCode, requestID, detail, readErr)
	}

	var apiResp responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("concentrate: decode response: %w", err)
	}

	return c.toEyrieResponse(apiResp), nil
}

// StreamChat implements core.Provider using the Responses API with SSE streaming.
func (c *ConcentrateResponsesClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	req, err := c.buildRequest(messages, opts, true)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("concentrate: marshal request: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("concentrate: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", "eyrie-model-catalog/1.0")
	httpReq.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	resp, err := core.DoWithRetry(streamCtx, c.httpClient, httpReq, c.retry, c.logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("concentrate: stream request failed: %w", err)
	}

	requestID := resp.Header.Get("X-Request-Id")

	if resp.StatusCode != http.StatusOK {
		detail, readErr := core.ParseProviderError(resp.Body)
		_ = resp.Body.Close()
		cancel()
		return nil, core.FormatAPIError("concentrate", "stream", resp.StatusCode, requestID, detail, readErr)
	}

	return c.handleStream(streamCtx, cancel, resp, requestID), nil
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

func (c *ConcentrateResponsesClient) buildRequest(messages []core.EyrieMessage, opts core.ChatOptions, stream bool) (responsesRequest, error) {
	req := responsesRequest{
		Model:           opts.Model,
		Input:           c.messagesToInput(messages),
		MaxOutputTokens: opts.MaxTokens,
		Stream:          stream,
		Metadata:        opts.Metadata,
		Store:           opts.Store,
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
			req.Tools = append(req.Tools, responsesTool{
				Type:        "function",
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  normalizeToolParams(tool.Parameters),
				Strict:      true,
			})
		}
		parallel := true
		if opts.ToolChoice != nil && opts.ToolChoice.DisableParallelToolUse {
			parallel = false
		}
		req.ParallelToolCalls = &parallel
		req.ToolChoice = concentrateToolChoice(opts.ToolChoice)
	}

	if opts.ResponseFormat != nil {
		format := map[string]interface{}{"type": opts.ResponseFormat.Type}
		if opts.ResponseFormat.Type == "json_schema" {
			if strings.TrimSpace(opts.ResponseFormat.Schema) == "" {
				return responsesRequest{}, fmt.Errorf("concentrate: json_schema response format requires a schema")
			}
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(opts.ResponseFormat.Schema), &schema); err != nil {
				return responsesRequest{}, fmt.Errorf("concentrate: invalid response schema: %w", err)
			}
			format["name"] = "hawk_response"
			format["schema"] = schema
		}
		req.Text = &responsesTextConfig{Format: format}
	}

	// Map extended Responses API parameters
	if opts.ServiceTier != "" {
		req.ServiceTier = opts.ServiceTier
	}

	return req, nil
}

func concentrateToolChoice(choice *core.ToolChoiceOption) interface{} {
	if choice == nil {
		return nil
	}
	switch choice.Type {
	case "any":
		return "required"
	case "tool":
		if choice.Name != "" {
			return map[string]interface{}{"type": "function", "name": choice.Name}
		}
		return "required"
	default:
		return choice.Type
	}
}

// normalizeToolParams ensures tool parameters conform to Concentrate's strict mode
// requirements: additionalProperties must be false at the top level when strict=true.
// The input map is never mutated: a shallow copy is returned so the caller's
// tool definition (which may be reused across requests) stays intact.
// See: https://concentrate.ai/docs/api-reference/endpoint/tool-calling
func normalizeToolParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	// Only enforce for object-typed schemas
	if t, ok := params["type"]; ok && t == "object" {
		if _, has := params["additionalProperties"]; !has {
			normalized := make(map[string]interface{}, len(params)+1)
			for k, v := range params {
				normalized[k] = v
			}
			normalized["additionalProperties"] = false
			return normalized
		}
	}
	return params
}

func (c *ConcentrateResponsesClient) messagesToInput(messages []core.EyrieMessage) []map[string]interface{} {
	input := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		if len(msg.ToolResults) > 0 {
			for _, result := range msg.ToolResults {
				item := map[string]interface{}{
					"type":    "function_call_output",
					"call_id": result.ToolUseID,
					"output":  result.Content,
				}
				if result.IsError {
					item["is_error"] = true
				}
				input = append(input, item)
			}
			continue
		}

		if msg.Content != "" || len(msg.ContentParts) > 0 || len(msg.Images) > 0 {
			item := map[string]interface{}{"role": msg.Role}
			switch {
			case len(msg.ContentParts) > 0:
				item["content"] = concentrateContentParts(msg.ContentParts)
			case len(msg.Images) > 0:
				parts := make([]map[string]interface{}, 0, len(msg.Images)+1)
				if msg.Content != "" {
					parts = append(parts, map[string]interface{}{"type": "input_text", "text": msg.Content})
				}
				for _, image := range msg.Images {
					parts = append(parts, map[string]interface{}{"type": "input_image", "image_url": core.OpenAIImageURL(image)})
				}
				item["content"] = parts
			default:
				item["content"] = msg.Content
			}
			input = append(input, item)
		}

		for _, call := range msg.ToolUse {
			arguments, _ := json.Marshal(call.Arguments)
			input = append(input, map[string]interface{}{
				"type":      "function_call",
				"call_id":   call.ID,
				"name":      call.Name,
				"arguments": string(arguments),
			})
		}
	}
	return input
}

func concentrateContentParts(parts []core.ContentPart) []map[string]interface{} {
	content := make([]map[string]interface{}, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			content = append(content, map[string]interface{}{"type": "input_text", "text": part.Text})
		case "image_url":
			if part.ImageURL == nil {
				continue
			}
			item := map[string]interface{}{"type": "input_image", "image_url": part.ImageURL.URL}
			if part.ImageURL.Detail != "" {
				item["detail"] = part.ImageURL.Detail
			}
			content = append(content, item)
		}
	}
	return content
}

func (c *ConcentrateResponsesClient) toEyrieResponse(resp responsesResponse) *core.EyrieResponse {
	eyrieResp := &core.EyrieResponse{
		Content:      c.extractOutputText(resp.Output),
		FinishReason: concentrateFinishReason(resp),
		RequestID:    resp.ID,
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
				ID:        item.CallID,
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
				if content.Type == "output_text" {
					text.WriteString(content.Text)
				}
			}
		}
	}
	return text.String()
}

func concentrateFinishReason(resp responsesResponse) string {
	if resp.IncompleteDetails != nil && resp.IncompleteDetails.Reason != "" {
		return resp.IncompleteDetails.Reason
	}
	for _, item := range resp.Output {
		if item.Type == "function_call" {
			return "tool_calls"
		}
	}
	if resp.Status == "failed" {
		return "error"
	}
	return "stop"
}

// streamEvent represents a single SSE event from the Responses API.
type streamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber int             `json:"sequence_number,omitempty"`
	Delta          string          `json:"delta,omitempty"`
	Arguments      string          `json:"arguments,omitempty"`
	Name           string          `json:"name,omitempty"`
	CallID         string          `json:"call_id,omitempty"`
	Message        string          `json:"message,omitempty"`
	Code           string          `json:"code,omitempty"`
	Item           json.RawMessage `json:"item,omitempty"`
	Response       json.RawMessage `json:"response,omitempty"`
	OutputIndex    int             `json:"output_index,omitempty"`
	ContentIndex   int             `json:"content_index,omitempty"`
}

func (c *ConcentrateResponsesClient) handleStream(ctx context.Context, cancel context.CancelFunc, resp *http.Response, requestID string) *core.StreamResult {
	events := make(chan core.EyrieStreamEvent, core.StreamChannelBuffer)

	go func() {
		defer close(events)
		defer resp.Body.Close()
		defer cancel()

		reader := newSSEReader(resp.Body)
		toolCalls := map[int]outputItem{}
		emittedToolCall := false
		for {
			event, err := reader.Read()
			if err != nil {
				if err != io.EOF {
					sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "error", Error: err.Error()})
				}
				return
			}

			switch event.Type {
			case "response.output_text.delta":
				if event.Delta != "" {
					if !sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "content", Content: event.Delta}) {
						return
					}
				}
			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
				if event.Delta != "" {
					if !sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "thinking", Thinking: event.Delta}) {
						return
					}
				}
			case "response.output_item.added":
				var item outputItem
				if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
					toolCalls[event.OutputIndex] = item
				}
			case "response.function_call_arguments.delta":
				item := toolCalls[event.OutputIndex]
				if event.CallID != "" {
					item.CallID = event.CallID
				}
				item.Arguments += event.Delta
				toolCalls[event.OutputIndex] = item
			case "response.function_call_arguments.done":
				item := toolCalls[event.OutputIndex]
				if event.CallID != "" {
					item.CallID = event.CallID
				}
				if event.Name != "" {
					item.Name = event.Name
				}
				if event.Arguments != "" {
					item.Arguments = event.Arguments
				}
				var args map[string]interface{}
				if item.Arguments != "" {
					_ = json.Unmarshal([]byte(item.Arguments), &args)
				}
				emittedToolCall = true
				if !sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{
					Type: "tool_call",
					ToolCall: &core.ToolCall{
						ID:        item.CallID,
						Name:      item.Name,
						Arguments: args,
					},
				}) {
					return
				}
			case "response.completed":
				var r responsesResponse
				if err := json.Unmarshal(event.Response, &r); err == nil && r.Usage != nil {
					if !sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "usage", Usage: &core.EyrieUsage{
						PromptTokens:     r.Usage.InputTokens,
						CompletionTokens: r.Usage.OutputTokens,
						TotalTokens:      r.Usage.TotalTokens,
					}}) {
						return
					}
				}
				stopReason := "stop"
				if emittedToolCall {
					stopReason = "tool_calls"
				}
				sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "done", StopReason: stopReason})
				return
			case "response.incomplete":
				var r responsesResponse
				_ = json.Unmarshal(event.Response, &r)
				if r.Usage != nil {
					if !sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "usage", Usage: &core.EyrieUsage{
						PromptTokens:     r.Usage.InputTokens,
						CompletionTokens: r.Usage.OutputTokens,
						TotalTokens:      r.Usage.TotalTokens,
					}}) {
						return
					}
				}
				sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "done", StopReason: concentrateFinishReason(r)})
				return
			case "response.failed":
				var r responsesResponse
				_ = json.Unmarshal(event.Response, &r)
				message := "response failed"
				if r.Error != nil && r.Error.Message != "" {
					message = r.Error.Message
				}
				sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "error", Error: message})
				return
			case "error":
				message := event.Message
				if message == "" {
					message = "stream error"
				}
				sendConcentrateStreamEvent(ctx, events, core.EyrieStreamEvent{Type: "error", Error: message})
				return
			}
		}
	}()

	return llm.NewStreamResult(events, requestID, cancel)
}

func sendConcentrateStreamEvent(ctx context.Context, events chan<- core.EyrieStreamEvent, event core.EyrieStreamEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// sseReader reads SSE events from a stream.
type sseReader struct {
	reader *bufio.Reader
}

func newSSEReader(r io.Reader) *sseReader {
	return &sseReader{reader: bufio.NewReader(r)}
}

func (s *sseReader) Read() (streamEvent, error) {
	var event streamEvent
	var data []string
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return event, err
			}
			if len(line) == 0 {
				if len(data) == 0 {
					return event, io.EOF
				}
				return decodeConcentrateSSEData(data)
			}
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" || err == io.EOF {
			if len(data) == 0 {
				if err == io.EOF {
					return event, io.EOF
				}
				continue
			}
			return decodeConcentrateSSEData(data)
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func decodeConcentrateSSEData(data []string) (streamEvent, error) {
	payload := strings.Join(data, "\n")
	if payload == "[DONE]" {
		return streamEvent{Type: "response.completed"}, nil
	}
	var event streamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return event, fmt.Errorf("concentrate: unmarshal stream event: %w", err)
	}
	return event, nil
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
	c.retry = rc
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
	return c.retry
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
