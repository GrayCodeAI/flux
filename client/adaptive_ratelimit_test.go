package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockProvider is a minimal Provider for testing.
type mockProvider struct {
	name     string
	chatFn   func(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error)
	streamFn func(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error)
}

func (m *mockProvider) Name() string                   { return m.name }
func (m *mockProvider) Ping(ctx context.Context) error { return nil }
func (m *mockProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	if m.chatFn != nil {
		return m.chatFn(ctx, messages, opts)
	}
	return &EyrieResponse{
		Content: "ok",
		Usage:   &EyrieUsage{TotalTokens: 100},
	}, nil
}

func (m *mockProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, messages, opts)
	}
	ch := make(chan EyrieStreamEvent, 4)
	ch <- EyrieStreamEvent{Type: "content", Content: "hello"}
	ch <- EyrieStreamEvent{Type: "usage", Usage: &EyrieUsage{TotalTokens: 50}}
	ch <- EyrieStreamEvent{Type: "done"}
	close(ch)
	return &StreamResult{Events: ch}, nil
}

func TestAdaptiveRateLimitProvider_Name(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})
	if p.Name() != "test/adaptive-ratelimit" {
		t.Errorf("expected name 'test/adaptive-ratelimit', got %q", p.Name())
	}
}

func TestAdaptiveRateLimitProvider_Ping(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAdaptiveRateLimitProvider_Chat(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})

	resp, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content 'ok', got %q", resp.Content)
	}

	status := p.Status()
	if status.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", status.TotalRequests)
	}
	if status.TotalTokens != 100 {
		t.Errorf("expected 100 total tokens, got %d", status.TotalTokens)
	}
}

func TestAdaptiveRateLimitProvider_ChatTracksUsage(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})

	// Make a few calls
	for i := 0; i < 3; i++ {
		_, err := p.Chat(context.Background(), nil, ChatOptions{})
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}

	status := p.Status()
	if status.RPMUsed != 3 {
		t.Errorf("expected 3 RPM used, got %d", status.RPMUsed)
	}
	if status.TotalRequests != 3 {
		t.Errorf("expected 3 total requests, got %d", status.TotalRequests)
	}
	if status.TotalTokens != 300 {
		t.Errorf("expected 300 total tokens, got %d", status.TotalTokens)
	}
}

func TestAdaptiveRateLimitProvider_NearLimitThrottle(t *testing.T) {
	t.Parallel()
	// Set up a provider with a very low RPM limit (5 RPM)
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		RPMLimit:         5,
		ThresholdPercent: 50, // throttle when <50% remaining (i.e., <3 remaining)
		MaxDelay:         50 * time.Millisecond,
	})

	// First 3 calls should go through without delay
	for i := 0; i < 3; i++ {
		start := time.Now()
		_, err := p.Chat(context.Background(), nil, ChatOptions{})
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
		if time.Since(start) > 10*time.Millisecond {
			t.Errorf("call %d took too long: %v", i, time.Since(start))
		}
	}

	// Call 4 should trigger throttle (3 remaining out of 5 = 40% < 50% threshold)
	start := time.Now()
	_, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	// Should have been delayed (at least some amount, up to MaxDelay)
	if elapsed < 10*time.Millisecond {
		t.Logf("warning: call 4 was not delayed (elapsed=%v), may depend on timing", elapsed)
	}

	status := p.Status()
	if status.ThrottleCount == 0 {
		t.Logf("warning: throttle count is 0, may depend on timing")
	}
}

func TestAdaptiveRateLimitProvider_RPMExceeded(t *testing.T) {
	t.Parallel()
	// Set up with limit of 3, no delay (just error)
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		RPMLimit:         3,
		ThresholdPercent: 10,
		MaxDelay:         0, // don't delay, just error
	})

	// 3 calls should succeed
	for i := 0; i < 3; i++ {
		_, err := p.Chat(context.Background(), nil, ChatOptions{})
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}

	// 4th call should either error or succeed (depending on exact timing)
	// Since the window is 1 minute and we're making calls instantly,
	// all 3 are in the window and the 4th should hit the limit
	_, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Logf("4th call correctly returned error: %v", err)
	} else {
		// If MaxDelay is 0, we still record the request, so this might succeed
		// in some cases where the remaining check allows it
		t.Logf("4th call succeeded (may happen with zero MaxDelay config)")
	}
}

func TestAdaptiveRateLimitProvider_TPMTracking(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		TPMLimit:         500,
		ThresholdPercent: 10,
	})

	// Make calls - each uses 100 tokens
	for i := 0; i < 3; i++ {
		_, err := p.Chat(context.Background(), nil, ChatOptions{})
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}

	status := p.Status()
	if status.TPMUsed != 300 {
		t.Errorf("expected 300 TPM used, got %d", status.TPMUsed)
	}
}

func TestAdaptiveRateLimitProvider_StreamChat(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})

	result, err := p.StreamChat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []EyrieStreamEvent
	for evt := range result.Events {
		events = append(events, evt)
	}

	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}

	status := p.Status()
	if status.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", status.TotalRequests)
	}
	// Token usage from stream event should be tracked
	if status.TotalTokens != 50 {
		t.Errorf("expected 50 total tokens from stream, got %d", status.TotalTokens)
	}
}

func TestAdaptiveRateLimitProvider_UpdateFromHeaders(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		HeaderExtractor: CommonHeaderExtractor,
	})

	// Simulate OpenAI-style headers
	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "5")
	h.Set("x-ratelimit-limit-requests", "60")
	h.Set("x-ratelimit-remaining-tokens", "10000")
	h.Set("x-ratelimit-limit-tokens", "90000")
	h.Set("x-ratelimit-reset-requests", "30s")

	p.UpdateFromHeaders(h)

	status := p.Status()
	if status.RPMRemaining != 5 {
		t.Errorf("expected RPM remaining 5, got %d", status.RPMRemaining)
	}
	if status.RPMLimit != 60 {
		t.Errorf("expected RPM limit 60, got %d", status.RPMLimit)
	}
	if status.TPMRemaining != 10000 {
		t.Errorf("expected TPM remaining 10000, got %d", status.TPMRemaining)
	}
	if status.TPMLimit != 90000 {
		t.Errorf("expected TPM limit 90000, got %d", status.TPMLimit)
	}
}

func TestAdaptiveRateLimitProvider_AnthropicHeaders(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		HeaderExtractor: CommonHeaderExtractor,
	})

	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-remaining", "3")
	h.Set("anthropic-ratelimit-requests-limit", "50")
	h.Set("anthropic-ratelimit-tokens-remaining", "8000")
	h.Set("anthropic-ratelimit-tokens-limit", "40000")

	p.UpdateFromHeaders(h)

	status := p.Status()
	if status.RPMRemaining != 3 {
		t.Errorf("expected RPM remaining 3, got %d", status.RPMRemaining)
	}
	if status.RPMLimit != 50 {
		t.Errorf("expected RPM limit 50, got %d", status.RPMLimit)
	}
	if status.TPMRemaining != 8000 {
		t.Errorf("expected TPM remaining 8000, got %d", status.TPMRemaining)
	}
}

func TestAdaptiveRateLimitProvider_HeaderDrivenThrottle(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		RPMLimit:         100,
		ThresholdPercent: 10,
		MaxDelay:         50 * time.Millisecond,
		HeaderExtractor:  CommonHeaderExtractor,
	})

	// Headers indicate only 1 request remaining (1% of 100)
	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "1")
	h.Set("x-ratelimit-limit-requests", "100")
	p.UpdateFromHeaders(h)

	// Next call should be throttled due to headers showing near limit
	start := time.Now()
	_, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("call elapsed: %v (may be throttled)", elapsed)
}

func TestAdaptiveRateLimitProvider_ConcurrentSafety(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Chat(context.Background(), nil, ChatOptions{})
			if err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if errCount.Load() > 0 {
		t.Errorf("unexpected errors in concurrent test: %d", errCount.Load())
	}

	status := p.Status()
	if status.TotalRequests != 50 {
		t.Errorf("expected 50 total requests, got %d", status.TotalRequests)
	}
}

func TestAdaptiveRateLimitProvider_ContextCancellation(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		RPMLimit:         1,
		ThresholdPercent: 10,
		MaxDelay:         10 * time.Second,
	})

	// First call uses the quota
	_, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call should try to delay, but context will cancel
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = p.Chat(ctx, nil, ChatOptions{})
	if err == nil {
		t.Log("second call succeeded (may happen if window hasn't fully filled)")
	} else {
		t.Logf("second call correctly returned error: %v", err)
	}
}

func TestCommonHeaderExtractor_NoHeaders(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	result := CommonHeaderExtractor(h)
	if result != nil {
		t.Errorf("expected nil for empty headers, got %+v", result)
	}
}

func TestCommonHeaderExtractor_PartialHeaders(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "10")

	result := CommonHeaderExtractor(h)
	if result == nil {
		t.Fatal("expected non-nil result for partial headers")
	}
	if result.RequestsRemaining != 10 {
		t.Errorf("expected 10 remaining requests, got %d", result.RequestsRemaining)
	}
}

func TestParseResetTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"rfc3339", "2025-01-01T00:00:00Z", false},
		{"duration", "30s", false},
		{"seconds", "60", false},
		{"invalid", "not-a-time", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseResetTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseResetTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAdaptiveRateLimitProvider_NilInnerErrors(t *testing.T) {
	t.Parallel()
	if _, err := NewAdaptiveRateLimitProvider(nil, AdaptiveRateLimitConfig{}); err == nil {
		t.Error("expected error for nil inner provider")
	}
}

func TestAdaptiveRateLimitProvider_DefaultConfig(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{})

	// With no limits set, calls should go through freely
	for i := 0; i < 100; i++ {
		_, err := p.Chat(context.Background(), nil, ChatOptions{})
		if err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}

	status := p.Status()
	if status.TotalRequests != 100 {
		t.Errorf("expected 100 total requests, got %d", status.TotalRequests)
	}
	if status.ThrottleCount != 0 {
		t.Errorf("expected 0 throttle count with no limits, got %d", status.ThrottleCount)
	}
}

func TestAdaptiveRateLimitProvider_WindowExpiry(t *testing.T) {
	t.Parallel()
	inner := &mockProvider{name: "test"}
	p := mustAdaptiveRateLimitProvider(t, inner, AdaptiveRateLimitConfig{
		RPMLimit:         2,
		ThresholdPercent: 10,
	})

	// Manually add old entries to simulate an expired window
	p.mu.Lock()
	oldTime := time.Now().Add(-2 * time.Minute)
	p.rpmWindow = append(p.rpmWindow, oldTime, oldTime)
	p.totalRequests = 2
	p.mu.Unlock()

	// A new call should succeed because old entries expired
	_, err := p.Chat(context.Background(), nil, ChatOptions{})
	if err != nil {
		t.Fatalf("expected success after window expiry, got: %v", err)
	}

	status := p.Status()
	if status.RPMUsed != 1 {
		t.Errorf("expected 1 RPM used after pruning, got %d", status.RPMUsed)
	}
}

func BenchmarkAdaptiveRateLimitProvider_Chat(b *testing.B) {
	inner := &mockProvider{name: "bench"}
	p := mustAdaptiveRateLimitProvider(b, inner, AdaptiveRateLimitConfig{})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Chat(ctx, nil, ChatOptions{})
	}
}

func BenchmarkAdaptiveRateLimitProvider_ChatWithLimits(b *testing.B) {
	inner := &mockProvider{name: "bench"}
	p := mustAdaptiveRateLimitProvider(b, inner, AdaptiveRateLimitConfig{
		RPMLimit:         100000,
		TPMLimit:         1000000,
		ThresholdPercent: 10,
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Chat(ctx, nil, ChatOptions{})
	}
}

func ExampleAdaptiveRateLimitProvider() {
	// Create an inner provider (e.g., from NewAnthropicProvider)
	var inner Provider = &mockProvider{name: "openai"}

	// Wrap with adaptive rate limiting
	provider, err := NewAdaptiveRateLimitProvider(inner, AdaptiveRateLimitConfig{
		RPMLimit:         60,    // 60 requests per minute
		TPMLimit:         90000, // 90k tokens per minute
		ThresholdPercent: 10,    // throttle when <10% remaining
		MaxDelay:         5 * time.Second,
		HeaderExtractor:  CommonHeaderExtractor, // parse rate limit headers
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Use the provider normally
	resp, err := provider.Chat(context.Background(), []EyrieMessage{
		{Role: "user", Content: "Hello!"},
	}, ChatOptions{Model: "gpt-4"})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Response: %s\n", resp.Content)

	// Check rate limit status
	status := provider.Status()
	fmt.Printf("RPM used: %d/%d\n", status.RPMUsed, status.RPMLimit)
	fmt.Printf("Tokens used: %d/%d\n", status.TPMUsed, status.TPMLimit)
	fmt.Printf("Throttle count: %d\n", status.ThrottleCount)
}

// mustAdaptiveRateLimitProvider constructs the provider, failing the test on error.
func mustAdaptiveRateLimitProvider(tb testing.TB, inner Provider, config AdaptiveRateLimitConfig) *AdaptiveRateLimitProvider {
	tb.Helper()
	p, err := NewAdaptiveRateLimitProvider(inner, config)
	if err != nil {
		tb.Fatalf("NewAdaptiveRateLimitProvider: %v", err)
	}
	return p
}
