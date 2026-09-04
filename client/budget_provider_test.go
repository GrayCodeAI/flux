package client

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryBudgetStore_EnforcesLimit(t *testing.T) {
	t.Parallel()
	s := NewMemoryBudgetStore()
	s.SetBudget("team-a", 1.00)

	if err := s.CheckBudget(context.Background(), "team-a", 0.50); err != nil {
		t.Fatalf("expected under-budget request to pass: %v", err)
	}
	if err := s.RecordUsage(context.Background(), "team-a", 0.80, 100, 50); err != nil {
		t.Fatal(err)
	}
	// 0.80 used + 0.50 est > 1.00 limit → exceeded.
	if err := s.CheckBudget(context.Background(), "team-a", 0.50); !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
	used, in, out, ok := s.Usage("team-a")
	if !ok || used != 0.80 || in != 100 || out != 50 {
		t.Errorf("unexpected usage: used=%f in=%d out=%d ok=%v", used, in, out, ok)
	}
}

func TestMemoryBudgetStore_UnknownKey(t *testing.T) {
	t.Parallel()
	s := NewMemoryBudgetStore()
	if err := s.CheckBudget(context.Background(), "nope", 0.01); !errors.Is(err, ErrUnknownVirtualKey) {
		t.Errorf("expected ErrUnknownVirtualKey, got %v", err)
	}
}

func TestMemoryBudgetStore_UnlimitedWhenZero(t *testing.T) {
	t.Parallel()
	s := NewMemoryBudgetStore()
	s.SetBudget("free", 0) // unlimited
	if err := s.CheckBudget(context.Background(), "free", 1000); err != nil {
		t.Errorf("zero limit should be unlimited, got %v", err)
	}
}

func TestBudgetProvider_BlocksOverBudget(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	store := NewMemoryBudgetStore()
	store.SetBudget("tiny", 0.0000001) // effectively zero budget
	bp := NewBudgetProvider(mock, store)

	_, err := bp.Chat(context.Background(),
		userMsg("a reasonably long prompt that will cost something to process"),
		ChatOptions{Model: "gpt-4o", VirtualKeyID: "tiny"})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
	if mock.CallCount() != 0 {
		t.Errorf("over-budget request must not reach inner provider; got %d calls", mock.CallCount())
	}
}

func TestBudgetProvider_AllowsAndRecords(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "ok"
	store := NewMemoryBudgetStore()
	store.SetBudget("rich", 100.0)
	bp := NewBudgetProvider(mock, store)

	ctx := WithVirtualKey(context.Background(), "rich")
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
	bp := NewBudgetProvider(mock, NewMemoryBudgetStore())
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
	usage := &GraycodeRouterUsage{PromptTokens: 1000, CompletionTokens: 1000}
	cost := ActualCostUSD("gpt-4o", usage)
	// 1000*2.5/1e6 + 1000*10/1e6 = 0.0025 + 0.01 = 0.0125
	if cost < 0.0124 || cost > 0.0126 {
		t.Errorf("unexpected cost %f", cost)
	}
}
