package engine

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestNativeCompactionUsesInjectedCredentialStore(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if eng.SupportsNativeCompaction(ctx, "anthropic", "claude-sonnet-4-6") {
		t.Fatal("compaction reported available without injected credential")
	}
	if err := store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-test"); err != nil {
		t.Fatal(err)
	}
	if !eng.SupportsNativeCompaction(ctx, "anthropic", "claude-sonnet-4-6") {
		t.Fatal("compaction did not use injected credential")
	}
}
