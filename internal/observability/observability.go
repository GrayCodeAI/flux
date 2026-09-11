// Package eyrie observability provides OpenTelemetry-compatible structured
// tracing and metrics collection for all LLM provider calls.
//
// Design follows OpenTelemetry Go SDK patterns (TraceID/SpanID, span lifecycle,
// metric instruments) while remaining zero-dependency (Go stdlib only).
//
// Usage is opt-in: a nil *Telemetry adds zero overhead.
package eyrie

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- Standard span names (OpenTelemetry semantic conventions for GenAI) ---

const (
	SpanLLMChat     = "llm.chat"
	SpanLLMStream   = "llm.stream"
	SpanLLMRetry    = "llm.retry"
	SpanLLMCacheHit = "llm.cache_hit"
)

// --- Standard attribute keys ---

const (
	AttrLLMProvider     = "llm.provider"
	AttrLLMModel        = "llm.model"
	AttrLLMInputTokens  = "llm.input_tokens"  // #nosec G101 -- OTel attribute key string, not a secret value
	AttrLLMOutputTokens = "llm.output_tokens" // #nosec G101 -- OTel attribute key string, not a secret value
	AttrLLMCostUSD      = "llm.cost_usd"
	AttrLLMLatencyMs    = "llm.latency_ms"
	AttrLLMStatus       = "llm.status"
)

// SpanStatus represents the outcome of a span.
type SpanStatus string

const (
	StatusOK    SpanStatus = "ok"
	StatusError SpanStatus = "error"
	StatusUnset SpanStatus = "unset"
)

// --- Span ---

// Span represents a single timed operation in a trace, modeled after
// OpenTelemetry's Span concept. It carries a TraceID, SpanID, timing
// information, and arbitrary string attributes.
type Span struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	Name       string            `json:"name"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Provider   string            `json:"provider,omitempty"`
	Model      string            `json:"model,omitempty"`
	Status     SpanStatus        `json:"status"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Events     []SpanEvent       `json:"events,omitempty"`
}

// SpanEvent records a timestamped event within a span.
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Duration returns the span duration. Returns 0 if not yet ended.
func (s *Span) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// SetAttribute sets a single attribute on the span.
func (s *Span) SetAttribute(key, value string) {
	if s.Attributes == nil {
		s.Attributes = make(map[string]string)
	}
	s.Attributes[key] = value
}

// AddEvent adds a timestamped event to the span.
func (s *Span) AddEvent(name string, attrs map[string]string) {
	s.Events = append(s.Events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	})
}

// --- Telemetry ---

// Telemetry wraps observability for all LLM calls. A nil *Telemetry is safe
// to use and adds zero overhead (all methods are no-ops on nil receiver).
type Telemetry struct {
	mu      sync.Mutex
	spans   []*Span
	metrics *MetricsCollector

	// OnSpanEnd is an optional callback invoked when a span ends.
	// Useful for exporting spans to external systems.
	OnSpanEnd func(*Span)
}

const maxTelemetrySpans = 10000 // cap to prevent unbounded memory growth

// NewTelemetry creates a new Telemetry instance with an initialized
// MetricsCollector.
func NewTelemetry() *Telemetry {
	return &Telemetry{
		spans:   make([]*Span, 0, 64),
		metrics: NewMetricsCollector(),
	}
}

// Metrics returns the underlying MetricsCollector, or nil if Telemetry is nil.
func (t *Telemetry) Metrics() *MetricsCollector {
	if t == nil {
		return nil
	}
	return t.metrics
}

// StartSpan creates and starts a new Span with the given name and attributes.
// Returns nil if the Telemetry receiver is nil (opt-in pattern).
func (t *Telemetry) StartSpan(name string, attrs map[string]string) *Span {
	if t == nil {
		return nil
	}
	span := &Span{
		TraceID:    generateTraceID(),
		SpanID:     generateSpanID(),
		Name:       name,
		StartTime:  time.Now(),
		Status:     StatusUnset,
		Attributes: make(map[string]string),
	}
	for k, v := range attrs {
		span.Attributes[k] = v
	}
	// Extract provider/model from attributes for convenience.
	if p, ok := attrs[AttrLLMProvider]; ok {
		span.Provider = p
	}
	if m, ok := attrs[AttrLLMModel]; ok {
		span.Model = m
	}
	return span
}

// EndSpan completes a span, recording its status and duration. If err is
// non-nil, the span status is set to error and the error message is recorded.
// No-op if either t or span is nil.
func (t *Telemetry) EndSpan(span *Span, err error) {
	if t == nil || span == nil {
		return
	}
	span.EndTime = time.Now()
	if err != nil {
		span.Status = StatusError
		span.SetAttribute("error.message", err.Error())
	} else {
		span.Status = StatusOK
	}
	// Record latency attribute.
	latencyMs := span.Duration().Milliseconds()
	span.SetAttribute(AttrLLMLatencyMs, fmt.Sprintf("%d", latencyMs))
	span.SetAttribute(AttrLLMStatus, string(span.Status))

	t.mu.Lock()
	t.spans = append(t.spans, span)
	// Evict oldest half when cap exceeded to prevent unbounded growth
	if len(t.spans) > maxTelemetrySpans {
		t.spans = t.spans[maxTelemetrySpans/2:]
	}
	t.mu.Unlock()

	// Update metrics from span attributes.
	t.recordSpanMetrics(span)

	if t.OnSpanEnd != nil {
		t.OnSpanEnd(span)
	}
}

// RecordMetric records a named metric value with attributes. This is a
// general-purpose method for recording custom metrics.
// No-op if t is nil.
func (t *Telemetry) RecordMetric(name string, value float64, attrs map[string]string) {
	if t == nil {
		return
	}
	t.metrics.RecordCustom(name, value, attrs)
}

// Spans returns a copy of all completed spans.
func (t *Telemetry) Spans() []*Span {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]*Span, len(t.spans))
	copy(cp, t.spans)
	return cp
}

// recordSpanMetrics updates the MetricsCollector based on span data.
func (t *Telemetry) recordSpanMetrics(span *Span) {
	if t.metrics == nil {
		return
	}
	provider := span.Provider
	model := span.Model
	key := provider + "/" + model

	// Request count
	t.metrics.incrRequestCount(key)

	// Latency
	t.metrics.recordLatency(key, span.Duration())

	// Error rate
	if span.Status == StatusError {
		t.metrics.incrErrors(provider)
	}

	// Cache hit
	if span.Name == SpanLLMCacheHit {
		t.metrics.incrCacheHits()
	}
	t.metrics.incrCacheTotal()
}

// --- MetricsCollector ---

// MetricsCollector aggregates metrics for LLM operations. It uses atomic
// operations and mutexes for thread safety with minimal contention.
type MetricsCollector struct {
	mu sync.RWMutex

	// Request counts per provider/model key.
	requestCounts map[string]*atomic.Int64

	// Token usage per provider/model key.
	inputTokens  map[string]*atomic.Int64
	outputTokens map[string]*atomic.Int64

	// Latency samples per provider/model key for histogram computation.
	latencies map[string][]time.Duration

	// Error counts per provider.
	errorCounts map[string]*atomic.Int64

	// Cost accumulator per provider/model key (in micro-USD for precision).
	costMicroUSD map[string]*atomic.Int64

	// Cache statistics.
	cacheHits  atomic.Int64
	cacheTotal atomic.Int64

	// Custom metrics.
	customMetrics map[string][]customSample
}

type customSample struct {
	Value float64
	Attrs map[string]string
	Time  time.Time
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestCounts: make(map[string]*atomic.Int64),
		inputTokens:   make(map[string]*atomic.Int64),
		outputTokens:  make(map[string]*atomic.Int64),
		latencies:     make(map[string][]time.Duration),
		errorCounts:   make(map[string]*atomic.Int64),
		costMicroUSD:  make(map[string]*atomic.Int64),
		customMetrics: make(map[string][]customSample),
	}
}

// RecordRequest records a completed LLM request with all its metrics.
func (mc *MetricsCollector) RecordRequest(provider, model string, inputTok, outputTok int, latency time.Duration, costUSD float64, isError bool) {
	if mc == nil {
		return
	}
	key := provider + "/" + model

	mc.incrRequestCount(key)
	mc.addInputTokens(key, int64(inputTok))
	mc.addOutputTokens(key, int64(outputTok))
	mc.recordLatency(key, latency)
	mc.addCost(key, costUSD)
	if isError {
		mc.incrErrors(provider)
	}
}

// RequestCount returns total requests for a provider/model key.
func (mc *MetricsCollector) RequestCount(key string) int64 {
	if mc == nil {
		return 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if c, ok := mc.requestCounts[key]; ok {
		return c.Load()
	}
	return 0
}

// TotalRequests returns the sum of all request counts.
func (mc *MetricsCollector) TotalRequests() int64 {
	if mc == nil {
		return 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	var total int64
	for _, c := range mc.requestCounts {
		total += c.Load()
	}
	return total
}

// TokensUsed returns (inputTokens, outputTokens) for a provider/model key.
func (mc *MetricsCollector) TokensUsed(key string) (int64, int64) {
	if mc == nil {
		return 0, 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	var inp, out int64
	if c, ok := mc.inputTokens[key]; ok {
		inp = c.Load()
	}
	if c, ok := mc.outputTokens[key]; ok {
		out = c.Load()
	}
	return inp, out
}

// LatencyHistogram returns P50, P95, P99 latencies in milliseconds for a key.
func (mc *MetricsCollector) LatencyHistogram(key string) (p50, p95, p99 float64) {
	if mc == nil {
		return 0, 0, 0
	}
	mc.mu.RLock()
	samples := mc.latencies[key]
	mc.mu.RUnlock()

	if len(samples) == 0 {
		return 0, 0, 0
	}

	// Sort a copy.
	sorted := make([]float64, len(samples))
	for i, d := range samples {
		sorted[i] = float64(d.Milliseconds())
	}
	sort.Float64s(sorted)

	p50 = percentile(sorted, 50)
	p95 = percentile(sorted, 95)
	p99 = percentile(sorted, 99)
	return
}

// ErrorRate returns the error rate for a provider (errors / total requests
// involving that provider).
func (mc *MetricsCollector) ErrorRate(provider string) float64 {
	if mc == nil {
		return 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var totalReqs int64
	for key, c := range mc.requestCounts {
		if strings.HasPrefix(key, provider+"/") {
			totalReqs += c.Load()
		}
	}
	if totalReqs == 0 {
		return 0
	}
	var errors int64
	if c, ok := mc.errorCounts[provider]; ok {
		errors = c.Load()
	}
	return float64(errors) / float64(totalReqs)
}

// CostAccumulator returns the total cost in USD for a provider/model key.
func (mc *MetricsCollector) CostAccumulator(key string) float64 {
	if mc == nil {
		return 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if c, ok := mc.costMicroUSD[key]; ok {
		return float64(c.Load()) / 1_000_000.0
	}
	return 0
}

// TotalCost returns the sum of all accumulated costs in USD.
func (mc *MetricsCollector) TotalCost() float64 {
	if mc == nil {
		return 0
	}
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	var total int64
	for _, c := range mc.costMicroUSD {
		total += c.Load()
	}
	return float64(total) / 1_000_000.0
}

// CacheHitRate returns the ratio of cache hits to total cache-eligible requests.
func (mc *MetricsCollector) CacheHitRate() float64 {
	if mc == nil {
		return 0
	}
	total := mc.cacheTotal.Load()
	if total == 0 {
		return 0
	}
	return float64(mc.cacheHits.Load()) / float64(total)
}

const maxCustomSamplesPerMetric = 1000 // cap per metric name to prevent unbounded growth

// RecordCustom records a custom named metric.
func (mc *MetricsCollector) RecordCustom(name string, value float64, attrs map[string]string) {
	if mc == nil {
		return
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	samples := mc.customMetrics[name]
	if len(samples) >= maxCustomSamplesPerMetric {
		// Evict oldest half
		samples = samples[maxCustomSamplesPerMetric/2:]
	}
	mc.customMetrics[name] = append(samples, customSample{
		Value: value,
		Attrs: attrs,
		Time:  time.Now(),
	})
}

// --- Export methods ---

// metricsSnapshot captures all metrics for serialization.
type metricsSnapshot struct {
	RequestCounts map[string]int64           `json:"request_counts"`
	InputTokens   map[string]int64           `json:"input_tokens"`
	OutputTokens  map[string]int64           `json:"output_tokens"`
	Latency       map[string]latencySnapshot `json:"latency_histograms"`
	ErrorRates    map[string]float64         `json:"error_rates"`
	Costs         map[string]float64         `json:"costs_usd"`
	CacheHitRate  float64                    `json:"cache_hit_rate"`
	TotalRequests int64                      `json:"total_requests"`
	TotalCostUSD  float64                    `json:"total_cost_usd"`
	CustomMetrics map[string][]customSample  `json:"custom_metrics,omitempty"`
}

type latencySnapshot struct {
	P50     float64 `json:"p50_ms"`
	P95     float64 `json:"p95_ms"`
	P99     float64 `json:"p99_ms"`
	Samples int     `json:"samples"`
}

// ExportJSON dumps all metrics as a JSON string.
func (mc *MetricsCollector) ExportJSON() string {
	if mc == nil {
		return "{}"
	}
	snap := mc.snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// ExportPrometheus dumps metrics in Prometheus exposition format.
func (mc *MetricsCollector) ExportPrometheus() string {
	if mc == nil {
		return ""
	}
	snap := mc.snapshot()
	var b strings.Builder

	// Request counts
	b.WriteString("# HELP eyrie_requests_total Total LLM API requests.\n")
	b.WriteString("# TYPE eyrie_requests_total counter\n")
	for key, count := range snap.RequestCounts {
		provider, model := splitKey(key)
		_, _ = fmt.Fprintf(&b, "eyrie_requests_total{provider=%q,model=%q} %d\n", provider, model, count)
	}

	// Input tokens
	b.WriteString("# HELP eyrie_input_tokens_total Total input tokens consumed.\n")
	b.WriteString("# TYPE eyrie_input_tokens_total counter\n")
	for key, tokens := range snap.InputTokens {
		provider, model := splitKey(key)
		_, _ = fmt.Fprintf(&b, "eyrie_input_tokens_total{provider=%q,model=%q} %d\n", provider, model, tokens)
	}

	// Output tokens
	b.WriteString("# HELP eyrie_output_tokens_total Total output tokens generated.\n")
	b.WriteString("# TYPE eyrie_output_tokens_total counter\n")
	for key, tokens := range snap.OutputTokens {
		provider, model := splitKey(key)
		_, _ = fmt.Fprintf(&b, "eyrie_output_tokens_total{provider=%q,model=%q} %d\n", provider, model, tokens)
	}

	// Latency histograms (as summary quantiles)
	b.WriteString("# HELP eyrie_request_duration_ms Request latency in milliseconds.\n")
	b.WriteString("# TYPE eyrie_request_duration_ms summary\n")
	for key, lat := range snap.Latency {
		provider, model := splitKey(key)
		_, _ = fmt.Fprintf(&b, "eyrie_request_duration_ms{provider=%q,model=%q,quantile=\"0.5\"} %.2f\n", provider, model, lat.P50)
		_, _ = fmt.Fprintf(&b, "eyrie_request_duration_ms{provider=%q,model=%q,quantile=\"0.95\"} %.2f\n", provider, model, lat.P95)
		_, _ = fmt.Fprintf(&b, "eyrie_request_duration_ms{provider=%q,model=%q,quantile=\"0.99\"} %.2f\n", provider, model, lat.P99)
		_, _ = fmt.Fprintf(&b, "eyrie_request_duration_ms_count{provider=%q,model=%q} %d\n", provider, model, lat.Samples)
	}

	// Error rates
	b.WriteString("# HELP eyrie_error_rate Error rate per provider.\n")
	b.WriteString("# TYPE eyrie_error_rate gauge\n")
	for provider, rate := range snap.ErrorRates {
		_, _ = fmt.Fprintf(&b, "eyrie_error_rate{provider=%q} %.4f\n", provider, rate)
	}

	// Cost
	b.WriteString("# HELP eyrie_cost_usd_total Accumulated cost in USD.\n")
	b.WriteString("# TYPE eyrie_cost_usd_total counter\n")
	for key, cost := range snap.Costs {
		provider, model := splitKey(key)
		_, _ = fmt.Fprintf(&b, "eyrie_cost_usd_total{provider=%q,model=%q} %.6f\n", provider, model, cost)
	}

	// Cache hit rate
	b.WriteString("# HELP eyrie_cache_hit_rate Ratio of cache hits to total cache-eligible requests.\n")
	b.WriteString("# TYPE eyrie_cache_hit_rate gauge\n")
	_, _ = fmt.Fprintf(&b, "eyrie_cache_hit_rate %.4f\n", snap.CacheHitRate)

	return b.String()
}

// snapshot creates a point-in-time copy of all metrics.
func (mc *MetricsCollector) snapshot() metricsSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	snap := metricsSnapshot{
		RequestCounts: make(map[string]int64),
		InputTokens:   make(map[string]int64),
		OutputTokens:  make(map[string]int64),
		Latency:       make(map[string]latencySnapshot),
		ErrorRates:    make(map[string]float64),
		Costs:         make(map[string]float64),
		CacheHitRate:  mc.CacheHitRate(),
		CustomMetrics: make(map[string][]customSample),
	}

	for key, c := range mc.requestCounts {
		snap.RequestCounts[key] = c.Load()
	}
	for key, c := range mc.inputTokens {
		snap.InputTokens[key] = c.Load()
	}
	for key, c := range mc.outputTokens {
		snap.OutputTokens[key] = c.Load()
	}
	for key, samples := range mc.latencies {
		if len(samples) == 0 {
			continue
		}
		sorted := make([]float64, len(samples))
		for i, d := range samples {
			sorted[i] = float64(d.Milliseconds())
		}
		sort.Float64s(sorted)
		snap.Latency[key] = latencySnapshot{
			P50:     percentile(sorted, 50),
			P95:     percentile(sorted, 95),
			P99:     percentile(sorted, 99),
			Samples: len(samples),
		}
	}

	// Compute error rates per provider.
	providers := make(map[string]bool)
	for key := range mc.requestCounts {
		p, _ := splitKey(key)
		providers[p] = true
	}
	for p := range providers {
		var totalReqs int64
		for key, c := range mc.requestCounts {
			if strings.HasPrefix(key, p+"/") {
				totalReqs += c.Load()
			}
		}
		var errors int64
		if c, ok := mc.errorCounts[p]; ok {
			errors = c.Load()
		}
		if totalReqs > 0 {
			snap.ErrorRates[p] = float64(errors) / float64(totalReqs)
		}
	}

	for key, c := range mc.costMicroUSD {
		snap.Costs[key] = float64(c.Load()) / 1_000_000.0
	}

	var totalReqs int64
	for _, c := range mc.requestCounts {
		totalReqs += c.Load()
	}
	snap.TotalRequests = totalReqs

	var totalCost int64
	for _, c := range mc.costMicroUSD {
		totalCost += c.Load()
	}
	snap.TotalCostUSD = float64(totalCost) / 1_000_000.0

	for name, samples := range mc.customMetrics {
		snap.CustomMetrics[name] = samples
	}

	return snap
}

// --- Internal helpers ---

func (mc *MetricsCollector) incrRequestCount(key string) {
	mc.mu.Lock()
	c, ok := mc.requestCounts[key]
	if !ok {
		c = &atomic.Int64{}
		mc.requestCounts[key] = c
	}
	mc.mu.Unlock()
	c.Add(1)
}

func (mc *MetricsCollector) addInputTokens(key string, n int64) {
	mc.mu.Lock()
	c, ok := mc.inputTokens[key]
	if !ok {
		c = &atomic.Int64{}
		mc.inputTokens[key] = c
	}
	mc.mu.Unlock()
	c.Add(n)
}

func (mc *MetricsCollector) addOutputTokens(key string, n int64) {
	mc.mu.Lock()
	c, ok := mc.outputTokens[key]
	if !ok {
		c = &atomic.Int64{}
		mc.outputTokens[key] = c
	}
	mc.mu.Unlock()
	c.Add(n)
}

func (mc *MetricsCollector) recordLatency(key string, d time.Duration) {
	mc.mu.Lock()
	const maxLatencySamples = 100
	if len(mc.latencies[key]) >= maxLatencySamples {
		// Rotate: drop oldest sample to cap memory usage.
		mc.latencies[key] = mc.latencies[key][1:]
	}
	mc.latencies[key] = append(mc.latencies[key], d)
	mc.mu.Unlock()
}

func (mc *MetricsCollector) incrErrors(provider string) {
	mc.mu.Lock()
	c, ok := mc.errorCounts[provider]
	if !ok {
		c = &atomic.Int64{}
		mc.errorCounts[provider] = c
	}
	mc.mu.Unlock()
	c.Add(1)
}

func (mc *MetricsCollector) addCost(key string, costUSD float64) {
	microUSD := int64(costUSD * 1_000_000)
	mc.mu.Lock()
	c, ok := mc.costMicroUSD[key]
	if !ok {
		c = &atomic.Int64{}
		mc.costMicroUSD[key] = c
	}
	mc.mu.Unlock()
	c.Add(microUSD)
}

func (mc *MetricsCollector) incrCacheHits() {
	mc.cacheHits.Add(1)
}

func (mc *MetricsCollector) incrCacheTotal() {
	mc.cacheTotal.Add(1)
}

// --- Utility functions ---

// generateTraceID generates a 32-character hex trace ID (16 random bytes).
func generateTraceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// generateSpanID generates a 16-character hex span ID (8 random bytes).
func generateSpanID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// percentile computes the p-th percentile from a sorted slice of float64.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}
	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// splitKey splits a "provider/model" key into its parts.
func splitKey(key string) (provider, model string) {
	idx := strings.Index(key, "/")
	if idx < 0 {
		return key, ""
	}
	return key[:idx], key[idx+1:]
}
