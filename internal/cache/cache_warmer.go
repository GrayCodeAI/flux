package graycoderouter

import (
	"context"
	"math"
	"sync"
	"time"
)

// Default cache warmer settings.
const (
	// DefaultWarmInterval is the default interval between cache warming pings.
	// Anthropic's prompt cache TTL is 5 minutes; we warm at 4 minutes to stay ahead.
	DefaultWarmInterval = 4 * time.Minute

	// anthropicInputPricePerToken is the per-token input price for Claude models (USD).
	// Based on Claude Sonnet pricing: $3 per 1M input tokens.
	anthropicInputPricePerToken = 3.0 / 1_000_000

	// anthropicCacheWriteMultiplier is the cost multiplier for writing to cache (1.25x).
	anthropicCacheWriteMultiplier = 1.25

	// anthropicCacheReadMultiplier is the cost multiplier for reading from cache (0.1x).
	anthropicCacheReadMultiplier = 0.1

	// maxCacheBreakpoints is the maximum number of cache breakpoints Anthropic allows.
	maxCacheBreakpoints = 4

	// minBlockSizeForBreakpoint is the minimum content length to warrant a breakpoint.
	minBlockSizeForBreakpoint = 200
)

// Message is a minimal message struct for cache warmer use.
type Message struct {
	Role    string
	Content string
}

// ChatOptions is a minimal options struct for cache warmer use.
type ChatOptions struct {
	Model         string
	MaxTokens     int
	System        string
	EnableCaching bool
}

// CacheStats tracks cache warming statistics.
type CacheStats struct {
	WarmingRequests     int64
	CacheHits           int64
	CacheMisses         int64
	EstimatedSavingsUSD float64
	LastWarmedAt        time.Time
}

// CacheWarmer keeps Anthropic's prompt cache warm by periodically sending
// minimal requests with the system prompt, ensuring subsequent real requests
// get cache hits at a 90% discount.
type CacheWarmer struct {
	Provider     string
	Model        string
	SystemPrompt string
	Interval     time.Duration
	Enabled      bool

	done  chan struct{}
	mu    sync.Mutex
	Stats CacheStats

	// ChatFn is the function used to send warming requests.
	// It should send a chat completion request to the provider.
	ChatFn func(ctx context.Context, messages []Message, opts ChatOptions) error
}

// NewCacheWarmer creates a new CacheWarmer configured for the given provider.
// The chatFn is called to send warming pings; it should make a real API call.
func NewCacheWarmer(chatFn func(ctx context.Context, messages []Message, opts ChatOptions) error, systemPrompt, provider, model string) *CacheWarmer {
	return &CacheWarmer{
		Provider:     provider,
		Model:        model,
		SystemPrompt: systemPrompt,
		Interval:     DefaultWarmInterval,
		Enabled:      true,
		ChatFn:       chatFn,
	}
}

// Start begins the background cache warming loop. It sends a warming ping
// immediately, then repeats every Interval until Stop is called or the
// context is cancelled. For non-Anthropic providers this is a no-op.
func (cw *CacheWarmer) Start(ctx context.Context) error {
	if !cw.isAnthropicProvider() {
		return nil
	}

	cw.mu.Lock()
	if cw.done != nil {
		cw.mu.Unlock()
		return nil // already running
	}
	cw.done = make(chan struct{})
	cw.mu.Unlock()

	// Send initial warming request
	_ = cw.Warm(ctx)

	go cw.loop(ctx)
	return nil
}

// loop is the background goroutine that periodically warms the cache.
func (cw *CacheWarmer) loop(ctx context.Context) {
	ticker := time.NewTicker(cw.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cw.getDone():
			return
		case <-ticker.C:
			if !cw.Enabled {
				continue
			}
			_ = cw.Warm(ctx)
		}
	}
}

// getDone returns the done channel safely.
func (cw *CacheWarmer) getDone() <-chan struct{} {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.done
}

// Stop stops the cache warmer background loop.
func (cw *CacheWarmer) Stop() {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if cw.done != nil {
		close(cw.done)
		cw.done = nil
	}
}

// Warm sends a single warming request immediately. This keeps the system prompt
// cached on Anthropic's side. Returns an error if the request fails.
// For non-Anthropic providers this is a no-op.
func (cw *CacheWarmer) Warm(ctx context.Context) error {
	if !cw.isAnthropicProvider() {
		return nil
	}

	if cw.ChatFn == nil {
		return nil
	}

	messages := []Message{
		{Role: "user", Content: "ping"},
	}
	opts := ChatOptions{
		Model:         cw.Model,
		MaxTokens:     1,
		System:        cw.SystemPrompt,
		EnableCaching: true,
	}

	err := cw.ChatFn(ctx, messages, opts)

	cw.mu.Lock()
	cw.Stats.WarmingRequests++
	cw.Stats.LastWarmedAt = time.Now()
	if err == nil {
		if cw.Stats.WarmingRequests > 1 {
			cw.Stats.CacheHits++
		} else {
			cw.Stats.CacheMisses++
		}
	} else {
		cw.Stats.CacheMisses++
	}
	cw.mu.Unlock()

	return err
}

// ShouldWarm returns true if the cache has likely expired (more than 4 minutes
// since the last warming request) and should be refreshed.
func (cw *CacheWarmer) ShouldWarm() bool {
	if !cw.isAnthropicProvider() {
		return false
	}

	cw.mu.Lock()
	lastWarmed := cw.Stats.LastWarmedAt
	cw.mu.Unlock()

	if lastWarmed.IsZero() {
		return true
	}

	return time.Since(lastWarmed) > cw.Interval
}

// EstimateSavings calculates the cost savings from prompt caching in USD.
//
// Without cache: inputTokens * requestCount * price_per_token
// With cache: (inputTokens * price_per_token * 1.25 first time) +
//
//	(inputTokens * (requestCount-1) * price_per_token * 0.1)
//
// Returns the difference (savings amount).
func (cw *CacheWarmer) EstimateSavings(inputTokens int, requestCount int) float64 {
	if inputTokens <= 0 || requestCount <= 0 {
		return 0
	}

	tokens := float64(inputTokens)
	count := float64(requestCount)

	// Cost without caching
	withoutCache := tokens * count * anthropicInputPricePerToken

	// Cost with caching
	// First request: cache write costs 1.25x
	cacheWrite := tokens * anthropicInputPricePerToken * anthropicCacheWriteMultiplier
	// Subsequent requests: cache read costs 0.1x
	cacheReads := 0.0
	if requestCount > 1 {
		cacheReads = tokens * (count - 1) * anthropicInputPricePerToken * anthropicCacheReadMultiplier
	}
	withCache := cacheWrite + cacheReads

	savings := withoutCache - withCache
	// Round to avoid floating point noise
	return math.Round(savings*1_000_000) / 1_000_000
}

// CacheBreakpoints suggests where to place cache breakpoints in a message list.
// Anthropic allows up to 4 breakpoints. The strategy is:
//   - After system prompt (index 0 in returned slice signals "system prompt")
//   - After the first user message
//   - After large context blocks (messages with content > 200 chars)
//
// Returns indices into the messages array where breakpoints should be placed.
// The special index -1 indicates the system prompt should be cached.
func (cw *CacheWarmer) CacheBreakpoints(systemPrompt string, conversationPrefix []Message) []int {
	var breakpoints []int

	// Always cache the system prompt if present
	if systemPrompt != "" {
		breakpoints = append(breakpoints, -1)
	}

	if len(conversationPrefix) == 0 {
		return breakpoints
	}

	// Place breakpoint after first user message
	for i, msg := range conversationPrefix {
		if msg.Role == "user" {
			breakpoints = append(breakpoints, i)
			break
		}
	}

	// Place breakpoints after large content blocks
	for i, msg := range conversationPrefix {
		if len(breakpoints) >= maxCacheBreakpoints {
			break
		}
		if len(msg.Content) >= minBlockSizeForBreakpoint {
			// Avoid duplicating an already-added index
			alreadyAdded := false
			for _, bp := range breakpoints {
				if bp == i {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				breakpoints = append(breakpoints, i)
			}
		}
	}

	// Enforce max breakpoints limit
	if len(breakpoints) > maxCacheBreakpoints {
		breakpoints = breakpoints[:maxCacheBreakpoints]
	}

	return breakpoints
}

// isAnthropicProvider returns true if the provider is Anthropic.
func (cw *CacheWarmer) isAnthropicProvider() bool {
	return cw.Provider == "anthropic"
}
