package adapters

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

func TestNewStreamWithReasoningFallbackChatFirst(t *testing.T) {
	t.Parallel()
	primaryEvents := make(chan core.EyrieStreamEvent, 4)
	primaryEvents <- core.EyrieStreamEvent{Type: "thinking", Thinking: "internal reasoning"}
	primaryEvents <- core.EyrieStreamEvent{Type: "done", StopReason: "end_turn"}
	close(primaryEvents)
	primary := llm.NewStreamResult(primaryEvents, "", func() {})

	var chatCalled, streamCalled bool
	fallback := protocolStreamFallback{
		chat: func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.EyrieResponse, error) {
			chatCalled = true
			return &core.EyrieResponse{Content: "Hello from chat fallback!", FinishReason: "stop"}, nil
		},
		stream: func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.StreamResult, error) {
			streamCalled = true
			return nil, fmt.Errorf("stream fallback should not run")
		},
	}

	result := newStreamWithReasoningFallback(context.Background(), nil, core.ChatOptions{}, primary, fallback)
	var content string
	for event := range result.Events {
		switch event.Type {
		case "thinking":
			t.Fatal("primary reasoning must not leak before chat fallback succeeds")
		case "content":
			content += event.Content
		}
	}
	if content != "Hello from chat fallback!" {
		t.Fatalf("content = %q, want Hello from chat fallback!", content)
	}
	if !chatCalled {
		t.Fatal("expected chat fallback to run")
	}
	if streamCalled {
		t.Fatal("stream fallback must not run when chat fallback succeeds")
	}
}

func TestNewStreamWithReasoningFallbackStreamWhenChatEmpty(t *testing.T) {
	t.Parallel()
	primaryEvents := make(chan core.EyrieStreamEvent, 4)
	primaryEvents <- core.EyrieStreamEvent{Type: "thinking", Thinking: "internal reasoning"}
	primaryEvents <- core.EyrieStreamEvent{Type: "done", StopReason: "end_turn"}
	close(primaryEvents)
	primary := llm.NewStreamResult(primaryEvents, "", func() {})

	fallbackEvents := make(chan core.EyrieStreamEvent, 4)
	fallbackEvents <- core.EyrieStreamEvent{Type: "content", Content: "stream answer"}
	fallbackEvents <- core.EyrieStreamEvent{Type: "done", StopReason: "stop"}
	close(fallbackEvents)

	var chatCalled, streamCalled bool
	fallback := protocolStreamFallback{
		chat: func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.EyrieResponse, error) {
			chatCalled = true
			return &core.EyrieResponse{Thinking: "still thinking"}, nil
		},
		stream: func(context.Context, []core.EyrieMessage, core.ChatOptions) (*core.StreamResult, error) {
			streamCalled = true
			return llm.NewStreamResult(fallbackEvents, "", func() {}), nil
		},
	}

	result := newStreamWithReasoningFallback(context.Background(), nil, core.ChatOptions{}, primary, fallback)
	var content string
	for event := range result.Events {
		if event.Type == "content" {
			content += event.Content
		}
	}
	if content != "stream answer" {
		t.Fatalf("content = %q, want stream answer", content)
	}
	if !chatCalled || !streamCalled {
		t.Fatalf("chatCalled=%v streamCalled=%v, want both true", chatCalled, streamCalled)
	}
}

func TestProtocolRouterChatFallbackOnError(t *testing.T) {
	t.Parallel()
	openAI := NewOpenAIClient("key", "https://example/openai", nil)
	anthropic := NewAnthropicClient("key", "https://example")
	openAI.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": "down"}), nil
	})}
	anthropic.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "fallback"}},
			"stop_reason": "end_turn",
		}), nil
	})}

	router := ProtocolRouter{OpenAI: openAI, Anthropic: anthropic}
	response, err := router.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "test", MaxTokens: 16,
	}, ChatProtocolCompletions, func(err error, _ *core.EyrieResponse) bool {
		return err != nil
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Content != "fallback" {
		t.Fatalf("content = %q, want fallback", response.Content)
	}
}

func TestProtocolRouterNoFallbackWhenNil(t *testing.T) {
	t.Parallel()
	openAI := NewOpenAIClient("key", "https://example/openai", nil)
	openAI.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadGateway, map[string]string{"error": "down"}), nil
	})}
	router := ProtocolRouter{OpenAI: openAI}
	_, err := router.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "test", MaxTokens: 16,
	}, ChatProtocolCompletions, nil)
	if err == nil {
		t.Fatal("expected error without fallback")
	}
}
