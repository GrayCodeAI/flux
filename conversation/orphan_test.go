package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/storage"
)

type orphanMockProvider struct{}

func (orphanMockProvider) Name() string { return "mock" }
func (orphanMockProvider) Ping(_ context.Context) error {
	return nil
}

func (orphanMockProvider) Chat(_ context.Context, _ []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	return &client.GraycodeRouterResponse{Content: "ok", FinishReason: "end_turn"}, nil
}

func (orphanMockProvider) StreamChat(_ context.Context, _ []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.GraycodeRouterStreamEvent, 2)
	ch <- client.GraycodeRouterStreamEvent{Type: "content", Content: "ok"}
	ch <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn"}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func TestInjectSyntheticToolResults_InjectsAfterOrphanNode(t *testing.T) {
	t.Parallel()
	ancestors := []*storage.Node{
		{ID: "u1", NodeType: storage.NodeTypeUser, Content: "hi", Sequence: 1},
		{ID: "a1", NodeType: storage.NodeTypeAssistant, Content: `[{"type":"tool_use","id":"t1","name":"search","input":{}}]`, Sequence: 2},
	}
	orphans := map[string][]string{"a1": {"t1"}}
	result := injectSyntheticToolResults(ancestors, orphans)
	if len(result) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result))
	}
	if result[2].NodeType != storage.NodeTypeToolResult {
		t.Fatalf("expected synthetic tool_result, got %s", result[2].NodeType)
	}
	if !strings.Contains(result[2].Content, "t1") {
		t.Fatalf("expected synthetic result for t1, got %q", result[2].Content)
	}
}

func TestPromptFrom_RepairsOrphanedToolUse(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "orphan.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	rootID := "root-1"
	if err := store.CreateNode(ctx, &storage.Node{
		ID: rootID, RootID: rootID, NodeType: storage.NodeTypeUser, Content: "start", Sequence: 1, Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	assistantID := "assistant-1"
	toolContent := `[{"type":"text","text":"checking"},{"type":"tool_use","id":"orphan_id","name":"search","input":{}}]`
	if err := store.CreateNode(ctx, &storage.Node{
		ID: assistantID, ParentID: rootID, RootID: rootID, NodeType: storage.NodeTypeAssistant,
		Content: toolContent, Sequence: 2, Status: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.IndexToolIDs(ctx, assistantID, []string{"orphan_id"}, "use"); err != nil {
		t.Fatal(err)
	}

	engine := New(store, orphanMockProvider{})
	events, err := engine.PromptFrom(ctx, assistantID, "continue please", PromptOpts{Model: "mock"})
	if err != nil {
		t.Fatalf("PromptFrom with orphaned tool_use: %v", err)
	}
	var nodeID string
	for ev := range events {
		if ev.Type == EventDone {
			nodeID = ev.NodeID
		}
		if ev.Type == EventError {
			t.Fatalf("unexpected error event: %s", ev.Error)
		}
	}
	if nodeID == "" {
		t.Fatal("expected node_id — orphan fix should have prevented provider failure")
	}
}
