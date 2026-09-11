package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/live"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestPreflightDistinguishesLocalAndLiveReadiness(t *testing.T) {
	ctx := context.Background()
	eng := readyPreflightEngine(t, ctx)
	stubLiveProvider(t, "openai", func(map[string]string) ([]live.Entry, error) {
		return []live.Entry{{ID: catalog.GPT_4o}}, nil
	})

	local := eng.Preflight(ctx)
	if !local.Ready || local.LiveVerified {
		t.Fatalf("local preflight = %+v", local)
	}
	if formatted := FormatPreflight(local); !strings.Contains(formatted, "locally ready") || !strings.Contains(formatted, "not checked") {
		t.Fatalf("local readiness label is ambiguous: %q", formatted)
	}

	live := eng.PreflightWithOptions(ctx, PreflightOptions{VerifyLive: true})
	if !live.Ready || !live.LiveVerified {
		t.Fatalf("live preflight = %+v", live)
	}
	if formatted := FormatPreflight(live); !strings.Contains(formatted, "live verified") {
		t.Fatalf("live readiness label missing: %q", formatted)
	}
}

func TestLivePreflightFailsClosedWhenProviderProbeFails(t *testing.T) {
	ctx := context.Background()
	eng := readyPreflightEngine(t, ctx)
	stubLiveProvider(t, "openai", func(map[string]string) ([]live.Entry, error) {
		return nil, errors.New("authentication rejected")
	})
	report := eng.PreflightWithOptions(ctx, PreflightOptions{VerifyLive: true})
	if report.Ready || report.LiveVerified {
		t.Fatalf("failed provider probe reported ready: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "provider_live" && check.Status == CheckFail && strings.Contains(check.Detail, "verification failed") {
			return
		}
	}
	t.Fatalf("live failure check missing: %+v", report)
}

func TestLivePreflightRequiresSelectedModelInProviderList(t *testing.T) {
	ctx := context.Background()
	eng := readyPreflightEngine(t, ctx)
	stubLiveProvider(t, "openai", func(map[string]string) ([]live.Entry, error) {
		return []live.Entry{{ID: "different-live-model"}}, nil
	})
	report := eng.PreflightWithOptions(ctx, PreflightOptions{VerifyLive: true})
	if report.Ready || report.LiveVerified {
		t.Fatalf("missing selected live model reported ready: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "provider_live" && check.Status == CheckFail && strings.Contains(check.Detail, "not present") {
			return
		}
	}
	t.Fatalf("selected-model live failure check missing: %+v", report)
}

func TestCustomGatewayPreflightDoesNotRequireCatalog(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	if err := store.Set(ctx, credentials.AccountForEnv("PRIVATE_API_KEY"), "private-live-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{
		SecretStore: store, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{{
			ID: "private", BaseURL: "https://private.example.test/v1",
			CredentialEnv: "PRIVATE_API_KEY", DefaultModel: "private/model",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "private", "private/model"); err != nil {
		t.Fatal(err)
	}
	report := eng.Preflight(ctx)
	if !report.Ready {
		t.Fatalf("custom-only local preflight requires catalog: %+v", report)
	}
	assertPreflightCheck(t, report, "catalog", CheckOK, "not required for selected custom gateway")
}

func readyPreflightEngine(t *testing.T, ctx context.Context) *Engine {
	t.Helper()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir(), CustomGateways: []CustomGateway{}})
	if err != nil {
		t.Fatal(err)
	}
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(eng.catalogPath, &seed); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-live-preflight-1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "openai", catalog.GPT_4o); err != nil {
		t.Fatal(err)
	}
	return eng
}

func stubLiveProvider(t *testing.T, provider string, fetch live.FetchFunc) {
	t.Helper()
	previous := live.Registry[provider]
	live.Registry[provider] = fetch
	t.Cleanup(func() { live.Registry[provider] = previous })
}
