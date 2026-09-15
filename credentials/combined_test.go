package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestCombinedStore_WritesKeychainOnly(t *testing.T) {
	store := NewCombinedStore()
	ctx := context.Background()
	if err := store.Set(ctx, AccountForEnv("OPENAI_API_KEY"), "sk-test"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, AccountForEnv("OPENAI_API_KEY"))
	if err != nil || got != "sk-test" {
		t.Fatalf("Get = %q err = %v", got, err)
	}
}

func TestMigrateEnvFileCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	rhoDir := filepath.Join(dir, ".rho")
	if err := os.MkdirAll(rhoDir, 0o700); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(rhoDir, "env")
	if err := os.WriteFile(envPath, []byte("export ANTHROPIC_API_KEY=sk-ant-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	n, err := MigrateEnvFileCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("migrated = %d, want 1", n)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatal("old ~/.rho/env should be removed after migration")
	}
	store := NewCombinedStore()
	got, err := store.Get(ctx, AccountForEnv("ANTHROPIC_API_KEY"))
	if err != nil || got != "sk-ant-test" {
		t.Fatalf("keychain read = %q err = %v", got, err)
	}
}
