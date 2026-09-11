package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MockMode controls how the mock provider responds.
type MockMode string

const (
	// MockModeEcho echoes the last user message back.
	MockModeEcho MockMode = "echo"
	// MockModeFixed returns a fixed response set via MockProvider.Response.
	MockModeFixed MockMode = "fixed"
	// MockModeToolUse returns a tool call response.
	MockModeToolUse MockMode = "tool_use"
	// MockModeError always returns an error.
	MockModeError MockMode = "error"
	// MockModeMaxTokens returns a response with stop_reason=max_tokens (for testing continuation).
	MockModeMaxTokens MockMode = "max_tokens"
)

// MockProvider is a Provider implementation for testing.
// It never makes real HTTP requests.
type MockProvider struct {
	mu       sync.Mutex
	Mode     MockMode
	Response string // used in MockModeFixed
	ToolName string // used in MockModeToolUse
	ToolArgs map[string]interface{}
	Delay    time.Duration // simulate latency
	Calls    []MockCall    // recorded calls for assertions
}

// MockCall records a single call to the mock provider.
type MockCall struct {
	Messages []EyrieMessage
	Options  ChatOptions
}

// Compile-time check.
var _ Provider = (*MockProvider)(nil)

// NewMockProvider creates a mock provider with the given mode.
func NewMockProvider(mode MockMode) *MockProvider {
	return &MockProvider{Mode: mode}
}

// Name returns "mock".
func (m *MockProvider) Name() string { return "mock" }

// Ping always succeeds.
func (m *MockProvider) Ping(_ context.Context) error { return nil }

// Chat returns a mock response based on Mode.
func (m *MockProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Messages: messages, Options: opts})
	m.mu.Unlock()

	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	switch m.Mode {
	case MockModeError:
		return nil, fmt.Errorf("eyrie: mock error")
	case MockModeMaxTokens:
		return &EyrieResponse{Content: "partial response", FinishReason: "max_tokens", Usage: &EyrieUsage{PromptTokens: 10, CompletionTokens: 100, TotalTokens: 110}}, nil
	case MockModeToolUse:
		name := m.ToolName
		if name == "" {
			name = "mock_tool"
		}
		args := m.ToolArgs
		if args == nil {
			args = map[string]interface{}{"input": "test"}
		}
		return &EyrieResponse{
			ToolCalls:    []ToolCall{{ID: "mock-tc-1", Name: name, Arguments: args}},
			FinishReason: "tool_use",
			Usage:        &EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}, nil
	case MockModeFixed:
		return &EyrieResponse{Content: m.Response, FinishReason: "end_turn", Usage: &EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}, nil
	default: // echo
		last := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				last = messages[i].Content
				break
			}
		}
		return &EyrieResponse{Content: "echo: " + last, FinishReason: "end_turn", Usage: &EyrieUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}, nil
	}
}

// StreamChat streams a mock response word by word.
func (m *MockProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	resp, err := m.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan EyrieStreamEvent, 64)

	go func() {
		defer close(ch)
		if resp.FinishReason == "tool_use" && len(resp.ToolCalls) > 0 {
			emit(streamCtx, ch, EyrieStreamEvent{Type: "tool_call", ToolCall: &resp.ToolCalls[0]})
		} else {
			words := strings.Fields(resp.Content)
			for _, w := range words {
				if m.Delay > 0 {
					select {
					case <-time.After(m.Delay / time.Duration(len(words)+1)):
					case <-streamCtx.Done():
						return
					}
				}
				emit(streamCtx, ch, EyrieStreamEvent{Type: "content", Content: w + " "})
			}
		}
		emit(streamCtx, ch, EyrieStreamEvent{Type: "done"})
	}()

	return NewStreamResult(ch, cancel), nil
}

// CallCount returns the number of recorded calls.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

// LastCall returns the most recent recorded call, or nil.
func (m *MockProvider) LastCall() *MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) == 0 {
		return nil
	}
	c := m.Calls[len(m.Calls)-1]
	return &c
}

// Reset clears recorded calls.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
}

// MarshalCalls returns recorded calls as JSON for debugging.
func (m *MockProvider) MarshalCalls() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.MarshalIndent(m.Calls, "", "  ")
	return string(b)
}
