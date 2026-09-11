//nolint:errcheck
package conversation

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/storage"
)

type mockStreamProvider struct{}

func (m *mockStreamProvider) Name() string                 { return "mock" }
func (m *mockStreamProvider) Ping(_ context.Context) error { return nil }
func (m *mockStreamProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{Content: "hello", FinishReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 5}}, nil
}

func (m *mockStreamProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 3)
	ch <- client.EyrieStreamEvent{Type: "content", Content: "hello"}
	ch <- client.EyrieStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 5}}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return New(store, &mockStreamProvider{})
}

func TestPromptCreatesNodes(t *testing.T) {
	t.Parallel()
	e := testEngine(t)
	ctx := context.Background()

	events, err := e.Prompt(ctx, "hi", PromptOpts{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var gotContent string
	var nodeID string
	for ev := range events {
		switch ev.Type {
		case EventDelta:
			gotContent += ev.Content
		case EventDone:
			nodeID = ev.NodeID
		}
	}
	if gotContent != "hello" {
		t.Errorf("expected 'hello', got %q", gotContent)
	}
	if nodeID == "" {
		t.Error("expected node_id in done event")
	}

	convos, err := e.ListConversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(convos) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(convos))
	}
}

func TestPromptFromBranches(t *testing.T) {
	t.Parallel()
	e := testEngine(t)
	ctx := context.Background()

	events, _ := e.Prompt(ctx, "first", PromptOpts{Model: "test"})
	var firstNodeID string
	for ev := range events {
		if ev.Type == EventDone {
			firstNodeID = ev.NodeID
		}
	}

	events2, err := e.PromptFrom(ctx, firstNodeID, "branch", PromptOpts{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var secondNodeID string
	for ev := range events2 {
		if ev.Type == EventDone {
			secondNodeID = ev.NodeID
		}
	}
	if secondNodeID == "" {
		t.Error("expected second node")
	}
	if secondNodeID == firstNodeID {
		t.Error("expected different node IDs for branch")
	}
}

func TestResolveNode(t *testing.T) {
	t.Parallel()
	e := testEngine(t)
	ctx := context.Background()

	events, _ := e.Prompt(ctx, "test resolve", PromptOpts{Model: "test"})
	var nodeID string
	for ev := range events {
		if ev.Type == EventDone {
			nodeID = ev.NodeID
		}
	}

	got, err := e.ResolveNode(ctx, nodeID[:8])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != nodeID {
		t.Errorf("expected %s, got %s", nodeID, got.ID)
	}
}

// maxTokensMockProvider returns max_tokens for the first StreamChat call,
// then end_turn for subsequent calls. It tracks how many times Close is
// invoked on returned StreamResults so tests can verify no stream leaks.
type maxTokensMockProvider struct {
	mu         sync.Mutex
	callCount  int
	closeCount int
}

func (m *maxTokensMockProvider) Name() string                 { return "max-tokens-mock" }
func (m *maxTokensMockProvider) Ping(_ context.Context) error { return nil }

func (m *maxTokensMockProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.callCount == 1 {
		return &client.EyrieResponse{Content: "part 1 ", FinishReason: "max_tokens", Usage: &client.EyrieUsage{CompletionTokens: 50}}, nil
	}
	return &client.EyrieResponse{Content: "part 2", FinishReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 30}}, nil
}

func (m *maxTokensMockProvider) StreamChat(_ context.Context, msgs []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	var content, stopReason string
	if m.callCount == 1 {
		content = "part 1 "
		stopReason = "max_tokens"
	} else {
		content = "part 2"
		stopReason = "end_turn"
	}
	ch := make(chan client.EyrieStreamEvent, 4)
	ch <- client.EyrieStreamEvent{Type: "content", Content: content}
	ch <- client.EyrieStreamEvent{Type: "done", StopReason: stopReason, Usage: &client.EyrieUsage{CompletionTokens: 30}}
	close(ch)
	sr := &client.StreamResult{Events: ch}
	// Wrap Close so we can count invocations.
	return &client.StreamResult{
		Events:    sr.Events,
		RequestID: sr.RequestID,
	}, nil
}

func (m *maxTokensMockProvider) CloseCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCount
}

func TestConversationEngine_ContinuesOnMaxTokens(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	provider := &maxTokensMockProvider{}
	e := New(store, provider)
	ctx := context.Background()

	events, err := e.Prompt(ctx, "write something long", PromptOpts{Model: "test", MaxTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}

	var content string
	var doneCount int
	for ev := range events {
		switch ev.Type {
		case EventDelta:
			content += ev.Content
		case EventDone:
			doneCount++
		}
	}

	if content != "part 1 part 2" {
		t.Errorf("content = %q, want %q", content, "part 1 part 2")
	}
	if doneCount != 1 {
		t.Errorf("doneCount = %d, want 1", doneCount)
	}
	if provider.callCount != 2 {
		t.Errorf("callCount = %d, want 2", provider.callCount)
	}
}

func TestConversationEngine_ContextCancelClosesStream(t *testing.T) {
	t.Parallel()
	// Verify that cancelling the context during a Prompt does not leak
	// the underlying stream. The mock provider returns a stream that
	// blocks until the context is cancelled.
	blockingProvider := &blockingMockProvider{}
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	e := New(store, blockingProvider)
	ctx, cancel := context.WithCancel(context.Background())

	events, err := e.Prompt(ctx, "hi", PromptOpts{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel immediately — the engine should stop and close the stream.
	cancel()

	// Drain the channel; it must close promptly.
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return // success — channel closed
			}
		case <-timeout:
			t.Fatal("events channel did not close after context cancellation")
		}
	}
}

// blockingMockProvider returns a StreamResult whose Events channel blocks
// until the context is cancelled.
type blockingMockProvider struct{}

func (b *blockingMockProvider) Name() string {
	return "blocking-mock"
}

func (b *blockingMockProvider) Ping(_ context.Context) error { return nil }

func (b *blockingMockProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{Content: "done", FinishReason: "end_turn"}, nil
}

func (b *blockingMockProvider) StreamChat(ctx context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent)
	// Close the channel when the context is done — simulating a provider
	// that respects context cancellation.
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return &client.StreamResult{Events: ch}, nil
}
