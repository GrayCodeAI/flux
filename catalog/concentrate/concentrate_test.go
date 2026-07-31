package concentrate

import (
	"testing"
)

func TestProtocolFromOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ownedBy string
		want    string
	}{
		{"anthropic", "anthropic"},
		{"Anthropic", "anthropic"},
		{"ANTHROPIC", "anthropic"},
		{"openai", "openai"},
		{"google", "openai"},
		{"mistral", "openai"},
		{"", "openai"},
	}
	for _, tt := range tests {
		got := protocolFromOwner(tt.ownedBy)
		if got != tt.want {
			t.Errorf("protocolFromOwner(%q) = %q, want %q", tt.ownedBy, got, tt.want)
		}
	}
}

func TestUpdateProtocolMap(t *testing.T) {
	t.Parallel()
	ResetProtocolMap()
	t.Cleanup(ResetProtocolMap)

	entries := []struct{ ID, Protocol string }{
		{"claude-opus-5", "anthropic"},
		{"gpt-5", "openai"},
		{"claude-sonnet-4", "anthropic"},
	}
	UpdateProtocolMap(entries)

	if got := ProtocolMapSnapshot(); len(got) != 3 {
		t.Errorf("protocol map len = %d, want 3", len(got))
	}
	if !UsesMessagesAPI("claude-opus-5") {
		t.Error("claude-opus-5 should use Messages API")
	}
	if UsesMessagesAPI("gpt-5") {
		t.Error("gpt-5 should NOT use Messages API")
	}
	if !UsesMessagesAPI("claude-sonnet-4") {
		t.Error("claude-sonnet-4 should use Messages API")
	}
}

func TestUsesMessagesAPI_NoLiveData(t *testing.T) {
	t.Parallel()
	ResetProtocolMap()
	t.Cleanup(ResetProtocolMap)

	// Without live data, Claude models still use Messages API via heuristic
	if !UsesMessagesAPI("claude-opus-5") {
		t.Error("without live data, Claude models should use Messages API via heuristic")
	}
	// Non-Claude models don't use Messages API without live data
	if UsesMessagesAPI("gpt-5") {
		t.Error("without live data, GPT models should NOT use Messages API")
	}
}

func TestProtocolMapSnapshot_Isolation(t *testing.T) {
	t.Parallel()
	ResetProtocolMap()
	t.Cleanup(ResetProtocolMap)

	UpdateProtocolMap([]struct{ ID, Protocol string }{{"claude-opus-5", "anthropic"}})
	snap := ProtocolMapSnapshot()
	snap["gpt-5"] = "anthropic" // mutating snapshot shouldn't affect map

	if UsesMessagesAPI("gpt-5") {
		t.Error("mutating snapshot should not affect internal map")
	}
}

func TestResetProtocolMap(t *testing.T) {
	t.Parallel()
	// Map a non-Claude model to anthropic protocol
	UpdateProtocolMap([]struct{ ID, Protocol string }{{"some-model", "anthropic"}})
	if !UsesMessagesAPI("some-model") {
		t.Fatal("expected some-model to use Messages API before reset")
	}
	ResetProtocolMap()
	if UsesMessagesAPI("some-model") {
		t.Error("after reset, some-model should NOT use Messages API")
	}
	// Claude models still use Messages API via heuristic even after reset
	if !UsesMessagesAPI("claude-opus-5") {
		t.Error("after reset, Claude models should still use Messages API via heuristic")
	}
}
