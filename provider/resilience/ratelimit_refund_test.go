package resilience

import (
	"context"
	"testing"
	"time"
)

// TestTokenBucketRefundsTokenOnCancelDuringMinInterval verifies that when a
// caller's context is cancelled while wait() is parked on the minInterval
// spacing timer, the token optimistically consumed (b.tokens--) is returned to
// the bucket rather than being silently lost.
func TestTokenBucketRefundsTokenOnCancelDuringMinInterval(t *testing.T) {
	t.Parallel()

	// refillRate 0 keeps the count deterministic (no background refill), and a
	// long minInterval guarantees the second call parks on the spacing timer so
	// the test can cancel it mid-wait via a much shorter context timeout.
	b := &tokenBucket{
		tokens:      2,
		maxTokens:   2,
		refillRate:  0,
		lastRefill:  time.Now(),
		minInterval: 10 * time.Second,
	}

	// First call consumes a token (2 -> 1) and stamps lastRequest. It does not
	// wait: lastRequest was the zero time, so the elapsed interval far exceeds
	// minInterval.
	if err := b.wait(context.Background()); err != nil {
		t.Fatalf("first wait: unexpected error: %v", err)
	}

	// Second call consumes a token (1 -> 0) and then blocks on the 10s spacing
	// timer. The 30ms context cancels well before it fires, so wait() exits
	// through the refund path.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := b.wait(ctx); err == nil {
		t.Fatal("second wait: want context error, got nil")
	}

	b.mu.Lock()
	got := b.tokens
	b.mu.Unlock()

	// After first consume (2 -> 1) and the second consume+refund (1 -> 0 -> 1)
	// the bucket must be back to 1. Without the refund it would be 0.
	if got < 0.999 || got > 1.001 {
		t.Fatalf("tokens after cancelled min-interval wait = %v, want 1 (token refunded)", got)
	}
}
