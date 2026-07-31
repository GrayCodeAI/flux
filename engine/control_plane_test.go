package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestControlPlaneUsesInjectedCredentialStore(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{
		SecretStore: store,
		StateDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	providers := eng.CredentialProviders(ctx)
	if len(providers) == 0 {
		t.Fatal("expected registry-backed credential providers")
	}
	resolved := eng.ResolveCredential(ctx, "short")
	if resolved.FormatOK {
		t.Fatal("expected invalid credential format")
	}

	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-injected-secret"); err != nil {
		t.Fatal(err)
	}
	status, err := eng.CredentialStatus(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Masked != "••••••••cret" {
		t.Fatalf("unexpected safe status: %+v", status)
	}
	for _, gateway := range eng.Gateways(ctx) {
		if gateway.ID == "openai" && !gateway.CredentialConfigured {
			t.Fatal("gateway status ignored injected credential store")
		}
	}

	if eng.catalogPath != filepath.Join(filepath.Dir(eng.providerConfigPath), "model_catalog.json") {
		// Both paths must derive from the injected StateDir. The exact assertion
		// catches accidental fallback to process-global Hawk paths.
		t.Fatalf("control-plane paths escaped injected state dir: catalog=%q provider=%q", eng.catalogPath, eng.providerConfigPath)
	}
}

func TestMaskedCredentialNeverReturnsFullSecret(t *testing.T) {
	secret := "sk-sensitive-value"
	masked := maskedCredential(secret)
	if masked == secret || masked == "" {
		t.Fatalf("unsafe masked value %q", masked)
	}
}

func TestCredentialStatusReportsEnvironmentConflictWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-keychain-value"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "sk-environment-value")

	status, err := eng.CredentialStatus(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if status.EnvironmentVariable == "" || !status.EnvironmentConflict {
		t.Fatalf("expected environment conflict, got %+v", status)
	}
	encoded := fmt.Sprintf("%+v", status)
	if strings.Contains(encoded, "sk-keychain-value") || strings.Contains(encoded, "sk-environment-value") {
		t.Fatalf("credential status exposed a secret: %s", encoded)
	}

	t.Setenv("OPENAI_API_KEY", "sk-keychain-value")
	status, err = eng.CredentialStatus(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if status.EnvironmentVariable == "" || status.EnvironmentConflict {
		t.Fatalf("matching environment credential reported conflict: %+v", status)
	}
}

func TestGatewayReadinessRejectsPlaceholderAndDiskSecrets(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "changeme"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{Deployments: map[string]config.DeploymentConfig{
		"openai-direct": {APIKey: "sk-secret-on-disk"},
	}}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	for _, gateway := range eng.Gateways(ctx) {
		if gateway.ID == "openai" && (gateway.CredentialConfigured || gateway.DeploymentConfigured) {
			t.Fatalf("stored or placeholder secret counted as ready: %+v", gateway)
		}
	}
	if selection := eng.EffectiveSelection(ctx, SelectionOptions{}); selection.HasConfiguredDeployment {
		t.Fatalf("stored disk secret made selection ready: %+v", selection)
	}
}

func TestEffectiveSelectionUsesEngineOwnedState(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	stateDir := t.TempDir()
	eng, err := New(Options{SecretStore: store, StateDir: stateDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-injected-secret"); err != nil {
		t.Fatal(err)
	}
	if err := eng.SetActiveProvider(ctx, "openai"); err != nil {
		t.Fatal(err)
	}

	selection := eng.EffectiveSelection(ctx, SelectionOptions{ModelOverride: "gpt-test"})
	if selection.Provider != "openai" || selection.Model != "gpt-test" {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if !selection.HasConfiguredDeployment {
		t.Fatal("selection ignored injected credential store")
	}
}

func TestPreflightRequiresPersistedModelSelection(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-injected-secret"); err != nil {
		t.Fatal(err)
	}
	report := eng.Preflight(ctx)
	if report.Ready {
		t.Fatalf("preflight auto-selected a model instead of requiring user selection: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "model" && check.Status == CheckFail {
			return
		}
	}
	t.Fatalf("preflight did not report missing persisted model: %+v", report)
}

func TestHostControlUsesInjectedStoreAndPaths(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.SaveCredentialEnv(ctx, "OPENAI_API_KEY", "sk-injected-value"); err != nil {
		t.Fatal(err)
	}
	if !eng.HasCredentialEnv(ctx, "OPENAI_API_KEY") {
		t.Fatal("host control ignored injected credential store")
	}
	if err := eng.SetGatewayRegion(ctx, "zai_coding", "international"); err != nil {
		t.Fatal(err)
	}
	label, required := eng.GatewayRegion("zai_coding")
	if label == "" || required {
		t.Fatalf("unexpected gateway region label=%q required=%v", label, required)
	}
	status, err := eng.DeploymentStatus(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, eng.providerConfigPath) || !strings.Contains(status, eng.catalogPath) {
		t.Fatalf("deployment status escaped injected paths: %s", status)
	}
}

func TestProviderStateSecurityFailsClosedOnCorruptState(t *testing.T) {
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eng.providerConfigPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := eng.ProviderStateSecurityStatus()
	if status.Error == "" {
		t.Fatal("expected corrupt provider state to be reported")
	}
}

func TestProviderSecretMigrationStaysInsideEnginePath(t *testing.T) {
	dir := t.TempDir()
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{
		OpenAIAPIKey: "sk-top-level-stored",
		Deployments: map[string]config.DeploymentConfig{
			"openai-direct": {APIKey: "sk-stored", BaseURL: "https://example.test"},
		},
	}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	if status := eng.ProviderStateSecurityStatus(); !status.HasSecrets {
		t.Fatal("expected secret-bearing provider state")
	}
	if err := eng.MigrateProviderSecrets(); err != nil {
		t.Fatal(err)
	}
	if status := eng.ProviderStateSecurityStatus(); status.HasSecrets {
		t.Fatalf("provider state still contains secrets: %+v", status)
	}
	saved := config.LoadProviderConfig(eng.providerConfigPath)
	if saved.OpenAIAPIKey != "" {
		t.Fatal("migration retained a top-level stored credential")
	}
	if saved.Deployments["openai-direct"].BaseURL != "https://example.test" {
		t.Fatal("migration lost non-secret routing metadata")
	}
	// A marker is an audit artifact, not permission to trust later plaintext.
	saved.OpenAIAPIKey = "sk-reintroduced"
	writeProviderConfigFixture(t, eng.providerConfigPath, saved)
	if err := eng.MigrateProviderSecrets(); err != nil {
		t.Fatal(err)
	}
	if config.LoadProviderConfig(eng.providerConfigPath).OpenAIAPIKey != "" {
		t.Fatal("migration marker allowed a reintroduced plaintext credential")
	}
}
