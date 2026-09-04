package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

// newNilTransportEngine builds an Engine with a valid selection but a transport
// that returns (nil, nil), reproducing a misbehaving custom gateway. Without the
// guard in resolveProvider, this nil provider nil-panics at the downstream
// provider.Chat / provider.StreamChat call sites.
func newNilTransportEngine(t *testing.T) (*Engine, GenerateRequest) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cat := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(filepath.Join(dir, "model_catalog.json"), &cat); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Options{StateDir: dir, SecretStore: &credentials.MapStore{}})
	if err != nil {
		t.Fatal(err)
	}
	modelID := firstCatalogModelID(cat)
	if modelID == "" {
		t.Fatal("seed catalog has no model")
	}
	if err := eng.SetSelection(ctx, "", modelID); err != nil {
		t.Fatal(err)
	}
	eng.resolveTransport = func(context.Context, Route) (client.Provider, error) { return nil, nil }

	req := GenerateRequest{
		Messages:   []Message{{Role: "user", Content: "hi"}},
		Preference: Preference{PreferredModelID: modelID},
		Limits:     Limits{MaxOutputTokens: 256},
	}
	return eng, req
}

// TestResolveProviderNilTransportGenerate verifies Generate returns a classified
// error (instead of panicking) when the transport yields a nil provider.
func TestResolveProviderNilTransportGenerate(t *testing.T) {
	eng, req := newNilTransportEngine(t)

	resp, err := eng.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("Generate() with nil transport: want error, got nil")
	}
	if resp != nil {
		t.Fatalf("Generate() with nil transport: want nil response, got %+v", resp)
	}
}

// TestResolveProviderNilTransportStream verifies Stream returns a classified
// error (instead of panicking) when the transport yields a nil provider.
func TestResolveProviderNilTransportStream(t *testing.T) {
	eng, req := newNilTransportEngine(t)

	stream, err := eng.Stream(context.Background(), req)
	if err == nil {
		if stream != nil {
			stream.Close()
		}
		t.Fatal("Stream() with nil transport: want error, got nil")
	}
	if stream != nil {
		t.Fatalf("Stream() with nil transport: want nil stream, got %+v", stream)
	}
}
