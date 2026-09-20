package router

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing — requests are rejected immediately
	CircuitHalfOpen                     // Testing if the deployment has recovered
)

// CircuitBreaker tracks failure rates for a single deployment and opens when failures exceed a threshold.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failureCount    int
	lastFailureTime time.Time
	cooldown        time.Duration
	threshold       int
	probeInFlight   bool
}

// NewCircuitBreaker creates a circuit breaker that opens after `threshold` consecutive failures
// and stays open for `cooldown` duration before transitioning to half-open.
func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &CircuitBreaker{
		state:     CircuitClosed,
		threshold: threshold,
		cooldown:  cooldown,
	}
}

// Ready reports whether a request could be admitted. It does not reserve the
// half-open probe, so route filtering cannot consume that probe before an
// endpoint is selected.
func (cb *CircuitBreaker) Ready() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case CircuitOpen:
		return time.Since(cb.lastFailureTime) >= cb.cooldown
	case CircuitHalfOpen:
		return !cb.probeInFlight
	default:
		return true
	}
}

// Allow atomically admits a request. At most one request is admitted while
// the circuit is half-open; its Success or Failure completes the probe.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) >= cb.cooldown {
			cb.state = CircuitHalfOpen
			cb.probeInFlight = true
			return true
		}
		return false
	case CircuitHalfOpen:
		if cb.probeInFlight {
			return false
		}
		cb.probeInFlight = true
		return true
	default:
		return true
	}
}

// Success records a successful call and resets the circuit.
func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = CircuitClosed
	cb.probeInFlight = false
}

// Failure records a failed call. If the threshold is reached, the circuit opens.
// In HalfOpen state a single failure immediately re-opens the circuit so the
// next probe only occurs after another full cooldown.
func (cb *CircuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()
	cb.probeInFlight = false
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		return
	}
	if cb.failureCount >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Reset forces the circuit back to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = CircuitClosed
	cb.probeInFlight = false
}
