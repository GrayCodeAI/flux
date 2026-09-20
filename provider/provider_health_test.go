package provider

import (
	"testing"
	"time"
)

func TestHealthRecordSuccess(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	ph.RecordSuccess("provider-a", 100*time.Millisecond)
	ph.RecordSuccess("provider-a", 150*time.Millisecond)

	score := ph.Score("provider-a")
	if score < 0.9 {
		t.Errorf("score after 2 successes = %f, want >= 0.9", score)
	}
}

func TestHealthRecordFailure(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	ph.RecordSuccess("provider-b", 100*time.Millisecond)
	ph.RecordFailure("provider-b", 200*time.Millisecond)

	score := ph.Score("provider-b")
	// 1 success, 1 failure = 50% success rate, plus recency penalty
	if score >= 0.5 {
		t.Errorf("score after 1 success + 1 failure = %f, want < 0.5 (due to recency penalty)", score)
	}
}

func TestHealthScoreCalculation(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	// 8 successes, 2 failures => 80% success rate
	for i := 0; i < 8; i++ {
		ph.RecordSuccess("provider-c", 50*time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		ph.RecordFailure("provider-c", 100*time.Millisecond)
	}

	score := ph.Score("provider-c")
	// Base = 0.8, recency penalty = 0.2 (recent failure), consecutive penalty = 0.1 (consecutive < 0)
	// Expected: 0.8 - 0.2 - 0.1 = 0.5
	if score < 0.4 || score > 0.6 {
		t.Errorf("score = %f, want ~0.5 (base 0.8 - penalties)", score)
	}
}

func TestHealthScoreUnknownProvider(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	score := ph.Score("unknown-provider")
	if score != 1.0 {
		t.Errorf("unknown provider score = %f, want 1.0", score)
	}
}

func TestHealthHealthiestReturnsUnknown(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	// Record some failures for a known provider
	ph.RecordFailure("known", 100*time.Millisecond)
	ph.RecordFailure("known", 100*time.Millisecond)

	// Unknown provider should be preferred (assumed healthy)
	best := ph.Healthiest([]string{"known", "unknown"})
	if best != "unknown" {
		t.Errorf("Healthiest = %q, want unknown (assumed healthy)", best)
	}
}

func TestHealthHealthiestReturnsBestScore(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	// Provider A: all successes
	for i := 0; i < 10; i++ {
		ph.RecordSuccess("provider-a", 50*time.Millisecond)
	}
	// Provider B: mixed
	for i := 0; i < 5; i++ {
		ph.RecordSuccess("provider-b", 50*time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		ph.RecordFailure("provider-b", 100*time.Millisecond)
	}

	best := ph.Healthiest([]string{"provider-a", "provider-b"})
	if best != "provider-a" {
		t.Errorf("Healthiest = %q, want provider-a", best)
	}
}

func TestHealthHealthiestEmptyCandidates(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	best := ph.Healthiest([]string{})
	if best != "" {
		t.Errorf("Healthiest([]) = %q, want empty string", best)
	}
}

func TestHealthConsecutiveFailures(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	// Start with successes to build up some history
	for i := 0; i < 5; i++ {
		ph.RecordSuccess("provider-d", 50*time.Millisecond)
	}

	// Record 4 consecutive failures (triggers -0.3 penalty at consecutive < -3)
	for i := 0; i < 4; i++ {
		ph.RecordFailure("provider-d", 100*time.Millisecond)
	}

	score := ph.Score("provider-d")
	// 5 success, 4 failures => ~55.5% success rate
	// Recency penalty: 0.2 (recent failure)
	// Consecutive penalty: 0.3 (consecutive = -4 < -3)
	// Expected: ~0.555 - 0.2 - 0.3 = ~0.055
	if score > 0.2 {
		t.Errorf("score with 4 consecutive failures = %f, want < 0.2 (heavy penalties)", score)
	}
}

func TestHealthConsecutiveResets(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	// Build consecutive failures
	ph.RecordFailure("provider-e", 50*time.Millisecond)
	ph.RecordFailure("provider-e", 50*time.Millisecond)
	ph.RecordFailure("provider-e", 50*time.Millisecond)
	ph.RecordFailure("provider-e", 50*time.Millisecond)

	// Then a success resets consecutive
	ph.RecordSuccess("provider-e", 50*time.Millisecond)

	// Score should improve after success breaks streak
	// But still has 4 failures, 1 success = 20% base
	score := ph.Score("provider-e")
	// With recency penalty still (last error was recent) but no consecutive > -3 penalty
	// since consecutive is now +1 after success
	// The lastError is still recent though, so recency penalty of 0.2 applies
	// Expected: 0.2 - 0.2 = 0.0
	if score > 0.1 {
		t.Errorf("score = %f, want <= 0.1 (low success rate with recency penalty)", score)
	}
}

func TestHealthAllScores(t *testing.T) {
	t.Parallel()
	ph := NewProviderHealth()

	ph.RecordSuccess("alpha", 50*time.Millisecond)
	ph.RecordSuccess("alpha", 50*time.Millisecond)
	ph.RecordFailure("beta", 100*time.Millisecond)

	scores := ph.AllScores()
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	// Should be sorted by score descending
	if scores[0].Name != "alpha" {
		t.Errorf("first score name = %q, want alpha (highest)", scores[0].Name)
	}
	if !scores[0].IsHealthy {
		t.Error("alpha should be healthy")
	}
	if scores[1].IsHealthy {
		t.Error("beta should not be healthy (score <= 0.5)")
	}
}
