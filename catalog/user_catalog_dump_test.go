package catalog_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

func TestUserCatalog_GatewayCountsMatchDeploymentOfferings(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err) // TODO: https://github.com/GrayCodeAI/graycode-router/issues/31
	}
	path := filepath.Join(home, ".graycode-router", "model_catalog.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("no user catalog") // TODO: https://github.com/GrayCodeAI/graycode-router/issues/31
	}
	compiled, err := catalog.LoadCatalog(context.Background(), catalog.LoadCatalogOptions{
		CachePath:    path,
		RequireCache: true,
	})
	if err != nil {
		t.Skipf("no user catalog: %v", err)
	}
	for _, gw := range []string{"openrouter", "canopywave", "gemini"} {
		spec, ok := registry.SpecByProviderID(gw)
		if !ok {
			t.Fatalf("missing spec for %q", gw)
		}
		entries := catalog.ModelEntriesForProvider(compiled, gw)
		raw := len(compiled.OfferingsByDeployment[spec.DeploymentID])
		if len(entries) != raw {
			t.Fatalf("%s: entries=%d raw deployment offerings=%d", gw, len(entries), raw)
		}
	}
}
