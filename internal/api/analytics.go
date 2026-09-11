package api

import (
	"net/http"
	"time"

	eyrie "github.com/GrayCodeAI/eyrie/internal/health"
	"github.com/GrayCodeAI/eyrie/storage"
)

// --- Usage analytics (#166) ---

// usageResponse is the JSON shape returned by GET /api/usage.
type usageResponse struct {
	Period    string               `json:"period"`
	Since     string               `json:"since"`
	Generated string               `json:"generated"`
	Providers []storage.UsageStats `json:"providers"`
}

func (s *Server) handleUsageAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.analytics == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics not configured"})
		return
	}

	since := parseSinceParam(r, 24*time.Hour)

	stats, err := s.analytics.GetUsageStats(r.Context(), since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stats == nil {
		stats = []storage.UsageStats{}
	}

	writeJSON(w, http.StatusOK, usageResponse{
		Period:    r.URL.Query().Get("period"),
		Since:     since.Format(time.RFC3339),
		Generated: time.Now().Format(time.RFC3339),
		Providers: stats,
	})
}

// --- Persistent cost tracking (#167) ---

// costResponse is the JSON shape returned by GET /api/costs.
type costResponse struct {
	Period    string               `json:"period"`
	Since     string               `json:"since"`
	Generated string               `json:"generated"`
	Providers []storage.UsageStats `json:"providers"`
	TotalUSD  float64              `json:"total_usd"`
}

func (s *Server) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	if s.analytics == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics not configured"})
		return
	}

	since := parseSinceParam(r, 24*time.Hour)

	stats, err := s.analytics.GetCostSummary(r.Context(), since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stats == nil {
		stats = []storage.UsageStats{}
	}

	var total float64
	for _, s := range stats {
		total += s.TotalCostUSD
	}

	writeJSON(w, http.StatusOK, costResponse{
		Period:    r.URL.Query().Get("period"),
		Since:     since.Format(time.RFC3339),
		Generated: time.Now().Format(time.RFC3339),
		Providers: stats,
		TotalUSD:  total,
	})
}

// RecordCost persists a cost entry. Callers (e.g. the conversation engine or
// middleware) should invoke this after each completed LLM request.
func (s *Server) RecordCost(rec *storage.CostRecord) {
	if s.analytics == nil {
		return
	}
	_ = s.analytics.RecordCost(s.bgCtx, rec)
}

// --- Provider health dashboard (#168) ---

// providerHealthEntry is a single provider in the health dashboard response.
type providerHealthEntry struct {
	Provider      string  `json:"provider"`
	PingState     string  `json:"ping_state"`
	PingLatencyMs int64   `json:"ping_latency_ms"`
	PingError     string  `json:"ping_error,omitempty"`
	PingMessage   string  `json:"ping_message,omitempty"`
	RequestCount  int     `json:"request_count"`
	ErrorCount    int     `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P50LatencyMs  float64 `json:"p50_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
	P99LatencyMs  float64 `json:"p99_latency_ms"`
	LastRequestAt string  `json:"last_request_at,omitempty"`
}

// providerHealthResponse is the JSON shape returned by GET /api/health/providers.
type providerHealthResponse struct {
	Generated string                `json:"generated"`
	Providers []providerHealthEntry `json:"providers"`
}

func (s *Server) handleProviderHealth(w http.ResponseWriter, r *http.Request) {
	resp := providerHealthResponse{
		Generated: time.Now().Format(time.RFC3339),
		Providers: []providerHealthEntry{},
	}

	// Collect ping-based health from the HealthChecker.
	pingStatuses := make(map[string]eyrie.HealthStatus)
	if s.healthChecker != nil {
		pingStatuses = s.healthChecker.AllProviderHealth()
	}

	// Collect request-based health from the analytics store (last 24h).
	var reqHealth []storage.ProviderHealthRecord
	if s.analytics != nil {
		since := time.Now().Add(-24 * time.Hour)
		reqHealth, _ = s.analytics.GetProviderHealth(r.Context(), since)
	}

	// Merge: start with ping data, overlay request stats.
	seen := make(map[string]bool)
	for name, ps := range pingStatuses {
		entry := providerHealthEntry{
			Provider:      name,
			PingState:     ps.State.String(),
			PingLatencyMs: ps.Latency.Milliseconds(),
			PingError:     ps.Error,
			PingMessage:   ps.Message,
		}
		// Find matching request health.
		for _, rh := range reqHealth {
			if rh.Provider == name {
				entry.RequestCount = rh.RequestCount
				entry.ErrorCount = rh.ErrorCount
				entry.ErrorRate = rh.ErrorRate
				entry.AvgLatencyMs = rh.AvgLatencyMs
				entry.P50LatencyMs = rh.P50LatencyMs
				entry.P95LatencyMs = rh.P95LatencyMs
				entry.P99LatencyMs = rh.P99LatencyMs
				if !rh.LastRequestAt.IsZero() {
					entry.LastRequestAt = rh.LastRequestAt.Format(time.RFC3339)
				}
				break
			}
		}
		resp.Providers = append(resp.Providers, entry)
		seen[name] = true
	}

	// Add providers that only appear in request data (no ping registration).
	for _, rh := range reqHealth {
		if seen[rh.Provider] {
			continue
		}
		entry := providerHealthEntry{
			Provider:     rh.Provider,
			PingState:    "unknown",
			RequestCount: rh.RequestCount,
			ErrorCount:   rh.ErrorCount,
			ErrorRate:    rh.ErrorRate,
			AvgLatencyMs: rh.AvgLatencyMs,
			P50LatencyMs: rh.P50LatencyMs,
			P95LatencyMs: rh.P95LatencyMs,
			P99LatencyMs: rh.P99LatencyMs,
		}
		if !rh.LastRequestAt.IsZero() {
			entry.LastRequestAt = rh.LastRequestAt.Format(time.RFC3339)
		}
		resp.Providers = append(resp.Providers, entry)
	}

	writeJSON(w, http.StatusOK, resp)
}

// --- Helpers ---

// parseSinceParam reads the ?period= query parameter and returns the
// corresponding start time. Supported values: 1h, 6h, 24h, 7d, 30d.
// Falls back to defaultDur if the parameter is missing or unrecognised.
func parseSinceParam(r *http.Request, defaultDur time.Duration) time.Time {
	d := defaultDur
	switch r.URL.Query().Get("period") {
	case "1h":
		d = time.Hour
	case "6h":
		d = 6 * time.Hour
	case "24h":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	}
	return time.Now().Add(-d)
}
