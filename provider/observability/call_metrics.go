package observability

import (
	"sync"
	"time"
)

// CallMetrics records telemetry for a single LLM API call.
type CallMetrics struct {
	Model               string    `json:"model"`
	Provider            string    `json:"provider"`
	InputTokens         int       `json:"input_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	CacheReadTokens     int       `json:"cache_read_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	LatencyMs           int64     `json:"latency_ms"`
	Timestamp           time.Time `json:"timestamp"`
}

const metricsBufferSize = 100

// MetricsCollector stores recent call metrics in a ring buffer.
type MetricsCollector struct {
	mu    sync.Mutex
	buf   [metricsBufferSize]CallMetrics
	pos   int // next write position
	count int // total items written (for knowing how many are valid)
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// Record adds a new CallMetrics entry to the ring buffer.
func (mc *MetricsCollector) Record(m CallMetrics) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.buf[mc.pos] = m
	mc.pos = (mc.pos + 1) % metricsBufferSize
	mc.count++
}

// Recent returns the last n call metrics, most recent first.
// If fewer than n entries exist, all available entries are returned.
func (mc *MetricsCollector) Recent(n int) []CallMetrics {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	available := mc.count
	if available > metricsBufferSize {
		available = metricsBufferSize
	}
	if n > available {
		n = available
	}
	if n <= 0 {
		return nil
	}

	result := make([]CallMetrics, n)
	for i := 0; i < n; i++ {
		// Walk backwards from the most recently written position
		idx := (mc.pos - 1 - i + metricsBufferSize) % metricsBufferSize
		result[i] = mc.buf[idx]
	}
	return result
}

// TotalCost estimates the total cost across all recorded metrics using
// a simplified pricing model (per 1M tokens):
//   - Input tokens: $3.00 / 1M
//   - Output tokens: $15.00 / 1M
//   - Cache read tokens: $0.30 / 1M
//   - Cache creation tokens: $3.75 / 1M
func (mc *MetricsCollector) TotalCost() float64 {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	available := mc.count
	if available > metricsBufferSize {
		available = metricsBufferSize
	}

	var cost float64
	for i := 0; i < available; i++ {
		idx := (mc.pos - 1 - i + metricsBufferSize) % metricsBufferSize
		m := mc.buf[idx]
		cost += float64(m.InputTokens) * 3.0 / 1_000_000
		cost += float64(m.OutputTokens) * 15.0 / 1_000_000
		cost += float64(m.CacheReadTokens) * 0.3 / 1_000_000
		cost += float64(m.CacheCreationTokens) * 3.75 / 1_000_000
	}
	return cost
}
