package router

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

// scriptedStreamProvider streams a fixed script through
// core.CoordinateStreamResult, the way real adapters wrap their streams. With
// hold set, the stream stays open after the script until its context ends.
type scriptedStreamProvider struct {
	name    string
	events  []core.FluxStreamEvent
	openErr error
	hold    bool
	calls   atomic.Int32
}

func (p *scriptedStreamProvider) Chat(ctx context.Context, _ []core.FluxMessage, _ core.ChatOptions) (*core.FluxResponse, error) {
	p.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.openErr != nil {
		return nil, p.openErr
	}
	return &core.FluxResponse{Content: "from " + p.name}, nil
}

func (p *scriptedStreamProvider) StreamChat(ctx context.Context, _ []core.FluxMessage, _ core.ChatOptions) (*core.StreamResult, error) {
	p.calls.Add(1)
	if p.openErr != nil {
		return nil, p.openErr
	}
	ch := make(chan core.FluxStreamEvent)
	go func() {
		defer close(ch)
		for _, event := range p.events {
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
		if p.hold {
			<-ctx.Done()
		}
	}()
	return core.CoordinateStreamResult(ctx, llm.NewStreamResult(ch, p.name+"-request", nil)), nil
}

func (p *scriptedStreamProvider) Ping(context.Context) error { return nil }
func (p *scriptedStreamProvider) Name() string               { return p.name }

func healthyScript(name string) []core.FluxStreamEvent {
	return []core.FluxStreamEvent{
		{Type: "content", Content: "from " + name},
		{Type: "done", StopReason: "end_turn"},
	}
}

func newTwoStageRouter(t *testing.T, primary, fallback core.Provider) *DeploymentRouter {
	t.Helper()
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: primary},
			"anthropic-vertex": {Provider: fallback},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{"anthropic": {
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}}},
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-vertex", Weight: 100}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func startRouterStream(t *testing.T, ctx context.Context, r *DeploymentRouter) *core.StreamResult {
	t.Helper()
	stream, err := r.StreamChat(ctx, []core.FluxMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stream.Close)
	return stream
}

// drainStream reads until the channel closes, failing the test if it stalls.
func drainStream(t *testing.T, stream *core.StreamResult) []core.FluxStreamEvent {
	t.Helper()
	var events []core.FluxStreamEvent
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-deadline:
			t.Fatalf("stream did not close; events so far: %+v", events)
		}
	}
}

func eventTypes(events []core.FluxStreamEvent) []string {
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func breakerFailures(r *DeploymentRouter, deploymentID string) int {
	cb := r.getCircuitBreaker(deploymentID)
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failureCount
}

func assertFailedOver(t *testing.T, events []core.FluxStreamEvent, fallback *scriptedStreamProvider) {
	t.Helper()
	for _, event := range events {
		if event.Type == "error" || event.Type == "cancelled" {
			t.Fatalf("unexpected %s event %+v; events = %v", event.Type, event, eventTypes(events))
		}
	}
	if len(events) == 0 || events[len(events)-1].Type != "done" {
		t.Fatalf("events = %v, want the fallback to finish with done", eventTypes(events))
	}
	if got := fallback.calls.Load(); got != 1 {
		t.Fatalf("fallback calls = %d, want 1", got)
	}
}

func TestDeploymentRouterUpstreamTimeoutFailsOver(t *testing.T) {
	t.Parallel()
	primary := &scriptedStreamProvider{name: "direct", events: []core.FluxStreamEvent{
		{Type: "error", Error: "context deadline exceeded (Client.Timeout exceeded while reading body)"},
	}}
	fallback := &scriptedStreamProvider{name: "vertex", events: healthyScript("vertex")}
	r := newTwoStageRouter(t, primary, fallback)

	events := drainStream(t, startRouterStream(t, context.Background(), r))
	assertFailedOver(t, events, fallback)
	if got := breakerFailures(r, "anthropic-direct"); got != 1 {
		t.Fatalf("primary breaker failures = %d, want 1 (an upstream timeout is a health signal)", got)
	}
}

func TestDeploymentRouterUpstreamCancelledEventFailsOver(t *testing.T) {
	t.Parallel()
	// A "cancelled" event while the caller's context is live comes from a
	// context the deployment owns, such as an adapter-side timeout.
	primary := &scriptedStreamProvider{name: "direct", events: []core.FluxStreamEvent{
		{Type: "cancelled", Error: "context canceled"},
	}}
	fallback := &scriptedStreamProvider{name: "vertex", events: healthyScript("vertex")}
	r := newTwoStageRouter(t, primary, fallback)

	assertFailedOver(t, drainStream(t, startRouterStream(t, context.Background(), r)), fallback)
}

func TestDeploymentRouterStreamOpenTimeoutFailsOver(t *testing.T) {
	t.Parallel()
	// net/http's Client.Timeout errors match context.DeadlineExceeded.
	primary := &scriptedStreamProvider{name: "direct", openErr: fmt.Errorf("post: %w", context.DeadlineExceeded)}
	fallback := &scriptedStreamProvider{name: "vertex", events: healthyScript("vertex")}
	r := newTwoStageRouter(t, primary, fallback)

	assertFailedOver(t, drainStream(t, startRouterStream(t, context.Background(), r)), fallback)
	if got := breakerFailures(r, "anthropic-direct"); got != 1 {
		t.Fatalf("primary breaker failures = %d, want 1", got)
	}
}

func TestDeploymentRouterCallerCancelEmitsSingleTerminal(t *testing.T) {
	t.Parallel()
	primary := &scriptedStreamProvider{name: "direct", hold: true, events: []core.FluxStreamEvent{
		{Type: "content", Content: "partial"},
	}}
	fallback := &scriptedStreamProvider{name: "vertex", events: healthyScript("vertex")}
	r := newTwoStageRouter(t, primary, fallback)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := startRouterStream(t, ctx, r)
	for event := range stream.Events {
		if event.Type == "content" {
			break
		}
	}
	cancel()

	events := drainStream(t, stream)
	if len(events) != 1 || events[0].Type != "cancelled" {
		t.Fatalf("events after cancel = %v, want exactly one cancelled terminal", eventTypes(events))
	}
	if info := events[0].ErrorInfo; info == nil || info.Kind != llm.ErrKindCanceled || info.Retryable {
		t.Fatalf("terminal error info = %+v, want non-retryable canceled", info)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("fallback calls = %d, want no failover on caller cancellation", got)
	}
	if got := breakerFailures(r, "anthropic-direct"); got != 0 {
		t.Fatalf("primary breaker failures = %d, want 0 for caller cancellation", got)
	}
}

func TestDeploymentRouterChatCallerDeadlineStopsFailover(t *testing.T) {
	t.Parallel()
	primary := &scriptedStreamProvider{name: "direct"}
	fallback := &scriptedStreamProvider{name: "vertex"}
	r := newTwoStageRouter(t, primary, fallback)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := r.Chat(ctx, []core.FluxMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the caller's deadline", err)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("fallback calls = %d, want no failover after the caller's deadline", got)
	}
	if got := breakerFailures(r, "anthropic-direct"); got != 0 {
		t.Fatalf("primary breaker failures = %d, want 0 for the caller's deadline", got)
	}
}

func TestDeploymentRouterForwardsEventsAfterOutputUntilDone(t *testing.T) {
	t.Parallel()
	// Anthropic reports output usage in message_delta, after the text and
	// before message_stop; signed thinking blocks also follow output.
	primary := &scriptedStreamProvider{name: "direct", events: []core.FluxStreamEvent{
		{Type: "usage", Usage: &core.FluxUsage{PromptTokens: 10}},
		{Type: "content", Content: "a"},
		{Type: "usage", Usage: &core.FluxUsage{CompletionTokens: 3}},
		{Type: "provider_block", ProviderBlock: &llm.ProviderBlock{Provider: "anthropic", Type: "thinking"}},
		{Type: "content", Content: "b"},
		{Type: "done", StopReason: "end_turn"},
	}}
	fallback := &scriptedStreamProvider{name: "vertex", events: healthyScript("vertex")}
	r := newTwoStageRouter(t, primary, fallback)

	events := drainStream(t, startRouterStream(t, context.Background(), r))
	want := []string{"route_changed", "usage", "content", "usage", "provider_block", "content", "done"}
	if got := eventTypes(events); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Fatalf("fallback calls = %d, want 0", got)
	}
}
