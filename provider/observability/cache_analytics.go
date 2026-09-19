package observability

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// CacheAnalytics tracks prompt caching effectiveness and cost savings.
// Shows developers exactly how much money and latency caching saves.
type CacheAnalytics struct {
	mu           sync.Mutex
	totalCalls   int
	cacheHits    int
	cacheMisses  int
	tokensSaved  int
	costSaved    float64
	latencySaved time.Duration
}

// CacheReport summarizes caching effectiveness.
type CacheReport struct {
	TotalCalls   int           `json:"total_calls"`
	CacheHits    int           `json:"cache_hits"`
	CacheMisses  int           `json:"cache_misses"`
	HitRate      float64       `json:"hit_rate"`
	TokensSaved  int           `json:"tokens_saved"`
	CostSaved    float64       `json:"cost_saved_usd"`
	LatencySaved time.Duration `json:"latency_saved"`
}

// NewCacheAnalytics creates a cache analytics tracker.
func NewCacheAnalytics() *CacheAnalytics {
	return &CacheAnalytics{}
}

// RecordCall records a call's cache usage from its metrics.
func (ca *CacheAnalytics) RecordCall(m CallMetrics) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.totalCalls++

	if m.CacheReadTokens > 0 {
		ca.cacheHits++
		ca.tokensSaved += m.CacheReadTokens
		// Cached tokens cost 90% less on Anthropic
		ca.costSaved += float64(m.CacheReadTokens) * 0.9 * pricePerToken(m.Model, true)
		// Cached responses are ~2x faster
		ca.latencySaved += time.Duration(m.LatencyMs/2) * time.Millisecond
	} else {
		ca.cacheMisses++
	}
}

// Report returns the current cache effectiveness report.
func (ca *CacheAnalytics) Report() CacheReport {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	r := CacheReport{
		TotalCalls:   ca.totalCalls,
		CacheHits:    ca.cacheHits,
		CacheMisses:  ca.cacheMisses,
		TokensSaved:  ca.tokensSaved,
		CostSaved:    ca.costSaved,
		LatencySaved: ca.latencySaved,
	}
	if ca.totalCalls > 0 {
		r.HitRate = float64(ca.cacheHits) / float64(ca.totalCalls)
	}
	return r
}

// FormatSummary returns a human-readable summary.
func (ca *CacheAnalytics) FormatSummary() string {
	r := ca.Report()
	if r.TotalCalls == 0 {
		return ""
	}
	return fmt.Sprintf("Cache: %.0f%% hit rate (%d/%d), saved %d tokens ($%.4f, %s)",
		r.HitRate*100, r.CacheHits, r.TotalCalls, r.TokensSaved, r.CostSaved, r.LatencySaved.Round(time.Millisecond))
}

func pricePerToken(model string, isInput bool) float64 {
	// Simplified pricing (per token, not per million)
	switch {
	case strings.Contains(model, "opus"):
		if isInput {
			return 15.0 / 1_000_000
		}
		return 75.0 / 1_000_000
	case strings.Contains(model, "sonnet"):
		if isInput {
			return 3.0 / 1_000_000
		}
		return 15.0 / 1_000_000
	case strings.Contains(model, "haiku"):
		if isInput {
			return 0.25 / 1_000_000
		}
		return 1.25 / 1_000_000
	case strings.Contains(model, "gpt-4"):
		if isInput {
			return 2.5 / 1_000_000
		}
		return 10.0 / 1_000_000
	case strings.Contains(model, "gemini"):
		if isInput {
			return 1.25 / 1_000_000
		}
		return 5.0 / 1_000_000
	case strings.Contains(model, "grok"):
		if isInput {
			return 2.0 / 1_000_000
		}
		return 10.0 / 1_000_000
	case strings.Contains(model, "deepseek"):
		if isInput {
			return 0.14 / 1_000_000
		}
		return 0.28 / 1_000_000
	case strings.Contains(model, "llama"), strings.Contains(model, "mistral"):
		if isInput {
			return 0.20 / 1_000_000
		}
		return 0.20 / 1_000_000
	default:
		if isInput {
			return 1.0 / 1_000_000
		}
		return 3.0 / 1_000_000
	}
}

// PricePerToken returns the catalog-derived USD price for one token.
func PricePerToken(model string, isInput bool) float64 {
	return pricePerToken(model, isInput)
}
