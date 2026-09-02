package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestHasAnyConfiguredDeployment_FromStore(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test-key-long-enough")
	if !HasAnyConfiguredDeployment(context.Background()) {
		t.Fatal("expected configured deployment from credential store")
	}
}

type emptyCredentialStore struct{}

func (emptyCredentialStore) Set(context.Context, string, string) error   { return nil }
func (emptyCredentialStore) Get(context.Context, string) (string, error) { return "", nil }
func (emptyCredentialStore) Delete(context.Context, string) error        { return nil }

func TestHasAnyConfiguredDeployment_RejectsPlaceholder(t *testing.T) {
	t.Setenv("HAWK_CONFIG_DIR", t.TempDir())
	credentials.SetDefaultStore(emptyCredentialStore{})
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	compiled, err := catalog.LoadCatalogForDiscovery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
		t.Setenv(k, "")
	}
	t.Setenv("OPENROUTER_API_KEY", "your-api-key")
	if HasAnyConfiguredDeployment(ctx) {
		t.Fatal("placeholder key should not count as configured")
	}
}

func TestValidateCredentialSecret(t *testing.T) {
	t.Parallel()
	if err := ValidateCredentialSecret("OPENAI_API_KEY", "short"); err == nil {
		t.Fatal("expected error for short key")
	}
	if err := ValidateCredentialSecret("OPENAI_API_KEY", "sk-valid-test-key-1234567890"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
