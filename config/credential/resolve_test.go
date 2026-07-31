package credential

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestValidateKeyFormat(t *testing.T) {
	t.Parallel()
	if err := ValidateKeyFormat(""); err == nil {
		t.Fatal("expected error for empty")
	}
	if err := ValidateKeyFormat("your-api-key"); err == nil {
		t.Fatal("expected placeholder error")
	}
	if err := ValidateKeyFormat("sk-ant-api03-test-key-1234567890"); err != nil {
		t.Fatalf("valid key: %v", err)
	}
}

func TestValidateKeyFormat_NoPrefixRequired(t *testing.T) {
	t.Parallel()
	// Gateway is chosen in UI; secrets need not match vendor prefix patterns.
	keys := []string{
		"0123456789abcdef",                    // no sk- prefix
		"mimo-secret-token-abcdef01",          // xiaomi without tp-
		"random_openrouter_key_1234567890",    // no sk-or-
		"cw_custom_canopywave_key_1234567890", // no cw_ prefix required
	}
	for _, key := range keys {
		if err := ValidateKeyFormat(key); err != nil {
			t.Fatalf("ValidateKeyFormat(%q): %v", key, err)
		}
		if err := ValidateCredentialSecret("XIAOMI_MIMO_PAYG_API_KEY", key); err != nil {
			t.Fatalf("ValidateCredentialSecret xiaomi: %v", err)
		}
	}
}

func TestResolveCredential_ListsAllProviders(t *testing.T) {
	t.Parallel()
	res := ResolveCredential(context.Background(), "sk-ant-api03-test-key-1234567890")
	if !res.FormatOK {
		t.Fatalf("format should be ok: %s", res.FormatError)
	}
	expected := 0
	for _, spec := range registry.All() {
		if spec.RequiresKey {
			expected++
		}
	}
	if len(res.Providers) != expected {
		t.Fatalf("expected %d key-required providers, got %d", expected, len(res.Providers))
	}
	for _, p := range res.Providers {
		if p.Inferred {
			t.Fatalf("provider %q should not be inferred without gateway selection", p.ProviderID)
		}
	}
	if res.Providers[0].ProviderID != "agnes" {
		t.Fatalf("first provider = %q, want agnes (registry order)", res.Providers[0].ProviderID)
	}
}

func TestResolveCredential_InvalidFormat(t *testing.T) {
	t.Parallel()
	res := ResolveCredential(context.Background(), "short")
	if res.FormatOK {
		t.Fatal("expected format error")
	}
}

func TestListCredentialProviders_Count(t *testing.T) {
	t.Parallel()
	if n := len(ListCredentialProviders()); n != len(registry.All()) {
		t.Fatalf("expected %d providers, got %d", len(registry.All()), n)
	}
}
