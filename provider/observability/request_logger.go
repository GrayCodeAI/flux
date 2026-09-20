package observability

import (
	"fmt"
	"sync"
	"time"
)

// RequestLogger stores a bounded in-memory request log for debugging and
// operational inspection.
type RequestLogger struct {
	mu      sync.Mutex
	enabled bool
	entries []RequestLogEntry
	maxSize int
}

// RequestLogEntry is a single logged API call.
type RequestLogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	LatencyMs    int64     `json:"latency_ms"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	CacheHit     bool      `json:"cache_hit"`
}

func NewRequestLogger(enabled bool) *RequestLogger {
	return &RequestLogger{enabled: enabled, entries: make([]RequestLogEntry, 0, 100), maxSize: 500}
}

func (rl *RequestLogger) Log(entry RequestLogEntry) {
	if !rl.enabled {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	entry.Timestamp = time.Now()
	rl.entries = append(rl.entries, entry)
	if len(rl.entries) > rl.maxSize {
		rl.entries = rl.entries[len(rl.entries)-rl.maxSize:]
	}
}

func (rl *RequestLogger) Recent(n int) []RequestLogEntry {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if n > len(rl.entries) {
		n = len(rl.entries)
	}
	result := make([]RequestLogEntry, n)
	copy(result, rl.entries[len(rl.entries)-n:])
	return result
}

func (rl *RequestLogger) Summary() string {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.entries) == 0 {
		return "No API calls logged."
	}
	total := len(rl.entries)
	var errors, cacheHits int
	var totalLatency int64
	var totalIn, totalOut int
	for _, e := range rl.entries {
		if e.Status == "error" {
			errors++
		}
		if e.CacheHit {
			cacheHits++
		}
		totalLatency += e.LatencyMs
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
	}
	return fmt.Sprintf("API calls: %d (errors: %d, cache hits: %d, avg latency: %dms, tokens: %d in / %d out)",
		total, errors, cacheHits, totalLatency/int64(total), totalIn, totalOut)
}
