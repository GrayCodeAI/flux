package observability

import (
	"math"
	"testing"
)

func TestCacheAnalyticsRecordHitIncrementsCounters(t *testing.T) {
	t.Parallel()
	ca := NewCacheAnalytics()

	ca.RecordCall(CallMetrics{
		Model:           "claude-sonnet-4-6",
		CacheReadTokens: 500,
		LatencyMs:       200,
	})

	r := ca.Report()
	if r.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", r.TotalCalls)
	}
	if r.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", r.CacheHits)
	}
	if r.CacheMisses != 0 {
		t.Errorf("CacheMisses = %d, want 0", r.CacheMisses)
	}
	if r.TokensSaved != 500 {
		t.Errorf("TokensSaved = %d, want 500", r.TokensSaved)
	}
}

func TestCacheAnalyticsRecordMissIncrementsCounters(t *testing.T) {
	t.Parallel()
	ca := NewCacheAnalytics()

	ca.RecordCall(CallMetrics{
		Model:           "claude-sonnet-4-6",
		CacheReadTokens: 0,
		LatencyMs:       200,
	})

	r := ca.Report()
	if r.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", r.TotalCalls)
	}
	if r.CacheHits != 0 {
		t.Errorf("CacheHits = %d, want 0", r.CacheHits)
	}
	if r.CacheMisses != 1 {
		t.Errorf("CacheMisses = %d, want 1", r.CacheMisses)
	}
}

func TestCacheAnalyticsHitRateCalculation(t *testing.T) {
	t.Parallel()
	ca := NewCacheAnalytics()

	// 3 hits, 1 miss => 75% hit rate
	for i := 0; i < 3; i++ {
		ca.RecordCall(CallMetrics{
			Model:           "claude-sonnet-4-6",
			CacheReadTokens: 100,
			LatencyMs:       100,
		})
	}
	ca.RecordCall(CallMetrics{
		Model:           "claude-sonnet-4-6",
		CacheReadTokens: 0,
		LatencyMs:       100,
	})

	r := ca.Report()
	if r.TotalCalls != 4 {
		t.Errorf("TotalCalls = %d, want 4", r.TotalCalls)
	}
	if r.CacheHits != 3 {
		t.Errorf("CacheHits = %d, want 3", r.CacheHits)
	}
	expectedRate := 0.75
	if math.Abs(r.HitRate-expectedRate) > 1e-9 {
		t.Errorf("HitRate = %f, want %f", r.HitRate, expectedRate)
	}
}

func TestCacheAnalyticsResetClears(t *testing.T) {
	t.Parallel()
	// The CacheAnalytics type doesn't have a Reset method,
	// but creating a new instance effectively resets state.
	ca := NewCacheAnalytics()
	ca.RecordCall(CallMetrics{
		Model:           "claude-sonnet-4-6",
		CacheReadTokens: 100,
		LatencyMs:       50,
	})

	// Verify state exists
	r := ca.Report()
	if r.TotalCalls != 1 {
		t.Fatalf("TotalCalls = %d, want 1", r.TotalCalls)
	}

	// "Reset" by creating fresh instance
	ca = NewCacheAnalytics()
	r = ca.Report()
	if r.TotalCalls != 0 {
		t.Errorf("after reset: TotalCalls = %d, want 0", r.TotalCalls)
	}
	if r.CacheHits != 0 {
		t.Errorf("after reset: CacheHits = %d, want 0", r.CacheHits)
	}
	if r.CacheMisses != 0 {
		t.Errorf("after reset: CacheMisses = %d, want 0", r.CacheMisses)
	}
	if r.HitRate != 0 {
		t.Errorf("after reset: HitRate = %f, want 0", r.HitRate)
	}
	if r.TokensSaved != 0 {
		t.Errorf("after reset: TokensSaved = %d, want 0", r.TokensSaved)
	}
}

func TestCacheAnalyticsCostSaved(t *testing.T) {
	t.Parallel()
	ca := NewCacheAnalytics()
	ca.RecordCall(CallMetrics{
		Model:           "claude-sonnet-4-6",
		CacheReadTokens: 1000,
		LatencyMs:       200,
	})

	r := ca.Report()
	if r.CostSaved <= 0 {
		t.Errorf("CostSaved = %f, want > 0", r.CostSaved)
	}
	// For sonnet: 1000 tokens * 0.9 * (3.0/1_000_000) = 0.0000027
	expectedSavings := 1000.0 * 0.9 * (3.0 / 1_000_000)
	if math.Abs(r.CostSaved-expectedSavings) > 1e-10 {
		t.Errorf("CostSaved = %f, want %f", r.CostSaved, expectedSavings)
	}
}

func TestCacheAnalyticsFormatSummaryEmpty(t *testing.T) {
	t.Parallel()
	ca := NewCacheAnalytics()
	s := ca.FormatSummary()
	if s != "" {
		t.Errorf("FormatSummary on empty analytics = %q, want empty string", s)
	}
}
