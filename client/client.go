// Package client provides LLM provider clients for Anthropic, OpenAI,
// and OpenAI-compatible APIs with streaming, retry, and provider detection.
package client

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/core"

	"github.com/GrayCodeAI/graycode-router/catalog"
)

// Version mirrors core.Version for backward compatibility; the canonical
// value lives in client/core so subpackages can build User-Agent strings.
// Default is "dev" until the root package initialises.
var Version = "dev"

// SetVersion is called by the root graycode-router package's init to wire the canonical
// version from the VERSION file into this sub-package (and client/core).
func SetVersion(v string) {
	Version = v
	core.SetVersion(v)
}

// GraycodeRouterClient is the universal LLM client.
// It is safe for concurrent use.
type GraycodeRouterClient struct {
	mu              sync.RWMutex
	defaultProvider string
	apiKeys         map[string]string
	baseURLs        map[string]string
	providers       map[string]Provider // cached provider clients
	coalescer       *Coalescer          // optional request coalescing
}

// Client creates an GraycodeRouterClient.
func Client(cfg *GraycodeRouterConfig, opts ...ClientOption) *GraycodeRouterClient {
	c := &GraycodeRouterClient{
		defaultProvider: DetectProvider(),
		apiKeys:         make(map[string]string),
		baseURLs:        make(map[string]string),
		providers:       make(map[string]Provider),
	}
	if cfg != nil {
		if cfg.Provider != "" {
			c.defaultProvider = cfg.Provider
		}
		if cfg.APIKey != "" {
			c.apiKeys[c.defaultProvider] = cfg.APIKey
		}
		if cfg.BaseURL != "" {
			c.baseURLs[c.defaultProvider] = cfg.BaseURL
		}
	}
	// Apply options (including coalescing)
	for _, opt := range opts {
		opt.ApplyGraycodeRouter(c)
	}
	return c
}

// SetCoalescingTTL enables request coalescing with the given reuse TTL.
// Implements core.GraycodeRouterConfigurable for WithCoalescing.
func (c *GraycodeRouterClient) SetCoalescingTTL(ttl time.Duration) {
	c.coalescer = NewCoalescer(ttl)
}

// SetAPIKey sets an API key for a provider.
func (c *GraycodeRouterClient) SetAPIKey(provider, apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKeys[provider] = apiKey
	delete(c.providers, provider) // invalidate cached client
}

// Ping checks connectivity to the specified (or default) provider.
func (c *GraycodeRouterClient) Ping(ctx context.Context, provider string) error {
	if provider == "" {
		provider = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return err
	}
	return p.Ping(ctx)
}

// AnthropicClientConfig holds config for creating an Anthropic client.
type AnthropicClientConfig struct {
	APIKey         string            `json:"-"`
	DefaultHeaders map[string]string `json:"default_headers,omitempty"`
	Timeout        int               `json:"timeout,omitempty"`
	MaxRetries     int               `json:"max_retries,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	BaseURL        string            `json:"base_url,omitempty"`
}

// ParseCustomHeaders parses GRAYCODE_CUSTOM_HEADERS env var into a map.
func ParseCustomHeaders() map[string]string {
	result := make(map[string]string)
	raw := os.Getenv("GRAYCODE_CUSTOM_HEADERS")
	if raw == "" {
		return result
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ":"); idx > 0 {
			name := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Reject header names/values containing control characters to prevent injection.
			if strings.IndexFunc(name, func(r rune) bool { return r < 0x20 }) >= 0 ||
				strings.IndexFunc(value, func(r rune) bool { return r < 0x20 }) >= 0 {
				continue
			}
			result[name] = value
		}
	}
	return result
}

var (
	cachedCatalog   *catalog.CompiledCatalog
	catalogLoadOnce sync.Once
)

// NewImageMessage creates a user message with an image from a URL or data URI.
// The url parameter accepts HTTP(S) URLs or data URIs (data:image/png;base64,...).
func NewImageMessage(url string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "image_url", ImageURL: &ImageURLPart{URL: url}},
		},
	}
}

// NewImageMessageWithText creates a user message with text and an image from a URL or data URI.
func NewImageMessageWithText(text, url string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: text},
			{Type: "image_url", ImageURL: &ImageURLPart{URL: url}},
		},
	}
}

// NewBase64ImageMessage creates a user message with a base64-encoded image.
// mediaType should be a MIME type like "image/png" or "image/jpeg".
func NewBase64ImageMessage(data, mediaType string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "image_url", ImageURL: &ImageURLPart{
				URL: "data:" + mediaType + ";base64," + data,
			}},
		},
	}
}

// NewBase64ImageMessageWithText creates a user message with text and a base64-encoded image.
func NewBase64ImageMessageWithText(text, data, mediaType string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: text},
			{Type: "image_url", ImageURL: &ImageURLPart{
				URL: "data:" + mediaType + ";base64," + data,
			}},
		},
	}
}

// NewAudioMessage creates a user message with base64-encoded audio.
// format should be "wav" or "mp3".
func NewAudioMessage(base64Data, format string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "input_audio", InputAudio: &InputAudioPart{
				Data:   base64Data,
				Format: format,
			}},
		},
	}
}

// NewAudioMessageWithText creates a user message with text and base64-encoded audio.
func NewAudioMessageWithText(text, base64Data, format string) GraycodeRouterMessage {
	return GraycodeRouterMessage{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: text},
			{Type: "input_audio", InputAudio: &InputAudioPart{
				Data:   base64Data,
				Format: format,
			}},
		},
	}
}

// ResolveDefaultModel resolves the default model for a provider from the catalog.
func ResolveDefaultModel(provider string) string {
	catalogLoadOnce.Do(func() {
		cat, err := catalog.LoadCatalog(context.Background(), catalog.LoadCatalogOptions{})
		if err == nil {
			cachedCatalog = cat
		}
	})
	if cachedCatalog == nil {
		return ""
	}
	models := catalog.ModelEntriesForProvider(cachedCatalog, provider)
	if len(models) > 0 {
		return models[0].ID
	}
	return ""
}
