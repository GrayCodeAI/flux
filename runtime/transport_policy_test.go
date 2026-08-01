package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func boolPtr(v bool) *bool { return &v }

func TestPreferredProvider_PrefersOpenAIOverAnthropicWhenBothConfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-openai-test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test"); err != nil {
		t.Fatal(err)
	}

	if got := PreferredProvider(ctx); got != "openai" {
		t.Fatalf("PreferredProvider() = %q, want openai", got)
	}
}

func TestEffectiveSelection_InfersProviderFromModelOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-openai-test"); err != nil {
		t.Fatal(err)
	}

	state := EffectiveSelection(ctx, SelectionOpts{
		ModelOverride: "openai/gpt-4o",
	})
	if state.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", state.Provider)
	}
	if state.Model != "openai/gpt-4o" {
		t.Fatalf("model = %q, want openai/gpt-4o", state.Model)
	}
}

func TestResolveChatTransport_DirectOpenAISingleProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-openai-test"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"), "sk-ant-test"); err != nil {
		t.Fatal(err)
	}

	transport, err := ResolveChatTransport(ctx, ChatTransportOpts{
		Selection: SelectionOpts{
			ProviderOverride:          "openai",
			DeploymentRoutingOverride: boolPtr(false),
		},
	})
	if err != nil {
		t.Fatalf("ResolveChatTransport() error = %v", err)
	}
	if transport.Provider == nil {
		t.Fatal("expected transport provider")
	}
	if got := transport.Provider.Name(); got != "openai" {
		t.Fatalf("provider name = %q, want openai (no fallback chain)", got)
	}
	if transport.Selection.Provider != "openai" {
		t.Fatalf("selection provider = %q, want openai", transport.Selection.Provider)
	}
	if transport.Selection.DeploymentRouting {
		t.Fatal("expected deployment routing disabled")
	}
}
