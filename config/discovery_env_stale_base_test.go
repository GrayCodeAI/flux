package config

import (
	"context"
	"testing"
)

func TestDiscoveryCredentials_StaleBaseURLUsesRegion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLUX_CONFIG_DIR", dir)
	cfg := &ProviderConfig{
		Version:                    "1",
		XiaomiMimoTokenPlanRegion:  "sgp",
		XiaomiMimoTokenPlanBaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
	}
	if err := SaveProviderConfig(cfg, ""); err != nil {
		t.Fatal(err)
	}
	creds := DiscoveryCredentials(context.Background())
	want := "https://token-plan-sgp.xiaomimimo.com/v1"
	if creds.APIKeys[EnvXiaomiTokenPlanBaseURL] != want {
		t.Fatalf("base = %q, want %s", creds.APIKeys[EnvXiaomiTokenPlanBaseURL], want)
	}
}
