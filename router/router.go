package router

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GrayCodeAI/graycode-router/client"
)

type RouteEntry struct {
	Provider client.Provider
	Weight   int
	Retry    *RetryConfig
	// Cost is an optional relative cost used by StrategyCostBased (e.g. price
	// per 1K tokens in some fixed unit). When zero, Weight is used as a proxy.
	Cost int
}

// cost returns the cost proxy for the entry: the explicit Cost when set,
// otherwise the Weight. Falls back to 0 when neither is configured.
func (e RouteEntry) cost() int {
	if e.Cost > 0 {
		return e.Cost
	}
	return e.Weight
}

type Router struct {
	entries      []RouteEntry
	fallback     []client.Provider
	totalWeight  int
	defaultRetry RetryConfig
	strategy     Strategy
	stratState   *strategyState
	mu           sync.RWMutex
	stats        map[string]*atomic.Int64
}

// Option configures a Router at construction time.
type Option func(*Router)

// WithStrategy sets the load-balancing routing strategy. The default is
// StrategyWeighted, which preserves the router's original weighted-random
// behavior.
func WithStrategy(s Strategy) Option {
	return func(r *Router) { r.strategy = s }
}

var _ client.Provider = (*Router)(nil)

func New(entries []RouteEntry, fallback []client.Provider, defaultRetry *RetryConfig, opts ...Option) *Router {
	total := 0
	for _, e := range entries {
		total += e.Weight
	}
	stats := make(map[string]*atomic.Int64)
	for _, e := range entries {
		stats[e.Provider.Name()] = &atomic.Int64{}
	}
	for _, p := range fallback {
		if _, ok := stats[p.Name()]; !ok {
			stats[p.Name()] = &atomic.Int64{}
		}
	}
	dr := DefaultRetryConfig()
	if defaultRetry != nil {
		dr = *defaultRetry
	}
	r := &Router{
		entries:      entries,
		fallback:     fallback,
		totalWeight:  total,
		defaultRetry: dr,
		strategy:     StrategyWeighted,
		stratState:   newStrategyState(entries),
		stats:        stats,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Router) Name() string {
	names := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		names = append(names, e.Provider.Name())
	}
	return "router[" + strings.Join(names, ",") + "]"
}

func (r *Router) Ping(ctx context.Context) error {
	if len(r.entries) > 0 {
		return r.entries[0].Provider.Ping(ctx)
	}
	if len(r.fallback) > 0 {
		return r.fallback[0].Ping(ctx)
	}
	return fmt.Errorf("router: no providers configured")
}

func (r *Router) Chat(ctx context.Context, messages []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	provider, retry := r.selectProvider()
	r.stratState.beginInFlight(provider.Name())
	start := time.Now()
	resp, err := r.chatWithRetry(ctx, provider, messages, opts, retry)
	r.stratState.recordLatency(provider.Name(), float64(time.Since(start).Milliseconds()))
	r.stratState.endInFlight(provider.Name())
	if err == nil {
		r.recordSuccess(provider.Name())
		r.recordUsage(provider.Name(), resp)
		return resp, nil
	}
	if !IsTransient(err) {
		return nil, err
	}
	for _, fp := range r.fallback {
		if fp.Name() == provider.Name() {
			continue
		}
		resp, err = fp.Chat(ctx, messages, opts)
		if err == nil {
			r.recordSuccess(fp.Name())
			r.recordUsage(fp.Name(), resp)
			return resp, nil
		}
		if !IsTransient(err) {
			return nil, err
		}
	}
	return nil, err
}

func (r *Router) StreamChat(ctx context.Context, messages []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.StreamResult, error) {
	provider, retry := r.selectProvider()
	r.stratState.beginInFlight(provider.Name())
	start := time.Now()
	sr, err := r.streamWithRetry(ctx, provider, messages, opts, retry)
	r.stratState.recordLatency(provider.Name(), float64(time.Since(start).Milliseconds()))
	r.stratState.endInFlight(provider.Name())
	if err == nil {
		r.recordSuccess(provider.Name())
		return sr, nil
	}
	if !IsTransient(err) {
		return nil, err
	}
	for _, fp := range r.fallback {
		if fp.Name() == provider.Name() {
			continue
		}
		sr, err = fp.StreamChat(ctx, messages, opts)
		if err == nil {
			r.recordSuccess(fp.Name())
			return sr, nil
		}
		if !IsTransient(err) {
			return nil, err
		}
	}
	return nil, err
}

func (r *Router) Stats() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]int64, len(r.stats))
	for k, v := range r.stats {
		result[k] = v.Load()
	}
	return result
}

func (r *Router) selectProvider() (client.Provider, RetryConfig) {
	e := r.selectEntry()
	rc := r.defaultRetry
	if e.Retry != nil {
		rc = *e.Retry
	}
	return e.Provider, rc
}

// selectEntry picks a RouteEntry according to the router's configured strategy.
// StrategyWeighted preserves the original weighted-random behavior, including
// the zero-total-weight fallback to the first entry.
func (r *Router) selectEntry() RouteEntry {
	if len(r.entries) == 0 {
		return RouteEntry{}
	}
	idx := r.stratState.selectIndex(r.strategy, r.entries, r.totalWeight)
	return r.entries[idx]
}

func (r *Router) chatWithRetry(ctx context.Context, p client.Provider, messages []client.GraycodeRouterMessage, opts client.ChatOptions, cfg RetryConfig) (*client.GraycodeRouterResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		resp, err := p.Chat(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !IsTransient(err) {
			return nil, err
		}
		if attempt < cfg.MaxRetries {
			delay := BackoffDelay(attempt, cfg)
			if cfg.OnRetry != nil {
				cfg.OnRetry(RetryEvent{Err: err, Attempt: attempt + 1, MaxRetries: cfg.MaxRetries, Delay: delay})
			}
			timer := newTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, lastErr
}

// streamWithRetry mirrors chatWithRetry for the streaming path: transient
// errors on stream setup are retried with backoff. Errors that surface
// mid-stream (after a successful setup) are not retried here — the caller
// owns the event channel by then.
func (r *Router) streamWithRetry(ctx context.Context, p client.Provider, messages []client.GraycodeRouterMessage, opts client.ChatOptions, cfg RetryConfig) (*client.StreamResult, error) {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		sr, err := p.StreamChat(ctx, messages, opts)
		if err == nil {
			return sr, nil
		}
		lastErr = err
		if !IsTransient(err) {
			return nil, err
		}
		if attempt < cfg.MaxRetries {
			delay := BackoffDelay(attempt, cfg)
			if cfg.OnRetry != nil {
				cfg.OnRetry(RetryEvent{Err: err, Attempt: attempt + 1, MaxRetries: cfg.MaxRetries, Delay: delay})
			}
			timer := newTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, lastErr
}

func (r *Router) recordSuccess(name string) {
	r.mu.RLock()
	s, ok := r.stats[name]
	r.mu.RUnlock()
	if ok {
		s.Add(1)
	}
}

// recordUsage folds the token usage from a response into the usage-based
// strategy counters. It is a no-op when the response carries no usage data.
func (r *Router) recordUsage(name string, resp *client.GraycodeRouterResponse) {
	if resp == nil || resp.Usage == nil {
		return
	}
	tokens := resp.Usage.TotalTokens
	if tokens == 0 {
		tokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens
	}
	if tokens > 0 {
		r.stratState.recordUsage(name, int64(tokens))
	}
}
