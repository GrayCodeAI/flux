package observability

import (
	"math"
	"testing"
	"time"
)

func TestMetricsCollector_Record_and_Recent(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	// Record 3 entries
	mc.Record(CallMetrics{Model: "claude-4", Provider: "anthropic", InputTokens: 100, OutputTokens: 50, LatencyMs: 200, Timestamp: time.Now()})
	mc.Record(CallMetrics{Model: "claude-4", Provider: "anthropic", InputTokens: 200, OutputTokens: 100, LatencyMs: 300, Timestamp: time.Now()})
	mc.Record(CallMetrics{Model: "gpt-4", Provider: "openai", InputTokens: 300, OutputTokens: 150, LatencyMs: 400, Timestamp: time.Now()})

	// Recent(2) should return last 2, most recent first
	recent := mc.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent, got %d", len(recent))
	}
	if recent[0].Model != "gpt-4" {
		t.Errorf("most recent should be gpt-4, got %q", recent[0].Model)
	}
	if recent[1].InputTokens != 200 {
		t.Errorf("second most recent should have 200 input tokens, got %d", recent[1].InputTokens)
	}

	// Recent(10) should return all 3
	all := mc.Recent(10)
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
}

func TestMetricsCollector_RingBuffer_Wraps(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	// Fill beyond buffer capacity
	for i := 0; i < 150; i++ {
		mc.Record(CallMetrics{
			Model:       "model",
			InputTokens: i,
			Timestamp:   time.Now(),
		})
	}

	// Should only have last 100
	recent := mc.Recent(200)
	if len(recent) != 100 {
		t.Fatalf("expected 100 (buffer cap), got %d", len(recent))
	}

	// Most recent should be i=149
	if recent[0].InputTokens != 149 {
		t.Errorf("most recent should be 149, got %d", recent[0].InputTokens)
	}
	// Oldest in buffer should be i=50
	if recent[99].InputTokens != 50 {
		t.Errorf("oldest should be 50, got %d", recent[99].InputTokens)
	}
}

func TestMetricsCollector_TotalCost(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.Record(CallMetrics{
		InputTokens:         1000000, // $3.00
		OutputTokens:        1000000, // $15.00
		CacheReadTokens:     1000000, // $0.30
		CacheCreationTokens: 1000000, // $3.75
		Timestamp:           time.Now(),
	})

	cost := mc.TotalCost()
	expected := 3.0 + 15.0 + 0.3 + 3.75 // $22.05
	if math.Abs(cost-expected) > 0.001 {
		t.Errorf("expected cost %.4f, got %.4f", expected, cost)
	}
}

func TestMetricsCollector_Empty(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	recent := mc.Recent(5)
	if len(recent) != 0 {
		t.Errorf("expected empty, got %d", len(recent))
	}

	cost := mc.TotalCost()
	if cost != 0 {
		t.Errorf("expected 0 cost, got %f", cost)
	}
}
