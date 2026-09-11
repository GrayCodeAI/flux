package client

import (
	"math"
	"testing"
)

func TestCostEstimateForKnownModels(t *testing.T) {
	t.Parallel()
	ce := NewCostEstimator()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello, how are you doing today?"},
	}

	tests := []struct {
		model string
	}{
		{"claude-opus-4-6"},
		{"claude-sonnet-4-6"},
		{"claude-haiku-4-5"},
	}

	for _, tc := range tests {
		est := ce.Estimate(messages, tc.model, 1000)
		if est.InputCostUSD <= 0 {
			t.Errorf("model=%s: InputCostUSD = %f, want > 0", tc.model, est.InputCostUSD)
		}
		if est.OutputCostUSD <= 0 {
			t.Errorf("model=%s: OutputCostUSD = %f, want > 0", tc.model, est.OutputCostUSD)
		}
		if math.Abs(est.TotalCostUSD-(est.InputCostUSD+est.OutputCostUSD)) > 1e-12 {
			t.Errorf("model=%s: TotalCostUSD = %f, want %f", tc.model, est.TotalCostUSD, est.InputCostUSD+est.OutputCostUSD)
		}
		if est.Model != tc.model {
			t.Errorf("model=%s: est.Model = %q", tc.model, est.Model)
		}
	}

	// Opus should cost more than Sonnet
	opusEst := ce.Estimate(messages, "claude-opus-4-6", 1000)
	sonnetEst := ce.Estimate(messages, "claude-sonnet-4-6", 1000)
	if opusEst.TotalCostUSD <= sonnetEst.TotalCostUSD {
		t.Errorf("Opus (%f) should cost more than Sonnet (%f)", opusEst.TotalCostUSD, sonnetEst.TotalCostUSD)
	}
}

func TestCostEstimateWithCacheTokens(t *testing.T) {
	t.Parallel()
	ce := NewCostEstimator()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello, this is a test message for caching."},
	}

	est := ce.Estimate(messages, "claude-sonnet-4-6", 1000)

	// CacheDiscount should be 90% of input cost
	expectedDiscount := est.InputCostUSD * 0.9
	if math.Abs(est.CacheDiscount-expectedDiscount) > 1e-12 {
		t.Errorf("CacheDiscount = %f, want %f (90%% of input cost)", est.CacheDiscount, expectedDiscount)
	}
}

func TestCostEstimateUnknownModelReturnsNonZero(t *testing.T) {
	t.Parallel()
	// The code uses default pricing for unknown models ($1/MTok in, $3/MTok out)
	// so it doesn't return zero. Let's verify it uses the default tier.
	ce := NewCostEstimator()
	messages := []EyrieMessage{
		{Role: "user", Content: "test message here"},
	}

	est := ce.Estimate(messages, "unknown-model-xyz", 1000)
	// Default: $1/MTok input, $3/MTok output
	expectedInPrice := 1.0 / 1_000_000
	expectedOutPrice := 3.0 / 1_000_000

	inputTokens := estimateTextTokens("test message here")
	expectedInput := float64(inputTokens) * expectedInPrice
	expectedOutput := float64(1000) * expectedOutPrice

	if math.Abs(est.InputCostUSD-expectedInput) > 1e-12 {
		t.Errorf("unknown model InputCostUSD = %f, want %f", est.InputCostUSD, expectedInput)
	}
	if math.Abs(est.OutputCostUSD-expectedOutput) > 1e-12 {
		t.Errorf("unknown model OutputCostUSD = %f, want %f", est.OutputCostUSD, expectedOutput)
	}
}

func TestCostStreamingTokenCounterWithCache(t *testing.T) {
	t.Parallel()
	stc := NewStreamingTokenCounter("claude-sonnet-4-6", 1000)
	stc.AddCached(800)
	stc.AddOutput("Hello world, this is output text")

	cost := stc.CurrentCost()
	if cost <= 0 {
		t.Errorf("CurrentCost = %f, want > 0", cost)
	}

	// Verify cached tokens reduce cost: compare with no cache
	stcNoCache := NewStreamingTokenCounter("claude-sonnet-4-6", 1000)
	stcNoCache.AddOutput("Hello world, this is output text")
	costNoCache := stcNoCache.CurrentCost()

	if cost >= costNoCache {
		t.Errorf("cost with cache (%f) should be less than without cache (%f)", cost, costNoCache)
	}
}

func TestCostIsExpensive(t *testing.T) {
	t.Parallel()
	ce := NewCostEstimator()
	messages := []EyrieMessage{
		{Role: "user", Content: "test"},
	}
	est := ce.Estimate(messages, "claude-opus-4-6", 100000)

	if !ce.IsExpensive(est, 0.001) {
		t.Error("opus with 100k output tokens should be expensive at $0.001 threshold")
	}
	if ce.IsExpensive(est, 100.0) {
		t.Error("should not be expensive at $100 threshold")
	}
}
