package credentials

import (
	"context"
	"strings"
	"testing"
)

func TestStoredEnvKeys_StoreOnly(t *testing.T) {
	store := &MapStore{}
	SetDefaultStore(store)
	t.Cleanup(func() { SetDefaultStore(nil) })

	ctx := context.Background()
	t.Setenv("OPENROUTER_API_KEY", "sk-or-from-shell")
	_ = store.Set(ctx, AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-store")

	keys := StoredEnvKeys(ctx)
	if len(keys) != 1 || keys[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("StoredEnvKeys = %v, want [ANTHROPIC_API_KEY]", keys)
	}
}

func TestFormatStorageReport_ListsStoredKeys(t *testing.T) {
	report := StorageReport{
		PlatformStore:    "macOS Keychain",
		KeychainWritable: true,
		StoredEnvKeys:    []string{"ANTHROPIC_API_KEY", "OPENROUTER_API_KEY"},
	}
	out := FormatStorageReport(report)
	if !strings.Contains(out, "Stored:") {
		t.Fatal("expected Stored section")
	}
	if !strings.Contains(out, "ANTHROPIC_API_KEY") || !strings.Contains(out, "OPENROUTER_API_KEY") {
		t.Fatalf("expected env keys in output, got:\n%s", out)
	}
	if strings.Contains(out, "Keys stored:") {
		t.Fatal("should not show stale key count line")
	}
}

func TestDeleteSecret(t *testing.T) {
	store := &MapStore{}
	SetDefaultStore(store)
	t.Cleanup(func() { SetDefaultStore(nil) })

	ctx := context.Background()
	_ = store.Set(ctx, AccountForEnv("OPENAI_API_KEY"), "sk-test")
	if err := DeleteSecret(ctx, "OPENAI_API_KEY"); err != nil {
		t.Fatal(err)
	}
	if HasSecret(ctx, "OPENAI_API_KEY") {
		t.Fatal("expected key to be removed")
	}
}
