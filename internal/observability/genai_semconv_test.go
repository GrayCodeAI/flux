package graycoderouter

import "testing"

// TestGenAISemConvKeys pins the canonical gen_ai.* attribute keys so that the
// ecosystem-wide convention documented in docs/OTEL-CONVENTIONS.md cannot drift
// silently. Other hawk-eco repos mirror these exact strings.
func TestGenAISemConvKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"system", AttrGenAISystem, "gen_ai.system"},
		{"request.model", AttrGenAIRequestModel, "gen_ai.request.model"},
		{"response.model", AttrGenAIResponseModel, "gen_ai.response.model"},
		{"usage.input_tokens", AttrGenAIUsageInputTokens, "gen_ai.usage.input_tokens"},
		{"usage.output_tokens", AttrGenAIUsageOutputTokens, "gen_ai.usage.output_tokens"},
		{"operation.name", AttrGenAIOperationName, "gen_ai.operation.name"},
		{"cost.usd", AttrCostUSD, "cost.usd"},
		{"tool.name", AttrToolName, "tool.name"},
		{"session.id", AttrSessionID, "session.id"},
		{"agent.id", AttrAgentID, "agent.id"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// TestGenAISemConvKeysUnique guards against two constants accidentally sharing
// the same attribute key.
func TestGenAISemConvKeysUnique(t *testing.T) {
	t.Parallel()
	keys := []string{
		AttrGenAISystem,
		AttrGenAIRequestModel,
		AttrGenAIResponseModel,
		AttrGenAIUsageInputTokens,
		AttrGenAIUsageOutputTokens,
		AttrGenAIOperationName,
		AttrCostUSD,
		AttrToolName,
		AttrSessionID,
		AttrAgentID,
	}
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate attribute key: %q", k)
		}
		seen[k] = true
	}
}
