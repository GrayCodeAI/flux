package llm_test

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/GrayCodeAI/flux/llm"
)

// TestLlmParity pins the wire schema of llm.FluxMessage (with a ContentPart)
// to the exact JSON the eagle llm contract produces.
func TestLlmParity(t *testing.T) {
	msg := llm.FluxMessage{
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

func TestStreamResultCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	var closes atomic.Int32
	result := llm.NewStreamResult(nil, "", func() { closes.Add(1) })
	copied := *result
	result.Close()
	copied.Close()

	if got := closes.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}
