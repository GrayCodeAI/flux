package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/graycode-router/tools"
)

// TestToolsParity pins the wire schema of tools.ToolCall and tools.ToolResult
// to the exact JSON the eagle tools contract produces.
func TestToolsParity(t *testing.T) {
	call := tools.ToolCall{
		ID:        "tc1",
		Name:      "search",
		Arguments: map[string]interface{}{"q": "go"},
	}

	gotCall, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal ToolCall: %v", err)
	}

	wantCall := `{"id":"tc1","name":"search","arguments":{"q":"go"}}`
	if string(gotCall) != wantCall {
		t.Fatalf("ToolCall schema parity mismatch\n got: %s\nwant: %s", gotCall, wantCall)
	}

	result := tools.ToolResult{
		ToolUseID: "tc1",
		Content:   "found",
	}

	gotResult, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal ToolResult: %v", err)
	}

	wantResult := `{"tool_use_id":"tc1","content":"found"}`
	if string(gotResult) != wantResult {
		t.Fatalf("ToolResult schema parity mismatch\n got: %s\nwant: %s", gotResult, wantResult)
	}
}
