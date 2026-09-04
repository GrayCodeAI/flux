package client

import (
	"context"
	"testing"
)

func TestContinuation_StopsWhenNotMaxTokens(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "complete answer"

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 3, MaxTotalTokens: 32000}

	resp, err := ChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("ChatWithContinuation() error: %v", err)
	}
	if resp.Content != "complete answer" {
		t.Errorf("Content = %q, want %q", resp.Content, "complete answer")
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "end_turn")
	}
	// Should only call the provider once
	if mock.CallCount() != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount())
	}
}

func TestContinuation_ContinuesOnMaxTokens(t *testing.T) {
	t.Parallel()
	// Use a mock that returns max_tokens for the first call, then end_turn for the second
	mock := &sequentialMock{
		responses: []mockResponse{
			{content: "part 1 ", finishReason: "max_tokens", usage: &GraycodeRouterUsage{PromptTokens: 10, CompletionTokens: 50, TotalTokens: 60}},
			{content: "part 2", finishReason: "end_turn", usage: &GraycodeRouterUsage{PromptTokens: 20, CompletionTokens: 30, TotalTokens: 50}},
		},
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "write something long"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 5, MaxTotalTokens: 32000}

	resp, err := ChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("ChatWithContinuation() error: %v", err)
	}
	if resp.Content != "part 1 part 2" {
		t.Errorf("Content = %q, want %q", resp.Content, "part 1 part 2")
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "end_turn")
	}
	if mock.callCount != 2 {
		t.Errorf("callCount = %d, want 2", mock.callCount)
	}
	// Usage should be accumulated
	if resp.Usage == nil {
		t.Fatal("expected non-nil Usage")
	}
	if resp.Usage.CompletionTokens != 80 {
		t.Errorf("CompletionTokens = %d, want 80", resp.Usage.CompletionTokens)
	}
}

func TestContinuation_RespectsMaxRetries(t *testing.T) {
	t.Parallel()
	// All responses return max_tokens — should stop after MaxContinuations
	mock := &sequentialMock{
		responses: []mockResponse{
			{content: "a", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 10}},
			{content: "b", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 10}},
			{content: "c", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 10}},
			{content: "d", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 10}},
		},
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "go"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 2, MaxTotalTokens: 0}

	resp, err := ChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("ChatWithContinuation() error: %v", err)
	}
	// MaxContinuations=2 means loop runs i=0,1,2 (3 iterations total)
	if mock.callCount != 3 {
		t.Errorf("callCount = %d, want 3", mock.callCount)
	}
	if resp.FinishReason != "max_tokens" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "max_tokens")
	}
	if resp.Content != "abc" {
		t.Errorf("Content = %q, want %q", resp.Content, "abc")
	}
}

func TestContinuation_RespectsMaxTotalTokens(t *testing.T) {
	t.Parallel()
	mock := &sequentialMock{
		responses: []mockResponse{
			{content: "big chunk", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 5000}},
			{content: " more", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 5000}},
		},
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "go"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 10, MaxTotalTokens: 5000}

	resp, err := ChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("ChatWithContinuation() error: %v", err)
	}
	// Should stop after first call because CompletionTokens (5000) >= MaxTotalTokens (5000)
	if mock.callCount != 1 {
		t.Errorf("callCount = %d, want 1", mock.callCount)
	}
	if resp.FinishReason != "max_tokens" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "max_tokens")
	}
}

func TestContinuation_StopsOnToolCalls(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeToolUse)

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "use a tool"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 5, MaxTotalTokens: 32000}

	resp, err := ChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("ChatWithContinuation() error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in response")
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_use")
	}
	// Should only call once — tool calls mean don't continue
	if mock.CallCount() != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount())
	}
}

func TestContinuation_StreamNoContinuationNeeded(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "complete"

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 3, MaxTotalTokens: 32000}

	result, err := StreamChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("StreamChatWithContinuation() error: %v", err)
	}

	var content string
	var gotDone bool
	for evt := range result.Events {
		switch evt.Type {
		case "content":
			content += evt.Content
		case "done":
			gotDone = true
		}
	}

	if content == "" {
		t.Error("expected content from stream")
	}
	if !gotDone {
		t.Error("expected done event")
	}
	// Only one call needed
	if mock.CallCount() != 1 {
		t.Errorf("CallCount = %d, want 1", mock.CallCount())
	}
}

func TestContinuation_StreamContinuesOnMaxTokens(t *testing.T) {
	t.Parallel()
	// Use the sequential mock which responds max_tokens then end_turn
	mock := &sequentialMock{
		responses: []mockResponse{
			{content: "first", finishReason: "max_tokens", usage: &GraycodeRouterUsage{CompletionTokens: 100}},
			{content: " second", finishReason: "end_turn", usage: &GraycodeRouterUsage{CompletionTokens: 50}},
		},
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "go"}}
	opts := ChatOptions{Model: "test"}
	cfg := ContinuationConfig{MaxContinuations: 5, MaxTotalTokens: 32000}

	result, err := StreamChatWithContinuation(ctx, mock, msgs, opts, cfg)
	if err != nil {
		t.Fatalf("StreamChatWithContinuation() error: %v", err)
	}

	var content string
	var continuations int
	var gotDone bool
	for evt := range result.Events {
		switch evt.Type {
		case "content":
			content += evt.Content
		case "continuation":
			continuations++
		case "done":
			gotDone = true
		}
	}

	if content == "" {
		t.Error("expected content from continued stream")
	}
	if continuations != 1 {
		t.Errorf("continuations = %d, want 1", continuations)
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if mock.callCount != 2 {
		t.Errorf("callCount = %d, want 2", mock.callCount)
	}
}

// --- test helpers ---

// mockResponse defines a canned response for the sequentialMock.
type mockResponse struct {
	content      string
	finishReason string
	usage        *GraycodeRouterUsage
	toolCalls    []ToolCall
}

// sequentialMock is a Provider that returns a sequence of canned responses.
type sequentialMock struct {
	responses []mockResponse
	callCount int
}

func (s *sequentialMock) Name() string { return "sequential-mock" }

func (s *sequentialMock) Ping(_ context.Context) error { return nil }

func (s *sequentialMock) Chat(_ context.Context, _ []GraycodeRouterMessage, _ ChatOptions) (*GraycodeRouterResponse, error) {
	idx := s.callCount
	s.callCount++
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	r := s.responses[idx]
	return &GraycodeRouterResponse{
		Content:      r.content,
		FinishReason: r.finishReason,
		Usage:        r.usage,
		ToolCalls:    r.toolCalls,
	}, nil
}

func (s *sequentialMock) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	resp, err := s.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan GraycodeRouterStreamEvent, 64)

	go func() {
		defer close(ch)
		if resp.Content != "" {
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "content", Content: resp.Content}:
			case <-streamCtx.Done():
				return
			}
		}
		for i := range resp.ToolCalls {
			tc := resp.ToolCalls[i]
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "tool_call", ToolCall: &tc}:
			case <-streamCtx.Done():
				return
			}
		}
		if resp.Usage != nil {
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "usage", Usage: resp.Usage}:
			case <-streamCtx.Done():
				return
			}
		}
		select {
		case ch <- GraycodeRouterStreamEvent{Type: "done", StopReason: resp.FinishReason}:
		case <-streamCtx.Done():
		}
	}()

	return NewStreamResult(ch, cancel), nil
}
