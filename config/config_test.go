//nolint:errcheck
package config

import (
	"context"
	"os"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestResolveProviderRequest(t *testing.T) {
	// Clear provider env vars to test default resolution
	os.Unsetenv("OPENAI_BASE_URL")
	os.Unsetenv("OPENAI_API_BASE")
	r := ResolveProviderRequest("gpt-4o", "", "")
	if r.ResolvedModel != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", r.ResolvedModel)
	}
	if r.BaseURL != DefaultOpenAIBaseURL {
		t.Errorf("expected default base URL, got %s", r.BaseURL)
	}
	if r.Transport != TransportChatCompletions {
		t.Errorf("expected chat_completions transport")
	}
}

func TestResolveProviderRequestWithReasoning(t *testing.T) {
	t.Parallel()
	r := ResolveProviderRequest("gpt-4o?reasoning=high", "", "")
	if r.ResolvedModel != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", r.ResolvedModel)
	}
	if r.Reasoning == nil || r.Reasoning.Effort != ReasoningHigh {
		t.Error("expected reasoning effort high")
	}
}

func TestIsLocalProviderURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want bool
	}{
		{"http://localhost:11434/v1", true},
		{"http://127.0.0.1:8080", true},
		{"https://api.openai.com/v1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsLocalProviderURL(tt.url); got != tt.want {
			t.Errorf("IsLocalProviderURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsOpenAICompatibleRuntimeEnabled(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	ClearProviderRuntimeEnv()
	t.Cleanup(ClearProviderRuntimeEnv)

	if IsOpenAICompatibleRuntimeEnabled() {
		t.Error("expected false with no keys set")
	}
	_ = store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "test-key")
	if !IsOpenAICompatibleRuntimeEnabled() {
		t.Error("expected true with OPENAI_API_KEY in secure store")
	}
}

func TestNormalizeOllamaOpenAIBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ input, want string }{
		{"http://localhost:11434", "http://localhost:11434/v1"},
		{"http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"http://localhost:11434/", "http://localhost:11434/v1"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeOllamaOpenAIBaseURL(tt.input); got != tt.want {
			t.Errorf("NormalizeOllamaOpenAIBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProviderDetectionOrder(t *testing.T) {
	t.Parallel()
	if len(APIProviderDetectionOrder) != 23 {
		t.Errorf("expected 23 providers in detection order, got %d", len(APIProviderDetectionOrder))
	}
	if APIProviderDetectionOrder[0] != ProviderAnthropic {
		t.Error("expected anthropic first in detection order")
	}
}

func TestValidateAPIKey(t *testing.T) {
	t.Parallel()
	if msg := ValidateAPIKey("", "test"); msg == "" {
		t.Error("expected error for empty key")
	}
	if msg := ValidateAPIKey("SUA_CHAVE", "test"); msg == "" {
		t.Error("expected error for placeholder key")
	}
	if msg := ValidateAPIKey("short", "test"); msg == "" {
		t.Error("expected error for short key")
	}
	if msg := ValidateAPIKey("sk-valid-api-key-here-1234567890", "test"); msg != "" {
		t.Errorf("expected no error for valid key, got %q", msg)
	}
}
