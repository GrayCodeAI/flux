package setup

import (
	"context"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/discover"
)

// DiscoverModelCatalogOptions controls catalog discovery behavior.
type DiscoverModelCatalogOptions struct {
	// ForceRefresh re-fetches the published remote catalog and all live provider APIs.
	ForceRefresh bool
}

// DiscoverModelCatalog refreshes the remote eyrie catalog and enriches it using the supplied API keys.
// Pass config.DiscoveryCredentials(ctx) or explicit keys; eyrie owns all model metadata.
func DiscoverModelCatalog(ctx context.Context, creds catalog.Credentials) (*catalog.RefreshResult, error) {
	return DiscoverModelCatalogWithOptions(ctx, creds, DiscoverModelCatalogOptions{})
}

// DiscoverModelCatalogWithOptions runs discover with optional force refresh (manual hawk models refresh).
func DiscoverModelCatalogWithOptions(ctx context.Context, creds catalog.Credentials, opts DiscoverModelCatalogOptions) (*catalog.RefreshResult, error) {
	cachePath := catalog.DefaultCachePath()
	refreshRemote := opts.ForceRefresh
	if !refreshRemote {
		refreshRemote = true
		if compiled, ok := catalog.LoadValidCatalogCache(cachePath); ok && compiled.Catalog != nil {
			if !compiled.Catalog.StaleAfter.IsZero() && time.Now().UTC().Before(compiled.Catalog.StaleAfter) {
				refreshRemote = false
			}
		}
	}
	return discover.Run(ctx, discover.Options{
		LoadCatalogOptions: catalog.LoadCatalogOptions{
			CachePath:     cachePath,
			RefreshRemote: refreshRemote,
		},
		Credentials: creds,
	})
}

// DiscoverProviderCatalog fetches live models for one provider after API key setup.
func DiscoverProviderCatalog(ctx context.Context, providerID string, creds catalog.Credentials) (*catalog.RefreshResult, error) {
	return discover.RefreshProvider(ctx, providerID, creds)
}

// LoadCompiledCatalog returns the compiled catalog from cache/embedded data without network refresh.
func LoadCompiledCatalog(ctx context.Context) (*catalog.CompiledCatalog, error) {
	return catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
		CachePath:    catalog.DefaultCachePath(),
		RequireCache: true,
	})
}
