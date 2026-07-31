package adapters

import (
	"context"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
)

func TestPoolsideClientReasoningOnlyStreamFallsBackToChat(t *testing.T) {
	t.Parallel()
	var requests int
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		var body struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := jsonDecodeRequest(req, &body); err != nil {
			t.Fatalf("decode fallback request: %v", err)
		}
		if requests == 1 {
			if len(body.Tools) == 0 {
				t.Fatal("primary Poolside completion lost agent tools")
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"choices": []map[string]any{{
					"message":       map[string]string{"role": "assistant", "reasoning_content": "thinking"},
					"finish_reason": "length",
				}},
			}), nil
		}
		if len(body.Tools) != 0 {
			t.Fatalf("recovery request retained %d tools, want text-only recovery", len(body.Tools))
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]string{"role": "assistant", "content": "Hi"},
				"finish_reason": "stop",
			}},
		}), nil
	})

	client := NewPoolsideClient("poolside-test-key", "https://poolside.example/v1")
	client.openAI.httpClient = &http.Client{Transport: transport}
	result, err := client.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{
		Model: "poolside/laguna-m.1", MaxTokens: 512,
		Tools: []core.EyrieTool{{Name: "read_file", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for event := range result.Events {
		switch event.Type {
		case "thinking":
			t.Fatal("reasoning-only primary stream leaked before fallback")
		case "content":
			content += event.Content
		case "error":
			t.Fatalf("unexpected stream error: %s", event.Error)
		}
	}
	if content != "Hi" {
		t.Fatalf("content = %q, want Hi", content)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want stream plus chat fallback", requests)
	}
}
