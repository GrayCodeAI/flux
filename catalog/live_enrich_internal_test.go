package catalog

import (
	"encoding/json"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog/live"
)

func TestCanonicalModelIDForLiveEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		providerID string
		entry      live.Entry
		want       string
	}{
		{
			name:       "qualifies ownerless Ollama tag without pricing",
			providerID: "ollama",
			entry:      live.Entry{ID: "qwen3:4b"},
			want:       "ollama/qwen3:4b",
		},
		{
			name:       "preserves matching canonical owner",
			providerID: "gemini",
			entry:      live.Entry{ID: "google/gemini-2.5-pro"},
			want:       "google/gemini-2.5-pro",
		},
		{
			name:       "preserves upstream owner without gateway pricing",
			providerID: "openrouter",
			entry:      live.Entry{ID: "anthropic/claude-sonnet-4"},
			want:       "anthropic/claude-sonnet-4",
		},
		{
			name:       "qualifies gateway-priced upstream model",
			providerID: "openrouter",
			entry: live.Entry{
				ID:      "anthropic/claude-sonnet-4",
				RawJSON: json.RawMessage(`{"input_token_price_per_m": 3}`),
			},
			want: "openrouter/anthropic/claude-sonnet-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := canonicalModelIDForLiveEntry(tt.providerID, tt.entry); got != tt.want {
				t.Fatalf("canonicalModelIDForLiveEntry(%q, %q) = %q, want %q", tt.providerID, tt.entry.ID, got, tt.want)
			}
		})
	}
}
