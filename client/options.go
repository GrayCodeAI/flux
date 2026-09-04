package client

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// ClientOption and the adapter-level With* constructors live in client/core
// (see core.Configurable); they are aliased/wrapped here so the public
// client.* API is unchanged. Only WithCoalescing is defined locally — it
// configures the GraycodeRouterClient itself, which lives in this package.

// ClientOption configures clients.
type ClientOption = core.ClientOption

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption { return core.WithTimeout(d) }

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption { return core.WithHTTPClient(hc) }

// WithRetry sets retry configuration.
func WithRetry(rc RetryConfig) ClientOption { return core.WithRetry(rc) }

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) ClientOption { return core.WithLogger(l) }

// WithAPIKey sets the API key.
func WithAPIKey(key string) ClientOption { return core.WithAPIKey(key) }

// WithBaseURL sets the base URL.
func WithBaseURL(url string) ClientOption { return core.WithBaseURL(url) }

// WithModel sets the default model for requests.
func WithModel(model string) ClientOption { return core.WithModel(model) }

// WithMaxTokens sets the default max tokens for requests.
func WithMaxTokens(n int) ClientOption { return core.WithMaxTokens(n) }

// WithTemperature sets the default temperature for requests.
func WithTemperature(t float64) ClientOption { return core.WithTemperature(t) }

// WithGuardrails attaches output guardrails to the client. Guardrails run
// after the LLM response but before returning to the caller. Blocked
// responses are replaced with an error; redacted responses have matches
// replaced with asterisks.
func WithGuardrails(rules ...GuardrailRule) ClientOption { return core.WithGuardrails(rules...) }

// WithGuardrailType attaches output guardrails using built-in rules for the
// specified types. For example, WithGuardrailType(GuardrailPII, GuardrailSecretLeak)
// enables PII redaction and secret leak blocking with default patterns.
func WithGuardrailType(types ...GuardrailType) ClientOption { return core.WithGuardrailType(types...) }

// WithProviderName sets the OpenAI client provider name for errors/logging.
// No-op for the Anthropic adapter, which reports a fixed provider name.
func WithProviderName(name string) ClientOption { return core.WithProviderName(name) }

// WithMimoAuth uses api-key header per MiMo documentation (OpenAI + Anthropic compat).
func WithMimoAuth() ClientOption { return core.WithMimoAuth() }

// WithCoalescing enables request coalescing for identical concurrent requests.
// When enabled, multiple goroutines sending identical requests (same provider,
// model, messages, temperature, max_tokens) will be deduplicated into a single
// API call, with the result broadcast to all waiters.
//
// The ttl parameter controls how long completed requests remain in the coalescer
// for potential reuse. A typical value is 100-500ms.
func WithCoalescing(ttl time.Duration) ClientOption {
	return core.NewGraycodeRouterOption(func(e core.GraycodeRouterConfigurable) { e.SetCoalescingTTL(ttl) })
}
