package graycoderouter

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthState represents the health condition of a provider.
type HealthState int

const (
	// Healthy indicates the provider is responding normally.
	Healthy HealthState = iota
	// Degraded indicates the provider is responding but with elevated latency or intermittent errors.
	Degraded
	// Unhealthy indicates the provider is not responding or consistently failing.
	Unhealthy
)

// String returns a human-readable representation of HealthState.
func (h HealthState) String() string {
	switch h {
	case Healthy:
		return "healthy"
	case Degraded:
		return "degraded"
	case Unhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthStatus holds the current health status for a provider, including
// measured latency and the time of the last health check.
type HealthStatus struct {
	State       HealthState   `json:"state"`
	Latency     time.Duration `json:"latency"`
	LastChecked time.Time     `json:"last_checked"`
	Error       string        `json:"error,omitempty"`
	Message     string        `json:"message,omitempty"`
}

// IsHealthy returns true if the provider state is Healthy.
func (hs HealthStatus) IsHealthy() bool {
	return hs.State == Healthy
}

// ProviderPinger is the interface that providers must implement for health checking.
// This is satisfied by the client.Provider interface's Ping method.
type ProviderPinger interface {
	Ping(ctx context.Context) error
	Name() string
}

// HealthCheckConfig configures the HealthChecker behavior.
type HealthCheckConfig struct {
	// Interval between periodic health checks. Default: 30s.
	Interval time.Duration
	// Timeout for each individual ping. Default: 5s.
	Timeout time.Duration
	// DegradedThreshold: latency above this marks provider as Degraded. Default: 2s.
	DegradedThreshold time.Duration
	// UnhealthyAfter: number of consecutive failures before marking Unhealthy. Default: 3.
	UnhealthyAfter int
	// DegradedAfter: number of consecutive failures before marking Degraded. Default: 1.
	DegradedAfter int
}

// DefaultHealthCheckConfig returns sensible default configuration.
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Interval:          30 * time.Second,
		Timeout:           5 * time.Second,
		DegradedThreshold: 2 * time.Second,
		UnhealthyAfter:    3,
		DegradedAfter:     1,
	}
}

// HealthChecker periodically pings LLM providers to determine their health.
// It is safe for concurrent use. A nil *HealthChecker is safe (all methods are no-ops).
type HealthChecker struct {
	mu        sync.RWMutex
	providers map[string]ProviderPinger
	statuses  map[string]*healthEntry
	config    HealthCheckConfig

	cancel context.CancelFunc
	done   chan struct{}
}

type healthEntry struct {
	status              HealthStatus
	consecutiveFailures int
}

// NewHealthChecker creates a new HealthChecker with the given configuration.
// Pass a zero-value config to use defaults.
func NewHealthChecker(cfg HealthCheckConfig) *HealthChecker {
	if cfg.Interval == 0 {
		cfg = DefaultHealthCheckConfig()
	}
	return &HealthChecker{
		providers: make(map[string]ProviderPinger),
		statuses:  make(map[string]*healthEntry),
		config:    cfg,
	}
}

// Register adds a provider to be health-checked. Can be called at any time.
func (hc *HealthChecker) Register(provider ProviderPinger) {
	if hc == nil || provider == nil {
		return
	}
	hc.mu.Lock()
	defer hc.mu.Unlock()
	name := provider.Name()
	hc.providers[name] = provider
	if _, ok := hc.statuses[name]; !ok {
		hc.statuses[name] = &healthEntry{
			status: HealthStatus{
				State:   Healthy,
				Message: "not yet checked",
			},
		}
	}
}

// Unregister removes a provider from health checking.
func (hc *HealthChecker) Unregister(name string) {
	if hc == nil {
		return
	}
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.providers, name)
	delete(hc.statuses, name)
}

// Check performs an immediate health check on a single provider and returns
// the resulting HealthStatus. Returns an Unhealthy status if the provider is
// not registered.
func (hc *HealthChecker) Check(provider string) HealthStatus {
	if hc == nil {
		return HealthStatus{State: Unhealthy, Error: "health checker is nil"}
	}

	hc.mu.RLock()
	p, ok := hc.providers[provider]
	hc.mu.RUnlock()

	if !ok {
		return HealthStatus{
			State:       Unhealthy,
			LastChecked: time.Now(),
			Error:       fmt.Sprintf("provider %q not registered", provider),
		}
	}

	return hc.checkProvider(p)
}

// AllProviderHealth returns the current health status of all registered providers.
func (hc *HealthChecker) AllProviderHealth() map[string]HealthStatus {
	if hc == nil {
		return nil
	}
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[string]HealthStatus, len(hc.statuses))
	for name, entry := range hc.statuses {
		result[name] = entry.status
	}
	return result
}

// Start begins periodic background health checks. Call Stop() to halt.
// No-op if already started or if hc is nil.
func (hc *HealthChecker) Start() {
	if hc == nil {
		return
	}
	hc.mu.Lock()
	if hc.cancel != nil {
		hc.mu.Unlock()
		return // already running
	}
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel
	hc.done = make(chan struct{})
	hc.mu.Unlock()

	go hc.loop(ctx)
}

// Stop halts periodic health checking. Blocks until the background loop exits.
// No-op if not started or if hc is nil.
func (hc *HealthChecker) Stop() {
	if hc == nil {
		return
	}
	hc.mu.Lock()
	if hc.cancel == nil {
		hc.mu.Unlock()
		return
	}
	hc.cancel()
	hc.cancel = nil
	done := hc.done
	hc.mu.Unlock()

	if done != nil {
		<-done
	}
}

// loop runs periodic checks until the context is cancelled.
func (hc *HealthChecker) loop(ctx context.Context) {
	defer close(hc.done)

	// Do an initial check immediately.
	hc.checkAll()

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll()
		}
	}
}

// checkAll pings all registered providers concurrently.
func (hc *HealthChecker) checkAll() {
	hc.mu.RLock()
	providers := make([]ProviderPinger, 0, len(hc.providers))
	for _, p := range hc.providers {
		providers = append(providers, p)
	}
	hc.mu.RUnlock()

	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(prov ProviderPinger) {
			defer wg.Done()
			status := hc.checkProvider(prov)
			hc.updateStatus(prov.Name(), status)
		}(p)
	}
	wg.Wait()
}

// checkProvider performs a single ping and returns the resulting status.
func (hc *HealthChecker) checkProvider(p ProviderPinger) HealthStatus {
	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()

	start := time.Now()
	err := p.Ping(ctx)
	latency := time.Since(start)
	if latency == 0 {
		latency = time.Nanosecond
	}

	status := HealthStatus{
		Latency:     latency,
		LastChecked: time.Now(),
	}

	switch {
	case err != nil:
		status.Error = err.Error()
		// Determine if degraded or unhealthy based on consecutive failures.
		hc.mu.RLock()
		entry, ok := hc.statuses[p.Name()]
		hc.mu.RUnlock()

		failures := 1
		if ok {
			failures = entry.consecutiveFailures + 1
		}
		switch {
		case failures >= hc.config.UnhealthyAfter:
			status.State = Unhealthy
			status.Message = fmt.Sprintf("unhealthy after %d consecutive failures", failures)
		case failures >= hc.config.DegradedAfter:
			status.State = Degraded
			status.Message = fmt.Sprintf("degraded after %d consecutive failures", failures)
		default:
			status.State = Degraded
			status.Message = "single failure"
		}
	case latency > hc.config.DegradedThreshold:
		status.State = Degraded
		status.Message = fmt.Sprintf("high latency: %v", latency)
	default:
		status.State = Healthy
		status.Message = "ok"
	}

	return status
}

// updateStatus records the check result and updates consecutive failure count.
func (hc *HealthChecker) updateStatus(name string, status HealthStatus) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	entry, ok := hc.statuses[name]
	if !ok {
		entry = &healthEntry{}
		hc.statuses[name] = entry
	}

	if status.Error != "" {
		entry.consecutiveFailures++
	} else {
		entry.consecutiveFailures = 0
	}
	entry.status = status
}
