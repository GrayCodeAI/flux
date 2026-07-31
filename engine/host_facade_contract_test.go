package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestGatewayDefinitionsArePureMetadataWithSeparateRanks(t *testing.T) {
	store := &countingStore{inner: &credentials.MapStore{}}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir(), CustomGateways: []CustomGateway{}})
	if err != nil {
		t.Fatal(err)
	}
	definitions := eng.GatewayDefinitions()
	if store.gets != 0 {
		t.Fatalf("pure gateway metadata read the credential store %d times", store.gets)
	}
	if paths := eng.StatePaths(); paths.Catalog != eng.catalogPath || paths.ProviderConfig != eng.providerConfigPath || store.gets != 0 {
		t.Fatalf("StatePaths() performed work or returned wrong paths: %+v", paths)
	}
	var anthropic, openai Gateway
	for i, gateway := range definitions {
		if i > 0 && definitions[i-1].SortOrder > gateway.SortOrder {
			t.Fatalf("gateway definitions are not in UI order: %+v", definitions)
		}
		switch gateway.ID {
		case "anthropic":
			anthropic = gateway
		case "openai":
			openai = gateway
		}
	}
	if anthropic.ID == "" || openai.ID == "" ||
		anthropic.SortOrder >= openai.SortOrder || openai.ChatPreference >= anthropic.ChatPreference {
		t.Fatalf("UI and chat ranks were conflated: anthropic=%+v openai=%+v", anthropic, openai)
	}
	configured := map[string]bool{"anthropic": true, "openai": true}
	if got := preferredConfiguredGateway(definitions, configured); got != "openai" {
		t.Fatalf("preferred gateway = %q, want chat-ranked openai", got)
	}
}

func TestCustomGatewayOptionsOverrideProcessGlobalRegistry(t *testing.T) {
	registerCustomGatewayForTest(t, CustomGateway{
		ID: "global-only-contract", BaseURL: "https://global.example.test/v1", DefaultModel: "global/model",
	})
	store := &credentials.MapStore{}
	explicit, err := New(Options{
		SecretStore: store, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{{
			ID: "instance-only-contract", BaseURL: "https://instance.example.test/v1", DefaultModel: "instance/model",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := explicit.customGateway("instance-only-contract"); !ok {
		t.Fatal("per-engine custom gateway missing")
	}
	if _, ok := explicit.customGateway("global-only-contract"); ok {
		t.Fatal("per-engine options leaked process-global compatibility gateway")
	}
	compat, err := New(Options{SecretStore: store, StateDir: t.TempDir(), UseRegisteredCustomGateways: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := compat.customGateway("global-only-contract"); !ok {
		t.Fatal("nil custom options did not preserve compatibility registration")
	}
}

func TestCustomGatewayCredentialStatusAndRemovalUseInjectedStore(t *testing.T) {
	ctx := context.Background()
	const envKey = "INSTANCE_ONLY_API_KEY"
	store := &credentials.MapStore{}
	if err := store.Set(ctx, credentials.AccountForEnv(envKey), "instance-live-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{
		SecretStore: store, StateDir: t.TempDir(),
		CustomGateways: []CustomGateway{{
			ID: "instance-only", BaseURL: "https://instance.example.test/v1",
			CredentialEnv: envKey, DefaultModel: "instance/model",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := eng.CredentialStatus(ctx, "instance-only")
	if err != nil || !status.Configured || status.EnvVar != envKey {
		t.Fatalf("custom credential status = %+v, err=%v", status, err)
	}
	if err := eng.RemoveCredential(ctx, "instance-only"); err != nil {
		t.Fatal(err)
	}
	status, err = eng.CredentialStatus(ctx, "instance-only")
	if err != nil || status.Configured {
		t.Fatalf("custom credential survived removal: %+v, err=%v", status, err)
	}
}

func TestCredentialAliasesDriveStatusDiscoveryAndRemoval(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		provider string
		primary  string
		alias    string
	}{
		{provider: "anthropic", primary: "ANTHROPIC_API_KEY", alias: "CLAUDE_API_KEY"},
		{provider: "gemini", primary: "GEMINI_API_KEY", alias: "GOOGLE_API_KEY"},
		{provider: "xiaomi_mimo_payg", primary: "XIAOMI_MIMO_PAYG_API_KEY", alias: "XIAOMI_MIMO_API_KEY"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			dir := t.TempDir()
			seed := catalog.SeedCatalog()
			if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
				t.Fatal(err)
			}
			compiled, err := catalog.CompileCatalog(&seed)
			if err != nil {
				t.Fatal(err)
			}
			store := &credentials.MapStore{}
			if err := store.Set(ctx, credentials.AccountForEnv(test.alias), "alias-live-secret-1234567890"); err != nil {
				t.Fatal(err)
			}
			eng, err := New(Options{SecretStore: store, StateDir: dir, CustomGateways: []CustomGateway{}})
			if err != nil {
				t.Fatal(err)
			}
			status, err := eng.CredentialStatus(ctx, test.provider)
			if err != nil || !status.Configured || status.EnvVar != test.primary {
				t.Fatalf("alias status = %+v, err=%v", status, err)
			}
			if got := eng.credentialEnv(ctx, compiled)[test.primary]; got != "alias-live-secret-1234567890" {
				t.Fatalf("canonical discovery credential = %q", got)
			}
			if err := eng.RemoveCredential(ctx, test.provider); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(ctx, credentials.AccountForEnv(test.alias)); err == nil {
				t.Fatal("credential alias survived removal")
			}
		})
	}
}

func TestModelRowsKeepOwnerGatewayCanonicalAndLiveMetadataDistinct(t *testing.T) {
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	var liveCanonical string
	for i := range seed.Offerings {
		if seed.Offerings[i].DeploymentID == "gemini-direct" {
			seed.Offerings[i].LiveMetadata = json.RawMessage(`{"owned_by":"google-live","future_field":true}`)
			liveCanonical = seed.Offerings[i].CanonicalModelID
			break
		}
	}
	if liveCanonical == "" {
		t.Fatal("seed catalog has no Gemini offering")
	}
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir, CustomGateways: []CustomGateway{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		gateway string
		owner   string
	}{
		{gateway: "gemini", owner: "google"},
		{gateway: "grok", owner: "xai"},
	} {
		models, err := eng.ListModels(context.Background(), test.gateway, false)
		if err != nil || len(models) == 0 {
			t.Fatalf("ListModels(%q) = %+v, err=%v", test.gateway, models, err)
		}
		for _, model := range models {
			if model.ProviderID != test.owner || model.GatewayID != test.gateway || model.CanonicalID == "" {
				t.Fatalf("model identity was conflated for %q: %+v", test.gateway, model)
			}
		}
	}
	models, err := eng.ListModels(context.Background(), "gemini", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, model := range models {
		if model.CanonicalID == liveCanonical {
			found = true
			var metadata map[string]interface{}
			if err := json.Unmarshal(model.LiveMetadata, &metadata); err != nil ||
				metadata["owned_by"] != "google-live" || metadata["future_field"] != true {
				t.Fatalf("live provider metadata was not preserved: %s", model.LiveMetadata)
			}
		}
	}
	if !found {
		t.Fatalf("live canonical row %q not found: %+v", liveCanonical, models)
	}
}

type countingStore struct {
	inner *credentials.MapStore
	gets  int
}

func (s *countingStore) Set(ctx context.Context, account, secret string) error {
	return s.inner.Set(ctx, account, secret)
}

func (s *countingStore) Get(ctx context.Context, account string) (string, error) {
	s.gets++
	return s.inner.Get(ctx, account)
}

func (s *countingStore) Delete(ctx context.Context, account string) error {
	return s.inner.Delete(ctx, account)
}
