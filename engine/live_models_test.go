package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/live"
	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestListLiveModelsUsesInjectedStateWithoutMutatingCache(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := &credentials.MapStore{}
	if err := store.Set(ctx, credentials.AccountForEnv("CANOPYWAVE_API_KEY"), "injected-live-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "unrelated-openai-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{SecretStore: store, StateDir: dir, CustomGateways: []CustomGateway{}})
	if err != nil {
		t.Fatal(err)
	}
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(eng.catalogPath, &seed); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(eng.catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CANOPYWAVE_API_KEY", "ambient-secret-must-not-be-used")

	previous := live.Registry["canopywave"]
	var captured map[string]string
	live.Registry["canopywave"] = func(env map[string]string) ([]live.Entry, error) {
		captured = make(map[string]string, len(env))
		for key, value := range env {
			captured[key] = value
		}
		return []live.Entry{{
			ID: "vendor/live-model", DisplayName: "Live Model", OwnedBy: "vendor",
			ContextWindow: 256_000, MaxOutput: 16_000,
			InputPricePer1M: 1.25, OutputPricePer1M: 5,
			Features: []string{"future_tool"},
			RawJSON:  json.RawMessage(`{"id":"vendor/live-model","future_field":true}`),
		}}, nil
	}
	t.Cleanup(func() { live.Registry["canopywave"] = previous })

	models, err := eng.ListLiveModels(ctx, "canopywave")
	if err != nil {
		t.Fatal(err)
	}
	if captured["CANOPYWAVE_API_KEY"] != "injected-live-secret-1234567890" {
		t.Fatalf("live listing escaped injected credentials: %#v", captured)
	}
	if _, leaked := captured["OPENAI_API_KEY"]; leaked {
		t.Fatalf("provider-scoped fetch received an unrelated credential: %#v", captured)
	}
	if len(models) != 1 || models[0].ID != "vendor/live-model" || models[0].Source != "live" ||
		models[0].GatewayID != "canopywave" || models[0].ProviderID != "canopywave" ||
		len(models[0].Capabilities) != 1 || models[0].Capabilities[0] != "future_tool" ||
		!json.Valid(models[0].LiveMetadata) {
		t.Fatalf("direct live model contract = %+v", models)
	}
	after, err := os.ReadFile(eng.catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("direct live model listing mutated the catalog cache")
	}
}
