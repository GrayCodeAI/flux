package core

import (
	"log/slog"
	"net/http"
	"time"
)

// Configurable is the setter surface protocol adapters expose to the
// functional-options system. It decouples ClientOption from concrete adapter
// types so the adapters can live in their own package.
type Configurable interface {
	SetTimeout(d time.Duration)
	SetHTTPClient(hc *http.Client)
	SetRetry(rc RetryConfig)
	SetLogger(l *slog.Logger)
	SetAPIKey(key string)
	SetBaseURL(url string)
	SetDefaultModel(model string)
	SetDefaultMaxTokens(n int)
	SetDefaultTemperature(t float64)
	SetGuardrails(g *Guardrails)
	SetProviderName(name string)
	SetMimoAuth()
}

// EyrieConfigurable is the setter surface the top-level EyrieClient exposes
// for options that configure the universal client rather than an adapter.
type EyrieConfigurable interface {
	SetCoalescingTTL(ttl time.Duration)
}

// ClientOption configures clients. Options built with the constructors below
// apply to any Configurable adapter; options built with NewEyrieOption apply
// to the top-level EyrieClient.
type ClientOption struct {
	applyConfigurable func(Configurable)
	applyEyrie        func(EyrieConfigurable)
}

// NewOption builds a ClientOption from an adapter-level apply function.
func NewOption(fn func(Configurable)) ClientOption {
	return ClientOption{applyConfigurable: fn}
}

// NewEyrieOption builds a ClientOption from an EyrieClient-level apply function.
func NewEyrieOption(fn func(EyrieConfigurable)) ClientOption {
	return ClientOption{applyEyrie: fn}
}

// Apply runs the option against an adapter. No-op for EyrieClient-level options.
func (o ClientOption) Apply(c Configurable) {
	if o.applyConfigurable != nil {
		o.applyConfigurable(c)
	}
}

// ApplyEyrie runs the option against the top-level client. No-op for
// adapter-level options.
func (o ClientOption) ApplyEyrie(e EyrieConfigurable) {
	if o.applyEyrie != nil {
		o.applyEyrie(e)
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return NewOption(func(c Configurable) { c.SetTimeout(d) })
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return NewOption(func(c Configurable) { c.SetHTTPClient(hc) })
}

// WithRetry sets retry configuration.
func WithRetry(rc RetryConfig) ClientOption {
	return NewOption(func(c Configurable) { c.SetRetry(rc) })
}

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) ClientOption {
	return NewOption(func(c Configurable) { c.SetLogger(l) })
}

// WithAPIKey sets the API key.
func WithAPIKey(key string) ClientOption {
	return NewOption(func(c Configurable) { c.SetAPIKey(key) })
}

// WithBaseURL sets the base URL.
func WithBaseURL(url string) ClientOption {
	return NewOption(func(c Configurable) { c.SetBaseURL(url) })
}

// WithModel sets the default model for requests.
func WithModel(model string) ClientOption {
	return NewOption(func(c Configurable) { c.SetDefaultModel(model) })
}

// WithMaxTokens sets the default max tokens for requests.
func WithMaxTokens(n int) ClientOption {
	return NewOption(func(c Configurable) { c.SetDefaultMaxTokens(n) })
}

// WithTemperature sets the default temperature for requests.
func WithTemperature(t float64) ClientOption {
	return NewOption(func(c Configurable) { c.SetDefaultTemperature(t) })
}

// WithGuardrails attaches output guardrails to the client. Guardrails run
// after the LLM response but before returning to the caller. Blocked
// responses are replaced with an error; redacted responses have matches
// replaced with asterisks.
func WithGuardrails(rules ...GuardrailRule) ClientOption {
	g := NewGuardrails(rules...)
	return NewOption(func(c Configurable) { c.SetGuardrails(g) })
}

// WithGuardrailType attaches output guardrails using built-in rules for the
// specified types. For example, WithGuardrailType(GuardrailPII, GuardrailSecretLeak)
// enables PII redaction and secret leak blocking with default patterns.
func WithGuardrailType(types ...GuardrailType) ClientOption {
	var rules []GuardrailRule
	for _, t := range types {
		rules = append(rules, RulesForType(t)...)
	}
	return WithGuardrails(rules...)
}

// WithProviderName sets the OpenAI client provider name for errors/logging.
// No-op for the Anthropic adapter, which reports a fixed provider name.
func WithProviderName(name string) ClientOption {
	return NewOption(func(c Configurable) { c.SetProviderName(name) })
}

// WithMimoAuth uses api-key header per MiMo documentation (OpenAI + Anthropic compat).
func WithMimoAuth() ClientOption {
	return NewOption(func(c Configurable) { c.SetMimoAuth() })
}
