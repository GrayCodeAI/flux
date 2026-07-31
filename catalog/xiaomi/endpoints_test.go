package xiaomi

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveOpenAIBase(t *testing.T) {
	t.Parallel()
	base, err := ResolveOpenAIBase(BillingPayAsYouGo, "", "")
	if err != nil || base != PayAsYouGoOpenAIBase {
		t.Fatalf("payg = %q err=%v", base, err)
	}
	base, err = ResolveOpenAIBase(BillingTokenPlan, RegionSGP, "")
	if err != nil || base != TokenPlanSGPOpenAIBase {
		t.Fatalf("sgp = %q err=%v", base, err)
	}
	override := "https://custom.example/v1"
	base, err = ResolveOpenAIBase(BillingTokenPlan, RegionCN, override)
	if err != nil || base != override {
		t.Fatalf("override = %q err=%v", base, err)
	}
}

func TestResolveOpenAIBasePreferRegion_IgnoresStaleOverride(t *testing.T) {
	t.Parallel()
	staleCN := TokenPlanCNOpenAIBase
	got, err := ResolveOpenAIBasePreferRegion(BillingTokenPlan, RegionSGP, staleCN)
	if err != nil || got != TokenPlanSGPOpenAIBase {
		t.Fatalf("sgp prefer region = %q err=%v", got, err)
	}
	got, err = ResolveOpenAIBasePreferRegion(BillingTokenPlan, RegionSGP, "")
	if err != nil || got != TokenPlanSGPOpenAIBase {
		t.Fatalf("sgp empty override = %q err=%v", got, err)
	}
}

func TestAppendKeyMismatchHint(t *testing.T) {
	t.Parallel()
	base := fmt.Errorf("credential probe failed: invalid API key (HTTP 401)")
	out := AppendKeyMismatchHint(base, ProviderPayAsYouGo, "tp-test")
	if out == nil || !strings.Contains(out.Error(), "Token Plan") {
		t.Fatalf("expected token plan hint, got %v", out)
	}
	if AppendKeyMismatchHint(base, ProviderPayAsYouGo, "sk-test") != base {
		t.Fatal("sk- on payg should not append hint")
	}
}

func TestKeyMismatchHint(t *testing.T) {
	t.Parallel()
	if h := KeyMismatchHint(BillingPayAsYouGo, "tp-abc"); h == "" {
		t.Fatal("expected tp hint on payg")
	}
	if h := KeyMismatchHint(BillingTokenPlan, "sk-abc"); h == "" {
		t.Fatal("expected sk hint on token plan")
	}
}
