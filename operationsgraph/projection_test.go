package operationsgraph_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-router/graph"
	llmcontracts "github.com/GrayCodeAI/graycode-router/llm"
	"github.com/GrayCodeAI/graycode-router/operationsgraph"
)

func TestBuildPrivacySafeOperationsProjection(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	export, err := operationsgraph.Build(operationsgraph.Input{
		Route: &llmcontracts.ResolvedRoute{
			Provider: "private-provider", Model: "private/model", DeploymentRouting: true,
		},
		Usage: &llmcontracts.GraycodeRouterUsage{
			PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120,
		},
		FinishReason: "stop", RequestID: "private-request-id",
		Content: "private generated content", ToolCallCount: 2,
		ObservedAt: at, Scope: graphcontracts.Scope{RepositoryID: "repo"},
		CorrelationID: "session-1",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(export.Nodes) != 2 || len(export.Edges) != 1 || len(export.Events) != 2 {
		t.Fatalf("unexpected sizes: nodes=%d edges=%d events=%d", len(export.Nodes), len(export.Edges), len(export.Events))
	}
	payload, _ := json.Marshal(export)
	for _, secret := range []string{"private-provider", "private/model", "private-request-id", "private generated content"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("projection leaked %q", secret)
		}
	}
	if export.Edges[0].Kind != graphcontracts.EdgeDependsOn {
		t.Fatalf("edge kind = %q", export.Edges[0].Kind)
	}
}

func TestBuildRequiresRouteOrUsage(t *testing.T) {
	t.Parallel()
	if _, err := operationsgraph.Build(operationsgraph.Input{}); err == nil {
		t.Fatal("expected empty input error")
	}
}
