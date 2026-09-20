package router

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(1, 10*time.Millisecond)
	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected open after threshold failure")
	}

	if cb.Allow() {
		t.Fatal("expected deny immediately after open")
	}

	time.Sleep(15 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected allow after cooldown elapses (half-open probe)")
	}

	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected half-open after cooldown-probe Allow()")
	}
}

func TestCircuitBreakerHalfOpenAdmitsOneConcurrentProbe(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Hour)
	cb.Failure()
	cb.mu.Lock()
	cb.lastFailureTime = time.Now().Add(-2 * time.Hour)
	cb.mu.Unlock()
	if !cb.Ready() || cb.State() != CircuitOpen {
		t.Fatal("readiness must not reserve or transition the circuit")
	}
	const callers = 64
	start := make(chan struct{})
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if cb.Allow() {
				admitted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := admitted.Load(); got != 1 {
		t.Fatalf("half-open admitted %d probes, want one", got)
	}
	if cb.Ready() {
		t.Fatal("half-open circuit must deny while probe is in flight")
	}
	cb.Success()
	if !cb.Ready() || !cb.Allow() {
		t.Fatal("successful probe must restore normal admission")
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(1, 10*time.Millisecond)
	cb.Failure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow()

	cb.Success()
	if cb.State() != CircuitClosed {
		t.Fatal("expected closed after half-open success")
	}
}

func TestCircuitBreaker_HalfOpenFailureReopensImmediately(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(1, 10*time.Millisecond)
	cb.Failure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow()

	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected half-open after cooldown probe")
	}

	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected open after half-open failure (immediate reopen)")
	}
}

func TestCircuitBreaker_ProbeFailThenProbeSucceed(t *testing.T) {
	t.Parallel()
	t.Skip("flaky: timing-dependent; use TestCircuitBreaker_ProbeFailThenProbeSucceed_Manual instead")
}

func TestCircuitBreaker_ProbeFailThenProbeSucceed_Manual(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(1, 1*time.Millisecond)
	cb.Failure()
	cb.lastFailureTime = time.Now().Add(-10 * time.Millisecond)

	cb.Allow()
	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected half-open after cooldown")
	}

	cb.Failure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected open after half-open failure")
	}

	cb.lastFailureTime = time.Now().Add(-10 * time.Millisecond)
	cb.Allow()
	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected half-open on second cooldown probe")
	}

	cb.Success()
	if cb.State() != CircuitClosed {
		t.Fatal("expected closed after half-open success")
	}
}
