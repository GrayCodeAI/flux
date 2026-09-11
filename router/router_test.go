//nolint:errcheck
package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/types"
)

type mockProvider struct {
	name string
	err  error
}

func (m *mockProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &client.EyrieResponse{Content: "from " + m.name}, nil
}

func (m *mockProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan client.EyrieStreamEvent, 1)
	ch <- client.EyrieStreamEvent{Type: "done"}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}
func (m *mockProvider) Ping(_ context.Context) error { return m.err }
func (m *mockProvider) Name() string                 { return m.name }

func TestRouterImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ client.Provider = (*Router)(nil)
}

func TestWeightedSelection(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 80}, {Provider: p2, Weight: 20}}, nil, nil)

	counts := map[string]int{}
	for i := 0; i < 1000; i++ {
		resp, _ := r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
		counts[resp.Content]++
	}
	if counts["from p1"] < 600 {
		t.Errorf("p1 (weight 80) got %d/1000, expected ~800", counts["from p1"])
	}
	if counts["from p2"] < 100 {
		t.Errorf("p2 (weight 20) got %d/1000, expected ~200", counts["from p2"])
	}
}

func TestFallbackOnError(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 500 internal")}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	resp, err := r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "from p2" {
		t.Errorf("expected fallback to p2, got %s", resp.Content)
	}
}

func TestAllProvidersFail(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 500")}
	p2 := &mockProvider{name: "p2", err: fmt.Errorf("HTTP 502")}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	_, err := r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err == nil {
		t.Error("expected error")
	}
}

func TestNonTransientNoFallback(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 401 unauthorized")}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	_, err := r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err == nil {
		t.Error("expected error — 401 should not fallback")
	}
}

func TestIsTransientCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg    string
		expect bool
	}{
		{"HTTP 429 rate limited", true},
		{"HTTP 500 internal", true},
		{"HTTP 502 bad gateway", true},
		{"connection refused", true},
		{"context deadline exceeded", true},
		{"HTTP 401 unauthorized", false},
		{"HTTP 404 not found", false},
		{"invalid input", false},
	}
	for _, tc := range cases {
		got := IsTransient(fmt.Errorf("%s", tc.msg))
		if got != tc.expect {
			t.Errorf("IsTransient(%q) = %v, want %v", tc.msg, got, tc.expect)
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	t.Parallel()
	cfg := NewRetryConfig(0, 100*time.Millisecond, 5*time.Second)
	// Run multiple times to account for jitter
	for i := 0; i < 10; i++ {
		d0 := BackoffDelay(0, cfg)
		d1 := BackoffDelay(1, cfg)
		d2 := BackoffDelay(2, cfg)
		if d0 > 200*time.Millisecond {
			t.Errorf("attempt 0 delay too large: %v", d0)
		}
		// With 0.5-1.5x jitter, base delays should still trend upward on average
		// Check that max of attempt N is generally > min of attempt N+1
		if d2 > 5*time.Second || d1 > 2*time.Second {
			t.Errorf("delay exceeds expected range: d1=%v, d2=%v", d1, d2)
		}
	}
}

func TestOnRetryCallback(t *testing.T) {
	t.Parallel()
	calls := 0
	p := &mockProvider{name: "p", err: fmt.Errorf("HTTP 500")}
	cfg := NewRetryConfig(2, time.Millisecond, time.Millisecond)
	cfg.OnRetry = func(e RetryEvent) { calls++ }
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, &cfg)

	r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if calls != 2 {
		t.Errorf("expected 2 OnRetry calls, got %d", calls)
	}
}

func TestToolFilter(t *testing.T) {
	t.Parallel()
	f := NewToolFilter(map[string][]string{
		"claude-3": {"web_search"},
	})
	tools := []client.EyrieTool{
		{Name: "web_search", Description: "search"},
		{Name: "code_exec", Description: "exec"},
		{Name: "my_func", Description: "custom", Parameters: map[string]interface{}{"type": "object"}},
	}
	filtered := f.FilterTools("claude-3", tools)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 tools (web_search + my_func), got %d", len(filtered))
	}
	names := map[string]bool{}
	for _, t := range filtered {
		names[t.Name] = true
	}
	if !names["web_search"] || !names["my_func"] {
		t.Errorf("unexpected tools: %v", names)
	}
}

func TestStreamFallback(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 503")}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	sr, err := r.StreamChat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for range sr.Events {
	}
}

func TestNewDefaultRetryConfig(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, nil)

	// Default retry config should be 3 retries with 1s base, 30s max.
	if r.defaultRetry.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d, want 3", r.defaultRetry.MaxRetries)
	}
	if r.defaultRetry.BaseDelay != 1*time.Second {
		t.Errorf("default BaseDelay = %v, want 1s", r.defaultRetry.BaseDelay)
	}
	if r.defaultRetry.MaxDelay != 30*time.Second {
		t.Errorf("default MaxDelay = %v, want 30s", r.defaultRetry.MaxDelay)
	}
}

func TestNewCustomRetryConfig(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	custom := &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 7, BaseDelay: 500 * time.Millisecond, MaxDelay: 10 * time.Second}}
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, custom)

	if r.defaultRetry.MaxRetries != 7 {
		t.Errorf("MaxRetries = %d, want 7", r.defaultRetry.MaxRetries)
	}
	if r.defaultRetry.BaseDelay != 500*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 500ms", r.defaultRetry.BaseDelay)
	}
}

func TestNewStatsInitialized(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "alpha"}
	p2 := &mockProvider{name: "beta"}
	fb := &mockProvider{name: "gamma"}
	r := New([]RouteEntry{{Provider: p1, Weight: 50}, {Provider: p2, Weight: 50}}, []client.Provider{fb}, nil)

	stats := r.Stats()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, ok := stats[name]; !ok {
			t.Errorf("stats missing provider %q", name)
		}
		if stats[name] != 0 {
			t.Errorf("stats[%q] = %d, want 0", name, stats[name])
		}
	}
}

func TestNewPerEntryRetryConfig(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	entryRetry := &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 10, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second}}
	r := New([]RouteEntry{{Provider: p, Weight: 100, Retry: entryRetry}}, nil, nil)

	// selectProvider should return the per-entry retry config, not the default.
	_, rc := r.selectProvider()
	if rc.MaxRetries != 10 {
		t.Errorf("entry retry MaxRetries = %d, want 10", rc.MaxRetries)
	}
	if rc.BaseDelay != 200*time.Millisecond {
		t.Errorf("entry retry BaseDelay = %v, want 200ms", rc.BaseDelay)
	}
}

func TestRouterName(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "openai"}
	p2 := &mockProvider{name: "anthropic"}
	r := New([]RouteEntry{{Provider: p1, Weight: 50}, {Provider: p2, Weight: 50}}, nil, nil)

	name := r.Name()
	if name != "router[openai,anthropic]" {
		t.Errorf("Name() = %q, want %q", name, "router[openai,anthropic]")
	}
}

func TestPingFirstEntry(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 50}, {Provider: p2, Weight: 50}}, nil, nil)

	if err := r.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestPingFirstEntryFails(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("connection refused")}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, nil, nil)

	if err := r.Ping(context.Background()); err == nil {
		t.Error("expected Ping error from failing provider")
	}
}

func TestPingFallbackOnly(t *testing.T) {
	t.Parallel()
	fb := &mockProvider{name: "fallback"}
	r := New(nil, []client.Provider{fb}, nil)

	if err := r.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestPingNoProviders(t *testing.T) {
	t.Parallel()
	r := New(nil, nil, nil)

	err := r.Ping(context.Background())
	if err == nil {
		t.Error("expected error for empty router")
	}
	if err.Error() != "router: no providers configured" {
		t.Errorf("error = %q, want %q", err.Error(), "router: no providers configured")
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	for i := 0; i < 5; i++ {
		r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	}

	stats := r.Stats()
	if stats["p1"] != 5 {
		t.Errorf("stats[p1] = %d, want 5", stats["p1"])
	}
}

func TestStatsAfterFallback(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 503")}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	for i := 0; i < 3; i++ {
		r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	}

	stats := r.Stats()
	if stats["p2"] != 3 {
		t.Errorf("stats[p2] = %d, want 3", stats["p2"])
	}
}

func TestSelectProviderZeroWeight(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	// All entries have weight 0; selectProvider falls back to first entry.
	r := New([]RouteEntry{{Provider: p, Weight: 0}}, nil, nil)

	provider, _ := r.selectProvider()
	if provider.Name() != "p" {
		t.Errorf("selectProvider() name = %q, want p", provider.Name())
	}
}

func TestContextCancellationDuringRetry(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p", err: fmt.Errorf("HTTP 500")}
	cfg := NewRetryConfig(5, 10*time.Second, 30*time.Second)
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, &cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := r.Chat(ctx, []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestShouldTryNextDeployment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg    string
		expect bool
	}{
		{"requires more credits, or fewer max_tokens; can only afford 5705", true},
		{"insufficient credits for this request", true},
		{"insufficient balance", true},
		{"payment required", true},
		{"out of credits", true},
		{"HTTP 402 payment required", true},
		{"HTTP 401 unauthorized", false},
		{"HTTP 500 internal", false},
		{"connection refused", false},
		{"", false},
	}
	for _, tc := range cases {
		var err error
		if tc.msg != "" {
			err = fmt.Errorf("%s", tc.msg)
		}
		got := ShouldTryNextDeployment(err)
		if got != tc.expect {
			t.Errorf("ShouldTryNextDeployment(%q) = %v, want %v", tc.msg, got, tc.expect)
		}
	}
}

func TestShouldTryNextDeploymentNil(t *testing.T) {
	t.Parallel()
	if ShouldTryNextDeployment(nil) {
		t.Error("ShouldTryNextDeployment(nil) should be false")
	}
}

func TestStreamNonTransientNoFallback(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 401 unauthorized")}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	_, err := r.StreamChat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err == nil {
		t.Error("expected error — 401 should not fallback on stream")
	}
}

func TestStreamAllProvidersFail(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1", err: fmt.Errorf("HTTP 500")}
	p2 := &mockProvider{name: "p2", err: fmt.Errorf("HTTP 502")}
	r := New([]RouteEntry{{Provider: p1, Weight: 100}}, []client.Provider{p2}, &RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})

	_, err := r.StreamChat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if err == nil {
		t.Error("expected error when all providers fail")
	}
}

func TestCircuitBreakerBasicFlow(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	// Closed: allows requests.
	if !cb.Allow() {
		t.Error("new circuit breaker should allow")
	}
	if cb.State() != CircuitClosed {
		t.Error("initial state should be closed")
	}

	// Reach threshold to open.
	cb.Failure()
	cb.Failure()
	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Error("should be open after 3 failures")
	}
	if cb.Allow() {
		t.Error("open circuit breaker should reject")
	}

	// After cooldown, the circuit allows probes through.
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Error("circuit should allow after cooldown")
	}
	// Allow() is a pure predicate and no longer transitions state.
	// A successful probe resets to Closed.
	cb.Success()
	if cb.State() != CircuitClosed {
		t.Error("should be closed after successful probe")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(2, time.Hour)
	cb.Failure()
	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Fatal("should be open")
	}
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Error("should be closed after Reset")
	}
	if !cb.Allow() {
		t.Error("should allow after Reset")
	}
}

func TestCircuitBreakerDefaultThresholds(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(0, 0)
	// Zero/negative values should get defaults (threshold=5, cooldown=30s).
	for i := 0; i < 4; i++ {
		cb.Failure()
	}
	if cb.State() != CircuitClosed {
		t.Error("4 failures should not open with default threshold=5")
	}
	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Error("5 failures should open with default threshold=5")
	}
}
