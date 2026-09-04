package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestProviderStateMutationsSerializeAcrossEngineInstances(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	one, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	two, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- one.SetGatewayRegion(ctx, "zai_coding", "china")
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- two.SetSelection(ctx, "openai", catalog.GPT_4o)
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	saved, err := config.LoadProviderConfigWithError(one.providerConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved == nil || saved.ActiveProvider != "openai" || saved.ActiveModel != "openai/gpt-4o" || saved.ZAICodingRegion != "cn" {
		t.Fatalf("concurrent mutation lost state: %#v", saved)
	}
}

func TestProviderStateMutationsRefuseCorruptConfig(t *testing.T) {
	ctx := context.Background()
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"active_provider":`)
	if err := os.WriteFile(eng.providerConfigPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	mutations := []func() error{
		func() error { return eng.SetActiveProvider(ctx, "openai") },
		func() error { return eng.SetSelection(ctx, "openai", "openai/gpt-4o") },
		func() error { return eng.ClearSelection(ctx) },
		func() error { return eng.SetGatewayRegion(ctx, "zai_coding", "china") },
	}
	for i, mutate := range mutations {
		if err := mutate(); err == nil {
			t.Fatalf("mutation %d accepted corrupt provider state", i)
		}
		after, err := os.ReadFile(eng.providerConfigPath)
		if err != nil || !bytes.Equal(after, original) {
			t.Fatalf("mutation %d overwrote corrupt provider state: %q err=%v", i, after, err)
		}
	}
}

func TestProviderStateRejectsUnknownFieldsAndVersions(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"_version":"future","active_provider":"openai"}`),
		[]byte(`{"version":"future","active_provider":"openai"}`),
		[]byte(`{"_version":"1","future_field":true}`),
		[]byte(`{"version":"1","future_field":true}`),
	} {
		eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(eng.providerConfigPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := eng.SetActiveProvider(context.Background(), "openai"); err == nil {
			t.Fatalf("unsupported provider state was overwritten: %s", raw)
		}
	}
}

func TestCorruptProviderStateBlocksCredentialAndRuntimeWrites(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &seed); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	eng, err := New(Options{
		SecretStore: store, StateDir: dir,
		CustomGateways: []CustomGateway{{ID: "private", BaseURL: "https://private.example.test/v1", DefaultModel: "private/model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eng.providerConfigPath, []byte(`{"active_provider":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.SaveCredential(ctx, "openai", "sk-openai-secret-1234567890"); err == nil {
		t.Fatal("credential write accepted corrupt provider state")
	}
	if _, err := store.Get(ctx, credentials.AccountForEnv("OPENAI_API_KEY")); !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("credential was persisted before state validation: %v", err)
	}
	if _, err := eng.Resolve(ctx, SelectionRequest{Preference: Preference{PreferredProvider: "private"}}); err == nil {
		t.Fatal("custom runtime accepted corrupt provider state")
	}
}
