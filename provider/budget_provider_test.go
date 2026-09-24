package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/GrayCodeAI/flux/provider/observability"
)

func TestMemoryBudgetStore_EnforcesLimit(t *testing.T) {
	t.Parallel()
	s := observability.NewMemoryBudgetStore()
	s.SetBudget("team-a", 1.00)

	if err := s.CheckBudget(context.Background(), "team-a", 0.50); err != nil {
		t.Fatalf("expected under-budget request to pass: %v", err)
	}
	if err := s.RecordUsage(context.Background(), "team-a", 0.80, 100, 50); err != nil {
		t.Fatal(err)
	}
	// 0.80 used + 0.50 est > 1.00 limit → exceeded.
	if err := s.CheckBudget(context.Background(), "team-a", 0.50); !errors.Is(err, observability.ErrBudgetExceeded) {
		t.Errorf("expected observability.ErrBudgetExceeded, got %v", err)
	}
	used, in, out, ok := s.Usage("team-a")
	if !ok || used != 0.80 || in != 100 || out != 50 {
		t.Errorf("unexpected usage: used=%f in=%d out=%d ok=%v", used, in, out, ok)
	}
}

func TestMemoryBudgetStore_UnknownKey(t *testing.T) {
	t.Parallel()
	s := observability.NewMemoryBudgetStore()
	if err := s.CheckBudget(context.Background(), "nope", 0.01); !errors.Is(err, observability.ErrUnknownVirtualKey) {
		t.Errorf("expected observability.ErrUnknownVirtualKey, got %v", err)
	}
}

func TestMemoryBudgetStore_UnlimitedWhenZero(t *testing.T) {
	t.Parallel()
	s := observability.NewMemoryBudgetStore()
	s.SetBudget("free", 0) // unlimited
	if err := s.CheckBudget(context.Background(), "free", 1000); err != nil {
		t.Errorf("zero limit should be unlimited, got %v", err)
	}
}

func TestBudgetProvider_BlocksOverBudget(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	store := observability.NewMemoryBudgetStore()
	store.SetBudget("tiny", 0.0000001) // effectively zero budget
	bp := observability.NewBudgetProvider(mock, store)

	_, err := bp.Chat(context.Background(),
		userMsg("a reasonably long prompt that will cost something to process"),
		ChatOptions{Model: "gpt-4o", VirtualKeyID: "tiny"})
	if !errors.Is(err, observability.ErrBudgetExceeded) {
		t.Fatalf("expected observability.ErrBudgetExceeded, got %v", err)
	}
	if mock.CallCount() != 0 {
		t.Errorf("over-budget request must not reach inner provider; got %d calls", mock.CallCount())
	}
}

func TestBudgetProvider_AllowsAndRecords(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	store := observability.NewMemoryBudgetStore()
	store.SetBudget("rich", 100.0)
	bp := observability.NewBudgetProvider(mock, store)

	ctx := observability.WithVirtualKey(context.Background(), "rich")
	if _, err := bp.Chat(ctx, userMsg("hello there"), ChatOptions{Model: "gpt-4o"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected inner call, got %d", mock.CallCount())
	}
	used, _, out, ok := store.Usage("rich")
	if !ok || used <= 0 {
		t.Errorf("expected recorded spend, got used=%f ok=%v", used, ok)
	}
	if out != 5 { // MockModeFixed reports 5 completion tokens
		t.Errorf("expected 5 output tokens recorded, got %d", out)
	}
}

func TestBudgetProvider_NoKeyPassesThrough(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	bp := observability.NewBudgetProvider(mock, observability.NewMemoryBudgetStore())
	// No virtual key in options or context → unmetered pass-through.
	if _, err := bp.Chat(context.Background(), userMsg("hi"), ChatOptions{Model: "gpt-4o"}); err != nil {
		t.Fatalf("unmetered request should succeed, got %v", err)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected pass-through call, got %d", mock.CallCount())
	}
}

func TestActualCostUSD(t *testing.T) {
	t.Parallel()
	usage := &FluxUsage{PromptTokens: 1000, CompletionTokens: 1000}
	cost := observability.ActualCostUSD("gpt-4o", usage)
	// 1000*2.5/1e6 + 1000*10/1e6 = 0.0025 + 0.01 = 0.0125
	if cost < 0.0124 || cost > 0.0126 {
		t.Errorf("unexpected cost %f", cost)
	}
}

type doneUsageStreamProvider struct {
	includeUsageEvent bool
}

func (*doneUsageStreamProvider) Name() string               { return "done-usage" }
func (*doneUsageStreamProvider) Ping(context.Context) error { return nil }
func (*doneUsageStreamProvider) Chat(context.Context, []FluxMessage, ChatOptions) (*FluxResponse, error) {
	return nil, nil
}

func (p *doneUsageStreamProvider) StreamChat(context.Context, []FluxMessage, ChatOptions) (*StreamResult, error) {
	events := make(chan FluxStreamEvent, 2)
	usage := &FluxUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}
	if p.includeUsageEvent {
		events <- FluxStreamEvent{Type: "usage", Usage: usage}
	}
	events <- FluxStreamEvent{Type: "done", Usage: usage}
	close(events)
	return NewStreamResult(events, func() {}), nil
}

func TestBudgetProvider_DeduplicatesStreamUsage(t *testing.T) {
	t.Parallel()
	for _, includeUsageEvent := range []bool{false, true} {
		t.Run(map[bool]string{false: "done_only", true: "usage_and_done"}[includeUsageEvent], func(t *testing.T) {
			store := observability.NewMemoryBudgetStore()
			store.SetBudget("stream", 1)
			provider := observability.NewBudgetProvider(&doneUsageStreamProvider{includeUsageEvent: includeUsageEvent}, store)
			result, err := provider.StreamChat(
				context.Background(),
				[]FluxMessage{{Role: "user", Content: "hello"}},
				ChatOptions{Model: "gpt-4o", MaxTokens: 100, VirtualKeyID: "stream"},
			)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			for range result.Events {
			}

			_, input, output, ok := store.Usage("stream")
			if !ok || input != 3 || output != 5 {
				t.Fatalf("usage = in:%d out:%d ok:%t, want 3/5", input, output, ok)
			}
		})
	}
}

type continuationUsageStreamProvider struct{}

func (*continuationUsageStreamProvider) Name() string               { return "continuation-usage" }
func (*continuationUsageStreamProvider) Ping(context.Context) error { return nil }
func (*continuationUsageStreamProvider) Chat(context.Context, []FluxMessage, ChatOptions) (*FluxResponse, error) {
	return nil, nil
}

func (*continuationUsageStreamProvider) StreamChat(context.Context, []FluxMessage, ChatOptions) (*StreamResult, error) {
	events := make(chan FluxStreamEvent, 4)
	usage := &FluxUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}
	events <- FluxStreamEvent{Type: "usage", Usage: usage}
	events <- FluxStreamEvent{Type: "continuation"}
	events <- FluxStreamEvent{Type: "usage", Usage: usage}
	events <- FluxStreamEvent{Type: "done", Usage: usage}
	close(events)
	return NewStreamResult(events, func() {}), nil
}

func TestBudgetProvider_ResetsUsageAtContinuation(t *testing.T) {
	t.Parallel()
	store := observability.NewMemoryBudgetStore()
	store.SetBudget("stream", 1)
	provider := observability.NewBudgetProvider(&continuationUsageStreamProvider{}, store)
	result, err := provider.StreamChat(
		context.Background(),
		[]FluxMessage{{Role: "user", Content: "hello"}},
		ChatOptions{Model: "gpt-4o", MaxTokens: 100, VirtualKeyID: "stream"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	for range result.Events {
	}

	_, input, output, ok := store.Usage("stream")
	if !ok || input != 6 || output != 10 {
		t.Fatalf("usage = in:%d out:%d ok:%t, want 6/10", input, output, ok)
	}
}
