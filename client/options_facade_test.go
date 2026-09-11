package client

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
)

type recordingConfigurable struct {
	timeout      time.Duration
	httpClient   *http.Client
	retry        core.RetryConfig
	logger       *slog.Logger
	apiKey       string
	baseURL      string
	model        string
	maxTokens    int
	temperature  float64
	guardrails   *core.Guardrails
	providerName string
	mimoAuth     bool
}

func (c *recordingConfigurable) SetTimeout(value time.Duration)       { c.timeout = value }
func (c *recordingConfigurable) SetHTTPClient(value *http.Client)     { c.httpClient = value }
func (c *recordingConfigurable) SetRetry(value core.RetryConfig)      { c.retry = value }
func (c *recordingConfigurable) SetLogger(value *slog.Logger)         { c.logger = value }
func (c *recordingConfigurable) SetAPIKey(value string)               { c.apiKey = value }
func (c *recordingConfigurable) SetBaseURL(value string)              { c.baseURL = value }
func (c *recordingConfigurable) SetDefaultModel(value string)         { c.model = value }
func (c *recordingConfigurable) SetDefaultMaxTokens(value int)        { c.maxTokens = value }
func (c *recordingConfigurable) SetDefaultTemperature(value float64)  { c.temperature = value }
func (c *recordingConfigurable) SetGuardrails(value *core.Guardrails) { c.guardrails = value }
func (c *recordingConfigurable) SetProviderName(value string)         { c.providerName = value }
func (c *recordingConfigurable) SetMimoAuth()                         { c.mimoAuth = true }

func TestOptionFacadeDelegatesWithoutExposingAdapterSecrets(t *testing.T) {
	t.Parallel()
	config := &recordingConfigurable{}
	httpClient := &http.Client{Timeout: 11 * time.Second}
	logger := slog.Default()
	retry := NewRetryConfig(2, time.Millisecond, time.Second)

	options := []ClientOption{
		WithTimeout(7 * time.Second),
		WithHTTPClient(httpClient),
		WithRetry(retry),
		WithLogger(logger),
		WithAPIKey("secret"),
		WithBaseURL("https://provider.example/v1"),
		WithModel("model"),
		WithMaxTokens(2048),
		WithTemperature(0.4),
		WithGuardrails(),
		WithProviderName("provider"),
		WithMimoAuth(),
	}
	for _, option := range options {
		option.Apply(config)
	}

	if config.timeout != 7*time.Second || config.httpClient != httpClient || config.retry.MaxRetries != 2 || config.logger != logger {
		t.Fatal("transport options were not delegated")
	}
	if config.apiKey != "secret" || config.baseURL != "https://provider.example/v1" || config.providerName != "provider" {
		t.Fatal("identity options were not delegated")
	}
	if config.model != "model" || config.maxTokens != 2048 || config.temperature != 0.4 || config.guardrails == nil || !config.mimoAuth {
		t.Fatal("request options were not delegated")
	}
}
