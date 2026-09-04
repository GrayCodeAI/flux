package adapters

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

func TestAnthropicBaseFromOpenAIV1(t *testing.T) {
	t.Parallel()
	if got := AnthropicBaseFromOpenAIV1("https://example.com/zen/go/v1"); got != "https://example.com/zen/go" {
		t.Fatalf("got %q", got)
	}
	if got := AnthropicBaseFromOpenAIV1("https://example.com/zen/go"); got != "https://example.com/zen/go" {
		t.Fatalf("got %q", got)
	}
	if got := AnthropicBaseFromOpenAIV1(" https://example.com/v1/ "); got != "https://example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestStreamResultFromChat(t *testing.T) {
	t.Parallel()
	result := streamResultFromChat(&core.GraycodeRouterResponse{
		Content:      "Hi there!",
		FinishReason: "stop",
	})
	var content string
	for event := range result.Events {
		if event.Type == "content" {
			content += event.Content
		}
	}
	if content != "Hi there!" {
		t.Fatalf("content = %q, want Hi there!", content)
	}
}

func TestStreamResultFromChat_FullResponse(t *testing.T) {
	t.Parallel()
	resp := &core.GraycodeRouterResponse{
		Thinking:     "Let me think...",
		Content:      "Hello there!",
		ToolCalls:    []core.ToolCall{{Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}}},
		Usage:        &core.GraycodeRouterUsage{TotalTokens: 42},
		FinishReason: "",
	}
	result := streamResultFromChat(resp)
	defer result.Close()

	var thinking, content string
	var toolCalls int
	var gotUsage, gotDone bool
	var stopReason string
	for evt := range result.Events {
		switch evt.Type {
		case "thinking":
			thinking = evt.Thinking
		case "content":
			content = evt.Content
		case "tool_call":
			toolCalls++
		case "usage":
			gotUsage = evt.Usage != nil
		case "done":
			gotDone = true
			stopReason = evt.StopReason
		}
	}
	if thinking != "Let me think..." {
		t.Errorf("thinking = %q", thinking)
	}
	if content != "Hello there!" {
		t.Errorf("content = %q", content)
	}
	if toolCalls != 1 {
		t.Errorf("tool calls = %d", toolCalls)
	}
	if !gotUsage {
		t.Error("expected usage event")
	}
	if !gotDone {
		t.Error("expected done event")
	}
	if stopReason != "stop" {
		t.Errorf("stop_reason = %q, expected 'stop'", stopReason)
	}
}

func TestStreamResultFromChat_NilResponse(t *testing.T) {
	t.Parallel()
	result := streamResultFromChat(nil)
	defer result.Close()

	var eventCount int
	for range result.Events {
		eventCount++
	}
	if eventCount != 0 {
		t.Errorf("expected 0 events for nil response, got %d", eventCount)
	}
}
