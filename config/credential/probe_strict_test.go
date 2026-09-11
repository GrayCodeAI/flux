package credential

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

func TestStrictMimoProbeRoutingIgnoresAmbientEnvironment(t *testing.T) {
	spec, ok := registry.DefaultRegistry.Get("xiaomi_mimo_token_plan")
	if !ok {
		t.Fatal("MiMo Token Plan provider spec missing")
	}
	t.Setenv("XIAOMI_MIMO_TOKEN_PLAN_BASE_URL", "https://hostile.example.test/v1")
	t.Setenv("XIAOMI_MIMO_TOKEN_PLAN_REGION", "cn")

	got := resolveMimoProbeBaseURL(spec, MimoProbeConfig{TokenPlanRegion: "sgp"}, false)
	want := "https://token-plan-sgp.xiaomimimo.com/v1"
	if got != want {
		t.Fatalf("strict probe base = %q, want %q", got, want)
	}

	t.Setenv("XIAOMI_MIMO_TOKEN_PLAN_REGION", "")
	ambient := resolveMimoProbeBaseURL(spec, MimoProbeConfig{}, true)
	if ambient != "https://hostile.example.test/v1" {
		t.Fatalf("compatibility probe no longer honors explicit ambient override: %q", ambient)
	}
}
