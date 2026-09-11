//nolint:errcheck
package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// latencyMockProvider sleeps for a fixed delay before returning, so latency-based
// selection can be exercised deterministically.
type latencyMockProvider struct {
	name  string
	delay time.Duration
}

func (m *latencyMockProvider) Chat(ctx context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &client.EyrieResponse{Content: "from " + m.name}, nil
}

func (m *latencyMockProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 1)
	ch <- client.EyrieStreamEvent{Type: "done"}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}
func (m *latencyMockProvider) Ping(_ context.Context) error { return nil }
func (m *latencyMockProvider) Name() string                 { return m.name }

// usageMockProvider returns a fixed token usage per call.
type usageMockProvider struct {
	name   string
	tokens int
}

func (m *usageMockProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{
		Content: "from " + m.name,
		Usage:   &client.EyrieUsage{TotalTokens: m.tokens},
	}, nil
}

func (m *usageMockProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 1)
	ch <- client.EyrieStreamEvent{Type: "done"}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}
func (m *usageMockProvider) Ping(_ context.Context) error { return nil }
func (m *usageMockProvider) Name() string                 { return m.name }

func TestDefaultStrategyIsWeighted(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, nil)
	if r.strategy != StrategyWeighted {
		t.Errorf("default strategy = %q, want %q", r.strategy, StrategyWeighted)
	}
}

func TestWithStrategyOption(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	r := New([]RouteEntry{{Provider: p, Weight: 100}}, nil, nil, WithStrategy(StrategyLeastBusy))
	if r.strategy != StrategyLeastBusy {
		t.Errorf("strategy = %q, want %q", r.strategy, StrategyLeastBusy)
	}
}

func TestSimpleShuffleDistribution(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	// Heavily skewed weights, but simple-shuffle must ignore them and pick uniformly.
	r := New([]RouteEntry{{Provider: p1, Weight: 99}, {Provider: p2, Weight: 1}}, nil, nil, WithStrategy(StrategySimpleShuffle))

	counts := map[string]int{}
	const n = 4000
	for i := 0; i < n; i++ {
		resp, _ := r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
		counts[resp.Content]++
	}
	// Each provider should get roughly half (allow generous slack for randomness).
	for _, name := range []string{"from p1", "from p2"} {
		if counts[name] < n*3/10 || counts[name] > n*7/10 {
			t.Errorf("%s got %d/%d, expected ~%d (uniform)", name, counts[name], n, n/2)
		}
	}
}

func TestLeastBusySelectsIdleProvider(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 1}, {Provider: p2, Weight: 1}}, nil, nil, WithStrategy(StrategyLeastBusy))

	// Mark p1 as busy with 3 in-flight requests; p2 should be selected.
	r.stratState.beginInFlight("p1")
	r.stratState.beginInFlight("p1")
	r.stratState.beginInFlight("p1")

	e := r.selectEntry()
	if e.Provider.Name() != "p2" {
		t.Errorf("least-busy selected %q, want p2 (p1 has 3 in-flight)", e.Provider.Name())
	}

	// Drain p1; now both are idle and p1 (first) wins the tie.
	r.stratState.endInFlight("p1")
	r.stratState.endInFlight("p1")
	r.stratState.endInFlight("p1")
	if e := r.selectEntry(); e.Provider.Name() != "p1" {
		t.Errorf("tie selected %q, want p1 (first on tie)", e.Provider.Name())
	}
}

func TestLatencyBasedSelectsFastest(t *testing.T) {
	t.Parallel()
	fast := &latencyMockProvider{name: "fast", delay: 0}
	slow := &latencyMockProvider{name: "slow", delay: 30 * time.Millisecond}
	r := New([]RouteEntry{{Provider: slow, Weight: 1}, {Provider: fast, Weight: 1}}, nil, nil, WithStrategy(StrategyLatencyBased))

	// Seed observed latencies directly so selection is deterministic.
	r.stratState.recordLatency("slow", 30)
	r.stratState.recordLatency("fast", 1)

	e := r.selectEntry()
	if e.Provider.Name() != "fast" {
		t.Errorf("latency-based selected %q, want fast", e.Provider.Name())
	}
}

func TestLatencyBasedRecordsEWMA(t *testing.T) {
	t.Parallel()
	p := &latencyMockProvider{name: "p", delay: 5 * time.Millisecond}
	r := New([]RouteEntry{{Provider: p, Weight: 1}}, nil, nil, WithStrategy(StrategyLatencyBased))

	r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	lat, ok := r.stratState.latency("p")
	if !ok {
		t.Fatal("expected a latency sample after Chat")
	}
	if lat <= 0 {
		t.Errorf("recorded latency = %v, want > 0", lat)
	}
}

func TestCostBasedSelectsCheapest(t *testing.T) {
	t.Parallel()
	cheap := &mockProvider{name: "cheap"}
	pricey := &mockProvider{name: "pricey"}
	r := New([]RouteEntry{
		{Provider: pricey, Weight: 1, Cost: 100},
		{Provider: cheap, Weight: 1, Cost: 5},
	}, nil, nil, WithStrategy(StrategyCostBased))

	e := r.selectEntry()
	if e.Provider.Name() != "cheap" {
		t.Errorf("cost-based selected %q, want cheap", e.Provider.Name())
	}
}

func TestCostBasedFallsBackToWeight(t *testing.T) {
	t.Parallel()
	a := &mockProvider{name: "a"}
	b := &mockProvider{name: "b"}
	// No Cost set; Weight is used as the cost proxy, so lower weight wins.
	r := New([]RouteEntry{
		{Provider: a, Weight: 80},
		{Provider: b, Weight: 20},
	}, nil, nil, WithStrategy(StrategyCostBased))

	e := r.selectEntry()
	if e.Provider.Name() != "b" {
		t.Errorf("cost-based (weight proxy) selected %q, want b (lower weight)", e.Provider.Name())
	}
}

func TestUsageBasedSelectsLeastUsed(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 1}, {Provider: p2, Weight: 1}}, nil, nil, WithStrategy(StrategyUsageBased))

	r.stratState.recordUsage("p1", 5000)
	r.stratState.recordUsage("p2", 100)

	e := r.selectEntry()
	if e.Provider.Name() != "p2" {
		t.Errorf("usage-based selected %q, want p2 (fewer tokens)", e.Provider.Name())
	}
}

func TestUsageBasedRecordsTokens(t *testing.T) {
	t.Parallel()
	p := &usageMockProvider{name: "p", tokens: 250}
	r := New([]RouteEntry{{Provider: p, Weight: 1}}, nil, nil, WithStrategy(StrategyUsageBased))

	r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})

	if got := r.stratState.usage["p"].Load(); got != 500 {
		t.Errorf("recorded usage = %d, want 500", got)
	}
}

func TestWeightedStrategyZeroWeightFallback(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	r := New([]RouteEntry{{Provider: p, Weight: 0}}, nil, nil)
	if e := r.selectEntry(); e.Provider.Name() != "p" {
		t.Errorf("zero-weight weighted selected %q, want p", e.Provider.Name())
	}
}

func TestInFlightDecrementedAfterChat(t *testing.T) {
	t.Parallel()
	p := &mockProvider{name: "p"}
	r := New([]RouteEntry{{Provider: p, Weight: 1}}, nil, nil, WithStrategy(StrategyLeastBusy))

	r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
	if got := r.stratState.inFlight["p"].Load(); got != 0 {
		t.Errorf("in-flight after Chat = %d, want 0", got)
	}
}

func TestLeastBusyConcurrentSafe(t *testing.T) {
	t.Parallel()
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	r := New([]RouteEntry{{Provider: p1, Weight: 1}, {Provider: p2, Weight: 1}}, nil, nil, WithStrategy(StrategyLeastBusy))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Chat(context.Background(), []client.EyrieMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{})
		}()
	}
	wg.Wait()
	for _, name := range []string{"p1", "p2"} {
		if got := r.stratState.inFlight[name].Load(); got != 0 {
			t.Errorf("in-flight[%s] = %d, want 0 after all calls returned", name, got)
		}
	}
}
