package adapters

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"
	"github.com/GrayCodeAI/graycode-router/types"
)

type (
	ClientOption = core.ClientOption
	RetryConfig  = core.RetryConfig
)

const defaultTimeout = core.DefaultTimeout

var (
	WithAPIKey       = core.WithAPIKey
	WithBaseURL      = core.WithBaseURL
	WithHTTPClient   = core.WithHTTPClient
	WithLogger       = core.WithLogger
	WithMaxTokens    = core.WithMaxTokens
	WithModel        = core.WithModel
	WithProviderName = core.WithProviderName
	WithRetry        = core.WithRetry
	WithTemperature  = core.WithTemperature
	WithTimeout      = core.WithTimeout
)

// --- Individual option tests ---

func TestWithAPIKeyAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("initial-key", "", WithAPIKey("new-key"))
	if c.apiKey != "new-key" {
		t.Errorf("expected apiKey 'new-key', got %q", c.apiKey)
	}
}

func TestWithAPIKeyOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("initial-key", "", nil, WithAPIKey("new-key"))
	if c.apiKey != "new-key" {
		t.Errorf("expected apiKey 'new-key', got %q", c.apiKey)
	}
}

func TestWithBaseURLAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "", WithBaseURL("https://custom.example.com"))
	if c.baseURL != "https://custom.example.com" {
		t.Errorf("expected baseURL 'https://custom.example.com', got %q", c.baseURL)
	}
}

func TestWithBaseURLOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil, WithBaseURL("https://custom.example.com/v1"))
	if c.baseURL != "https://custom.example.com/v1" {
		t.Errorf("expected baseURL 'https://custom.example.com/v1', got %q", c.baseURL)
	}
}

func TestWithModelAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "", WithModel("claude-sonnet-4-6"))
	if c.defaultModel != "claude-sonnet-4-6" {
		t.Errorf("expected defaultModel 'claude-sonnet-4-6', got %q", c.defaultModel)
	}
}

func TestWithModelOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil, WithModel("gpt-4o"))
	if c.defaultModel != "gpt-4o" {
		t.Errorf("expected defaultModel 'gpt-4o', got %q", c.defaultModel)
	}
}

func TestWithMaxTokensAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "", WithMaxTokens(4096))
	if c.defaultMaxTokens != 4096 {
		t.Errorf("expected defaultMaxTokens 4096, got %d", c.defaultMaxTokens)
	}
}

func TestWithMaxTokensOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil, WithMaxTokens(2048))
	if c.defaultMaxTokens != 2048 {
		t.Errorf("expected defaultMaxTokens 2048, got %d", c.defaultMaxTokens)
	}
}

func TestWithTemperatureAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "", WithTemperature(0.7))
	if c.defaultTemperature == nil {
		t.Fatal("expected defaultTemperature to be set, got nil")
	}
	if *c.defaultTemperature != 0.7 {
		t.Errorf("expected defaultTemperature 0.7, got %f", *c.defaultTemperature)
	}
}

func TestWithTemperatureOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil, WithTemperature(0.3))
	if c.defaultTemperature == nil {
		t.Fatal("expected defaultTemperature to be set, got nil")
	}
	if *c.defaultTemperature != 0.3 {
		t.Errorf("expected defaultTemperature 0.3, got %f", *c.defaultTemperature)
	}
}

func TestWithRetryAnthropic(t *testing.T) {
	t.Parallel()
	rc := RetryConfig{
		RetryConfig: types.RetryConfig{MaxRetries: 5, BaseDelay: time.Second, MaxDelay: time.Minute},
		RetryOn:     []int{429, 500},
	}
	c := NewAnthropicClient("key", "", WithRetry(rc))
	if c.retry.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", c.retry.MaxRetries)
	}
	if c.retry.BaseDelay != time.Second {
		t.Errorf("expected BaseDelay 1s, got %v", c.retry.BaseDelay)
	}
	if c.retry.MaxDelay != time.Minute {
		t.Errorf("expected MaxDelay 1m, got %v", c.retry.MaxDelay)
	}
	if len(c.retry.RetryOn) != 2 || c.retry.RetryOn[0] != 429 || c.retry.RetryOn[1] != 500 {
		t.Errorf("expected RetryOn [429 500], got %v", c.retry.RetryOn)
	}
}

func TestWithRetryOpenAI(t *testing.T) {
	t.Parallel()
	rc := RetryConfig{
		RetryConfig: types.RetryConfig{MaxRetries: 2, BaseDelay: 200 * time.Millisecond, MaxDelay: 10 * time.Second},
		RetryOn:     []int{503},
	}
	c := NewOpenAIClient("key", "", nil, WithRetry(rc))
	if c.retry.MaxRetries != 2 {
		t.Errorf("expected MaxRetries 2, got %d", c.retry.MaxRetries)
	}
	if c.retry.BaseDelay != 200*time.Millisecond {
		t.Errorf("expected BaseDelay 200ms, got %v", c.retry.BaseDelay)
	}
	if len(c.retry.RetryOn) != 1 || c.retry.RetryOn[0] != 503 {
		t.Errorf("expected RetryOn [503], got %v", c.retry.RetryOn)
	}
}

func TestWithTimeoutOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil, WithTimeout(15*time.Second))
	if c.httpClient.Timeout != 15*time.Second {
		t.Errorf("expected timeout 15s, got %v", c.httpClient.Timeout)
	}
}

func TestWithHTTPClientAnthropic(t *testing.T) {
	t.Parallel()
	hc := &http.Client{Timeout: 45 * time.Second}
	c := NewAnthropicClient("key", "", WithHTTPClient(hc))
	if c.httpClient != hc {
		t.Error("expected custom HTTP client to be set")
	}
	if c.httpClient.Timeout != 45*time.Second {
		t.Errorf("expected timeout 45s, got %v", c.httpClient.Timeout)
	}
}

func TestWithLoggerOpenAI(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	c := NewOpenAIClient("key", "", nil, WithLogger(logger))
	if c.logger != logger {
		t.Error("expected custom logger to be set")
	}
}

// --- Option application order tests ---

func TestOptionApplicationOrderAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient(
		"key", "",
		WithAPIKey("first"),
		WithAPIKey("second"),
		WithBaseURL("https://first.example.com"),
		WithBaseURL("https://second.example.com"),
	)
	if c.apiKey != "second" {
		t.Errorf("expected last apiKey wins, got %q", c.apiKey)
	}
	if c.baseURL != "https://second.example.com" {
		t.Errorf("expected last baseURL wins, got %q", c.baseURL)
	}
}

func TestOptionApplicationOrderOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient(
		"key", "", nil,
		WithAPIKey("first"),
		WithAPIKey("second"),
		WithBaseURL("https://first.example.com"),
		WithBaseURL("https://second.example.com"),
	)
	if c.apiKey != "second" {
		t.Errorf("expected last apiKey wins, got %q", c.apiKey)
	}
	if c.baseURL != "https://second.example.com" {
		t.Errorf("expected last baseURL wins, got %q", c.baseURL)
	}
}

func TestOptionOrderModelMaxTokensTemperature(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient(
		"key", "",
		WithModel("first-model"),
		WithMaxTokens(100),
		WithTemperature(0.1),
		WithModel("final-model"),
		WithMaxTokens(9999),
		WithTemperature(0.9),
	)
	if c.defaultModel != "final-model" {
		t.Errorf("expected last model wins, got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 9999 {
		t.Errorf("expected last maxTokens wins, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature == nil || *c.defaultTemperature != 0.9 {
		t.Errorf("expected last temperature 0.9, got %v", c.defaultTemperature)
	}
}

// --- Default values tests ---

func TestAnthropicDefaultValues(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	if c.apiKey != "key" {
		t.Errorf("expected apiKey 'key', got %q", c.apiKey)
	}
	if c.baseURL != "https://api.anthropic.com" {
		t.Errorf("expected default baseURL 'https://api.anthropic.com', got %q", c.baseURL)
	}
	if c.version != "2023-06-01" {
		t.Errorf("expected version '2023-06-01', got %q", c.version)
	}
	if c.httpClient == nil {
		t.Error("expected default httpClient to be set")
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultTimeout, c.httpClient.Timeout)
	}
	if c.logger == nil {
		t.Error("expected default logger to be set")
	}
	if c.defaultModel != "" {
		t.Errorf("expected empty defaultModel, got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 0 {
		t.Errorf("expected zero defaultMaxTokens, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature != nil {
		t.Errorf("expected nil defaultTemperature, got %v", c.defaultTemperature)
	}
}

func TestOpenAIDefaultValues(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	if c.apiKey != "key" {
		t.Errorf("expected apiKey 'key', got %q", c.apiKey)
	}
	if c.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default baseURL 'https://api.openai.com/v1', got %q", c.baseURL)
	}
	if c.httpClient == nil {
		t.Error("expected default httpClient to be set")
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultTimeout, c.httpClient.Timeout)
	}
	if c.logger == nil {
		t.Error("expected default logger to be set")
	}
	if c.compat == nil {
		t.Error("expected default compat to be set")
	}
	if c.defaultModel != "" {
		t.Errorf("expected empty defaultModel, got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 0 {
		t.Errorf("expected zero defaultMaxTokens, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature != nil {
		t.Errorf("expected nil defaultTemperature, got %v", c.defaultTemperature)
	}
}

// --- Provider-specific option tests ---

func TestOptionsApplyToBothAdapters(t *testing.T) {
	t.Parallel()
	// Options built through the clientConfigurable interface reach both
	// adapter types with a single constructor.
	opt := WithAPIKey("shared-key")
	a := NewAnthropicClient("key", "", opt)
	if a.apiKey != "shared-key" {
		t.Errorf("anthropic: expected apiKey 'shared-key', got %q", a.apiKey)
	}
	o := NewOpenAIClient("key", "", nil, opt)
	if o.apiKey != "shared-key" {
		t.Errorf("openai: expected apiKey 'shared-key', got %q", o.apiKey)
	}
}

func TestProviderNameOptionIsOpenAIOnly(t *testing.T) {
	t.Parallel()
	// WithProviderName mutates the OpenAI adapter and is a documented no-op
	// on the Anthropic adapter (fixed provider name).
	opt := WithProviderName("custom")
	o := NewOpenAIClient("key", "", nil, opt)
	if o.providerName != "custom" {
		t.Errorf("openai: expected providerName 'custom', got %q", o.providerName)
	}
	a := NewAnthropicClient("original", "", opt)
	if a.apiKey != "original" {
		t.Errorf("anthropic: expected apiKey unchanged 'original', got %q", a.apiKey)
	}
}

func TestZeroOptionNoPanic(t *testing.T) {
	t.Parallel()
	// A zero ClientOption (nil applyConfigurable) must not panic on either
	// constructor and must leave the client unchanged.
	var opt ClientOption
	c := NewOpenAIClient("original", "", nil, opt)
	if c.apiKey != "original" {
		t.Errorf("openai: expected apiKey unchanged 'original', got %q", c.apiKey)
	}
	c2 := NewAnthropicClient("original", "", opt)
	if c2.apiKey != "original" {
		t.Errorf("anthropic: expected apiKey unchanged 'original', got %q", c2.apiKey)
	}
}

// --- Combined option tests ---

func TestMultipleOptionsAnthropic(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient(
		"", "",
		WithAPIKey("my-key"),
		WithBaseURL("https://proxy.example.com"),
		WithModel("claude-sonnet-4-6"),
		WithMaxTokens(8192),
		WithTemperature(0.5),
	)
	if c.apiKey != "my-key" {
		t.Errorf("apiKey: expected 'my-key', got %q", c.apiKey)
	}
	if c.baseURL != "https://proxy.example.com" {
		t.Errorf("baseURL: expected 'https://proxy.example.com', got %q", c.baseURL)
	}
	if c.defaultModel != "claude-sonnet-4-6" {
		t.Errorf("defaultModel: expected 'claude-sonnet-4-6', got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 8192 {
		t.Errorf("defaultMaxTokens: expected 8192, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature == nil || *c.defaultTemperature != 0.5 {
		t.Errorf("defaultTemperature: expected 0.5, got %v", c.defaultTemperature)
	}
}

func TestMultipleOptionsOpenAI(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient(
		"", "", nil,
		WithAPIKey("my-key"),
		WithBaseURL("https://proxy.example.com/v1"),
		WithModel("gpt-4o"),
		WithMaxTokens(4096),
		WithTemperature(0.8),
	)
	if c.apiKey != "my-key" {
		t.Errorf("apiKey: expected 'my-key', got %q", c.apiKey)
	}
	if c.baseURL != "https://proxy.example.com/v1" {
		t.Errorf("baseURL: expected 'https://proxy.example.com/v1', got %q", c.baseURL)
	}
	if c.defaultModel != "gpt-4o" {
		t.Errorf("defaultModel: expected 'gpt-4o', got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 4096 {
		t.Errorf("defaultMaxTokens: expected 4096, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature == nil || *c.defaultTemperature != 0.8 {
		t.Errorf("defaultTemperature: expected 0.8, got %v", c.defaultTemperature)
	}
}

func TestOptionOverridesConstructorDefaults(t *testing.T) {
	t.Parallel()
	// WithBaseURL should override the constructor's default URL.
	c := NewAnthropicClient("key", "", WithBaseURL("https://override.example.com"))
	if c.baseURL != "https://override.example.com" {
		t.Errorf("expected baseURL override, got %q", c.baseURL)
	}

	c2 := NewOpenAIClient("key", "", nil, WithBaseURL("https://override.example.com/v1"))
	if c2.baseURL != "https://override.example.com/v1" {
		t.Errorf("expected baseURL override, got %q", c2.baseURL)
	}
}

func TestExistingOptions(t *testing.T) {
	t.Parallel()
	// WithTimeout
	c := NewAnthropicClient("key", "", WithTimeout(5*time.Second))
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", c.httpClient.Timeout)
	}

	// WithLogger
	logger := slog.Default()
	c2 := NewAnthropicClient("key", "", WithLogger(logger))
	if c2.logger != logger {
		t.Error("expected custom logger to be set")
	}

	// WithHTTPClient
	hc := &http.Client{Timeout: 30 * time.Second}
	c3 := NewOpenAIClient("key", "", nil, WithHTTPClient(hc))
	if c3.httpClient != hc {
		t.Error("expected custom HTTP client to be set")
	}
}

func TestNoOptionsUsesDefaults(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("key", "")
	if c.apiKey != "key" {
		t.Errorf("apiKey: expected 'key', got %q", c.apiKey)
	}
	if c.baseURL != "https://api.anthropic.com" {
		t.Errorf("baseURL: expected default, got %q", c.baseURL)
	}
	if c.defaultModel != "" {
		t.Errorf("defaultModel: expected empty, got %q", c.defaultModel)
	}
	if c.defaultMaxTokens != 0 {
		t.Errorf("defaultMaxTokens: expected 0, got %d", c.defaultMaxTokens)
	}
	if c.defaultTemperature != nil {
		t.Errorf("defaultTemperature: expected nil, got %v", c.defaultTemperature)
	}
}
