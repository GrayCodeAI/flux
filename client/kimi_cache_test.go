package client

import (
	"testing"
)

// TestKimiCacheRoleInjected verifies that when KimiContextCacheID is set and
// the compat config is KimiCompat (SupportsCacheRole=true), buildRequestBase
// prepends a {"role":"cache"} message as the first element of the messages
// array, per the MoonshotAI-Cookbook context-caching spec.
func TestKimiCacheRoleInjected(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{
		Model:              "moonshot-v1-8k",
		KimiContextCacheID: "cache-abc-123",
	}

	req := buildRequestBase(msgs, opts, false, &KimiCompat)

	if len(req.Messages) < 2 {
		t.Fatalf("expected at least 2 messages (cache + user), got %d", len(req.Messages))
	}
	first := req.Messages[0]
	if first["role"] != "cache" {
		t.Errorf("first message role = %q, want %q", first["role"], "cache")
	}
	if first["content"] != "cache-abc-123" {
		t.Errorf("first message content = %q, want %q", first["content"], "cache-abc-123")
	}
	// reset_ttl should not be set when KimiCacheResetTTL is false
	if _, ok := first["reset_ttl"]; ok {
		t.Error("reset_ttl should not be present when KimiCacheResetTTL is false")
	}
	// User message should follow
	if req.Messages[1]["role"] != "user" {
		t.Errorf("second message role = %q, want %q", req.Messages[1]["role"], "user")
	}
}

// TestKimiCacheRoleWithResetTTL verifies that reset_ttl is included in the
// cache message when KimiCacheResetTTL is true.
func TestKimiCacheRoleWithResetTTL(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{
		Model:              "moonshot-v1-8k",
		KimiContextCacheID: "cache-abc-123",
		KimiCacheResetTTL:  true,
	}

	req := buildRequestBase(msgs, opts, false, &KimiCompat)

	if len(req.Messages) < 1 {
		t.Fatal("expected at least 1 message")
	}
	first := req.Messages[0]
	if first["role"] != "cache" {
		t.Fatalf("first message role = %q, want %q", first["role"], "cache")
	}
	resetTTL, ok := first["reset_ttl"]
	if !ok {
		t.Fatal("reset_ttl should be present when KimiCacheResetTTL is true")
	}
	if resetTTL != true {
		t.Errorf("reset_ttl = %v, want true", resetTTL)
	}
}

// TestKimiCacheRoleNotInjectedWhenIDEmpty verifies that no cache message is
// prepended when KimiContextCacheID is empty, even with KimiCompat.
func TestKimiCacheRoleNotInjectedWhenIDEmpty(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{
		Model:              "moonshot-v1-8k",
		KimiContextCacheID: "", // empty — no injection
	}

	req := buildRequestBase(msgs, opts, false, &KimiCompat)

	if len(req.Messages) != 1 {
		t.Fatalf("expected exactly 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "user" {
		t.Errorf("first message role = %q, want %q", req.Messages[0]["role"], "user")
	}
}

// TestKimiCacheRoleNotInjectedForOtherProviders verifies that the cache-role
// injection is skipped when the compat config does not have SupportsCacheRole,
// even if KimiContextCacheID is set. This prevents accidental injection into
// OpenAI, Grok, or any other provider that does not support the cache role.
func TestKimiCacheRoleNotInjectedForOtherProviders(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{
		Model:              "gpt-4o",
		KimiContextCacheID: "cache-abc-123", // set, but compat doesn't support it
	}

	providers := []*OpenAICompatConfig{
		&OpenAICompat,
		&GrokCompat,
		&GeminiCompat,
		&ZAICompat,
		&DeepSeekCompat,
		&OllamaCompat,
	}

	for _, compat := range providers {
		req := buildRequestBase(msgs, opts, false, compat)
		for _, m := range req.Messages {
			if m["role"] == "cache" {
				t.Errorf("provider compat %+v: unexpected cache message injected", compat)
			}
		}
		// Should still have exactly the user message
		found := false
		for _, m := range req.Messages {
			if m["role"] == "user" {
				found = true
			}
		}
		if !found {
			t.Errorf("provider compat %+v: user message missing", compat)
		}
	}
}
