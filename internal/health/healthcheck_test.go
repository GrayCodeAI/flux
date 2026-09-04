package graycoderouter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockProvider struct {
	name string
	ping func(context.Context) error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Ping(ctx context.Context) error {
	if m.ping == nil {
		return nil
	}
	return m.ping(ctx)
}

func fullConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		DegradedThreshold: 2 * time.Second,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	}
}

func TestNewHealthChecker_Defaults(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(HealthCheckConfig{})
	if hc == nil {
		t.Fatal("expected non-nil HealthChecker")
	}
	if hc.config.Interval != 30*time.Second {
		t.Errorf("expected default Interval 30s, got %v", hc.config.Interval)
	}
	if hc.config.Timeout != 5*time.Second {
		t.Errorf("expected default Timeout 5s, got %v", hc.config.Timeout)
	}
	if hc.config.DegradedThreshold != 2*time.Second {
		t.Errorf("expected default DegradedThreshold 2s, got %v", hc.config.DegradedThreshold)
	}
	if hc.config.UnhealthyAfter != 3 {
		t.Errorf("expected default UnhealthyAfter 3, got %d", hc.config.UnhealthyAfter)
	}
	if hc.config.DegradedAfter != 1 {
		t.Errorf("expected default DegradedAfter 1, got %d", hc.config.DegradedAfter)
	}
}

func TestNewHealthChecker_CustomConfig(t *testing.T) {
	t.Parallel()
	cfg := HealthCheckConfig{
		Interval:          10 * time.Second,
		Timeout:           1 * time.Second,
		DegradedThreshold: 500 * time.Millisecond,
		UnhealthyAfter:    5,
		DegradedAfter:     2,
	}
	hc := NewHealthChecker(cfg)
	if hc.config.Interval != 10*time.Second {
		t.Errorf("expected custom Interval 10s, got %v", hc.config.Interval)
	}
}

func TestRegister_StartsHealthy(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{name: "test-provider"})
	status := hc.Check("test-provider")
	if status.State != Healthy {
		t.Errorf("expected Healthy, got %v", status.State)
	}
	if status.Message != "ok" {
		t.Errorf("expected message 'ok', got %q", status.Message)
	}
}

func TestRegister_Unregister(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{name: "p1"})
	hc.Unregister("p1")
	status := hc.Check("p1")
	if status.State != Unhealthy {
		t.Errorf("expected Unhealthy for unregistered provider, got %v", status.State)
	}
}

func TestRecordSuccess_TransitionToHealthy(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{name: "p1"})
	status := hc.Check("p1")
	if status.State != Healthy {
		t.Errorf("expected Healthy after success, got %v", status.State)
	}
	if status.Error != "" {
		t.Errorf("expected no error, got %q", status.Error)
	}
}

func TestRecordFailure_TransitionToDegraded(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{
		name: "p1",
		ping: func(ctx context.Context) error {
			return errors.New("connection refused")
		},
	})
	status := hc.Check("p1")
	if status.State != Degraded {
		t.Errorf("expected Degraded after 1 failure, got %v", status.State)
	}
	if status.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", status.Error)
	}
}

func TestRecordFailure_TransitionToUnhealthy(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(HealthCheckConfig{
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		DegradedThreshold: 2 * time.Second,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	})
	hc.Register(&mockProvider{
		name: "p1",
		ping: func(ctx context.Context) error {
			return errors.New("timeout")
		},
	})

	for i := 0; i < 3; i++ {
		status := hc.checkProvider(hc.providers["p1"])
		hc.updateStatus("p1", status)
		switch {
		case i < 2 && status.State != Degraded:
			t.Errorf("iteration %d: expected Degraded, got %v", i, status.State)
		case i == 2 && status.State != Unhealthy:
			t.Errorf("iteration %d: expected Unhealthy, got %v", i, status.State)
		}
	}
}

func TestRecoveryAfterFailure(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	fail := true
	hc.Register(&mockProvider{
		name: "p1",
		ping: func(ctx context.Context) error {
			if fail {
				return errors.New("fail")
			}
			return nil
		},
	})

	status := hc.checkProvider(hc.providers["p1"])
	hc.updateStatus("p1", status)
	if status.State != Degraded {
		t.Errorf("expected Degraded after failure, got %v", status.State)
	}

	fail = false
	hc.mu.RLock()
	p := hc.providers["p1"]
	hc.mu.RUnlock()
	status = hc.checkProvider(p)
	hc.updateStatus("p1", status)
	if status.State != Healthy {
		t.Errorf("expected Healthy after recovery, got %v", status.State)
	}
	if status.Error != "" {
		t.Errorf("expected no error after recovery, got %q", status.Error)
	}
}

func TestLatencyThreshold_Degradation(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(HealthCheckConfig{
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		DegradedThreshold: 10 * time.Millisecond,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	})
	hc.Register(&mockProvider{
		name: "p1",
		ping: func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	})
	status := hc.Check("p1")
	if status.State != Degraded {
		t.Errorf("expected Degraded due to high latency, got %v", status.State)
	}
	if status.Latency <= 0 {
		t.Errorf("expected positive latency, got %v", status.Latency)
	}
}

func TestLatencyThreshold_Healthy(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(HealthCheckConfig{
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		DegradedThreshold: 10 * time.Second,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	})
	hc.Register(&mockProvider{name: "p1"})
	status := hc.Check("p1")
	if status.State != Healthy {
		t.Errorf("expected Healthy, got %v", status.State)
	}
	if status.Latency <= 0 {
		t.Errorf("expected positive latency, got %v", status.Latency)
	}
}

func TestNilHealthChecker_Safety(t *testing.T) {
	t.Parallel()
	var hc *HealthChecker
	hc.Register(&mockProvider{name: "p1"})
	hc.Unregister("p1")
	status := hc.Check("p1")
	if status.State != Unhealthy {
		t.Errorf("expected Unhealthy from nil checker, got %v", status.State)
	}
	if result := hc.AllProviderHealth(); result != nil {
		t.Errorf("expected nil, got %v", result)
	}
	hc.Start()
	hc.Stop()
}

func TestConcurrentAccessSafety(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{name: "p1"})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = hc.Check("p1")
		}()
	}
	wg.Wait()
	hc.Register(&mockProvider{name: "p2"})
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = hc.AllProviderHealth()
		}()
	}
	wg.Wait()
	status := hc.Check("p1")
	if status.State != Healthy {
		t.Errorf("expected Healthy after concurrent checks, got %v", status.State)
	}
}

func TestStatusFormatting(t *testing.T) {
	t.Parallel()
	if Healthy.String() != "healthy" {
		t.Errorf("expected 'healthy', got %q", Healthy.String())
	}
	if Degraded.String() != "degraded" {
		t.Errorf("expected 'degraded', got %q", Degraded.String())
	}
	if Unhealthy.String() != "unhealthy" {
		t.Errorf("expected 'unhealthy', got %q", Unhealthy.String())
	}
	if HealthState(99).String() != "unknown" {
		t.Errorf("expected 'unknown', got %q", HealthState(99).String())
	}
	{
		hs := HealthStatus{State: Healthy}
		if !hs.IsHealthy() {
			t.Errorf("expected IsHealthy() = true for Healthy")
		}
	}
	{
		hs := HealthStatus{State: Degraded}
		if hs.IsHealthy() {
			t.Errorf("expected IsHealthy() = false for Degraded")
		}
	}
	{
		hs := HealthStatus{State: Unhealthy}
		if hs.IsHealthy() {
			t.Errorf("expected IsHealthy() = false for Unhealthy")
		}
	}
}

func TestAllProviderHealth(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	hc.Register(&mockProvider{name: "p1"})
	hc.Register(&mockProvider{name: "p2"})
	results := hc.AllProviderHealth()
	if len(results) != 2 {
		t.Errorf("expected 2 providers, got %d", len(results))
	}
	if _, ok := results["p1"]; !ok {
		t.Errorf("expected p1 in results")
	}
}

func TestStartStop(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(HealthCheckConfig{
		Interval:          50 * time.Millisecond,
		Timeout:           5 * time.Second,
		DegradedThreshold: 2 * time.Second,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	})
	hc.Register(&mockProvider{name: "p1"})
	hc.Start()
	defer hc.Stop()
	time.Sleep(100 * time.Millisecond)
	status := hc.Check("p1")
	if status.State != Healthy {
		t.Errorf("expected Healthy after background checks, got %v", status.State)
	}
	hc.Start()
}

func TestUnregisteredProviderCheck(t *testing.T) {
	t.Parallel()
	hc := NewHealthChecker(fullConfig())
	status := hc.Check("nonexistent")
	if status.State != Unhealthy {
		t.Errorf("expected Unhealthy for unregistered provider, got %v", status.State)
	}
	if status.Error == "" {
		t.Errorf("expected error message for unregistered provider")
	}
}
