package graycoderouter

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockChatFn creates a chat function that records calls and optionally returns an error.
func mockChatFn(calls *int64, returnErr error) func(ctx context.Context, messages []Message, opts ChatOptions) error {
	return func(ctx context.Context, messages []Message, opts ChatOptions) error {
		atomic.AddInt64(calls, 1)
		return returnErr
	}
}

func TestNewCacheWarmer(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "You are a helpful assistant.", "anthropic", "claude-sonnet-4-20250514")

	if cw.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", cw.Provider)
	}
	if cw.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", cw.Model)
	}
	if cw.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("unexpected system prompt: %q", cw.SystemPrompt)
	}
	if cw.Interval != DefaultWarmInterval {
		t.Errorf("expected interval %v, got %v", DefaultWarmInterval, cw.Interval)
	}
	if !cw.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestStartStop(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system prompt", "anthropic", "claude-sonnet-4-20250514")
	cw.Interval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := cw.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Wait for initial warm + at least one tick
	time.Sleep(120 * time.Millisecond)

	cw.Stop()

	// Give goroutine time to exit
	time.Sleep(20 * time.Millisecond)

	callCount := atomic.LoadInt64(&calls)
	if callCount < 2 {
		t.Errorf("expected at least 2 calls (initial + tick), got %d", callCount)
	}

	// Verify Stop is idempotent
	cw.Stop()
}

func TestStartAlreadyRunning(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")
	cw.Interval = 1 * time.Second

	ctx := context.Background()
	_ = cw.Start(ctx)
	defer cw.Stop()

	// Second start should be a no-op
	err := cw.Start(ctx)
	if err != nil {
		t.Fatalf("second Start should not error: %v", err)
	}
}

func TestNonAnthropicProviderNoOp(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system prompt", "openai", "gpt-4")
	cw.Interval = 10 * time.Millisecond

	ctx := context.Background()
	err := cw.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	cw.Stop()

	if atomic.LoadInt64(&calls) != 0 {
		t.Errorf("expected 0 calls for non-anthropic provider, got %d", calls)
	}
}

func TestWarmSendsCorrectRequest(t *testing.T) {
	t.Parallel()
	var capturedMessages []Message
	var capturedOpts ChatOptions
	var mu sync.Mutex

	fn := func(ctx context.Context, messages []Message, opts ChatOptions) error {
		mu.Lock()
		capturedMessages = messages
		capturedOpts = opts
		mu.Unlock()
		return nil
	}

	cw := NewCacheWarmer(fn, "You are a coding assistant.", "anthropic", "claude-sonnet-4-20250514")

	err := cw.Warm(context.Background())
	if err != nil {
		t.Fatalf("Warm returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(capturedMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", capturedMessages[0].Role)
	}
	if capturedMessages[0].Content != "ping" {
		t.Errorf("expected content 'ping', got %q", capturedMessages[0].Content)
	}
	if capturedOpts.MaxTokens != 1 {
		t.Errorf("expected MaxTokens=1, got %d", capturedOpts.MaxTokens)
	}
	if capturedOpts.System != "You are a coding assistant." {
		t.Errorf("expected system prompt in opts, got %q", capturedOpts.System)
	}
	if !capturedOpts.EnableCaching {
		t.Error("expected EnableCaching=true")
	}
	if capturedOpts.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", capturedOpts.Model)
	}
}

func TestWarmNonAnthropicNoOp(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "openai", "gpt-4")

	err := cw.Warm(context.Background())
	if err != nil {
		t.Fatalf("Warm returned error: %v", err)
	}
	if atomic.LoadInt64(&calls) != 0 {
		t.Errorf("expected 0 calls for non-anthropic, got %d", calls)
	}
}

func TestShouldWarmTimingLogic(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")
	cw.Interval = 100 * time.Millisecond

	// Should warm when never warmed
	if !cw.ShouldWarm() {
		t.Error("expected ShouldWarm=true when never warmed")
	}

	// Warm once
	_ = cw.Warm(context.Background())

	// Immediately after warming, should NOT need warming
	if cw.ShouldWarm() {
		t.Error("expected ShouldWarm=false immediately after warming")
	}

	// Wait for interval to pass
	time.Sleep(120 * time.Millisecond)

	// Now should need warming again
	if !cw.ShouldWarm() {
		t.Error("expected ShouldWarm=true after interval elapsed")
	}
}

func TestShouldWarmNonAnthropic(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "openai", "gpt-4")

	if cw.ShouldWarm() {
		t.Error("expected ShouldWarm=false for non-anthropic provider")
	}
}

func TestStatsTracking(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")

	ctx := context.Background()

	// First warm: should be a cache miss (first request)
	_ = cw.Warm(ctx)

	cw.mu.Lock()
	if cw.Stats.WarmingRequests != 1 {
		t.Errorf("expected 1 warming request, got %d", cw.Stats.WarmingRequests)
	}
	if cw.Stats.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss on first request, got %d", cw.Stats.CacheMisses)
	}
	if cw.Stats.CacheHits != 0 {
		t.Errorf("expected 0 cache hits on first request, got %d", cw.Stats.CacheHits)
	}
	if cw.Stats.LastWarmedAt.IsZero() {
		t.Error("expected LastWarmedAt to be set")
	}
	cw.mu.Unlock()

	// Second warm: should be a cache hit
	_ = cw.Warm(ctx)

	cw.mu.Lock()
	if cw.Stats.WarmingRequests != 2 {
		t.Errorf("expected 2 warming requests, got %d", cw.Stats.WarmingRequests)
	}
	if cw.Stats.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", cw.Stats.CacheHits)
	}
	cw.mu.Unlock()
}

func TestStatsTrackingWithError(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, context.DeadlineExceeded)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")

	_ = cw.Warm(context.Background())
	_ = cw.Warm(context.Background())

	cw.mu.Lock()
	if cw.Stats.CacheMisses != 2 {
		t.Errorf("expected 2 cache misses on error, got %d", cw.Stats.CacheMisses)
	}
	if cw.Stats.CacheHits != 0 {
		t.Errorf("expected 0 cache hits on error, got %d", cw.Stats.CacheHits)
	}
	cw.mu.Unlock()
}

func TestEstimateSavings(t *testing.T) {
	t.Parallel()
	cw := NewCacheWarmer(nil, "", "anthropic", "model")

	tests := []struct {
		name         string
		inputTokens  int
		requestCount int
		wantPositive bool
		wantZero     bool
	}{
		{
			name:         "zero tokens",
			inputTokens:  0,
			requestCount: 10,
			wantZero:     true,
		},
		{
			name:         "zero requests",
			inputTokens:  1000,
			requestCount: 0,
			wantZero:     true,
		},
		{
			name:         "single request no savings",
			inputTokens:  1000,
			requestCount: 1,
			wantPositive: false,
			wantZero:     false, // actually negative (cache write is more expensive)
		},
		{
			name:         "many requests significant savings",
			inputTokens:  10000,
			requestCount: 100,
			wantPositive: true,
		},
		{
			name:         "two requests small savings",
			inputTokens:  5000,
			requestCount: 2,
			wantPositive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			savings := cw.EstimateSavings(tt.inputTokens, tt.requestCount)
			if tt.wantZero && savings != 0 {
				t.Errorf("expected 0 savings, got %f", savings)
			}
			if tt.wantPositive && savings <= 0 {
				t.Errorf("expected positive savings, got %f", savings)
			}
		})
	}

	// Verify specific calculation:
	// 10000 tokens, 10 requests
	// Without cache: 10000 * 10 * 3/1M = 0.3
	// With cache: (10000 * 3/1M * 1.25) + (10000 * 9 * 3/1M * 0.1) = 0.0375 + 0.027 = 0.0645
	// Savings: 0.3 - 0.0645 = 0.0
	savings := cw.EstimateSavings(10000, 10)
	expected := 0.3 - (10000.0*3.0/1_000_000*1.25 + 10000.0*9.0*3.0/1_000_000*0.1)
	if math.Abs(savings-expected) > 0.000001 {
		t.Errorf("expected savings %f, got %f", expected, savings)
	}
}

func TestEstimateSavingsNegativeForSingleRequest(t *testing.T) {
	t.Parallel()
	cw := NewCacheWarmer(nil, "", "anthropic", "model")

	// Single request: cache write (1.25x) costs more than no cache (1x)
	savings := cw.EstimateSavings(1000, 1)
	if savings >= 0 {
		t.Errorf("expected negative savings for single request (cache write more expensive), got %f", savings)
	}
}

func TestCacheBreakpoints(t *testing.T) {
	t.Parallel()
	cw := NewCacheWarmer(nil, "", "anthropic", "model")

	t.Run("empty messages", func(t *testing.T) {
		bp := cw.CacheBreakpoints("system prompt", nil)
		if len(bp) != 1 {
			t.Fatalf("expected 1 breakpoint for system prompt, got %d", len(bp))
		}
		if bp[0] != -1 {
			t.Errorf("expected -1 for system prompt breakpoint, got %d", bp[0])
		}
	})

	t.Run("no system prompt", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "hello"},
		}
		bp := cw.CacheBreakpoints("", messages)
		if len(bp) != 1 {
			t.Fatalf("expected 1 breakpoint, got %d", len(bp))
		}
		if bp[0] != 0 {
			t.Errorf("expected breakpoint at index 0 (first user), got %d", bp[0])
		}
	})

	t.Run("system prompt and first user message", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		}
		bp := cw.CacheBreakpoints("system", messages)
		// Should have: -1 (system), 0 (first user)
		if len(bp) != 2 {
			t.Fatalf("expected 2 breakpoints, got %d: %v", len(bp), bp)
		}
		if bp[0] != -1 {
			t.Errorf("expected first breakpoint at -1, got %d", bp[0])
		}
		if bp[1] != 0 {
			t.Errorf("expected second breakpoint at 0, got %d", bp[1])
		}
	})

	t.Run("large content blocks", func(t *testing.T) {
		largeContent := make([]byte, 300)
		for i := range largeContent {
			largeContent[i] = 'a'
		}
		messages := []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: string(largeContent)},
			{Role: "user", Content: string(largeContent)},
			{Role: "assistant", Content: "short"},
		}
		bp := cw.CacheBreakpoints("system", messages)
		// Should have: -1 (system), 0 (first user), 1 (large), 2 (large)
		if len(bp) != 4 {
			t.Fatalf("expected 4 breakpoints, got %d: %v", len(bp), bp)
		}
	})

	t.Run("max breakpoints enforced", func(t *testing.T) {
		largeContent := make([]byte, 300)
		for i := range largeContent {
			largeContent[i] = 'x'
		}
		messages := []Message{
			{Role: "user", Content: string(largeContent)},
			{Role: "assistant", Content: string(largeContent)},
			{Role: "user", Content: string(largeContent)},
			{Role: "assistant", Content: string(largeContent)},
			{Role: "user", Content: string(largeContent)},
			{Role: "assistant", Content: string(largeContent)},
		}
		bp := cw.CacheBreakpoints("system", messages)
		if len(bp) > maxCacheBreakpoints {
			t.Errorf("expected at most %d breakpoints, got %d", maxCacheBreakpoints, len(bp))
		}
	})
}

func TestContextCancellationStopsWarmer(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")
	cw.Interval = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	_ = cw.Start(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Give goroutine time to notice cancellation
	time.Sleep(20 * time.Millisecond)

	// Record call count after cancellation
	countAfterCancel := atomic.LoadInt64(&calls)

	// Wait a bit more — no more calls should happen
	time.Sleep(80 * time.Millisecond)
	countLater := atomic.LoadInt64(&calls)

	if countLater > countAfterCancel {
		t.Errorf("expected no more calls after cancel, but count went from %d to %d", countAfterCancel, countLater)
	}

	// Cleanup
	cw.Stop()
}

func TestConcurrentStatsAccess(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")

	ctx := context.Background()
	var wg sync.WaitGroup

	// Multiple goroutines warming concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cw.Warm(ctx)
		}()
	}

	// Concurrent reads of ShouldWarm
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cw.ShouldWarm()
		}()
	}

	wg.Wait()

	cw.mu.Lock()
	totalRequests := cw.Stats.WarmingRequests
	totalHits := cw.Stats.CacheHits
	totalMisses := cw.Stats.CacheMisses
	cw.mu.Unlock()

	if totalRequests != 20 {
		t.Errorf("expected 20 total requests, got %d", totalRequests)
	}
	if totalHits+totalMisses != 20 {
		t.Errorf("expected hits+misses=20, got hits=%d misses=%d", totalHits, totalMisses)
	}
}

func TestWarmWithNilChatFn(t *testing.T) {
	t.Parallel()
	cw := NewCacheWarmer(nil, "system", "anthropic", "model")

	err := cw.Warm(context.Background())
	if err != nil {
		t.Errorf("expected nil error with nil ChatFn, got %v", err)
	}
}

func TestDisabledWarmerDoesNotPing(t *testing.T) {
	t.Parallel()
	var calls int64
	fn := mockChatFn(&calls, nil)
	cw := NewCacheWarmer(fn, "system", "anthropic", "model")
	cw.Interval = 30 * time.Millisecond
	cw.Enabled = false

	ctx := context.Background()
	_ = cw.Start(ctx)

	// Initial Warm still fires because Start calls Warm directly.
	// But ticks should not fire since Enabled=false.
	time.Sleep(100 * time.Millisecond)
	cw.Stop()

	// Only the initial warm should have been called
	count := atomic.LoadInt64(&calls)
	if count != 1 {
		t.Errorf("expected exactly 1 call (initial warm), got %d", count)
	}
}
