package concentrate

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPricingCache_SetAndGet(t *testing.T) {
	// Use a temp dir for testing (no Parallel — uses t.Setenv)
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	// Reset cache to pick up new dir
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	SetPricing("claude-opus-5", PricingCacheEntry{
		InputPrice:  15.0,
		OutputPrice: 75.0,
	})
	entry, ok := GetPricing("claude-opus-5")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if entry.InputPrice != 15.0 || entry.OutputPrice != 75.0 {
		t.Errorf("pricing = (%f, %f), want (15, 75)", entry.InputPrice, entry.OutputPrice)
	}
}

func TestPricingCache_Miss(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	_, ok := GetPricing("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestPricingCache_Expired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	// Write an entry directly with old timestamp
	cacheFile := filepath.Join(cacheDir(), "concentrate-pricing.json")
	old := `{"expired-model":{"input_price":1.0,"output_price":2.0,"cached_at":"2020-01-01T00:00:00Z"}}`
	os.WriteFile(cacheFile, []byte(old), 0o644)

	_, ok := GetPricing("expired-model")
	if ok {
		t.Error("expected cache miss for expired entry")
	}
}

func TestPricingCache_Persistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	SetPricing("gpt-5", PricingCacheEntry{
		InputPrice:  10.0,
		OutputPrice: 30.0,
	})

	// Simulate a new process by resetting the in-memory cache but keeping the file
	defaultCache = nil
	defaultCacheOnce = sync.Once{}

	entry, ok := GetPricing("gpt-5")
	if !ok {
		t.Fatal("expected cache hit after reload")
	}
	if entry.InputPrice != 10.0 || entry.OutputPrice != 30.0 {
		t.Errorf("pricing = (%f, %f), want (10, 30)", entry.InputPrice, entry.OutputPrice)
	}
}

func TestPricingCache_Reset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	SetPricing("test-model", PricingCacheEntry{
		InputPrice:  5.0,
		OutputPrice: 10.0,
	})
	ResetCache()
	_, ok := GetPricing("test-model")
	if ok {
		t.Error("expected cache miss after reset")
	}
}

func TestPricingCache_WithReasoningAndTiers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	defaultCache = nil
	defaultCacheOnce = sync.Once{}
	defer func() {
		defaultCache = nil
		defaultCacheOnce = sync.Once{}
	}()

	SetPricing("claude-opus-5", PricingCacheEntry{
		InputPrice:        15.0,
		OutputPrice:       75.0,
		ReasoningPrice:    150.0,
		TierThreshold:     200000,
		TieredInputPrice:  7.5,
		TieredOutputPrice: 37.5,
	})

	entry, ok := GetPricing("claude-opus-5")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if entry.ReasoningPrice != 150.0 {
		t.Errorf("reasoning price = %f, want 150", entry.ReasoningPrice)
	}
	if entry.TierThreshold != 200000 {
		t.Errorf("tier threshold = %d, want 200000", entry.TierThreshold)
	}
	if entry.TieredInputPrice != 7.5 {
		t.Errorf("tiered input = %f, want 7.5", entry.TieredInputPrice)
	}
	if entry.TieredOutputPrice != 37.5 {
		t.Errorf("tiered output = %f, want 37.5", entry.TieredOutputPrice)
	}
}
