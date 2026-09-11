package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimitState holds the current rate limit tracking state for a provider.
type RateLimitState struct {
	// RPM tracking
	RPMUsed    int       // requests used in the current window
	RPMLimit   int       // maximum requests per minute (0 = unknown)
	RPMResetAt time.Time // when the RPM window resets

	// TPM tracking
	TPMUsed    int       // tokens used in the current window
	TPMLimit   int       // maximum tokens per minute (0 = unknown)
	TPMResetAt time.Time // when the TPM window resets

	// Header-derived remaining counts (from x-ratelimit-remaining)
	RPMRemaining int // -1 if unknown
	TPMRemaining int // -1 if unknown

	// Last updated timestamp
	LastUpdated time.Time

	// Total requests and tokens tracked (lifetime)
	TotalRequests int64
	TotalTokens   int64

	// Number of times the provider was delayed due to near-limit
	ThrottleCount int64
}

// RateLimitHeaders contains rate limit information extracted from HTTP response
// headers. Different providers use different header names; common patterns
// include OpenAI's x-ratelimit-* and Anthropic's anthropic-ratelimit-* headers.
type RateLimitHeaders struct {
	RequestsRemaining int
	RequestsLimit     int
	TokensRemaining   int
	TokensLimit       int
	ResetTime         time.Time
}

// HeaderExtractor is a function that extracts rate limit information from
// HTTP response headers. Different providers use different header naming
// conventions (OpenAI uses x-ratelimit-*, Anthropic uses anthropic-ratelimit-*).
type HeaderExtractor func(h http.Header) *RateLimitHeaders

// CommonHeaderExtractor tries to parse rate limit headers from common LLM
// API providers (OpenAI, Anthropic, and compatible APIs).
func CommonHeaderExtractor(h http.Header) *RateLimitHeaders {
	var rl RateLimitHeaders
	var found bool

	// OpenAI-style headers: x-ratelimit-remaining-requests, x-ratelimit-limit-requests,
	// x-ratelimit-remaining-tokens, x-ratelimit-limit-tokens, x-ratelimit-reset-requests
	if v := h.Get("x-ratelimit-remaining-requests"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.RequestsRemaining = n
			found = true
		}
	}
	if v := h.Get("x-ratelimit-limit-requests"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.RequestsLimit = n
			found = true
		}
	}
	if v := h.Get("x-ratelimit-remaining-tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.TokensRemaining = n
			found = true
		}
	}
	if v := h.Get("x-ratelimit-limit-tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.TokensLimit = n
			found = true
		}
	}

	// Reset time from x-ratelimit-reset-requests or x-ratelimit-reset-tokens
	if v := h.Get("x-ratelimit-reset-requests"); v != "" {
		if t, err := parseResetTime(v); err == nil {
			rl.ResetTime = t
		}
	}

	// Anthropic-style headers: anthropic-ratelimit-requests-remaining,
	// anthropic-ratelimit-requests-limit, anthropic-ratelimit-tokens-remaining,
	// anthropic-ratelimit-tokens-limit, anthropic-ratelimit-reset-requests
	if v := h.Get("anthropic-ratelimit-requests-remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.RequestsRemaining = n
			found = true
		}
	}
	if v := h.Get("anthropic-ratelimit-requests-limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.RequestsLimit = n
			found = true
		}
	}
	if v := h.Get("anthropic-ratelimit-tokens-remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.TokensRemaining = n
			found = true
		}
	}
	if v := h.Get("anthropic-ratelimit-tokens-limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rl.TokensLimit = n
			found = true
		}
	}
	if v := h.Get("anthropic-ratelimit-reset-requests"); v != "" {
		if t, err := parseResetTime(v); err == nil {
			rl.ResetTime = t
		}
	}

	if !found {
		return nil
	}
	return &rl
}

// parseResetTime parses a reset time value that may be an RFC3339 timestamp
// or a duration string like "1s" or "60s".
func parseResetTime(v string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	// Try duration (e.g., "1s", "60s")
	if d, err := time.ParseDuration(v); err == nil {
		return time.Now().Add(d), nil
	}
	// Try unix timestamp seconds
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Unix(int64(secs), 0), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse reset time: %s", v)
}

// AdaptiveRateLimitConfig configures the AdaptiveRateLimitProvider.
type AdaptiveRateLimitConfig struct {
	// RPMLimit is the maximum requests per minute (0 = no limit).
	RPMLimit int
	// TPMLimit is the maximum tokens per minute (0 = no limit).
	TPMLimit int
	// ThresholdPercent is the percentage of remaining quota below which
	// the provider starts throttling. Default is 10 (i.e., <10% remaining).
	ThresholdPercent int
	// MaxDelay is the maximum time to delay when throttling.
	// Default is 10 seconds. Set to 0 to return an error instead of delaying.
	MaxDelay time.Duration
	// HeaderExtractor is an optional function to extract rate limit info from
	// HTTP response headers. If nil, only internal tracking is used.
	HeaderExtractor HeaderExtractor
}

// AdaptiveRateLimitProvider wraps any Provider with per-provider adaptive rate
// limiting. It tracks RPM (requests per minute) and TPM (tokens per minute),
// using a sliding window approach. When the remaining quota drops below a
// configurable threshold (default 10%), the provider either delays the request
// until the window resets or returns a clear error.
//
// Rate limit state can be updated from HTTP response headers when a
// HeaderExtractor is provided, enabling the wrapper to react to server-reported
// limits even when they differ from configured values.
//
// AdaptiveRateLimitProvider is safe for concurrent use.
type AdaptiveRateLimitProvider struct {
	inner  Provider
	config AdaptiveRateLimitConfig
	mu     sync.Mutex

	// RPM tracking: timestamps of requests in the current window
	rpmWindow    []time.Time
	rpmRemaining int // -1 if unknown (from headers)
	rpmLimit     int // effective limit (from config or headers)

	// TPM tracking: rolling window of (timestamp, token count)
	tpmWindow    []tpmEntry
	tpmRemaining int // -1 if unknown (from headers)
	tpmLimit     int // effective limit (from config or headers)

	// Reset time from headers (if known)
	resetTime time.Time

	// Lifetime counters
	totalRequests int64
	totalTokens   int64
	throttleCount int64
}

// tpmEntry tracks a token usage event.
type tpmEntry struct {
	timestamp time.Time
	tokens    int
}

// Compile-time check that AdaptiveRateLimitProvider implements Provider.
var _ Provider = (*AdaptiveRateLimitProvider)(nil)

// NewAdaptiveRateLimitProvider wraps inner with adaptive rate limiting.
// inner must not be nil (an error is returned otherwise). config may be
// zero-valued for sensible defaults.
func NewAdaptiveRateLimitProvider(inner Provider, config AdaptiveRateLimitConfig) (*AdaptiveRateLimitProvider, error) {
	if inner == nil {
		return nil, errors.New("eyrie: NewAdaptiveRateLimitProvider inner provider must not be nil")
	}
	if config.ThresholdPercent <= 0 {
		config.ThresholdPercent = 10
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 10 * time.Second
	}
	rpmLimit := config.RPMLimit
	tpmLimit := config.TPMLimit
	return &AdaptiveRateLimitProvider{
		inner:        inner,
		config:       config,
		rpmWindow:    make([]time.Time, 0, 64),
		rpmRemaining: -1,
		rpmLimit:     rpmLimit,
		tpmWindow:    make([]tpmEntry, 0, 64),
		tpmRemaining: -1,
		tpmLimit:     tpmLimit,
	}, nil
}

// Name returns the inner provider name suffixed with "/adaptive-ratelimit".
func (a *AdaptiveRateLimitProvider) Name() string {
	return a.inner.Name() + "/adaptive-ratelimit"
}

// Ping delegates directly to the inner provider.
func (a *AdaptiveRateLimitProvider) Ping(ctx context.Context) error {
	return a.inner.Ping(ctx)
}

// Chat sends a non-streaming chat request with adaptive rate limiting.
func (a *AdaptiveRateLimitProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	if err := a.checkAndWait(ctx); err != nil {
		return nil, err
	}

	resp, err := a.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	// Record usage from response
	tokens := a.extractTokens(resp.Usage)
	a.recordUsage(tokens)
	return resp, nil
}

// StreamChat sends a streaming chat request with adaptive rate limiting.
func (a *AdaptiveRateLimitProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	if err := a.checkAndWait(ctx); err != nil {
		return nil, err
	}

	result, err := a.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	// Wrap the events channel to intercept usage events for token tracking.
	// Also observe ctx cancellation so that:
	//   - we stop forwarding events promptly (the caller's wrappedCh is no
	//     longer being drained on the consumer side);
	//   - we release the inner stream's resources (body, goroutine) by
	//     closing it; otherwise it hangs on a blocking send.
	wrappedCh := make(chan EyrieStreamEvent, cap(result.Events))
	go func() {
		defer close(wrappedCh)
		for {
			select {
			case <-ctx.Done():
				result.Close()
				// Drain remaining events so the inner goroutine can complete
				// and close its own body.
				for range result.Events {
				}
				return
			case evt, ok := <-result.Events:
				if !ok {
					return
				}
				if evt.Type == "usage" && evt.Usage != nil {
					tokens := a.extractTokens(evt.Usage)
					a.recordTokens(tokens)
				}
				select {
				case wrappedCh <- evt:
				case <-ctx.Done():
					result.Close()
					for range result.Events {
					}
					return
				}
			}
		}
	}()

	return NewStreamResultWithRequestID(wrappedCh, result.RequestID, result.Close), nil
}

// UpdateFromHeaders updates the rate limit state from HTTP response headers.
// This can be called externally when headers are available (e.g., from a
// middleware or response interceptor).
func (a *AdaptiveRateLimitProvider) UpdateFromHeaders(h http.Header) {
	if a.config.HeaderExtractor == nil {
		return
	}
	rl := a.config.HeaderExtractor(h)
	if rl == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if rl.RequestsLimit > 0 {
		a.rpmLimit = rl.RequestsLimit
	}
	if rl.RequestsRemaining >= 0 {
		a.rpmRemaining = rl.RequestsRemaining
	}
	if rl.TokensLimit > 0 {
		a.tpmLimit = rl.TokensLimit
	}
	if rl.TokensRemaining >= 0 {
		a.tpmRemaining = rl.TokensRemaining
	}
	if !rl.ResetTime.IsZero() {
		a.resetTime = rl.ResetTime
	}
}

// Status returns a snapshot of the current rate limit state.
func (a *AdaptiveRateLimitProvider) Status() RateLimitState {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	a.pruneWindows(now)

	rpmUsed := len(a.rpmWindow)
	tpmUsed := 0
	for _, e := range a.tpmWindow {
		tpmUsed += e.tokens
	}

	rpmReset := a.windowResetTime()
	tpmReset := a.windowResetTime()

	return RateLimitState{
		RPMUsed:       rpmUsed,
		RPMLimit:      a.rpmLimit,
		RPMResetAt:    rpmReset,
		TPMUsed:       tpmUsed,
		TPMLimit:      a.tpmLimit,
		TPMResetAt:    tpmReset,
		RPMRemaining:  a.rpmRemaining,
		TPMRemaining:  a.tpmRemaining,
		LastUpdated:   now,
		TotalRequests: a.totalRequests,
		TotalTokens:   a.totalTokens,
		ThrottleCount: a.throttleCount,
	}
}

// checkAndWait checks rate limits and delays if near the threshold.
func (a *AdaptiveRateLimitProvider) checkAndWait(ctx context.Context) error {
	a.mu.Lock()

	now := time.Now()
	a.pruneWindows(now)

	// Check RPM limits
	if a.rpmLimit > 0 {
		rpmUsed := len(a.rpmWindow)
		remaining := a.rpmLimit - rpmUsed

		// If we have header-derived remaining, prefer that
		if a.rpmRemaining >= 0 && a.rpmRemaining < remaining {
			remaining = a.rpmRemaining
		}

		if remaining <= 0 || a.isNearLimit(remaining, a.rpmLimit) {
			delay := a.delayUntilReset(now)
			if remaining <= 0 && delay > a.config.MaxDelay {
				a.mu.Unlock()
				return fmt.Errorf("eyrie: adaptive ratelimit: %s RPM limit reached (%d/%d), resets in %v",
					a.inner.Name(), rpmUsed, a.rpmLimit, delay)
			}
			if delay > 0 && delay <= a.config.MaxDelay {
				a.throttleCount++
				a.mu.Unlock()
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				a.mu.Lock()
				now = time.Now()
				a.pruneWindows(now)
			}
		}
	}

	// Check TPM limits
	if a.tpmLimit > 0 {
		tpmUsed := 0
		for _, e := range a.tpmWindow {
			tpmUsed += e.tokens
		}
		remaining := a.tpmLimit - tpmUsed

		if a.tpmRemaining >= 0 && a.tpmRemaining < remaining {
			remaining = a.tpmRemaining
		}

		if remaining <= 0 || a.isNearLimit(remaining, a.tpmLimit) {
			delay := a.delayUntilReset(now)
			if remaining <= 0 && delay > a.config.MaxDelay {
				a.mu.Unlock()
				return fmt.Errorf("eyrie: adaptive ratelimit: %s TPM limit reached (%d/%d), resets in %v",
					a.inner.Name(), tpmUsed, a.tpmLimit, delay)
			}
			if delay > 0 && delay <= a.config.MaxDelay {
				a.throttleCount++
				a.mu.Unlock()
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
				a.mu.Lock()
				now = time.Now()
				a.pruneWindows(now)
			}
		}
	}

	// Record this request
	a.rpmWindow = append(a.rpmWindow, now)
	a.totalRequests++
	a.mu.Unlock()
	return nil
}

// recordUsage records both a request and token usage.
func (a *AdaptiveRateLimitProvider) recordUsage(tokens int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// For Chat (non-streaming), the request was already recorded in checkAndWait.
	// Only record tokens here.
	if tokens > 0 {
		now := time.Now()
		a.tpmWindow = append(a.tpmWindow, tpmEntry{timestamp: now, tokens: tokens})
		a.totalTokens += int64(tokens)
	}
}

// recordTokens records token usage (used by stream events).
func (a *AdaptiveRateLimitProvider) recordTokens(tokens int) {
	if tokens <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	a.tpmWindow = append(a.tpmWindow, tpmEntry{timestamp: now, tokens: tokens})
	a.totalTokens += int64(tokens)
}

// extractTokens gets the total token count from usage, falling back to
// prompt + completion if total is not set.
func (a *AdaptiveRateLimitProvider) extractTokens(usage *EyrieUsage) int {
	if usage == nil {
		return 0
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	return total
}

// isNearLimit returns true if the remaining count is less than
// ThresholdPercent of the limit.
func (a *AdaptiveRateLimitProvider) isNearLimit(remaining, limit int) bool {
	if limit <= 0 {
		return false
	}
	threshold := (limit * a.config.ThresholdPercent) / 100
	if threshold < 1 {
		threshold = 1
	}
	return remaining < threshold
}

// pruneWindows removes entries older than 1 minute from the sliding windows.
// Must be called with a.mu held.
func (a *AdaptiveRateLimitProvider) pruneWindows(now time.Time) {
	cutoff := now.Add(-1 * time.Minute)

	// Prune RPM window
	i := 0
	for _, t := range a.rpmWindow {
		if t.After(cutoff) {
			break
		}
		i++
	}
	if i > 0 {
		a.rpmWindow = a.rpmWindow[i:]
	}

	// Prune TPM window
	j := 0
	for _, e := range a.tpmWindow {
		if e.timestamp.After(cutoff) {
			break
		}
		j++
	}
	if j > 0 {
		a.tpmWindow = a.tpmWindow[j:]
	}
}

// delayUntilReset returns the duration until the current window resets.
// Must be called with a.mu held.
func (a *AdaptiveRateLimitProvider) delayUntilReset(now time.Time) time.Duration {
	// If we have a reset time from headers and it's in the future, use it
	if !a.resetTime.IsZero() && a.resetTime.After(now) {
		return a.resetTime.Sub(now)
	}
	// Otherwise, reset when the oldest entry in the window expires
	if len(a.rpmWindow) > 0 {
		return a.rpmWindow[0].Add(1 * time.Minute).Sub(now)
	}
	if len(a.tpmWindow) > 0 {
		return a.tpmWindow[0].timestamp.Add(1 * time.Minute).Sub(now)
	}
	return 0
}

// windowResetTime returns when the current window resets.
// Must be called with a.mu held.
func (a *AdaptiveRateLimitProvider) windowResetTime() time.Time {
	now := time.Now()
	if !a.resetTime.IsZero() && a.resetTime.After(now) {
		return a.resetTime
	}
	if len(a.rpmWindow) > 0 {
		return a.rpmWindow[0].Add(1 * time.Minute)
	}
	return time.Time{}
}
