package grpc

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/flux/conversation"
	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/storage"
)

// TestEngineChatServiceContract verifies the constructor surface and the
// placeholder paths. The engine-backed path is exercised below without the
// "grpc" build tag; server_grpc_test.go (tag "grpc") covers the wire framing
// with a stub ChatService.
func TestEngineChatServiceContract(t *testing.T) {
	if NewChatService() == nil {
		t.Fatal("NewChatService returned nil")
	}
	if _, err := NewChatService().Chat(context.Background(), &ChatRequest{Message: "hi"}); !errors.Is(err, ErrUnimplemented) {
		t.Fatalf("expected ErrUnimplemented from the placeholder service, got %v", err)
	}
	if svc := NewEngineChatService(nil); svc == nil {
		t.Fatal("NewEngineChatService returned nil")
	}
	if _, err := NewEngineChatService(nil).Chat(context.Background(), &ChatRequest{Message: "hi"}); !errors.Is(err, ErrUnimplemented) {
		t.Fatalf("expected ErrUnimplemented for nil engine, got %v", err)
	}
}

// scriptedProvider streams a fixed event sequence and records the request.
type scriptedProvider struct {
	events []core.FluxStreamEvent
	gotMsg chan []core.FluxMessage
	gotOpt chan core.ChatOptions
}

func (p *scriptedProvider) Name() string                 { return "scripted" }
func (p *scriptedProvider) Ping(_ context.Context) error { return nil }

func (p *scriptedProvider) Chat(context.Context, []core.FluxMessage, core.ChatOptions) (*core.FluxResponse, error) {
	return nil, errors.New("scriptedProvider: Chat is not used by conversation.Engine")
}

func (p *scriptedProvider) StreamChat(_ context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	p.gotMsg <- messages
	p.gotOpt <- opts
	ch := make(chan core.FluxStreamEvent, len(p.events))
	for _, evt := range p.events {
		ch <- evt
	}
	close(ch)
	return &core.StreamResult{Events: ch}, nil
}

func newScriptedEngine(t *testing.T, events ...core.FluxStreamEvent) (*conversation.Engine, *scriptedProvider) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "grpc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	prov := &scriptedProvider{
		events: events,
		gotMsg: make(chan []core.FluxMessage, 1),
		gotOpt: make(chan core.ChatOptions, 1),
	}
	return conversation.New(store, prov), prov
}

func TestEngineChatServiceAggregatesEngineStream(t *testing.T) {
	engine, prov := newScriptedEngine(
		t,
		core.FluxStreamEvent{Type: "content", Content: "hel"},
		core.FluxStreamEvent{Type: "content", Content: "lo"},
		core.FluxStreamEvent{Type: "done", StopReason: "end_turn", Usage: &core.FluxUsage{CompletionTokens: 2}},
	)

	resp, err := NewEngineChatService(engine).Chat(context.Background(), &ChatRequest{
		Model:        "test-model",
		SystemPrompt: "be brief",
		Message:      "hi",
		MaxTokens:    64,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("Content = %q, want the concatenated deltas %q", resp.Content, "hello")
	}
	if resp.NodeID == "" {
		t.Fatal("NodeID is empty; want the saved assistant node ID")
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop", resp.FinishReason)
	}

	msgs := <-prov.gotMsg
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("provider received messages %+v, want one user message %q", msgs, "hi")
	}
	opts := <-prov.gotOpt
	if opts.Model != "test-model" || opts.System != "be brief" || opts.MaxTokens != 64 {
		t.Fatalf("provider received options model=%q system=%q max=%d; want the ChatRequest fields", opts.Model, opts.System, opts.MaxTokens)
	}
}

func TestEngineChatServiceReturnsStreamError(t *testing.T) {
	engine, _ := newScriptedEngine(
		t,
		core.FluxStreamEvent{Type: "content", Content: "partial"},
		core.FluxStreamEvent{Type: "error", Error: "upstream failed"},
	)

	resp, err := NewEngineChatService(engine).Chat(context.Background(), &ChatRequest{Message: "hi"})
	if err == nil || err.Error() != "upstream failed" {
		t.Fatalf("Chat error = %v, want the stream error %q", err, "upstream failed")
	}
	if resp != nil {
		t.Fatalf("Chat response = %+v, want nil on stream error", resp)
	}
}

func TestEngineChatServiceRejectsNilRequest(t *testing.T) {
	engine, _ := newScriptedEngine(t)
	if _, err := NewEngineChatService(engine).Chat(context.Background(), nil); err == nil {
		t.Fatal("Chat(nil) succeeded; want an error")
	}
}
