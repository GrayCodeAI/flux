package adapters

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// Functional-option setter surface for the two protocol adapters (see
// core.Configurable in options.go). Exported so the implementations keep
// satisfying the interface when the adapters move to their own package.

var (
	_ core.Configurable = (*AnthropicClient)(nil)
	_ core.Configurable = (*OpenAIClient)(nil)
)

// SetTimeout sets the HTTP client timeout.
func (c *AnthropicClient) SetTimeout(d time.Duration) { c.httpClient.Timeout = d }

// SetHTTPClient replaces the HTTP client.
func (c *AnthropicClient) SetHTTPClient(hc *http.Client) { c.httpClient = hc }

// SetRetry sets the retry configuration.
func (c *AnthropicClient) SetRetry(rc core.RetryConfig) { c.retry = rc }

// SetLogger sets the logger.
func (c *AnthropicClient) SetLogger(l *slog.Logger) { c.logger = l }

// SetAPIKey sets the API key.
func (c *AnthropicClient) SetAPIKey(key string) { c.apiKey = key }

// SetBaseURL sets the base URL.
func (c *AnthropicClient) SetBaseURL(url string) { c.baseURL = url }

// SetDefaultModel sets the default model for requests.
func (c *AnthropicClient) SetDefaultModel(model string) { c.defaultModel = model }

// SetDefaultMaxTokens sets the default max tokens for requests.
func (c *AnthropicClient) SetDefaultMaxTokens(n int) { c.defaultMaxTokens = n }

// SetDefaultTemperature sets the default temperature for requests.
func (c *AnthropicClient) SetDefaultTemperature(t float64) { c.defaultTemperature = &t }

// SetGuardrails attaches output guardrails.
func (c *AnthropicClient) SetGuardrails(g *core.Guardrails) { c.guardrails = g }

// SetProviderName is a no-op: the Anthropic adapter reports a fixed provider
// name. Present to satisfy core.Configurable; core.WithProviderName only affects
// OpenAI-compatible adapters.
func (c *AnthropicClient) SetProviderName(string) {}

// SetMimoAuth switches authentication to MiMo's api-key header.
func (c *AnthropicClient) SetMimoAuth() { c.useMimoAuth = true }

// Inspection methods expose non-secret adapter configuration.

func (c *AnthropicClient) BaseURL() string              { return c.baseURL }
func (c *AnthropicClient) HTTPClient() *http.Client     { return c.httpClient }
func (c *AnthropicClient) Retry() core.RetryConfig      { return c.retry }
func (c *AnthropicClient) Logger() *slog.Logger         { return c.logger }
func (c *AnthropicClient) Guardrails() *core.Guardrails { return c.guardrails }
func (c *AnthropicClient) DefaultModel() string         { return c.defaultModel }
func (c *AnthropicClient) DefaultMaxTokens() int        { return c.defaultMaxTokens }
func (c *AnthropicClient) DefaultTemperature() *float64 { return c.defaultTemperature }
func (c *AnthropicClient) Version() string              { return c.version }

// SetTimeout sets the HTTP client timeout.
func (c *OpenAIClient) SetTimeout(d time.Duration) { c.httpClient.Timeout = d }

// SetHTTPClient replaces the HTTP client.
func (c *OpenAIClient) SetHTTPClient(hc *http.Client) { c.httpClient = hc }

// SetRetry sets the retry configuration.
func (c *OpenAIClient) SetRetry(rc core.RetryConfig) { c.retry = rc }

// SetLogger sets the logger.
func (c *OpenAIClient) SetLogger(l *slog.Logger) { c.logger = l }

// SetAPIKey sets the API key.
func (c *OpenAIClient) SetAPIKey(key string) { c.apiKey = key }

// SetBaseURL sets the base URL.
func (c *OpenAIClient) SetBaseURL(url string) { c.baseURL = url }

// Inspection methods expose non-secret adapter configuration.
func (c *OpenAIClient) BaseURL() string              { return c.baseURL }
func (c *OpenAIClient) HTTPClient() *http.Client     { return c.httpClient }
func (c *OpenAIClient) Retry() core.RetryConfig      { return c.retry }
func (c *OpenAIClient) ProviderName() string         { return c.providerName }
func (c *OpenAIClient) Logger() *slog.Logger         { return c.logger }
func (c *OpenAIClient) Guardrails() *core.Guardrails { return c.guardrails }
func (c *OpenAIClient) DefaultModel() string         { return c.defaultModel }
func (c *OpenAIClient) DefaultMaxTokens() int        { return c.defaultMaxTokens }
func (c *OpenAIClient) DefaultTemperature() *float64 { return c.defaultTemperature }
func (c *OpenAIClient) Compat() *OpenAICompatConfig  { return c.compat }

// SetDefaultModel sets the default model for requests.
func (c *OpenAIClient) SetDefaultModel(model string) { c.defaultModel = model }

// SetDefaultMaxTokens sets the default max tokens for requests.
func (c *OpenAIClient) SetDefaultMaxTokens(n int) { c.defaultMaxTokens = n }

// SetDefaultTemperature sets the default temperature for requests.
func (c *OpenAIClient) SetDefaultTemperature(t float64) { c.defaultTemperature = &t }

// SetGuardrails attaches output guardrails.
func (c *OpenAIClient) SetGuardrails(g *core.Guardrails) { c.guardrails = g }

// SetProviderName sets the provider name used in errors and logging.
func (c *OpenAIClient) SetProviderName(name string) { c.providerName = name }

// SetMimoAuth switches authentication to MiMo's api-key header.
func (c *OpenAIClient) SetMimoAuth() { c.useMimoAuth = true }

// Bedrock transport configuration.

func (c *BedrockClient) SetHTTPClient(hc *http.Client) { c.httpClient = hc }
func (c *BedrockClient) SetRetry(rc core.RetryConfig)  { c.retry = rc }
