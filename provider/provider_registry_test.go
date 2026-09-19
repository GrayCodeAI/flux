package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/GrayCodeAI/flux/credentials"
	"github.com/GrayCodeAI/flux/provider/core"
)

func TestGetOrCreateProvider_VertexUsesAnthropicVertexClient(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("VERTEX_PROJECT_ID"), "my-project"); err != nil {
		t.Fatalf("set VERTEX_PROJECT_ID: %v", err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("VERTEX_REGION"), "us-east1"); err != nil {
		t.Fatalf("set VERTEX_REGION: %v", err)
	}

	c := Client(&core.FluxConfig{Provider: "vertex", APIKey: "test-bearer-token"})
	p, err := c.getOrCreateProvider("vertex")
	if err != nil {
		t.Fatalf("getOrCreateProvider: %v", err)
	}
	vc, ok := p.(*VertexClient)
	if !ok {
		t.Fatalf("provider type = %T, want *VertexClient", p)
	}
	if vc.ProjectID() != "my-project" || vc.Region() != "us-east1" {
		t.Fatalf("Vertex project/region = %q/%q", vc.ProjectID(), vc.Region())
	}
	if got := vc.BaseURL(); got != "https://us-east1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east1/publishers/anthropic/models" {
		t.Errorf("baseURL() = %q, want Anthropic-on-Vertex URL", got)
	}
}

func TestGetOrCreateProvider_VertexRegionDefaultsToUsCentral1(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("VERTEX_PROJECT_ID"), "my-project"); err != nil {
		t.Fatal(err)
	}
	c := Client(&core.FluxConfig{Provider: "vertex", APIKey: "test-token"})
	p, err := c.getOrCreateProvider("vertex")
	if err != nil {
		t.Fatal(err)
	}
	vc, ok := p.(*VertexClient)
	if !ok {
		t.Fatalf("provider type = %T, want *VertexClient", p)
	}
	if vc.Region() != "us-central1" {
		t.Fatalf("Vertex region = %q", vc.Region())
	}
}

func TestGetOrCreateProvider_VertexRequiresProjectID(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	c := Client(&core.FluxConfig{Provider: "vertex", APIKey: "test-token"})
	_, err := c.getOrCreateProvider("vertex")
	if err == nil || err.Error() != "flux: vertex requires VERTEX_PROJECT_ID" {
		t.Fatalf("error = %v, want missing Vertex project", err)
	}
}

func TestUnknownProviderIgnoresAmbientOpenAIBase(t *testing.T) {
	t.Setenv("FLUX_ALLOW_DYNAMIC_PROVIDERS", "1")
	t.Setenv("OPENAI_API_BASE", "http://attacker.example/v1")
	c := Client(&core.FluxConfig{Provider: "openai", APIKey: "test-key"})
	_, err := c.getOrCreateProvider("ghost")
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("unknown provider error = %v", err)
	}
	if c.GetProviderInfo("ghost") != nil {
		t.Fatal("ambient URL registered a provider")
	}
}
