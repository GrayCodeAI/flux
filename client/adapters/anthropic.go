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

// maxAnthropicRequestSize is the maximum request body size for the Messages API (32 MB).
const maxAnthropicRequestSize = 32 * 1024 * 1024

// AnthropicClient implements core.Provider for the Anthropic Messages API.
type AnthropicClient struct {
	apiKey             string
	baseURL            string
	version            string
	httpClient         *http.Client
	retry              core.RetryConfig
	logger             *slog.Logger
	defaultModel       string
	defaultMaxTokens   int
	defaultTemperature *float64
	guardrails         *core.Guardrails
	useMimoAuth        bool
}

// Compile-time check that AnthropicClient implements core.Provider.
var _ core.Provider = (*AnthropicClient)(nil)

// NewAnthropicClient creates a configured Anthropic client.
func NewAnthropicClient(apiKey, baseURL string, opts ...core.ClientOption) *AnthropicClient {
	c := &AnthropicClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		version:    "2023-06-01",
		httpClient: core.NewPooledHTTPClient(core.DefaultTimeout),
		retry:      core.DefaultRetryConfig(),
		logger:     slog.Default(),
	}
	if c.baseURL == "" {
		c.baseURL = "https://api.anthropic.com"
	}
	for _, opt := range opts {
		opt.Apply(c)
	}
	return c
}

// Name returns the provider name.
func (c *AnthropicClient) Name() string { return "anthropic" }

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.useMimoAuth {
		req.Header.Set("api-key", c.apiKey)
	} else {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	req.Header.Set("Anthropic-Version", c.version)
	req.Header.Set("User-Agent", core.UserAgent())
}

type anthropicRequest struct {
	Model         string                   `json:"model"`
	MaxTokens     int                      `json:"max_tokens"`
	Messages      []map[string]interface{} `json:"messages"`
	System        string                   `json:"system,omitempty"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	TopP          *float64                 `json:"top_p,omitempty"`
	TopK          *int                     `json:"top_k,omitempty"`
	Stream        bool                     `json:"stream,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool          `json:"tools,omitempty"`
	ToolChoice    *AnthropicToolChoice     `json:"tool_choice,omitempty"`
	Thinking      *AnthropicThinking       `json:"thinking,omitempty"`
	Metadata      *AnthropicMetadata       `json:"metadata,omitempty"`
	ServiceTier   string                   `json:"service_tier,omitempty"`
	OutputConfig  *AnthropicOutputConfig   `json:"output_config,omitempty"`
}

type AnthropicRequest = anthropicRequest

type AnthropicResponse = anthropicResponse

// anthropicThinking enables Anthropic extended thinking.
// Type is "enabled" (with budget_tokens), "adaptive" (model decides), or "disabled".
type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"` // "summarized" or "omitted"
}

type AnthropicTool = anthropicTool

type AnthropicToolChoice = anthropicToolChoice

type AnthropicThinking = anthropicThinking

type AnthropicMetadata = anthropicMetadata

type AnthropicOutputConfig = anthropicOutputConfig

type AnthropicOutputFormat = anthropicOutputFormat

type anthropicToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

type anthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type anthropicOutputConfig struct {
	Effort string                 `json:"effort,omitempty"`
	Format *anthropicOutputFormat `json:"format,omitempty"`
}

type anthropicOutputFormat struct {
	Type   string                 `json:"type"`
	Schema map[string]interface{} `json:"schema,omitempty"`
}

func ResolveThinking(opts core.ChatOptions) *AnthropicThinking { return resolveThinking(opts) }

func ResolveToolChoice(tc *core.ToolChoiceOption) *AnthropicToolChoice { return resolveToolChoice(tc) }

func ResolveOutputConfig(opts core.ChatOptions) *AnthropicOutputConfig {
	return resolveOutputConfig(opts)
}

func AudioFormatToMediaType(format string) string { return audioFormatToMediaType(format) }

// thinkingForBudget returns an enabled thinking config when budget > 0, else nil.
func thinkingForBudget(budget int) *anthropicThinking {
	if budget <= 0 {
		return nil
	}
	return &anthropicThinking{Type: "enabled", BudgetTokens: budget}
}

// thinkingAdaptive returns an adaptive thinking config.
func thinkingAdaptive() *anthropicThinking {
	return &anthropicThinking{Type: "adaptive"}
}

// thinkingDisabled returns a disabled thinking config.
func thinkingDisabled() *anthropicThinking {
	return &anthropicThinking{Type: "disabled"}
}

// resolveThinking builds the thinking config from core.ChatOptions.
func resolveThinking(opts core.ChatOptions) *anthropicThinking {
	switch opts.ThinkingMode {
	case "adaptive":
		return thinkingAdaptive()
	case "disabled":
		return thinkingDisabled()
	case "enabled":
		// The legacy type:"enabled" with a fixed budget_tokens is deprecated on
		// Claude 4.6 and rejected on Claude 4.7+ (docs recommend migrating to
		// adaptive). Use adaptive so an explicit "enabled" request stays
		// compatible across model generations while keeping thinking on. A
		// caller-provided fixed budget is still honored via the legacy no-mode
		// path below (thinkingForBudget).
		return thinkingAdaptive()
	default:
		// Legacy behavior: ThinkingEnabled toggle wins, else budget > 0 enables with budget.
		if opts.ThinkingEnabled != nil {
			if *opts.ThinkingEnabled {
				return thinkingAdaptive()
			}
			return thinkingDisabled()
		}
		thinking := thinkingForBudget(opts.ThinkingBudgetTokens)
		if thinking != nil && opts.ThinkingDisplay != "" {
			thinking.Display = opts.ThinkingDisplay
		}
		return thinking
	}
}

// resolveToolChoice converts core.ChatOptions.ToolChoice to wire format.
func resolveToolChoice(tc *core.ToolChoiceOption) *anthropicToolChoice {
	if tc == nil {
		return nil
	}
	return &anthropicToolChoice{
		Type:                   tc.Type,
		Name:                   tc.Name,
		DisableParallelToolUse: tc.DisableParallelToolUse,
	}
}

// resolveMetadata builds metadata from core.ChatOptions.
func resolveMetadata(opts core.ChatOptions) *anthropicMetadata {
	if opts.MetadataUserID == "" {
		return nil
	}
	return &anthropicMetadata{UserID: opts.MetadataUserID}
}

// resolveOutputConfig builds output config from core.ChatOptions.
func resolveOutputConfig(opts core.ChatOptions) *anthropicOutputConfig {
	if opts.OutputEffort == "" && opts.OutputSchema == "" {
		return nil
	}
	cfg := &anthropicOutputConfig{Effort: opts.OutputEffort}
	if opts.OutputSchema != "" {
		var schema map[string]interface{}
		if json.Unmarshal([]byte(opts.OutputSchema), &schema) == nil {
			cfg.Format = &anthropicOutputFormat{Type: "json_schema", Schema: schema}
		}
	}
	return cfg
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Content []struct {
		Type      string          `json:"type"`
		Text      string          `json:"text,omitempty"`
		Thinking  string          `json:"thinking,omitempty"`  // thinking block content
		Signature string          `json:"signature,omitempty"` // thinking signature for multi-turn
		Data      string          `json:"data,omitempty"`      // redacted_thinking data
		ID        string          `json:"id,omitempty"`
		Name      string          `json:"name,omitempty"`
		Input     json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		OutputTokensDetails      struct {
			ThinkingTokens int `json:"thinking_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

// ParseAnthropicResponse converts a parsed Anthropic Messages API
// response into an core.GraycodeRouterResponse. Shared by Anthropic, Bedrock, and
// Vertex clients (all three receive the same wire format).
//
// Content blocks are extracted per type:
//   - "text" → Content (concatenated)
//   - "thinking" → Thinking (concatenated)
//   - "redacted_thinking" → skipped silently (safety-sensitive reasoning)
//   - "tool_use" → appended to ToolCalls with parsed Arguments
//
// requestID is required. orgID is the Anthropic-Organization-Id
// response header (Anthropic-specific; Bedrock and Vertex pass "").
func ParseAnthropicResponse(ar anthropicResponse, requestID, orgID string) *core.GraycodeRouterResponse {
	var content, thinkingContent string
	var toolCalls []core.ToolCall
	for _, block := range ar.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "thinking":
			thinkingContent += block.Thinking
		case "redacted_thinking":
			// Safety-sensitive reasoning — skip silently
			continue
		case "tool_use":
			var args map[string]interface{}
			if err := json.Unmarshal(block.Input, &args); err != nil {
				slog.Warn("anthropic: failed to parse tool_use input", "error", err, "tool_name", block.Name)
			}
			toolCalls = append(toolCalls, core.ToolCall{ID: block.ID, Name: block.Name, Arguments: args})
		}
	}
	return &core.GraycodeRouterResponse{
		Content: content, Thinking: thinkingContent, FinishReason: ar.StopReason, ToolCalls: toolCalls,
		RequestID: requestID, OrganizationID: orgID,
		Usage: &core.GraycodeRouterUsage{
			PromptTokens:        ar.Usage.InputTokens,
			CompletionTokens:    ar.Usage.OutputTokens,
			TotalTokens:         ar.Usage.InputTokens + ar.Usage.OutputTokens,
			CacheCreationTokens: ar.Usage.CacheCreationInputTokens,
			CacheReadTokens:     ar.Usage.CacheReadInputTokens,
			ThinkingTokens:      ar.Usage.OutputTokensDetails.ThinkingTokens,
		},
	}
}

func BuildAnthropicMessages(messages []core.GraycodeRouterMessage) ([]map[string]interface{}, string) {
	return buildAnthropicMessages(messages)
}

func buildAnthropicMessages(messages []core.GraycodeRouterMessage) ([]map[string]interface{}, string) {
	var system string
	msgs := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			system = m.Content
			continue
		}
		// Assistant message with tool_use blocks
		if m.Role == "assistant" && len(m.ToolUse) > 0 {
			content := make([]map[string]interface{}, 0)
			if m.Content != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolUse {
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Arguments,
				})
			}
			msgs = append(msgs, map[string]interface{}{"role": "assistant", "content": content})
			continue
		}
		// User message with tool results
		if m.Role == "user" && len(m.ToolResults) > 0 {
			content := make([]map[string]interface{}, 0, len(m.ToolResults))
			for _, tr := range m.ToolResults {
				block := map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": tr.ToolUseID,
					"content":     tr.Content,
				}
				if tr.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
			msgs = append(msgs, map[string]interface{}{"role": "user", "content": content})
			continue
		}
		// Handle ContentParts (multi-modal): takes precedence over Content/Images
		if len(m.ContentParts) > 0 {
			content := make([]map[string]interface{}, 0, len(m.ContentParts))
			for _, part := range m.ContentParts {
				switch part.Type {
				case "text":
					content = append(content, map[string]interface{}{"type": "text", "text": part.Text})
				case "image_url":
					if part.ImageURL != nil {
						mediaType, data, isBase64 := core.ParseImageString(part.ImageURL.URL)
						if isBase64 {
							content = append(content, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type":       "base64",
									"media_type": mediaType,
									"data":       data,
								},
							})
						} else {
							content = append(content, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type": "url",
									"url":  part.ImageURL.URL,
								},
							})
						}
					}
				case "input_audio":
					if part.InputAudio != nil {
						// Anthropic expects audio as an "audio" content block with base64 source
						mediaType := audioFormatToMediaType(part.InputAudio.Format)
						content = append(content, map[string]interface{}{
							"type": "audio",
							"source": map[string]interface{}{
								"type":       "base64",
								"media_type": mediaType,
								"data":       part.InputAudio.Data,
							},
						})
					}
				}
			}
			msgs = append(msgs, map[string]interface{}{"role": m.Role, "content": content})
			continue
		}
		// Handle legacy Images field: build multi-part content array
		if len(m.Images) > 0 {
			content := make([]map[string]interface{}, 0, 1+len(m.Images))
			if m.Content != "" {
				content = append(content, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, img := range m.Images {
				mediaType, data, isBase64 := core.ParseImageString(img)
				if isBase64 {
					content = append(content, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": mediaType,
							"data":       data,
						},
					})
				} else {
					// URL-based image
					content = append(content, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type": "url",
							"url":  img,
						},
					})
				}
			}
			msgs = append(msgs, map[string]interface{}{"role": m.Role, "content": content})
			continue
		}
		msgs = append(msgs, map[string]interface{}{"role": m.Role, "content": m.Content})
	}
	return msgs, system
}

// audioFormatToMediaType converts a short audio format string to a full MIME type.
func audioFormatToMediaType(format string) string {
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "webm":
		return "audio/webm"
	default:
		// If it looks like a full MIME type already, pass through
		if strings.Contains(format, "/") {
			return format
		}
		return "audio/" + format
	}
}

// Chat sends a non-streaming message to Anthropic.
// buildAnthropicRequest constructs the request body and http.Request
// for both Chat and StreamChat. If stream is true, the body sets
// `stream: true` and the request gets the `Accept: text/event-stream`
// header. Returns the http.Request (with GetBody set for retry) and
// the raw body bytes (needed by doRequestWithMimoAuthRetry for the
// MiMo 401 retry path).
//
// This helper removes ~120 lines of duplication between Chat and
// StreamChat (lines 375-446 and 496-565 in the previous version):
// every field — System, Temperature, TopP, TopK, StopSequences,
// EnableCaching, tools, thinking, metadata, serviceTier,
// outputConfig — was previously re-applied in both methods.
func (c *AnthropicClient) buildAnthropicRequest(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions, stream bool) (*http.Request, []byte, error) {
	if opts.ResponseFormat != nil {
		switch opts.ResponseFormat.Type {
		case "json_schema":
			if opts.ResponseFormat.Schema == "" {
				return nil, nil, fmt.Errorf("graycode-router: anthropic json_schema response format requires a schema")
			}
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(opts.ResponseFormat.Schema), &schema); err != nil {
				return nil, nil, fmt.Errorf("graycode-router: invalid anthropic response schema: %w", err)
			}
			opts.OutputSchema = opts.ResponseFormat.Schema
		case "json_object":
			return nil, nil, fmt.Errorf("graycode-router: anthropic does not support json_object without a schema; use json_schema")
		default:
			return nil, nil, fmt.Errorf("graycode-router: unsupported anthropic response format %q", opts.ResponseFormat.Type)
		}
	}
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, nil, fmt.Errorf("graycode-router: model is required for anthropic")
	}
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	thinking := resolveThinking(opts)

	var body []byte
	if opts.EnableCaching {
		allMessages := messages
		if opts.System != "" {
			allMessages = append([]core.GraycodeRouterMessage{{Role: "system", Content: opts.System}}, allMessages...)
		}
		tools := ConvertToAnthropicTools(opts.Tools)
		cachedReq := buildAnthropicCachedRequest(allMessages, opts.Model, maxTokens, opts.Temperature, stream, tools,
			thinking, resolveToolChoice(opts.ToolChoice), opts.TopP, opts.TopK, opts.StopSequences)
		var err error
		body, err = json.Marshal(cachedReq)
		if err != nil {
			return nil, nil, fmt.Errorf("graycode-router: marshal anthropic cached request: %w", err)
		}
	} else {
		msgs, system := buildAnthropicMessages(messages)
		if opts.System != "" {
			if system != "" {
				system = opts.System + "\n\n" + system
			} else {
				system = opts.System
			}
		}
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
		var err error
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, nil, fmt.Errorf("graycode-router: marshal anthropic request: %w", err)
		}
	}

	// Check request size (32 MB limit for Messages API)
	if len(body) > maxAnthropicRequestSize {
		return nil, nil, fmt.Errorf("graycode-router: request size %d bytes exceeds Anthropic limit of %d bytes", len(body), maxAnthropicRequestSize)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("graycode-router: failed to create request: %w", err)
	}
	c.setHeaders(req)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	return req, body, nil
}

// Anthropic structured output is sent through output_config.format. A
// schema-less json_object request is rejected instead of being silently
// ignored because Anthropic requires a JSON schema.
func (c *AnthropicClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	req, body, err := c.buildAnthropicRequest(ctx, messages, opts, false)
	if err != nil {
		return nil, err
	}
	c.logger.Debug("anthropic chat", "model", opts.Model, "caching", opts.EnableCaching)

	resp, err := c.doRequestWithMimoAuthRetry(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: anthropic request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("anthropic: close response body", "error", err)
		}
	}()

	requestID := resp.Header.Get("Request-Id")
	orgID := resp.Header.Get("Anthropic-Organization-Id")

	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("anthropic", "chat", resp.StatusCode, requestID, detail, readErr)
	}

	var ar anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("graycode-router: failed to decode anthropic response: %w", err)
	}

	graycodeRouterResp := ParseAnthropicResponse(ar, requestID, orgID)

	if err := core.ApplyGuardrails(ctx, graycodeRouterResp, c.guardrails); err != nil {
		return nil, err
	}

	return graycodeRouterResp, nil
}

// StreamChat sends a streaming message to Anthropic.
func (c *AnthropicClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	req, body, err := c.buildAnthropicRequest(ctx, messages, opts, true)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequestWithMimoAuthRetry(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: anthropic stream request failed: %w", err)
	}

	requestID := resp.Header.Get("Request-Id")

	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		_ = resp.Body.Close()
		return nil, core.FormatAPIError("anthropic", "stream", resp.StatusCode, requestID, detail, readErr)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	sseEvents := core.ParseSSEStream(streamCtx, resp.Body, c.logger)
	events := core.ProcessAnthropicStream(streamCtx, sseEvents, c.logger)

	return llm.NewStreamResult(events, requestID, cancel), nil
}

func (c *AnthropicClient) doRequestWithMimoAuthRetry(ctx context.Context, req *http.Request, body []byte) (*http.Response, error) {
	return doWithMimoAuthRetry(ctx, c.httpClient, c.retry, c.logger, c.useMimoAuth, req, body, func(req2 *http.Request) {
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+c.apiKey)
		req2.Header.Set("Anthropic-Version", c.version)
		req2.Header.Set("User-Agent", core.UserAgent())
	})
}

// Ping checks connectivity to the Anthropic API using a lightweight GET request.
func (c *AnthropicClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("graycode-router: anthropic ping failed: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graycode-router: anthropic ping failed: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("graycode-router: anthropic: invalid API key")
	}
	return nil
}

func ConvertToAnthropicTools(tools []core.GraycodeRouterTool) []anthropicTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicTool, len(tools))
	for i, t := range tools {
		out[i] = anthropicTool{Name: t.Name, Description: t.Description, InputSchema: t.Parameters}
	}
	return out
}

// TokenCountResult holds the result of a token count request.
type TokenCountResult struct {
	InputTokens int `json:"input_tokens"`
}

// CountTokens counts the number of tokens in a message without generating a response.
// Uses the same request format as Chat but hits the /v1/messages/count_tokens endpoint.
// The request body includes model, messages, system, and tools (but not max_tokens or stream).
func (c *AnthropicClient) CountTokens(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*TokenCountResult, error) {
	messages = core.SanitizeMessages(messages)
	if opts.Model == "" {
		return nil, fmt.Errorf("graycode-router: model is required for anthropic count_tokens")
	}

	msgs, system := buildAnthropicMessages(messages)
	if opts.System != "" {
		if system != "" {
			system = opts.System + "\n\n" + system
		} else {
			system = opts.System
		}
	}

	// Build a minimal request for token counting (no max_tokens, no stream)
	reqBody := map[string]interface{}{
		"model":    opts.Model,
		"messages": msgs,
	}
	if system != "" {
		reqBody["system"] = system
	}
	if len(opts.Tools) > 0 {
		reqBody["tools"] = ConvertToAnthropicTools(opts.Tools)
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: failed to marshal count_tokens request: %w", err)
	}

	// Check request size (32 MB limit for Messages API)
	if len(body) > maxAnthropicRequestSize {
		return nil, fmt.Errorf("graycode-router: request size %d bytes exceeds Anthropic limit of %d bytes", len(body), maxAnthropicRequestSize)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages/count_tokens", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("graycode-router: failed to create count_tokens request: %w", err)
	}
	c.setHeaders(req)
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	resp, err := c.doRequestWithMimoAuthRetry(ctx, req, body)
	if err != nil {
		return nil, fmt.Errorf("graycode-router: anthropic count_tokens failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("anthropic: close response body", "error", err)
		}
	}()

	if resp.StatusCode != 200 {
		detail, readErr := core.ParseProviderError(resp.Body)
		return nil, core.FormatAPIError("anthropic", "count_tokens", resp.StatusCode, resp.Header.Get("Request-Id"), detail, readErr)
	}

	var result TokenCountResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("graycode-router: failed to decode count_tokens response: %w", err)
	}
	return &result, nil
}

// BuildAnthropicRequest constructs a complete Anthropic Messages request.
func (c *AnthropicClient) BuildAnthropicRequest(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions, stream bool) (*http.Request, []byte, error) {
	return c.buildAnthropicRequest(ctx, messages, opts, stream)
}

// ThinkingForBudget builds the wire-level extended-thinking configuration.
func ThinkingForBudget(budget int) *anthropicThinking {
	return thinkingForBudget(budget)
}
