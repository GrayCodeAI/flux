package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/graycode-router/llm"
)

// TestLlmParity pins the wire schema of llm.GraycodeRouterMessage (with a ContentPart)
// to the exact JSON the eagle llm contract produces.
func TestLlmParity(t *testing.T) {
	msg := llm.GraycodeRouterMessage{
		Role:    "user",
		Content: "hello",
		ContentParts: []llm.ContentPart{
			{Type: "text", Text: "hi"},
		},
	}

	got, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	want := `{"role":"user","content":"hello","content_parts":[{"type":"text","text":"hi"}]}`
	if string(got) != want {
		t.Fatalf("schema parity mismatch\n got: %s\nwant: %s", got, want)
	}
}
