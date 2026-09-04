package registry_test

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// Official protocol surfaces (validated against provider docs, Jul 2026):
//
// OpenAI-only (Anthropic not needed / not advertised):
//   agnes, kimi, openai, grok, openrouter, groq, canopywave, poolside,
//   clinepass, ollama, azure, gemini (native/Gemini protocol), concentrate (Responses)
//
// Vendors documenting both OpenAI + Anthropic (hawk uses exactly one — OpenAI):
//   deepseek, zai_*, xiaomi_mimo_*, minimax_*, longcat
//   opencodego keeps both clients only for per-model routing (one protocol per call;
//   never cross-protocol fallback on the same request)
//
// Anthropic-primary:
//   anthropic, bedrock
//
// Rule: if a vendor is OpenAI-compatible only, do not invent an Anthropic client.
// If a vendor documents both, hawk uses OpenAI only — never both protocols for the
// same provider request (no OpenAI→Anthropic error fallback).

func TestProviderProtocolMatrix_OpenAIOnlyHaveNoAnthropicTransport(t *testing.T) {
	t.Parallel()
	openaiOnly := []string{
		"agnes", "kimi", "openai", "grok", "openrouter", "groq",
		"canopywave", "poolside", "clinepass", "ollama", "azure",
	}
	for _, id := range openaiOnly {
		spec, ok := registry.SpecByProviderID(id)
		if !ok {
			t.Fatalf("missing %q", id)
		}
		if spec.ProtocolID == "anthropic-messages" || spec.TransportKind == "anthropic" {
			t.Fatalf("%s must not use anthropic transport (ProtocolID=%q TransportKind=%q)",
				id, spec.ProtocolID, spec.TransportKind)
		}
		if spec.ProtocolID != "openai-chat-completions" && id != "azure" {
			// azure is still openai-chat-completions
			if spec.ProtocolID != "openai-chat-completions" {
				t.Fatalf("%s ProtocolID = %q, want openai-chat-completions", id, spec.ProtocolID)
			}
		}
	}
}

func TestProviderProtocolMatrix_DualOfficialStayOpenAIPrimary(t *testing.T) {
	t.Parallel()
	// Catalog primary protocol is OpenAI chat completions. Hawk clients for these
	// providers use OpenAI only (OpenCode Go is the exception: per-model single
	// protocol, still no cross-protocol fallback).
	dualPrimaryOpenAI := []string{
		"deepseek", "zai_payg", "zai_coding",
		"xiaomi_mimo_payg", "xiaomi_mimo_token_plan",
		"minimax_payg", "minimax_token_plan",
		"longcat", "opencodego",
	}
	for _, id := range dualPrimaryOpenAI {
		spec, ok := registry.SpecByProviderID(id)
		if !ok {
			t.Fatalf("missing %q", id)
		}
		if spec.ProtocolID != "openai-chat-completions" {
			t.Fatalf("%s catalog primary = %q, want openai-chat-completions", id, spec.ProtocolID)
		}
		if spec.TransportKind == "anthropic" {
			t.Fatalf("%s must not be anthropic-primary in catalog", id)
		}
	}
}

func TestProviderProtocolMatrix_AnthropicPrimary(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"anthropic", "bedrock"} {
		spec, ok := registry.SpecByProviderID(id)
		if !ok {
			t.Fatalf("missing %q", id)
		}
		if spec.ProtocolID != "anthropic-messages" {
			t.Fatalf("%s ProtocolID = %q, want anthropic-messages", id, spec.ProtocolID)
		}
	}
}
