package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/setup"
)

// ModelEntriesForProvider returns models for a provider from this runtime's catalog snapshot.
func (r *Runtime) ModelEntriesForProvider(provider string) []catalog.ModelCatalogEntry {
	if r == nil || r.Catalog == nil {
		return nil
	}
	return catalog.ModelEntriesForProvider(r.Catalog, provider)
}

// ModelsForProvider loads the catalog cache and returns models for provider.
// Prefer ListModels for host UIs (registry-aware live vs cache).
func ModelsForProvider(ctx context.Context, provider string) ([]catalog.ModelCatalogEntry, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return nil, fmt.Errorf("runtime: provider required")
	}
	rt, err := Load(ctx)
	if err != nil {
		return nil, err
	}
	if entries := rt.ModelEntriesForProvider(provider); len(entries) > 0 {
		return entries, nil
	}
	if _, err := Discover(ctx); err != nil {
		return nil, err
	}
	rt, err = Load(ctx)
	if err != nil {
		return nil, err
	}
	entries := rt.ModelEntriesForProvider(provider)
	if len(entries) == 0 {
		return nil, fmt.Errorf("runtime: no models for provider %q in graycode-router catalog (add deployment/model in graycode-router catalog source)", provider)
	}
	return entries, nil
}

// SetupUIFromCatalog builds provider/model picker rows from the current catalog cache.
func SetupUIFromCatalog(ctx context.Context, providerFilter string) (*setup.SetupUI, error) {
	rt, err := Load(ctx)
	if err != nil {
		return nil, err
	}
	if rt.Catalog == nil {
		return &setup.SetupUI{}, nil
	}
	return setup.BuildSetupUI(rt.Catalog, providerFilter), nil
}

// AllModelIDs returns every canonical model ID in the catalog (sorted).
func AllModelIDs(ctx context.Context) ([]string, error) {
	rt, err := Load(ctx)
	if err != nil {
		return nil, err
	}
	return rt.ModelIDs(), nil
}

// PrimaryAPIKeyEnv returns the main env var for a deployment ID from the catalog.
func PrimaryAPIKeyEnv(deploymentID string) string {
	rt, err := Load(context.Background())
	if err != nil || rt.Catalog == nil {
		return ""
	}
	return catalog.PrimaryAPIKeyEnvForDeployment(rt.Catalog, deploymentID)
}

// ProviderIDForDeployment maps a deployment id to catalog provider id.
func ProviderIDForDeployment(deploymentID string) string {
	rt, err := Load(context.Background())
	if err != nil || rt.Catalog == nil {
		return ""
	}
	dep, ok := rt.Catalog.DeploymentsByID[deploymentID]
	if !ok {
		return ""
	}
	return catalog.CanonicalProviderID(dep.ProviderID)
}
