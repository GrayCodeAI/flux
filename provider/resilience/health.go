package resilience

import (
	"math"
	"sort"
	"sync"
	"time"
)

// ProviderHealth tracks latency and error rates per provider.
// Enables intelligent routing: prefer the healthiest provider.
type ProviderHealth struct {
	mu        sync.RWMutex
	providers map[string]*providerStats
}

type providerStats struct {
	successes    int
	failures     int
	totalLatency time.Duration
	lastError    time.Time
	lastSuccess  time.Time
	consecutive  int // positive = consecutive successes, negative = failures
}

// ProviderScore represents a provider's health score.
type ProviderScore struct {
	Name         string  `json:"name"`
	Score        float64 `json:"score"` // 0-1 (1 = perfect health)
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
	IsHealthy    bool    `json:"is_healthy"`
}

// NewProviderHealth creates a health tracker.
func NewProviderHealth() *ProviderHealth {
	return &ProviderHealth{
		providers: make(map[string]*providerStats),
	}
}

// RecordSuccess records a successful API call.
func (ph *ProviderHealth) RecordSuccess(provider string, latency time.Duration) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	s := ph.getOrCreate(provider)
	s.successes++
	s.totalLatency += latency
	s.lastSuccess = time.Now()
	if s.consecutive < 0 {
		s.consecutive = 1
	} else {
		s.consecutive++
	}
}

// RecordFailure records a failed API call.
func (ph *ProviderHealth) RecordFailure(provider string, latency time.Duration) {
	ph.mu.Lock()
	defer ph.mu.Unlock()
	s := ph.getOrCreate(provider)
	s.failures++
	s.totalLatency += latency
	s.lastError = time.Now()
	if s.consecutive > 0 {
		s.consecutive = -1
	} else {
		s.consecutive--
	}
}

// Score returns health score for a provider (0-1).
func (ph *ProviderHealth) Score(provider string) float64 {
	ph.mu.RLock()
	defer ph.mu.RUnlock()
	s, ok := ph.providers[provider]
	if !ok {
		return 1.0 // unknown = assume healthy
	}
	return s.score()
}

// Healthiest returns the provider with the best health score.
func (ph *ProviderHealth) Healthiest(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	best := candidates[0]
	bestScore := 0.0

	for _, c := range candidates {
		s, ok := ph.providers[c]
		if !ok {
			return c // unknown provider = try it
		}
		score := s.score()
		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

// AllScores returns health scores for all tracked providers.
func (ph *ProviderHealth) AllScores() []ProviderScore {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	scores := make([]ProviderScore, 0, len(ph.providers))
	for name, s := range ph.providers {
		total := s.successes + s.failures
		avgLatency := int64(0)
		if total > 0 {
			avgLatency = int64(s.totalLatency) / int64(total) / int64(time.Millisecond)
		}
		errorRate := 0.0
		if total > 0 {
			errorRate = float64(s.failures) / float64(total)
		}
		scores = append(scores, ProviderScore{
			Name:         name,
			Score:        s.score(),
			AvgLatencyMs: avgLatency,
			ErrorRate:    errorRate,
			IsHealthy:    s.score() > 0.5,
		})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	return scores
}

func (ph *ProviderHealth) getOrCreate(provider string) *providerStats {
	s, ok := ph.providers[provider]
	if !ok {
		s = &providerStats{}
		ph.providers[provider] = s
	}
	return s
}

func (s *providerStats) score() float64 {
	total := s.successes + s.failures
	if total == 0 {
		return 1.0
	}

	// Apply exponential time decay to failure counts after a 5-minute grace period.
	// Recent failures (< 5 min) are kept at full weight. Older failures decay
	// with a 1-hour half-life, allowing providers to recover over time.
	decayedFailures := float64(s.failures)
	if !s.lastError.IsZero() {
		elapsed := time.Since(s.lastError)
		if elapsed > 5*time.Minute {
			// Decay starts after grace period, measured from end of grace
			decayElapsed := elapsed - 5*time.Minute
			decay := math.Exp(-math.Ln2 * decayElapsed.Hours()) // half-life = 1 hour
			decayedFailures *= decay
		}
	}
	decayedTotal := float64(s.successes) + decayedFailures
	if decayedTotal == 0 {
		return 1.0
	}

	// Base score from decayed success rate
	successRate := float64(s.successes) / decayedTotal

	// Penalty for recent failures
	recencyPenalty := 0.0
	if !s.lastError.IsZero() && time.Since(s.lastError) < 5*time.Minute {
		recencyPenalty = 0.2
	}

	// Penalty for consecutive failures
	consecutivePenalty := 0.0
	if s.consecutive < -3 {
		consecutivePenalty = 0.3
	} else if s.consecutive < 0 {
		consecutivePenalty = 0.1
	}

	score := successRate - recencyPenalty - consecutivePenalty
	if score < 0 {
		score = 0
	}
	return score
}
