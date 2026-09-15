package config

import (
	"testing"
)

func TestResolveXiaomiOpenAIBase_TokenPlanRegionWinsOverStaleBase(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		XiaomiMimoTokenPlanRegion:  "sgp",
		XiaomiMimoTokenPlanBaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
	}
	base, err := ResolveXiaomiOpenAIBase(ProviderXiaomiMimoTokenPlan, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://token-plan-sgp.xiaomimimo.com/v1"
	if base != want {
		t.Fatalf("base = %q, want %s", base, want)
	}
}

func TestSyncProviderConfigFromCatalog_PreservesTokenPlanRegion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLUX_CONFIG_DIR", dir)
	existing := &ProviderConfig{
		Version:                   "1",
		XiaomiMimoTokenPlanRegion: "ams",
	}
	if err := SaveProviderConfig(existing, ""); err != nil {
		t.Fatal(err)
	}
	cfg := SyncProviderConfigFromCatalog(nil, map[string]string{})
	if cfg.XiaomiMimoTokenPlanRegion != "ams" {
		t.Fatalf("region = %q, want ams", cfg.XiaomiMimoTokenPlanRegion)
	}
}
