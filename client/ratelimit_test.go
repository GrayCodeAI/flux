package client

import (
	"context"
	"testing"
	"time"
)

func TestRateLimitAllowsWithinRate(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 600, // 10/sec
		BurstSize:         10,
	})

	ctx := context.Background()
	// Should allow burst requests immediately
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx, "test-provider"); err != nil {
			t.Fatalf("request %d should succeed: %v", i, err)
		}
	}
}

func TestRateLimitBlocksExceedingRate(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 60, // 1/sec
		BurstSize:         2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Exhaust burst
	_ = rl.Wait(ctx, "test-provider")
	_ = rl.Wait(ctx, "test-provider")

	// Third request should block and hit context timeout
	err := rl.Wait(ctx, "test-provider")
	if err == nil {
		t.Error("expected error when rate exceeded and context times out")
	}
}

func TestRateLimitBurstAllowsImmediate(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 60,
		BurstSize:         5,
	})

	ctx := context.Background()
	start := time.Now()
	// All 5 burst tokens should be available immediately
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx, "burst-provider"); err != nil {
			t.Fatalf("burst request %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("burst requests took %v, expected near-instant", elapsed)
	}
}

func TestRateLimitContextCancellation(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 60,
		BurstSize:         1,
	})

	ctx := context.Background()
	// Exhaust the single token
	_ = rl.Wait(ctx, "cancel-provider")

	// Now cancel context before next request completes
	ctx2, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := rl.Wait(ctx2, "cancel-provider")
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

func TestRateLimitUnlimited(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 0, // unlimited
	})

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if err := rl.Wait(ctx, "unlimited-provider"); err != nil {
			t.Fatalf("unlimited request %d failed: %v", i, err)
		}
	}
}

func TestRateLimitProviderDelegation(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "delegated"

	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 600, BurstSize: 10})
	wrapped := WithRateLimit(mock, rl)

	// Name delegation
	if wrapped.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", wrapped.Name())
	}

	// Ping delegation
	if err := wrapped.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}

	// Chat delegation
	resp, err := wrapped.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "test"},
	}, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "delegated" {
		t.Errorf("Chat() content = %q, want delegated", resp.Content)
	}

	// StreamChat delegation
	sr, err := wrapped.StreamChat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "test"},
	}, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("StreamChat() error: %v", err)
	}
	defer sr.Close()
	// Drain events
	for range sr.Events {
	}
}

func TestRateLimitChatBlockedByContext(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "should not see this"

	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, BurstSize: 1})
	wrapped := WithRateLimit(mock, rl)

	ctx := context.Background()
	// Exhaust the single token
	_, _ = wrapped.Chat(ctx, []GraycodeRouterMessage{{Role: "user", Content: "first"}}, ChatOptions{})

	// Now use an already-cancelled context
	ctx2, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wrapped.Chat(ctx2, []GraycodeRouterMessage{{Role: "user", Content: "blocked"}}, ChatOptions{})
	if err == nil {
		t.Error("expected error when rate limited with cancelled context")
	}
}
