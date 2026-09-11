package client

import (
	"context"
	"strings"
	"testing"
)

func TestLLMSummarizingCondenser_NoTriggerUnderMaxSize(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "SUMMARY"
	c := NewLLMSummarizingCondenser(mock)

	msgs := []EyrieMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
	}
	got, err := c.Condense(context.Background(), msgs, CondenseOptions{MaxSize: 5, KeepFirst: 1})
	if err != nil {
		t.Fatalf("Condense: %v", err)
	}
	if len(got) != len(msgs) {
		t.Errorf("expected unchanged history of %d, got %d", len(msgs), len(got))
	}
	if mock.CallCount() != 0 {
		t.Errorf("summarizer must not be called under MaxSize; got %d calls", mock.CallCount())
	}
}

func TestLLMSummarizingCondenser_TriggersAndKeepsFirst(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "SUMMARY"
	c := NewLLMSummarizingCondenser(mock)

	msgs := []EyrieMessage{
		{Role: "system", Content: "keep-me-0"},
		{Role: "user", Content: "keep-me-1"},
		{Role: "assistant", Content: "mid-2"},
		{Role: "user", Content: "mid-3"},
		{Role: "assistant", Content: "mid-4"},
		{Role: "user", Content: "tail-5"},
		{Role: "assistant", Content: "tail-6"},
	}
	got, err := c.Condense(context.Background(), msgs, CondenseOptions{MaxSize: 4, KeepFirst: 2})
	if err != nil {
		t.Fatalf("Condense: %v", err)
	}

	// Result must fit within MaxSize.
	if len(got) > 4 {
		t.Errorf("condensed history len = %d, want <= 4", len(got))
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected exactly one summary call, got %d", mock.CallCount())
	}

	// First KeepFirst messages preserved verbatim.
	if got[0].Content != "keep-me-0" || got[1].Content != "keep-me-1" {
		t.Errorf("KeepFirst messages not preserved: %+v", got[:2])
	}
	// Summary note inserted after the head.
	if got[2].Role != "system" || !strings.Contains(got[2].Content, "SUMMARY") {
		t.Errorf("expected summary note at index 2, got %+v", got[2])
	}
	// Tail preserved at the end.
	if got[len(got)-1].Content != "tail-6" {
		t.Errorf("tail not preserved, last = %+v", got[len(got)-1])
	}

	// The summarizer should have received the middle span, not head/tail.
	summarized := mock.LastCall().Messages[0].Content
	if !strings.Contains(summarized, "mid-2") || strings.Contains(summarized, "keep-me-0") {
		t.Errorf("summarizer received wrong span: %q", summarized)
	}
}

func TestLLMSummarizingCondenser_UsesWeakRole(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "SUMMARY"
	c := NewLLMSummarizingCondenser(mock,
		WithCondenserRoles(ModelRoles{Primary: "big", Weak: "small"}))

	msgs := []EyrieMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
	}
	if _, err := c.Condense(context.Background(), msgs, CondenseOptions{MaxSize: 2, KeepFirst: 1}); err != nil {
		t.Fatalf("Condense: %v", err)
	}
	if got := mock.LastCall().Options.Model; got != "small" {
		t.Errorf("summary call model = %q, want weak model %q", got, "small")
	}
}

func TestCondensingProvider_CondensesBeforeChat(t *testing.T) {
	t.Parallel()
	// summarizer mock is distinct from the downstream chat mock so we can
	// assert the inner provider receives the reduced history.
	summarizer := NewMockProvider(MockModeFixed)
	summarizer.Response = "SUMMARY"
	cond := NewLLMSummarizingCondenser(summarizer)

	inner := NewMockProvider(MockModeFixed)
	inner.Response = "final"

	cp := NewCondensingProvider(inner, cond, CondenseOptions{MaxSize: 3, KeepFirst: 1})

	msgs := []EyrieMessage{
		{Role: "system", Content: "s"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
	}
	if _, err := cp.Chat(context.Background(), msgs, ChatOptions{Model: "big"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if summarizer.CallCount() != 1 {
		t.Errorf("expected summarizer to run once, got %d", summarizer.CallCount())
	}
	got := inner.LastCall().Messages
	if len(got) > 3 {
		t.Errorf("inner provider received %d messages, want <= 3", len(got))
	}
	// Inner model option must be untouched by condensation.
	if inner.LastCall().Options.Model != "big" {
		t.Errorf("inner model = %q, want %q", inner.LastCall().Options.Model, "big")
	}
}

func TestCondensingProvider_PassThroughWhenDisabled(t *testing.T) {
	t.Parallel()
	summarizer := NewMockProvider(MockModeFixed)
	cond := NewLLMSummarizingCondenser(summarizer)
	inner := NewMockProvider(MockModeFixed)
	inner.Response = "x"

	// MaxSize 0 disables condensation.
	cp := NewCondensingProvider(inner, cond, CondenseOptions{MaxSize: 0})
	msgs := []EyrieMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	}
	if _, err := cp.Chat(context.Background(), msgs, ChatOptions{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if summarizer.CallCount() != 0 {
		t.Errorf("disabled condenser must not summarize; got %d", summarizer.CallCount())
	}
	if len(inner.LastCall().Messages) != 3 {
		t.Errorf("expected pass-through of 3 messages, got %d", len(inner.LastCall().Messages))
	}
}
