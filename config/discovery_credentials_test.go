package config

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestDiscoveryCredentials_UsesStoreNotProcessEnv(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	t.Setenv("OPENROUTER_API_KEY", "sk-or-from-shell-should-not-appear")
	_ = store.Set(context.Background(), credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-store-only-key-1234567890")

	creds := DiscoveryCredentials(context.Background())
	if creds.APIKeys["OPENROUTER_API_KEY"] != "" {
		t.Fatal("DiscoveryCredentials must not read API keys from process env")
	}
	if creds.APIKeys["ANTHROPIC_API_KEY"] == "" {
		t.Fatal("expected ANTHROPIC_API_KEY from store")
	}
}

func TestDiscoveryCredentials_IncludesTokenPlanRegionFromProviderConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)

	cfg := &ProviderConfig{Version: "1", XiaomiMimoTokenPlanRegion: "sgp"}
	if err := SaveProviderConfig(cfg, ""); err != nil {
		t.Fatal(err)
	}

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	_ = store.Set(context.Background(), credentials.AccountForEnv(EnvXiaomiTokenPlanAPIKey), "tp-test-key-1234567890123456")

	creds := DiscoveryCredentials(context.Background())
	if creds.APIKeys[EnvXiaomiTokenPlanRegion] != "sgp" {
		t.Fatalf("region = %q, want sgp", creds.APIKeys[EnvXiaomiTokenPlanRegion])
	}
	wantBase := "https://token-plan-sgp.xiaomimimo.com/v1"
	if creds.APIKeys[EnvXiaomiTokenPlanBaseURL] != wantBase {
		t.Fatalf("base = %q, want %s", creds.APIKeys[EnvXiaomiTokenPlanBaseURL], wantBase)
	}
	if creds.APIKeys[EnvXiaomiTokenPlanAPIKey] == "" {
		t.Fatal("expected token plan API key from store")
	}
}
