package client

import (
	"testing"
)

func TestMergeConsecutiveRoles_Basic(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "user", Content: "How are you?"},
		{Role: "assistant", Content: "I'm fine"},
	}

	merged := MergeConsecutiveRoles(messages)
	if len(merged) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(merged))
	}
	if merged[0].Content != "Hello\nHow are you?" {
		t.Errorf("expected merged content, got %q", merged[0].Content)
	}
	if merged[1].Content != "I'm fine" {
		t.Errorf("expected assistant content, got %q", merged[1].Content)
	}
}

func TestMergeConsecutiveRoles_NoMerge(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Bye"},
	}

	merged := MergeConsecutiveRoles(messages)
	if len(merged) != 3 {
		t.Fatalf("expected 3 messages (no merge), got %d", len(merged))
	}
}

func TestMergeConsecutiveRoles_SkipToolUse(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "assistant", Content: "Let me check", ToolUse: []ToolCall{{Name: "read_file"}}},
		{Role: "assistant", Content: "Here is the result"},
	}

	merged := MergeConsecutiveRoles(messages)
	// Should NOT merge because first has ToolUse
	if len(merged) != 2 {
		t.Fatalf("expected 2 (no merge due to tool_use), got %d", len(merged))
	}
}

func TestMergeConsecutiveRoles_SkipToolResult(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Run the tool"},
		{Role: "user", ToolResults: []ToolResult{{ToolUseID: "abc", Content: "done"}}},
	}

	merged := MergeConsecutiveRoles(messages)
	// Should NOT merge because second has ToolResult
	if len(merged) != 2 {
		t.Fatalf("expected 2 (no merge due to tool_result), got %d", len(merged))
	}
}

func TestMergeConsecutiveRoles_MultipleConsecutive(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "A"},
		{Role: "user", Content: "B"},
		{Role: "user", Content: "C"},
		{Role: "assistant", Content: "X"},
		{Role: "assistant", Content: "Y"},
	}

	merged := MergeConsecutiveRoles(messages)
	if len(merged) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(merged))
	}
	if merged[0].Content != "A\nB\nC" {
		t.Errorf("expected A\\nB\\nC, got %q", merged[0].Content)
	}
	if merged[1].Content != "X\nY" {
		t.Errorf("expected X\\nY, got %q", merged[1].Content)
	}
}

func TestMergeConsecutiveRoles_Empty(t *testing.T) {
	t.Parallel()
	merged := MergeConsecutiveRoles(nil)
	if len(merged) != 0 {
		t.Errorf("expected empty, got %d", len(merged))
	}
}

func TestMergeConsecutiveRoles_Images(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "See this", Images: []string{"img1.png"}},
		{Role: "user", Content: "And this", Images: []string{"img2.png"}},
	}

	merged := MergeConsecutiveRoles(messages)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(merged))
	}
	if len(merged[0].Images) != 2 {
		t.Errorf("expected 2 images, got %d", len(merged[0].Images))
	}
}
