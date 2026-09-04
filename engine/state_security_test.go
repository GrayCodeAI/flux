package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestMigrateProviderSecretsImportsBeforeSanitizing(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{
		OpenAIAPIKey: "sk-stored-top-level-1234567890",
		Deployments: map[string]config.DeploymentConfig{
			"openai-direct": {
				APIKey:  "sk-stored-deployment-1234567890",
				BaseURL: "https://gateway.example.test/v1",
			},
		},
	}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	if err := eng.MigrateProviderSecretsContext(ctx); err != nil {
		t.Fatal(err)
	}
	secret, err := store.Get(ctx, credentials.AccountForEnv("OPENAI_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	if secret != "sk-stored-deployment-1234567890" {
		t.Fatalf("imported credential = %q", secret)
	}
	saved := config.LoadProviderConfig(eng.providerConfigPath)
	if saved == nil || config.ProviderConfigContainsSecrets(*saved) {
		t.Fatalf("provider state was not sanitized: %#v", saved)
	}
	if got := saved.Deployments["openai-direct"].BaseURL; got != "https://gateway.example.test/v1" {
		t.Fatalf("non-secret route metadata = %q", got)
	}
}

func TestMigrateProviderStateCanonicalizesVersionAlias(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	oldState := []byte(`{"version":"1","active_provider":"openai","openai_api_key":"sk-stored-version-alias-1234567890"}`)
	if err := os.WriteFile(eng.providerConfigPath, oldState, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := eng.MigrateProviderSecretsContext(ctx); err != nil {
		t.Fatal(err)
	}
	secret, err := store.Get(ctx, credentials.AccountForEnv("OPENAI_API_KEY"))
	if err != nil || secret != "sk-stored-version-alias-1234567890" {
		t.Fatalf("credential was not imported before rewrite: value=%q err=%v", secret, err)
	}
	persisted, err := os.ReadFile(eng.providerConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte(`"version"`)) {
		t.Fatalf("migration persisted the version alias: %s", persisted)
	}
	if !bytes.Contains(persisted, []byte(`"_version"`)) || bytes.Contains(persisted, []byte("sk-stored-version-alias")) {
		t.Fatalf("migration did not atomically canonicalize and sanitize provider state: %s", persisted)
	}
	cfg, err := config.LoadProviderConfigWithError(eng.providerConfigPath)
	if err != nil || cfg == nil || cfg.Version != "1" || cfg.ActiveProvider != "openai" || config.ProviderConfigContainsSecrets(*cfg) {
		t.Fatalf("canonical provider state is invalid: cfg=%#v err=%v", cfg, err)
	}
}

func TestMigrateProviderSecretsRollsBackOnStoreFailure(t *testing.T) {
	ctx := context.Background()
	store := &failSecondSetStore{inner: &credentials.MapStore{}}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{
		OpenAIAPIKey:    "sk-openai-stored-1234567890",
		AnthropicAPIKey: "sk-ant-stored-1234567890",
	}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	original, err := os.ReadFile(eng.providerConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.MigrateProviderSecretsContext(ctx); err == nil {
		t.Fatal("expected injected store failure")
	}
	after, err := os.ReadFile(eng.providerConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("provider state changed after credential import failure")
	}
	for _, envKey := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		if _, err := store.Get(ctx, credentials.AccountForEnv(envKey)); !errors.Is(err, credentials.ErrNotFound) {
			t.Fatalf("partial credential %s survived rollback: %v", envKey, err)
		}
	}
	if _, err := os.Stat(eng.providerConfigPath + ".pre-secret-migrate.bak"); !os.IsNotExist(err) {
		t.Fatalf("migration backup survived failed import: %v", err)
	}
}

func TestMigrateProviderSecretsRefusesUnmappedCredentialFields(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{Deployments: map[string]config.DeploymentConfig{
		"future-provider": {APIKey: "future-secret-1234567890", BaseURL: "https://future.example.test/v1"},
	}}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	original, err := os.ReadFile(eng.providerConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.MigrateProviderSecretsContext(ctx); err == nil {
		t.Fatal("expected unmapped credential migration to fail closed")
	}
	after, err := os.ReadFile(eng.providerConfigPath)
	if err != nil || string(after) != string(original) {
		t.Fatalf("failed migration changed provider state: err=%v", err)
	}
	if _, err := store.Get(ctx, credentials.AccountForEnv("FUTURE_PROVIDER_API_KEY")); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("unmapped credential reached store: %v", err)
	}
}

func TestEngineProviderWritesImportAndSanitizeStoredSecrets(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ProviderConfig{AnthropicAPIKey: "sk-ant-stored-1234567890"}
	writeProviderConfigFixture(t, eng.providerConfigPath, cfg)
	if err := eng.SetActiveProvider(ctx, "anthropic"); err != nil {
		t.Fatal(err)
	}
	secret, err := store.Get(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"))
	if err != nil || secret != "sk-ant-stored-1234567890" {
		t.Fatalf("credential was not safely imported: value=%q err=%v", secret, err)
	}
	saved := config.LoadProviderConfig(eng.providerConfigPath)
	if saved == nil || saved.ActiveProvider != "anthropic" || config.ProviderConfigContainsSecrets(*saved) {
		t.Fatalf("central provider write was not sanitized: %#v", saved)
	}
}

// writeProviderConfigFixture is deliberately test-only: production
// SaveProviderConfig refuses plaintext credential fields.
func writeProviderConfigFixture(t *testing.T, path string, cfg *config.ProviderConfig) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriteBytesAtomicTightensExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration-artifact")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeBytesAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("atomic state mode = %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("atomic state content = %q, err=%v", data, err)
	}
}

type failSecondSetStore struct {
	inner *credentials.MapStore
	sets  int
}

func (s *failSecondSetStore) Set(ctx context.Context, account, secret string) error {
	s.sets++
	if s.sets == 2 {
		return errors.New("injected secret-store write failure")
	}
	return s.inner.Set(ctx, account, secret)
}

func (s *failSecondSetStore) Get(ctx context.Context, account string) (string, error) {
	return s.inner.Get(ctx, account)
}

func (s *failSecondSetStore) Delete(ctx context.Context, account string) error {
	return s.inner.Delete(ctx, account)
}
