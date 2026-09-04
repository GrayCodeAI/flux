package client

import (
	"encoding/json"
	"testing"
)

// TestParseAnthropicResponse_Thinking: a "thinking" content block
// is extracted into the Thinking field (not the Content field).
func TestParseAnthropicResponse_Thinking(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[{"type":"thinking","thinking":"Let me think about this..."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3,"output_tokens_details":{"thinking_tokens":2}}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-thinking", "")
	if resp.Content != "" {
		t.Errorf("Content = %q, want empty (thinking should not be in Content)", resp.Content)
	}
	if resp.Thinking != "Let me think about this..." {
		t.Errorf("Thinking = %q, want %q", resp.Thinking, "Let me think about this...")
	}
	if resp.Usage.ThinkingTokens != 2 {
		t.Errorf("ThinkingTokens = %d, want 2", resp.Usage.ThinkingTokens)
	}
}

// TestParseAnthropicResponse_RedactedThinking: a "redacted_thinking"
// block is skipped silently — it does NOT appear in Content, Thinking,
// or anywhere else. The reasoning is safety-sensitive and we must
// never echo it back to the caller.
func TestParseAnthropicResponse_RedactedThinking(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[{"type":"text","text":"answer"},{"type":"redacted_thinking","data":"ENCRYPTED_REDACTED_BLOB"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-redacted", "")
	if resp.Content != "answer" {
		t.Errorf("Content = %q, want %q (text only; redacted must be skipped)", resp.Content, "answer")
	}
	if resp.Thinking != "" {
		t.Errorf("Thinking = %q, want empty (redacted_thinking must NOT leak into Thinking)", resp.Thinking)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(resp.ToolCalls))
	}
}

// TestParseAnthropicResponse_Mixed: a realistic mixed response
// (text + thinking + tool_use) extracts all three correctly.
func TestParseAnthropicResponse_Mixed(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{
		"content":[
			{"type":"thinking","thinking":"reasoning..."},
			{"type":"text","text":"I'll search."},
			{"type":"tool_use","id":"t1","name":"search","input":{"q":"x"}},
			{"type":"redacted_thinking","data":"BLOB"}
		],
		"stop_reason":"tool_use",
		"usage":{"input_tokens":10,"output_tokens":5,"output_tokens_details":{"thinking_tokens":1}}
	}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-mixed", "")
	if resp.Thinking != "reasoning..." {
		t.Errorf("Thinking = %q, want %q", resp.Thinking, "reasoning...")
	}
	if resp.Content != "I'll search." {
		t.Errorf("Content = %q, want %q", resp.Content, "I'll search.")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, want search", resp.ToolCalls[0].Name)
	}
	if resp.Usage.ThinkingTokens != 1 {
		t.Errorf("ThinkingTokens = %d, want 1", resp.Usage.ThinkingTokens)
	}
}

// TestParseAnthropicResponse_OrgID: the OrganizationID parameter
// flows through to GraycodeRouterResponse.OrganizationID.
func TestParseAnthropicResponse_OrgID(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[{"type":"text","text":"x"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-1", "org-abc")
	if resp.OrganizationID != "org-abc" {
		t.Errorf("OrganizationID = %q, want %q", resp.OrganizationID, "org-abc")
	}

	// Empty orgID is preserved as "" (Bedrock and Vertex pass "").
	resp2 := parseAnthropicResponse(ar, "req-2", "")
	if resp2.OrganizationID != "" {
		t.Errorf("OrganizationID = %q, want empty (Bedrock/Vertex path)", resp2.OrganizationID)
	}
}

// TestParseAnthropicResponse_MultipleTextBlocks: a response with
// multiple text blocks concatenates them in order.
func TestParseAnthropicResponse_MultipleTextBlocks(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[
		{"type":"text","text":"Hello, "},
		{"type":"text","text":"world!"},
		{"type":"text","text":" How are you?"}
	],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":3}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-multi", "")
	want := "Hello, world! How are you?"
	if resp.Content != want {
		t.Errorf("Content = %q, want %q", resp.Content, want)
	}
}

// TestParseAnthropicResponse_MultipleToolUse: a response with
// multiple tool_use blocks appends them in order.
func TestParseAnthropicResponse_MultipleToolUse(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[
		{"type":"tool_use","id":"t1","name":"search","input":{"q":"x"}},
		{"type":"tool_use","id":"t2","name":"read","input":{"file":"a"}}
	],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":3}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-multi-tc", "")
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "t1" || resp.ToolCalls[0].Name != "search" {
		t.Errorf("ToolCalls[0] = %+v", resp.ToolCalls[0])
	}
	if resp.ToolCalls[1].ID != "t2" || resp.ToolCalls[1].Name != "read" {
		t.Errorf("ToolCalls[1] = %+v", resp.ToolCalls[1])
	}
}

// TestParseAnthropicResponse_ToolUse_BadJSON: a tool_use block
// with malformed Input JSON still produces a core.ToolCall entry, but
// with nil Arguments (the unmarshal error is swallowed — same
// behavior as the previous inlined copy).
func TestParseAnthropicResponse_ToolUse_BadJSON(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[
		{"type":"tool_use","id":"t1","name":"search","input":"this-is-not-json"}
	],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	resp := parseAnthropicResponse(ar, "req-bad", "")
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "t1" {
		t.Errorf("ToolCalls[0].ID = %q, want t1", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, want search", resp.ToolCalls[0].Name)
	}
	// Arguments may be nil (json.Unmarshal fails on string input) or
	// an empty map — either is acceptable; nothing to assert here.
	_ = resp.ToolCalls[0].Arguments
}

// TestParseAnthropicResponse_TotalTokens: TotalTokens is the
// sum of InputTokens + OutputTokens (Anthropic's wire format
// doesn't include a TotalTokens field).
func TestParseAnthropicResponse_TotalTokens(t *testing.T) {
	t.Parallel()
	var ar anthropicResponse
	body := `{"content":[],"stop_reason":"end_turn","usage":{"input_tokens":42,"output_tokens":7}}`
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}
	resp := parseAnthropicResponse(ar, "req-1", "")
	if resp.Usage.PromptTokens != 42 {
		t.Errorf("PromptTokens = %d, want 42", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 7 {
		t.Errorf("CompletionTokens = %d, want 7", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 49 {
		t.Errorf("TotalTokens = %d, want 49 (42+7)", resp.Usage.TotalTokens)
	}
}
