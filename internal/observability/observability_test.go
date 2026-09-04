package graycoderouter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewTelemetry(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()
	if tel == nil {
		t.Fatal("NewTelemetry returned nil")
	}
	if tel.Metrics() == nil {
		t.Fatal("Metrics() returned nil on initialized Telemetry")
	}
}

func TestNilTelemetryIsNoOp(t *testing.T) {
	t.Parallel()
	var tel *Telemetry

	// All methods should be safe on nil receiver.
	span := tel.StartSpan("test", nil)
	if span != nil {
		t.Error("StartSpan on nil Telemetry should return nil")
	}
	tel.EndSpan(span, nil)
	tel.EndSpan(nil, errors.New("test"))
	tel.RecordMetric("foo", 1.0, nil)

	spans := tel.Spans()
	if spans != nil {
		t.Error("Spans on nil Telemetry should return nil")
	}

	mc := tel.Metrics()
	if mc != nil {
		t.Error("Metrics on nil Telemetry should return nil")
	}
}

func TestNilMetricsCollectorIsNoOp(t *testing.T) {
	t.Parallel()
	var mc *MetricsCollector

	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, false)
	mc.RecordCustom("test", 1.0, nil)

	if mc.RequestCount("openai/gpt-4") != 0 {
		t.Error("expected 0")
	}
	if mc.TotalRequests() != 0 {
		t.Error("expected 0")
	}
	inp, out := mc.TokensUsed("openai/gpt-4")
	if inp != 0 || out != 0 {
		t.Error("expected 0, 0")
	}
	p50, p95, p99 := mc.LatencyHistogram("openai/gpt-4")
	if p50 != 0 || p95 != 0 || p99 != 0 {
		t.Error("expected all zeros")
	}
	if mc.ErrorRate("openai") != 0 {
		t.Error("expected 0")
	}
	if mc.CostAccumulator("openai/gpt-4") != 0 {
		t.Error("expected 0")
	}
	if mc.TotalCost() != 0 {
		t.Error("expected 0")
	}
	if mc.CacheHitRate() != 0 {
		t.Error("expected 0")
	}
	if mc.ExportJSON() != "{}" {
		t.Errorf("expected empty JSON, got: %s", mc.ExportJSON())
	}
	if mc.ExportPrometheus() != "" {
		t.Error("expected empty string")
	}
}

func TestStartAndEndSpan(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	attrs := map[string]string{
		AttrLLMProvider: "anthropic",
		AttrLLMModel:    "claude-3-opus",
	}
	span := tel.StartSpan(SpanLLMChat, attrs)
	if span == nil {
		t.Fatal("StartSpan returned nil")
	}
	if span.TraceID == "" {
		t.Error("TraceID should not be empty")
	}
	if len(span.TraceID) != 32 {
		t.Errorf("TraceID should be 32 hex chars, got %d", len(span.TraceID))
	}
	if span.SpanID == "" {
		t.Error("SpanID should not be empty")
	}
	if len(span.SpanID) != 16 {
		t.Errorf("SpanID should be 16 hex chars, got %d", len(span.SpanID))
	}
	if span.Name != SpanLLMChat {
		t.Errorf("expected name %q, got %q", SpanLLMChat, span.Name)
	}
	if span.Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %q", span.Provider)
	}
	if span.Model != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %q", span.Model)
	}
	if span.Status != StatusUnset {
		t.Errorf("expected status unset, got %q", span.Status)
	}

	// Simulate some work.
	time.Sleep(5 * time.Millisecond)

	tel.EndSpan(span, nil)
	if span.Status != StatusOK {
		t.Errorf("expected status ok, got %q", span.Status)
	}
	if span.EndTime.IsZero() {
		t.Error("EndTime should be set")
	}
	if span.Duration() < 5*time.Millisecond {
		t.Errorf("duration should be >= 5ms, got %v", span.Duration())
	}
	if span.Attributes[AttrLLMStatus] != string(StatusOK) {
		t.Errorf("expected status attribute 'ok', got %q", span.Attributes[AttrLLMStatus])
	}
	if span.Attributes[AttrLLMLatencyMs] == "" {
		t.Error("latency_ms attribute should be set")
	}
}

func TestEndSpanWithError(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	span := tel.StartSpan(SpanLLMChat, map[string]string{
		AttrLLMProvider: "openai",
		AttrLLMModel:    "gpt-4",
	})
	tel.EndSpan(span, errors.New("rate limited"))

	if span.Status != StatusError {
		t.Errorf("expected status error, got %q", span.Status)
	}
	if span.Attributes["error.message"] != "rate limited" {
		t.Errorf("expected error message, got %q", span.Attributes["error.message"])
	}
}

func TestSpanAddEvent(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()
	span := tel.StartSpan(SpanLLMRetry, nil)
	span.AddEvent("retry_attempt", map[string]string{"attempt": "2"})

	if len(span.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(span.Events))
	}
	if span.Events[0].Name != "retry_attempt" {
		t.Errorf("expected event name retry_attempt, got %q", span.Events[0].Name)
	}
	if span.Events[0].Attributes["attempt"] != "2" {
		t.Error("expected attempt attribute")
	}
	tel.EndSpan(span, nil)
}

func TestMetricsCollectorRequestCount(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 100, 50, 200*time.Millisecond, 0.01, false)
	mc.RecordRequest("openai", "gpt-4", 200, 100, 300*time.Millisecond, 0.02, false)
	mc.RecordRequest("anthropic", "claude-3", 150, 75, 250*time.Millisecond, 0.015, false)

	if mc.RequestCount("openai/gpt-4") != 2 {
		t.Errorf("expected 2, got %d", mc.RequestCount("openai/gpt-4"))
	}
	if mc.RequestCount("anthropic/claude-3") != 1 {
		t.Errorf("expected 1, got %d", mc.RequestCount("anthropic/claude-3"))
	}
	if mc.TotalRequests() != 3 {
		t.Errorf("expected 3, got %d", mc.TotalRequests())
	}
}

func TestMetricsCollectorTokensUsed(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, false)
	mc.RecordRequest("openai", "gpt-4", 200, 100, time.Second, 0.02, false)

	inp, out := mc.TokensUsed("openai/gpt-4")
	if inp != 300 {
		t.Errorf("expected 300 input tokens, got %d", inp)
	}
	if out != 150 {
		t.Errorf("expected 150 output tokens, got %d", out)
	}
}

func TestMetricsCollectorLatencyHistogram(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	// Record 100 requests with increasing latency from 1ms to 100ms.
	for i := 1; i <= 100; i++ {
		mc.RecordRequest("openai", "gpt-4", 10, 5, time.Duration(i)*time.Millisecond, 0.001, false)
	}

	p50, p95, p99 := mc.LatencyHistogram("openai/gpt-4")

	// P50 should be around 50ms.
	if p50 < 45 || p50 > 55 {
		t.Errorf("P50 expected ~50ms, got %.2f", p50)
	}
	// P95 should be around 95ms.
	if p95 < 90 || p95 > 100 {
		t.Errorf("P95 expected ~95ms, got %.2f", p95)
	}
	// P99 should be around 99ms.
	if p99 < 95 || p99 > 100 {
		t.Errorf("P99 expected ~99ms, got %.2f", p99)
	}
}

func TestMetricsCollectorErrorRate(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, false)
	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, true)
	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, false)
	mc.RecordRequest("openai", "gpt-4", 100, 50, time.Second, 0.01, true)

	rate := mc.ErrorRate("openai")
	expected := 0.5
	if rate != expected {
		t.Errorf("expected error rate %.2f, got %.2f", expected, rate)
	}
}

func TestMetricsCollectorCost(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 1000, 500, time.Second, 0.05, false)
	mc.RecordRequest("openai", "gpt-4", 2000, 1000, time.Second, 0.10, false)
	mc.RecordRequest("anthropic", "claude-3", 1500, 750, time.Second, 0.075, false)

	cost := mc.CostAccumulator("openai/gpt-4")
	if cost < 0.149 || cost > 0.151 {
		t.Errorf("expected cost ~0.15, got %.6f", cost)
	}

	totalCost := mc.TotalCost()
	if totalCost < 0.224 || totalCost > 0.226 {
		t.Errorf("expected total cost ~0.225, got %.6f", totalCost)
	}
}

func TestMetricsCollectorCacheHitRate(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	// Simulate cache operations.
	mc.incrCacheTotal()
	mc.incrCacheHits()
	mc.incrCacheTotal()
	mc.incrCacheTotal()
	mc.incrCacheHits()
	mc.incrCacheTotal()

	rate := mc.CacheHitRate()
	expected := 0.5 // 2 hits / 4 total
	if rate != expected {
		t.Errorf("expected cache hit rate %.2f, got %.2f", expected, rate)
	}
}

func TestExportJSON(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 100, 50, 200*time.Millisecond, 0.01, false)
	mc.RecordRequest("openai", "gpt-4", 200, 100, 400*time.Millisecond, 0.02, true)

	jsonStr := mc.ExportJSON()
	if jsonStr == "" {
		t.Fatal("ExportJSON returned empty string")
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ExportJSON produced invalid JSON: %v", err)
	}

	// Check key fields exist.
	if _, ok := parsed["request_counts"]; !ok {
		t.Error("missing request_counts in JSON")
	}
	if _, ok := parsed["total_requests"]; !ok {
		t.Error("missing total_requests in JSON")
	}
	if _, ok := parsed["error_rates"]; !ok {
		t.Error("missing error_rates in JSON")
	}
}

func TestExportPrometheus(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()

	mc.RecordRequest("openai", "gpt-4", 100, 50, 200*time.Millisecond, 0.01, false)
	mc.RecordRequest("anthropic", "claude-3", 150, 75, 300*time.Millisecond, 0.015, true)

	prom := mc.ExportPrometheus()
	if prom == "" {
		t.Fatal("ExportPrometheus returned empty string")
	}

	// Check that it contains expected metric names.
	expectedMetrics := []string{
		"graycode_router_requests_total",
		"graycode_router_input_tokens_total",
		"graycode_router_output_tokens_total",
		"graycode_router_request_duration_ms",
		"graycode_router_error_rate",
		"graycode_router_cost_usd_total",
		"graycode_router_cache_hit_rate",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(prom, m) {
			t.Errorf("Prometheus output missing metric %q", m)
		}
	}

	// Check labels.
	if !strings.Contains(prom, `provider="openai"`) {
		t.Error("missing openai provider label")
	}
	if !strings.Contains(prom, `model="gpt-4"`) {
		t.Error("missing gpt-4 model label")
	}

	// Check HELP and TYPE annotations.
	if !strings.Contains(prom, "# HELP") {
		t.Error("missing HELP annotations")
	}
	if !strings.Contains(prom, "# TYPE") {
		t.Error("missing TYPE annotations")
	}
}

func TestTelemetryRecordsSpanMetrics(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	span := tel.StartSpan(SpanLLMChat, map[string]string{
		AttrLLMProvider: "openai",
		AttrLLMModel:    "gpt-4",
	})
	time.Sleep(10 * time.Millisecond)
	tel.EndSpan(span, nil)

	// Verify metrics were updated from span.
	mc := tel.Metrics()
	if mc.RequestCount("openai/gpt-4") != 1 {
		t.Errorf("expected 1 request, got %d", mc.RequestCount("openai/gpt-4"))
	}
}

func TestTelemetryCacheHitSpan(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	// Record a cache hit span.
	span := tel.StartSpan(SpanLLMCacheHit, map[string]string{
		AttrLLMProvider: "openai",
		AttrLLMModel:    "gpt-4",
	})
	tel.EndSpan(span, nil)

	// Record a non-cache span.
	span2 := tel.StartSpan(SpanLLMChat, map[string]string{
		AttrLLMProvider: "openai",
		AttrLLMModel:    "gpt-4",
	})
	tel.EndSpan(span2, nil)

	// Cache hit rate should be 0.5 (1 hit / 2 total).
	rate := tel.Metrics().CacheHitRate()
	if rate != 0.5 {
		t.Errorf("expected cache hit rate 0.5, got %.2f", rate)
	}
}

func TestTelemetryOnSpanEndCallback(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	var captured *Span
	tel.OnSpanEnd = func(s *Span) {
		captured = s
	}

	span := tel.StartSpan(SpanLLMStream, map[string]string{
		AttrLLMProvider: "anthropic",
		AttrLLMModel:    "claude-3-sonnet",
	})
	tel.EndSpan(span, nil)

	if captured == nil {
		t.Fatal("OnSpanEnd callback was not called")
	}
	if captured.Name != SpanLLMStream {
		t.Errorf("captured span name wrong: %q", captured.Name)
	}
}

func TestTelemetrySpansList(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	for i := 0; i < 5; i++ {
		span := tel.StartSpan(SpanLLMChat, map[string]string{
			AttrLLMProvider: "openai",
			AttrLLMModel:    "gpt-4",
		})
		tel.EndSpan(span, nil)
	}

	spans := tel.Spans()
	if len(spans) != 5 {
		t.Errorf("expected 5 spans, got %d", len(spans))
	}
}

func TestRecordMetric(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	tel.RecordMetric("custom.latency", 42.5, map[string]string{"region": "us-east"})
	tel.RecordMetric("custom.latency", 38.2, map[string]string{"region": "eu-west"})

	// The custom metrics should be in the exported JSON.
	jsonStr := tel.Metrics().ExportJSON()
	if !strings.Contains(jsonStr, "custom.latency") {
		t.Error("custom metric not found in exported JSON")
	}
}

func TestPercentileEdgeCases(t *testing.T) {
	t.Parallel()
	// Empty slice.
	if p := percentile(nil, 50); p != 0 {
		t.Errorf("expected 0, got %f", p)
	}

	// Single element.
	if p := percentile([]float64{42.0}, 99); p != 42.0 {
		t.Errorf("expected 42.0, got %f", p)
	}

	// Two elements.
	p50 := percentile([]float64{10.0, 20.0}, 50)
	if p50 != 15.0 {
		t.Errorf("expected 15.0, got %f", p50)
	}
}

func TestSplitKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key          string
		wantProvider string
		wantModel    string
	}{
		{"openai/gpt-4", "openai", "gpt-4"},
		{"anthropic/claude-3-opus-20240229", "anthropic", "claude-3-opus-20240229"},
		{"nomodel", "nomodel", ""},
		{"a/b/c", "a", "b/c"},
	}
	for _, tt := range tests {
		p, m := splitKey(tt.key)
		if p != tt.wantProvider || m != tt.wantModel {
			t.Errorf("splitKey(%q) = (%q, %q), want (%q, %q)", tt.key, p, m, tt.wantProvider, tt.wantModel)
		}
	}
}

func TestTraceAndSpanIDUniqueness(t *testing.T) {
	t.Parallel()
	tel := NewTelemetry()

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		span := tel.StartSpan(SpanLLMChat, nil)
		if ids[span.TraceID] {
			t.Fatalf("duplicate TraceID on iteration %d", i)
		}
		if ids[span.SpanID] {
			t.Fatalf("duplicate SpanID on iteration %d", i)
		}
		ids[span.TraceID] = true
		ids[span.SpanID] = true
		tel.EndSpan(span, nil)
	}
}

func TestConcurrentMetricsAccess(t *testing.T) {
	t.Parallel()
	mc := NewMetricsCollector()
	done := make(chan struct{})

	// Spawn multiple goroutines writing concurrently.
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				mc.RecordRequest("openai", "gpt-4", 100, 50, time.Millisecond*time.Duration(j+1), 0.01, j%5 == 0)
			}
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	total := mc.TotalRequests()
	if total != 1000 {
		t.Errorf("expected 1000 total requests, got %d", total)
	}
}
