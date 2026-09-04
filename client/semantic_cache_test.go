package client

import (
	"context"
	"testing"
	"time"
)

func TestCachedProviderCacheHit(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "cached response"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	// First call: cache miss.
	resp1, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp1.Content != "cached response" {
		t.Errorf("expected 'cached response', got %q", resp1.Content)
	}
	if inner.CallCount() != 1 {
		t.Errorf("expected 1 call to inner, got %d", inner.CallCount())
	}

	// Second call: cache hit, inner should NOT be called again.
	resp2, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.Content != "cached response" {
		t.Errorf("expected 'cached response', got %q", resp2.Content)
	}
	if inner.CallCount() != 1 {
		t.Errorf("expected inner still called 1 time, got %d", inner.CallCount())
	}
}

func TestCachedProviderDifferentInputsMiss(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeEcho)

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	// Two different messages should produce two different cache keys.
	resp1, err := cp.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "hello"},
	}, ChatOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp2, err := cp.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "world"},
	}, ChatOptions{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp1.Content == resp2.Content {
		t.Errorf("expected different responses for different inputs")
	}
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls to inner, got %d", inner.CallCount())
	}
}

func TestCachedProviderHighTempSkipsCache(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "varied"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	highTemp := 0.9
	opts := ChatOptions{Model: "test-model", Temperature: &highTemp}
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}

	// Both calls should go through to inner.
	_, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls (cache skipped), got %d", inner.CallCount())
	}
}

func TestCachedProviderLowTempUsesCacheEntry(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "deterministic"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	lowTemp := 0.0
	opts := ChatOptions{Model: "test-model", Temperature: &lowTemp}
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}

	_, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.CallCount() != 1 {
		t.Errorf("expected 1 call (cache hit), got %d", inner.CallCount())
	}
}

func TestCachedProviderTTLExpiration(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "ephemeral"

	cfg := CacheConfig{
		MaxAge:  50 * time.Millisecond,
		MaxSize: 100,
		Enabled: true,
	}
	cp := NewCachedProvider(inner, cfg)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	// First call: miss.
	_, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", inner.CallCount())
	}

	// Wait for TTL to expire.
	time.Sleep(60 * time.Millisecond)

	// Second call: should be a miss (expired).
	_, err = cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls (TTL expired), got %d", inner.CallCount())
	}
}

func TestCachedProviderLRUEviction(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeEcho)

	cfg := CacheConfig{
		MaxAge:  5 * time.Minute,
		MaxSize: 3,
		Enabled: true,
	}
	cp := NewCachedProvider(inner, cfg)

	opts := ChatOptions{Model: "test-model"}

	// Fill the cache with 3 entries.
	for i := 0; i < 3; i++ {
		_, err := cp.Chat(context.Background(), []GraycodeRouterMessage{
			{Role: "user", Content: string(rune('a' + i))},
		}, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if inner.CallCount() != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.CallCount())
	}

	stats := cp.CacheStats()
	if stats.Size != 3 {
		t.Errorf("expected cache size 3, got %d", stats.Size)
	}

	// Add a 4th entry -> should evict the LRU entry.
	_, err := cp.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "d"},
	}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats = cp.CacheStats()
	if stats.Size != 3 {
		t.Errorf("expected cache size still 3 after eviction, got %d", stats.Size)
	}

	// The oldest entry ("a") should have been evicted.
	// Accessing "a" again should produce another inner call.
	callsBefore := inner.CallCount()
	_, err = cp.Chat(context.Background(), []GraycodeRouterMessage{
		{Role: "user", Content: "a"},
	}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.CallCount() != callsBefore+1 {
		t.Errorf("expected cache miss for evicted 'a', but got hit")
	}
}

func TestCachedProviderDisabled(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "uncached"

	cfg := CacheConfig{
		MaxAge:  5 * time.Minute,
		MaxSize: 100,
		Enabled: false,
	}
	cp := NewCachedProvider(inner, cfg)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	_, _ = cp.Chat(context.Background(), msgs, opts)
	_, _ = cp.Chat(context.Background(), msgs, opts)
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls (caching disabled), got %d", inner.CallCount())
	}
}

func TestCachedProviderSetEnabled(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "ok"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	// Disable at runtime.
	cp.SetEnabled(false)
	_, _ = cp.Chat(context.Background(), msgs, opts)
	_, _ = cp.Chat(context.Background(), msgs, opts)
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls with caching disabled, got %d", inner.CallCount())
	}

	// Re-enable.
	inner.Reset()
	cp.SetEnabled(true)
	_, _ = cp.Chat(context.Background(), msgs, opts)
	_, _ = cp.Chat(context.Background(), msgs, opts)
	if inner.CallCount() != 1 {
		t.Errorf("expected 1 call with caching re-enabled, got %d", inner.CallCount())
	}
}

func TestCachedProviderClearCache(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "ok"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	_, _ = cp.Chat(context.Background(), msgs, opts)
	if inner.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", inner.CallCount())
	}

	cp.ClearCache()

	_, _ = cp.Chat(context.Background(), msgs, opts)
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls after cache clear, got %d", inner.CallCount())
	}
}

func TestCachedProviderStreamNotCached(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "streamed"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	sr, err := cp.StreamChat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sr.Close()

	// Stream calls go directly to inner, no caching.
	if inner.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", inner.CallCount())
	}
}

func TestCachedProviderName(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	cp := NewCachedProvider(inner, DefaultCacheConfig())
	if cp.Name() != "mock" {
		t.Errorf("expected 'mock', got %q", cp.Name())
	}
}

func TestCachedProviderPing(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	cp := NewCachedProvider(inner, DefaultCacheConfig())
	if err := cp.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestCachedProviderDifferentModels(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "ok"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}

	_, _ = cp.Chat(context.Background(), msgs, ChatOptions{Model: "model-a"})
	_, _ = cp.Chat(context.Background(), msgs, ChatOptions{Model: "model-b"})

	// Different models should produce different cache keys.
	if inner.CallCount() != 2 {
		t.Errorf("expected 2 calls for different models, got %d", inner.CallCount())
	}
}

func TestCachedProviderResponseIsolation(t *testing.T) {
	t.Parallel()
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "original"

	cp := NewCachedProvider(inner, DefaultCacheConfig())

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	resp1, _ := cp.Chat(context.Background(), msgs, opts)
	resp1.Content = "mutated"

	resp2, _ := cp.Chat(context.Background(), msgs, opts)
	if resp2.Content != "original" {
		t.Errorf("cache was mutated: expected 'original', got %q", resp2.Content)
	}
}

func TestBuildCacheKeyDeterministic(t *testing.T) {
	t.Parallel()
	msgs := []GraycodeRouterMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	temp := 0.3
	opts := ChatOptions{Model: "gpt-4", Temperature: &temp, System: "you are a helper"}

	key1 := buildCacheKey(msgs, opts)
	key2 := buildCacheKey(msgs, opts)

	if key1 != key2 {
		t.Errorf("cache keys are not deterministic: %s != %s", key1, key2)
	}

	// Different system prompt should produce different key.
	opts2 := opts
	opts2.System = "you are a different helper"
	key3 := buildCacheKey(msgs, opts2)
	if key1 == key3 {
		t.Error("different system prompts should produce different keys")
	}
}
