package adapters

import (
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestAnthropicConfigSetters(t *testing.T) {
	t.Parallel()
	c := NewAnthropicClient("sk-test", "https://api.anthropic.com")

	// SetTimeout
	c.SetTimeout(5 * time.Second)
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", c.httpClient.Timeout)
	}

	// SetHTTPClient
	custom := &http.Client{Timeout: 10 * time.Second}
	c.SetHTTPClient(custom)
	if c.httpClient != custom {
		t.Error("expected httpClient to be replaced")
	}

	// SetRetry
	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 3}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", c.retry.MaxRetries)
	}

	// SetLogger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	c.SetLogger(logger)
	if c.logger != logger {
		t.Error("expected logger to be set")
	}

	// SetAPIKey
	c.SetAPIKey("sk-test")
	if c.apiKey != "sk-test" {
		t.Errorf("expected apiKey=sk-test, got %q", c.apiKey)
	}

	// SetBaseURL
	c.SetBaseURL("https://test.example.com")
	if c.baseURL != "https://test.example.com" {
		t.Errorf("expected baseURL=https://test.example.com, got %q", c.baseURL)
	}

	// SetDefaultModel
	c.SetDefaultModel("claude-opus-4")
	if c.defaultModel != "claude-opus-4" {
		t.Errorf("expected defaultModel=claude-opus-4, got %q", c.defaultModel)
	}

	// SetDefaultMaxTokens
	c.SetDefaultMaxTokens(4096)
	if c.defaultMaxTokens != 4096 {
		t.Errorf("expected defaultMaxTokens=4096, got %d", c.defaultMaxTokens)
	}

	// SetDefaultTemperature
	c.SetDefaultTemperature(0.7)
	if c.defaultTemperature == nil || *c.defaultTemperature != 0.7 {
		t.Errorf("expected defaultTemperature=0.7, got %v", c.defaultTemperature)
	}

	// SetGuardrails
	g := core.NewGuardrails()
	c.SetGuardrails(g)
	if c.guardrails != g {
		t.Error("expected guardrails to be set")
	}

	// SetProviderName (no-op)
	c.SetProviderName("custom")
	// Should not panic or change anything

	// SetMimoAuth
	c.SetMimoAuth()
	if !c.useMimoAuth {
		t.Error("expected useMimoAuth=true after SetMimoAuth")
	}

	// SetProviderName (no-op for Anthropic)
	c.SetProviderName("custom")
}

func TestAnthropicConfigGetters(t *testing.T) {
	t.Parallel()
	g := core.NewGuardrails()
	temp := 0.5
	c := &AnthropicClient{
		baseURL:            "https://api.anthropic.com",
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		retry:              core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 5}},
		logger:             slog.Default(),
		guardrails:         g,
		defaultModel:       "claude-sonnet-4",
		defaultMaxTokens:   8192,
		defaultTemperature: &temp,
		version:            "1.0.0",
	}

	if c.BaseURL() != "https://api.anthropic.com" {
		t.Errorf("BaseURL mismatch")
	}
	if c.HTTPClient().Timeout != 30*time.Second {
		t.Errorf("HTTPClient mismatch")
	}
	if c.Retry().MaxRetries != 5 {
		t.Errorf("Retry mismatch")
	}
	if c.Logger() == nil {
		t.Error("Logger should not be nil")
	}
	if c.Guardrails() != g {
		t.Error("Guardrails mismatch")
	}
	if c.DefaultModel() != "claude-sonnet-4" {
		t.Errorf("DefaultModel mismatch")
	}
	if c.DefaultMaxTokens() != 8192 {
		t.Errorf("DefaultMaxTokens mismatch")
	}
	if c.DefaultTemperature() == nil || *c.DefaultTemperature() != 0.5 {
		t.Errorf("DefaultTemperature mismatch")
	}
	if c.Version() != "1.0.0" {
		t.Errorf("Version mismatch")
	}
}

func TestOpenAIConfigSetters(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("sk-test", "https://api.openai.com", nil)

	c.SetTimeout(5 * time.Second)
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", c.httpClient.Timeout)
	}

	custom := &http.Client{Timeout: 10 * time.Second}
	c.SetHTTPClient(custom)
	if c.httpClient != custom {
		t.Error("expected httpClient to be replaced")
	}

	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 3}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", c.retry.MaxRetries)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	c.SetLogger(logger)
	if c.logger != logger {
		t.Error("expected logger to be set")
	}

	c.SetAPIKey("sk-test")
	if c.apiKey != "sk-test" {
		t.Errorf("expected apiKey=sk-test, got %q", c.apiKey)
	}

	c.SetBaseURL("https://test.example.com")
	if c.baseURL != "https://test.example.com" {
		t.Errorf("expected baseURL=https://test.example.com, got %q", c.baseURL)
	}

	c.SetDefaultModel("gpt-5")
	if c.defaultModel != "gpt-5" {
		t.Errorf("expected defaultModel=gpt-5, got %q", c.defaultModel)
	}

	c.SetDefaultMaxTokens(4096)
	if c.defaultMaxTokens != 4096 {
		t.Errorf("expected defaultMaxTokens=4096, got %d", c.defaultMaxTokens)
	}

	c.SetDefaultTemperature(0.7)
	if c.defaultTemperature == nil || *c.defaultTemperature != 0.7 {
		t.Errorf("expected defaultTemperature=0.7, got %v", c.defaultTemperature)
	}

	g := core.NewGuardrails()
	c.SetGuardrails(g)
	if c.guardrails != g {
		t.Error("expected guardrails to be set")
	}

	c.SetProviderName("custom")
	if c.providerName != "custom" {
		t.Errorf("expected providerName=custom, got %q", c.providerName)
	}

	c.SetMimoAuth()
	if !c.useMimoAuth {
		t.Error("expected useMimoAuth=true after SetMimoAuth")
	}
}

func TestOpenAIConfigGetters(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompatConfig{MaxTokensField: "max_tokens"}
	c := &OpenAIClient{
		baseURL:    "https://api.openai.com",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retry:      core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 5}},
		logger:     slog.Default(),
		compat:     compat,
	}

	if c.BaseURL() != "https://api.openai.com" {
		t.Errorf("BaseURL mismatch")
	}
	if c.HTTPClient().Timeout != 30*time.Second {
		t.Errorf("HTTPClient mismatch")
	}
	if c.Retry().MaxRetries != 5 {
		t.Errorf("Retry mismatch")
	}
	if c.ProviderName() != "" {
		t.Errorf("ProviderName mismatch")
	}
	if c.Compat() != compat {
		t.Error("Compat mismatch")
	}
	if c.Logger() == nil {
		t.Error("Logger should not be nil")
	}
	if c.Guardrails() != nil {
		t.Error("Guardrails should be nil by default")
	}
	if c.DefaultModel() != "" {
		t.Errorf("DefaultModel = %q", c.DefaultModel())
	}
	if c.DefaultMaxTokens() != 0 {
		t.Errorf("DefaultMaxTokens = %d", c.DefaultMaxTokens())
	}
	if c.DefaultTemperature() != nil {
		t.Error("DefaultTemperature should be nil")
	}
}

func TestBedrockConfigSetters(t *testing.T) {
	t.Parallel()
	c := &BedrockClient{}

	custom := &http.Client{Timeout: 10 * time.Second}
	c.SetHTTPClient(custom)
	if c.httpClient != custom {
		t.Error("expected httpClient to be replaced with custom")
	}

	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 3}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", c.retry.MaxRetries)
	}
}
