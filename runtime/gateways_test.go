package runtime

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestSetupGatewayID(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{"empty", "", ""},
		{"whitespace", "  ", ""},
		{"openai", "openai", "openai"},
		{"lowercase normalization", "OpenAI", "openai"},
		{"whitespace trimming", "  openai  ", "openai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetupGatewayID(tt.provider)
			if got != "" && tt.want != "" {
				// For valid providers, result must be a known setup gateway.
				if !IsSetupGateway(got) {
					t.Errorf("SetupGatewayID(%q) = %q, not a known setup gateway", tt.provider, got)
				}
			}
		})
	}
}

func TestCatalogProviderID_Aliases(t *testing.T) {
	// "gemini" and "grok" are themselves setup gateways (registry ids),
	// so CatalogProviderID passes them through. The alias mapping applies
	// to non-setup-gateway inputs that resolve to a catalog owner.
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		// gemini/grok are setup gateways → passthrough
		{"gemini passthrough", "gemini", "gemini"},
		{"grok passthrough", "grok", "grok"},
		// google/xai are not setup gateways → mapped to catalog owner
		{"google maps to gemini", "google", "gemini"},
		{"xai maps to grok", "xai", "grok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CatalogProviderID(tt.provider)
			if got != tt.want {
				t.Errorf("CatalogProviderID(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestCatalogProviderID_SetupGatewayPassesThrough(t *testing.T) {
	// Setup gateways should pass through to themselves, not be remapped.
	for _, id := range SetupGateways() {
		got := CatalogProviderID(id)
		if got != id {
			t.Errorf("CatalogProviderID(%q) = %q, want passthrough", id, got)
		}
	}
}

func TestSetupGatewayCredentialEnv(t *testing.T) {
	// Every setup gateway that requires a key must have a non-empty env var.
	for _, id := range SetupGateways() {
		spec, ok := registry.SpecByProviderID(id)
		if !ok {
			continue
		}
		if !spec.RequiresKey {
			continue
		}
		env := SetupGatewayCredentialEnv(id)
		if env == "" {
			t.Errorf("SetupGatewayCredentialEnv(%q) = empty, want env var for key-requiring gateway", id)
		}
	}
}

func TestIsSetupGateway_KnownProviders(t *testing.T) {
	// All IDs returned by SetupGateways must be setup gateways.
	for _, id := range SetupGateways() {
		if !IsSetupGateway(id) {
			t.Errorf("IsSetupGateway(%q) = false, want true", id)
		}
	}
	// Unknown provider should not be a setup gateway.
	if IsSetupGateway("definitely-not-a-real-provider-xyz") {
		t.Error("IsSetupGateway returned true for unknown provider")
	}
}

func TestGatewayDisplayName(t *testing.T) {
	// Every setup gateway must have a non-empty display name.
	for _, id := range SetupGateways() {
		name := GatewayDisplayName(id)
		if name == "" {
			t.Errorf("GatewayDisplayName(%q) = empty, want non-empty", id)
		}
	}
	// Unknown provider returns the normalized id (dashes → underscores).
	unknown := "unknown-provider-abc"
	normalized := normalizeRuntimeProviderID(unknown)
	if got := GatewayDisplayName(unknown); got != normalized {
		t.Errorf("GatewayDisplayName(%q) = %q, want %q", unknown, got, normalized)
	}
}

func TestCredentialEnvKeys_NonEmptyForProviders(t *testing.T) {
	// Providers with credential env vars should return non-empty key lists.
	count := 0
	for _, id := range SetupGateways() {
		keys := CredentialEnvKeys(id)
		spec, ok := registry.SpecByProviderID(id)
		if !ok || spec.CredentialEnv == "" {
			continue
		}
		if len(keys) == 0 {
			t.Errorf("CredentialEnvKeys(%q) = empty, want at least %q", id, spec.CredentialEnv)
		}
		count++
	}
	if count == 0 {
		t.Skip("no providers with credential env vars found")
	}
}

func TestCredentialEnvKeys_Deduplicates(t *testing.T) {
	// CredentialEnvKeys must not return duplicate entries.
	for _, id := range SetupGateways() {
		keys := CredentialEnvKeys(id)
		seen := map[string]bool{}
		for _, k := range keys {
			if seen[k] {
				t.Errorf("CredentialEnvKeys(%q) contains duplicate %q", id, k)
			}
			seen[k] = true
		}
	}
}

func TestCredentialEnvKeys_UnknownProvider(t *testing.T) {
	// Unknown provider returns nil.
	keys := CredentialEnvKeys("not-a-real-provider-xyz")
	if keys != nil {
		t.Errorf("CredentialEnvKeys(unknown) = %v, want nil", keys)
	}
}

func TestSetupGateways_NonEmpty(t *testing.T) {
	// The setup gateway list should never be empty.
	gws := SetupGateways()
	if len(gws) == 0 {
		t.Error("SetupGateways() returned empty slice")
	}
}

func TestSetupGateways_AllValid(t *testing.T) {
	// Every ID in SetupGateways must be a known setup gateway.
	for _, id := range SetupGateways() {
		if id == "" {
			t.Error("SetupGateways() contains empty string")
			continue
		}
		if !IsSetupGateway(id) {
			t.Errorf("SetupGateways() contains %q which is not a setup gateway", id)
		}
	}
}

func TestGatewayStatusOpts_Empty(t *testing.T) {
	// GatewayStatuses with empty opts should not panic and should return
	// one entry per setup gateway.
	statuses := GatewayStatuses(context.Background(), GatewayStatusOpts{})
	if len(statuses) != len(SetupGateways()) {
		t.Errorf("GatewayStatuses returned %d statuses, want %d", len(statuses), len(SetupGateways()))
	}
	for _, s := range statuses {
		if s.ID == "" {
			t.Error("GatewayStatus with empty ID")
		}
		if s.DisplayName == "" {
			t.Errorf("GatewayStatus %q has empty DisplayName", s.ID)
		}
	}
}

func TestGatewayStatusOpts_WithActive(t *testing.T) {
	gws := SetupGateways()
	if len(gws) == 0 {
		t.Skip("no setup gateways")
	}
	active := gws[0]
	statuses := GatewayStatuses(context.Background(), GatewayStatusOpts{ActiveProvider: active})
	found := false
	for _, s := range statuses {
		if s.ID == active {
			found = true
			if !s.Active {
				t.Errorf("GatewayStatus %q should be active", active)
			}
		}
	}
	if !found {
		t.Errorf("active gateway %q not found in statuses", active)
	}
}

func TestNormalizeRuntimeProviderID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"openai", "openai"},
		{"OpenAI", "openai"},
		{"  openai  ", "openai"},
		{"z-ai-payg", "zai_payg"},
		{"z-ai-coding", "zai_coding"},
		{"xiaomi-mimo", "xiaomi_mimo_payg"},
		{"xiaomi-mimo-payg", "xiaomi_mimo_payg"},
	}
	for _, tt := range tests {
		got := normalizeRuntimeProviderID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeRuntimeProviderID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
