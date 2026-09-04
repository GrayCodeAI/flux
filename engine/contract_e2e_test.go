package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

type contractProvider struct {
	chatMessages   []client.GraycodeRouterMessage
	chatOptions    client.ChatOptions
	streamMessages []client.GraycodeRouterMessage
	streamOptions  client.ChatOptions
}

func (p *contractProvider) Name() string               { return "contract" }
func (p *contractProvider) Ping(context.Context) error { return nil }

func (p *contractProvider) Chat(_ context.Context, messages []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	p.chatMessages, p.chatOptions = messages, opts
	return &client.GraycodeRouterResponse{
		Content: "complete", FinishReason: "end_turn", RequestID: "req-blocking",
		Usage: &client.GraycodeRouterUsage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6},
	}, nil
}

func (p *contractProvider) StreamChat(_ context.Context, messages []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.StreamResult, error) {
	p.streamMessages, p.streamOptions = messages, opts
	events := make(chan client.GraycodeRouterStreamEvent, 4)
	events <- client.GraycodeRouterStreamEvent{Type: "content", Content: "checking"}
	events <- client.GraycodeRouterStreamEvent{Type: "tool_call", ToolCall: &client.ToolCall{ID: "call-1", Name: "read_file", Arguments: map[string]interface{}{"path": "main.go"}}}
	events <- client.GraycodeRouterStreamEvent{Type: "usage", Usage: &client.GraycodeRouterUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}}
	events <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn", RequestID: "req-stream"}
	close(events)
	return client.NewStreamResultWithRequestID(events, "req-stream", nil), nil
}

func TestEngineContractEndToEnd(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cat := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &cat); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{StateDir: dir, SecretStore: &credentials.MapStore{}})
	if err != nil {
		t.Fatal(err)
	}
	modelID := firstCatalogModelID(cat)
	if modelID == "" {
		t.Fatal("seed catalog has no model")
	}
	if err := eng.SetSelection(ctx, "", modelID); err != nil {
		t.Fatal(err)
	}
	provider := &contractProvider{}
	eng.resolveTransport = func(context.Context, Route) (client.Provider, error) { return provider, nil }

	topP := 0.8
	request := GenerateRequest{
		Messages:     []Message{{Role: "user", Content: "inspect main.go"}},
		SystemPrompt: "You are a coding agent.",
		Tools:        []Tool{{Name: "read_file", Description: "Read a file", Parameters: map[string]interface{}{"type": "object"}}},
		Preference:   Preference{PreferredModelID: modelID},
		Limits:       Limits{MaxOutputTokens: 1024, MaxContinuations: 2, MaxTotalOutputTokens: 4096},
		Metadata:     Metadata{SessionID: "session-1", TurnID: "turn-1", UserID: "user-1"},
		Options:      GenerationOptions{EnableCaching: true, ReasoningEffort: "high", TopP: &topP, ServiceTier: "priority"},
	}

	response, err := eng.Generate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "complete" || response.Usage == nil || response.Usage.TotalTokens != 6 {
		t.Fatalf("blocking response not normalized: %+v", response)
	}
	if provider.chatOptions.Model != modelID || provider.chatOptions.System != request.SystemPrompt || !provider.chatOptions.EnableCaching || provider.chatOptions.ReasoningEffort != "high" || provider.chatOptions.MetadataUserID != "user-1" {
		t.Fatalf("blocking options lost at boundary: %+v", provider.chatOptions)
	}
	if provider.chatOptions.Metadata["session.id"] != "session-1" || provider.chatOptions.Metadata["turn.id"] != "turn-1" {
		t.Fatalf("correlation metadata lost at boundary: %+v", provider.chatOptions.Metadata)
	}

	stream, err := eng.Stream(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var events []Event
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 || events[0].Type != EventRouteSelected || events[1].Type != EventContentDelta || events[2].Type != EventToolCallDone || events[3].Type != EventUsage || events[4].Type != EventDone {
		t.Fatalf("normalized event sequence: %+v", events)
	}
	if events[2].ToolCall == nil || events[2].ToolCall.Name != "read_file" {
		t.Fatalf("tool call not normalized: %+v", events[2])
	}
	if events[3].Usage == nil || events[3].Usage.TotalTokens != 8 {
		t.Fatalf("stream usage not normalized: %+v", events[3])
	}
	if len(provider.streamOptions.Tools) != 1 || provider.streamOptions.MaxTokens != 1024 || provider.streamOptions.ServiceTier != "priority" {
		t.Fatalf("stream options lost at boundary: %+v", provider.streamOptions)
	}
}

type continuationProvider struct {
	calls    int
	requests [][]client.GraycodeRouterMessage
}

func (p *continuationProvider) Name() string               { return "continuation" }
func (p *continuationProvider) Ping(context.Context) error { return nil }
func (p *continuationProvider) Chat(context.Context, []client.GraycodeRouterMessage, client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	return nil, nil
}

func (p *continuationProvider) StreamChat(_ context.Context, messages []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	p.calls++
	p.requests = append(p.requests, append([]client.GraycodeRouterMessage(nil), messages...))
	events := make(chan client.GraycodeRouterStreamEvent, 3)
	if p.calls == 1 {
		events <- client.GraycodeRouterStreamEvent{Type: "content", Content: "part one"}
		events <- client.GraycodeRouterStreamEvent{Type: "usage", Usage: &client.GraycodeRouterUsage{CompletionTokens: 2, TotalTokens: 4}}
		events <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "max_tokens", RequestID: "request-1"}
	} else {
		events <- client.GraycodeRouterStreamEvent{Type: "content", Content: "part two"}
		events <- client.GraycodeRouterStreamEvent{Type: "usage", Usage: &client.GraycodeRouterUsage{CompletionTokens: 2, TotalTokens: 5}}
		events <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn", RequestID: "request-2"}
	}
	close(events)
	return client.NewStreamResultWithRequestID(events, "request-id", nil), nil
}

func TestEngineContinuationPreservesEventsAndConversationShape(t *testing.T) {
	provider := &continuationProvider{}
	source, err := streamWithContinuation(
		context.Background(), provider,
		[]client.GraycodeRouterMessage{{Role: "user", Content: "write a long answer"}},
		client.ChatOptions{Model: "model"},
		Limits{MaxContinuations: 1, MaxTotalOutputTokens: 100},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	var events []client.GraycodeRouterStreamEvent
	for event := range source.Events {
		events = append(events, event)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
	if len(provider.requests[1]) != 3 || provider.requests[1][1].Role != "assistant" || provider.requests[1][1].Content != "part one" || provider.requests[1][2].Content != "Continue." {
		t.Fatalf("continuation conversation shape: %+v", provider.requests[1])
	}
	var sawContinuation, sawFinal bool
	for _, event := range events {
		if event.Type == "continuation" {
			sawContinuation = true
		}
		if event.Type == "done" && event.StopReason == "end_turn" && event.RequestID == "request-2" {
			sawFinal = true
		}
	}
	if !sawContinuation || !sawFinal {
		t.Fatalf("continuation metadata missing: %+v", events)
	}
}

func firstCatalogModelID(cat catalog.Catalog) string {
	for id := range cat.Models {
		return id
	}
	return ""
}
