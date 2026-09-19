package provider

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GrayCodeAI/flux/provider/core"
)

// ClientOption and the adapter-level With* constructors live in provider/core
// (see core.Configurable). Only WithCoalescing is defined locally — it
// configures the FluxClient itself, which lives in this package.

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) core.ClientOption { return core.WithTimeout(d) }

// WithHTTPClient sets a custom HTTP provider.
func WithHTTPClient(hc *http.Client) core.ClientOption { return core.WithHTTPClient(hc) }

// WithRetry sets retry configuration.
func WithRetry(rc core.RetryConfig) core.ClientOption { return core.WithRetry(rc) }

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) core.ClientOption { return core.WithLogger(l) }

// WithAPIKey sets the API key.
func WithAPIKey(key string) core.ClientOption { return core.WithAPIKey(key) }

// WithBaseURL sets the base URL.
func WithBaseURL(url string) core.ClientOption { return core.WithBaseURL(url) }

// WithModel sets the default model for requests.
func WithModel(model string) core.ClientOption { return core.WithModel(model) }

// WithMaxTokens sets the default max tokens for requests.
func WithMaxTokens(n int) core.ClientOption { return core.WithMaxTokens(n) }

// WithTemperature sets the default temperature for requests.
func WithTemperature(t float64) core.ClientOption { return core.WithTemperature(t) }

// WithGuardrails attaches output guardrails to the provider. core.Guardrails run
// after the LLM response but before returning to the caller. Blocked
// responses are replaced with an error; redacted responses have matches
// replaced with asterisks.
func WithGuardrails(rules ...core.GuardrailRule) core.ClientOption {
	return core.WithGuardrails(rules...)
}

// WithGuardrailType attaches output guardrails using built-in rules for the
// specified types. For example, WithGuardrailType(GuardrailPII, GuardrailSecretLeak)
// enables PII redaction and secret leak blocking with default patterns.
func WithGuardrailType(types ...core.GuardrailType) core.ClientOption {
	return core.WithGuardrailType(types...)
}

// WithProviderName sets the OpenAI client provider name for errors/logging.
// No-op for the Anthropic adapter, which reports a fixed provider name.
func WithProviderName(name string) core.ClientOption { return core.WithProviderName(name) }

// WithMimoAuth uses api-key header per MiMo documentation (OpenAI + Anthropic compat).
func WithMimoAuth() core.ClientOption { return core.WithMimoAuth() }

// WithCoalescing enables request coalescing for identical concurrent requests.
// When enabled, multiple goroutines sending identical requests (same provider,
// model, messages, temperature, max_tokens) will be deduplicated into a single
// API call, with the result broadcast to all waiters.
//
// The ttl parameter controls how long completed requests remain in the coalescer
// for potential reuse. A typical value is 100-500ms.
func WithCoalescing(ttl time.Duration) core.ClientOption {
	return core.NewFluxOption(func(e core.FluxConfigurable) { e.SetCoalescingTTL(ttl) })
}
