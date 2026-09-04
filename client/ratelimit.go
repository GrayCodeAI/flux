package client

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter per provider.
// It limits the number of requests per second to avoid hitting provider rate limits.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	defaults RateLimitConfig
}

// RateLimitConfig holds rate limit settings for a provider.
type RateLimitConfig struct {
	// RequestsPerMinute is the maximum requests per minute (0 = unlimited).
	RequestsPerMinute int
	// BurstSize is the maximum burst above the steady rate (default = RequestsPerMinute/10, min 1).
	BurstSize int
	// MinInterval is the minimum time between requests (e.g., 5ms for cloud, 0 for local).
	MinInterval time.Duration
}

type tokenBucket struct {
	tokens      float64
	maxTokens   float64
	refillRate  float64 // tokens per nanosecond
	lastRefill  time.Time
	minInterval time.Duration
	lastRequest time.Time
	mu          sync.Mutex
}

func newTokenBucket(cfg RateLimitConfig) *tokenBucket {
	if cfg.RequestsPerMinute <= 0 {
		return nil // unlimited
	}
	burst := cfg.BurstSize
	if burst <= 0 {
		burst = cfg.RequestsPerMinute / 10
		if burst < 1 {
			burst = 1
		}
	}
	rate := float64(cfg.RequestsPerMinute) / float64(time.Minute)
	return &tokenBucket{
		tokens:      float64(burst),
		maxTokens:   float64(burst),
		refillRate:  rate,
		lastRefill:  time.Now(),
		minInterval: cfg.MinInterval,
	}
}

func (b *tokenBucket) wait(ctx context.Context) error {
	for {
		// Check context before attempting to acquire token
		select {
		case <-ctx.Done():
			return fmt.Errorf("graycode-router: rate limiter: %w", ctx.Err())
		default:
		}

		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.lastRefill)
		b.tokens += elapsed.Seconds() * b.refillRate * float64(time.Second)
		if b.tokens > b.maxTokens {
			b.tokens = b.maxTokens
		}
		b.lastRefill = now

		if b.tokens >= 1 {
			b.tokens--
			// Enforce minimum interval between requests
			if b.minInterval > 0 {
				since := time.Since(b.lastRequest)
				if since < b.minInterval {
					wait := b.minInterval - since
					b.mu.Unlock()
					timer := time.NewTimer(wait)
					select {
					case <-ctx.Done():
						timer.Stop()
						// Refund the token consumed above (b.tokens--): the
						// request is aborting during the min-interval wait and
						// never proceeds, so the token must return to the bucket
						// rather than being silently lost.
						b.mu.Lock()
						b.tokens++
						b.mu.Unlock()
						return fmt.Errorf("graycode-router: rate limiter: %w", ctx.Err())
					case <-timer.C:
					}
					b.mu.Lock()
				}
				b.lastRequest = time.Now()
			}
			b.mu.Unlock()
			return nil
		}

		// Calculate wait time for next token — release lock before sleeping
		needed := 1 - b.tokens
		waitDur := time.Duration(needed / b.refillRate)
		b.mu.Unlock()

		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("graycode-router: rate limiter: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// NewRateLimiter creates a rate limiter with default config applied to all providers.
func NewRateLimiter(defaults RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		defaults: defaults,
	}
}

// SetProviderLimit sets a custom rate limit for a specific provider.
func (rl *RateLimiter) SetProviderLimit(provider string, cfg RateLimitConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buckets[provider] = newTokenBucket(cfg)
}

// Wait blocks until a request token is available for the given provider.
// Returns immediately if no rate limit is configured.
func (rl *RateLimiter) Wait(ctx context.Context, provider string) error {
	rl.mu.Lock()
	b, ok := rl.buckets[provider]
	if !ok {
		b = newTokenBucket(rl.defaults)
		rl.buckets[provider] = b
	}
	rl.mu.Unlock()

	if b == nil {
		return nil // unlimited
	}
	return b.wait(ctx)
}

// RateLimitedProvider wraps a Provider with rate limiting.
type RateLimitedProvider struct {
	inner   Provider
	limiter *RateLimiter
}

// Compile-time check.
var _ Provider = (*RateLimitedProvider)(nil)

// WithRateLimit wraps a provider with a rate limiter.
func WithRateLimit(p Provider, limiter *RateLimiter) Provider {
	return &RateLimitedProvider{inner: p, limiter: limiter}
}

func (r *RateLimitedProvider) Name() string                   { return r.inner.Name() }
func (r *RateLimitedProvider) Ping(ctx context.Context) error { return r.inner.Ping(ctx) }

func (r *RateLimitedProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	if err := r.limiter.Wait(ctx, r.inner.Name()); err != nil {
		return nil, err
	}
	return r.inner.Chat(ctx, messages, opts)
}

func (r *RateLimitedProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	if err := r.limiter.Wait(ctx, r.inner.Name()); err != nil {
		return nil, err
	}
	return r.inner.StreamChat(ctx, messages, opts)
}
