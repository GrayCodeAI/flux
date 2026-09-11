package client

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestModerationProvider_AllowsSafe(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithBlockedPatterns([]string{"(?i)forbidden"}),
		WithModerationMaxTokens(1000),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "Hello, how are you?"}}
	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.HasPrefix(resp.Content, "echo:") {
		t.Fatalf("expected echo response, got: %q", resp.Content)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call to inner provider, got %d", mock.CallCount())
	}
}

func TestModerationProvider_BlocksPattern(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithBlockedPatterns([]string{`(?i)\bforbidden\b`}),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "This contains forbidden content"}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err == nil {
		t.Fatal("expected error for blocked pattern, got nil")
	}
	if !strings.Contains(err.Error(), "blocked pattern") {
		t.Fatalf("expected 'blocked pattern' in error, got: %v", err)
	}
	if mock.CallCount() != 0 {
		t.Fatalf("expected 0 calls to inner provider, got %d", mock.CallCount())
	}
}

func TestModerationProvider_TokenLimit(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithModerationMaxTokens(5),
	)

	// 10 words exceeds limit of 5
	msgs := []EyrieMessage{{Role: "user", Content: "one two three four five six seven eight nine ten"}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err == nil {
		t.Fatal("expected error for token limit, got nil")
	}
	if !strings.Contains(err.Error(), "token count") {
		t.Fatalf("expected 'token count' in error, got: %v", err)
	}
	if mock.CallCount() != 0 {
		t.Fatalf("expected 0 calls to inner provider, got %d", mock.CallCount())
	}
}

func TestModerationProvider_TokenLimitAllowsUnderLimit(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithModerationMaxTokens(100),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "short message"}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("expected 1 call to inner provider, got %d", mock.CallCount())
	}
}

func TestModerationProvider_CustomChecker(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithCustomChecker(func(text string) error {
			if strings.Contains(strings.ToLower(text), "banned") {
				return errors.New("custom rule: banned word detected")
			}
			return nil
		}),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "This has a Banned word"}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err == nil {
		t.Fatal("expected error from custom checker, got nil")
	}
	if !strings.Contains(err.Error(), "banned word detected") {
		t.Fatalf("expected 'banned word detected' in error, got: %v", err)
	}
	if mock.CallCount() != 0 {
		t.Fatalf("expected 0 calls to inner provider, got %d", mock.CallCount())
	}
}

func TestModerationProvider_CustomCheckerAllows(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithCustomChecker(func(text string) error {
			return nil
		}),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "safe content"}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestModerationProvider_StreamChat(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithBlockedPatterns([]string{`(?i)forbidden`}),
		WithModerationMaxTokens(1000),
	)

	// Safe message should pass through.
	msgs := []EyrieMessage{{Role: "user", Content: "Hello"}}
	result, err := mp.StreamChat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer result.Close()

	var content strings.Builder
	for evt := range result.Events {
		if evt.Type == "content" {
			content.WriteString(evt.Content)
		}
		if evt.Type == "done" {
			break
		}
	}
	if !strings.HasPrefix(content.String(), "echo:") {
		t.Fatalf("expected echo response in stream, got: %q", content.String())
	}
}

func TestModerationProvider_StreamChatBlocked(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithBlockedPatterns([]string{`(?i)forbidden`}),
	)

	msgs := []EyrieMessage{{Role: "user", Content: "forbidden content"}}
	_, err := mp.StreamChat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err == nil {
		t.Fatal("expected error for blocked pattern in StreamChat, got nil")
	}
	if !strings.Contains(err.Error(), "blocked pattern") {
		t.Fatalf("expected 'blocked pattern' in error, got: %v", err)
	}
}

func TestModerationProvider_ContentParts(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(
		mock,
		WithBlockedPatterns([]string{`(?i)secret`}),
	)

	msgs := []EyrieMessage{{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "this has secret info"},
		},
	}}
	_, err := mp.Chat(context.Background(), msgs, ChatOptions{Model: "test"})
	if err == nil {
		t.Fatal("expected error for blocked pattern in ContentParts, got nil")
	}
}

func TestModerationProvider_Name(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	mp := NewModerationProvider(mock)
	if mp.Name() != "mock/moderation" {
		t.Fatalf("expected 'mock/moderation', got %q", mp.Name())
	}
}

func TestModerationProvider_NilInner(t *testing.T) {
	t.Parallel()
	mp := NewModerationProvider(nil)
	if mp != nil {
		t.Fatal("expected nil from NewModerationProvider with nil inner")
	}
}
