package client

import (
	"context"
	"testing"
	"time"
)

func TestNewMockProviderMode(t *testing.T) {
	t.Parallel()
	modes := []MockMode{MockModeEcho, MockModeFixed, MockModeToolUse, MockModeError, MockModeMaxTokens}
	for _, mode := range modes {
		mp := NewMockProvider(mode)
		if mp.Mode != mode {
			t.Errorf("NewMockProvider(%q).Mode = %q, want %q", mode, mp.Mode, mode)
		}
	}
}

func TestMockProviderName(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	if mp.Name() != "mock" {
		t.Errorf("Name() = %q, want %q", mp.Name(), "mock")
	}
}

func TestMockProviderPing(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	if err := mp.Ping(context.Background()); err != nil {
		t.Errorf("Ping() = %v, want nil", err)
	}
}

func TestMockProviderImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*MockProvider)(nil)
}

func TestMockProviderEchoMode(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	msgs := []EyrieMessage{
		{Role: "user", Content: "Hello world"},
	}
	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "echo: Hello world" {
		t.Errorf("Content = %q, want %q", resp.Content, "echo: Hello world")
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "end_turn")
	}
}

func TestMockProviderEchoModeNoUser(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	msgs := []EyrieMessage{
		{Role: "assistant", Content: "system message"},
	}
	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "echo: " {
		t.Errorf("Content = %q, want %q", resp.Content, "echo: ")
	}
}

func TestMockProviderFixedMode(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeFixed)
	mp.Response = "fixed answer"
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "fixed answer" {
		t.Errorf("Content = %q, want %q", resp.Content, "fixed answer")
	}
}

func TestMockProviderErrorMode(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeError)
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	_, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err == nil {
		t.Fatal("Chat should return error in error mode")
	}
	if err.Error() != "eyrie: mock error" {
		t.Errorf("error = %q, want %q", err.Error(), "eyrie: mock error")
	}
}

func TestMockProviderToolUseMode(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeToolUse)
	mp.ToolName = "search"
	mp.ToolArgs = map[string]interface{}{"query": "hello"}

	msgs := []EyrieMessage{{Role: "user", Content: "search for hello"}}
	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.FinishReason != "tool_use" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_use")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", tc.Name, "search")
	}
	if tc.Arguments["query"] != "hello" {
		t.Errorf("ToolCalls[0].Arguments[query] = %v, want %q", tc.Arguments["query"], "hello")
	}
}

func TestMockProviderToolUseModeDefaults(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeToolUse)
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "mock_tool" {
		t.Errorf("default ToolName = %q, want %q", tc.Name, "mock_tool")
	}
	if tc.Arguments["input"] != "test" {
		t.Errorf("default ToolArgs[input] = %v, want %q", tc.Arguments["input"], "test")
	}
}

func TestMockProviderMaxTokensMode(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeMaxTokens)
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.FinishReason != "max_tokens" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "max_tokens")
	}
	if resp.Content != "partial response" {
		t.Errorf("Content = %q, want %q", resp.Content, "partial response")
	}
}

func TestMockProviderCallRecording(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	if mp.CallCount() != 0 {
		t.Fatalf("initial CallCount = %d, want 0", mp.CallCount())
	}
	if mp.LastCall() != nil {
		t.Fatal("LastCall should be nil when no calls recorded")
	}

	msgs := []EyrieMessage{{Role: "user", Content: "first"}}
	opts := ChatOptions{Model: "test-model"}
	mp.Chat(context.Background(), msgs, opts)

	if mp.CallCount() != 1 {
		t.Fatalf("CallCount after 1 call = %d, want 1", mp.CallCount())
	}

	last := mp.LastCall()
	if last == nil {
		t.Fatal("LastCall returned nil")
	}
	if len(last.Messages) != 1 || last.Messages[0].Content != "first" {
		t.Errorf("LastCall.Messages[0].Content = %q, want %q", last.Messages[0].Content, "first")
	}
	if last.Options.Model != "test-model" {
		t.Errorf("LastCall.Options.Model = %q, want %q", last.Options.Model, "test-model")
	}
}

func TestMockProviderMultipleCalls(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)

	for i := 0; i < 5; i++ {
		mp.Chat(context.Background(), []EyrieMessage{{Role: "user", Content: "msg"}}, ChatOptions{})
	}
	if mp.CallCount() != 5 {
		t.Errorf("CallCount = %d, want 5", mp.CallCount())
	}
}

func TestMockProviderReset(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	mp.Chat(context.Background(), []EyrieMessage{{Role: "user", Content: "test"}}, ChatOptions{})
	if mp.CallCount() != 1 {
		t.Fatalf("before reset: CallCount = %d, want 1", mp.CallCount())
	}

	mp.Reset()
	if mp.CallCount() != 0 {
		t.Errorf("after reset: CallCount = %d, want 0", mp.CallCount())
	}
	if mp.LastCall() != nil {
		t.Error("after reset: LastCall should be nil")
	}
}

func TestMockProviderMarshalCalls(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	mp.Chat(context.Background(), []EyrieMessage{{Role: "user", Content: "test"}}, ChatOptions{})

	s := mp.MarshalCalls()
	if s == "" {
		t.Fatal("MarshalCalls returned empty string")
	}
	if s == "null" {
		t.Fatal("MarshalCalls returned null after a call was recorded")
	}
}

func TestMockProviderUsageInResponse(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeFixed)
	mp.Response = "test"
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	resp, err := mp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage is nil")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
}

func TestMockProviderContextCancellation(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	mp.Delay = 10 * time.Second // long delay

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := mp.Chat(ctx, []EyrieMessage{{Role: "user", Content: "test"}}, ChatOptions{})
	if err == nil {
		t.Fatal("Chat should return error when context is cancelled")
	}
}

func TestMockProviderStreamChat(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeEcho)
	msgs := []EyrieMessage{{Role: "user", Content: "Hello world"}}

	sr, err := mp.StreamChat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	var content string
	for evt := range sr.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
		if evt.Type == "done" {
			break
		}
	}
	if content != "echo: Hello world " {
		t.Errorf("streamed content = %q, want %q", content, "echo: Hello world ")
	}
}

func TestMockProviderStreamChatToolUse(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeToolUse)
	mp.ToolName = "calculator"
	msgs := []EyrieMessage{{Role: "user", Content: "compute"}}

	sr, err := mp.StreamChat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	var toolName string
	for evt := range sr.Events {
		if evt.Type == "tool_call" && evt.ToolCall != nil {
			toolName = evt.ToolCall.Name
		}
		if evt.Type == "done" {
			break
		}
	}
	if toolName != "calculator" {
		t.Errorf("streamed tool name = %q, want %q", toolName, "calculator")
	}
}

func TestMockProviderStreamChatError(t *testing.T) {
	t.Parallel()
	mp := NewMockProvider(MockModeError)
	msgs := []EyrieMessage{{Role: "user", Content: "test"}}

	_, err := mp.StreamChat(context.Background(), msgs, ChatOptions{})
	if err == nil {
		t.Fatal("StreamChat should return error in error mode")
	}
}
