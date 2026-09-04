package discover

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/live"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
	graycoderoutercfg "github.com/GrayCodeAI/graycode-router/config"
)

// RefreshProvider fetches live models for one provider, merges into the catalog cache,
// and returns the compiled catalog. Used after the user saves an API key in /config.
func RefreshProvider(ctx context.Context, providerID string, creds catalog.Credentials) (*catalog.RefreshResult, error) {
	return RefreshProviderWithOptions(ctx, providerID, ProviderRefreshOptions{Credentials: creds})
}

type ProviderRefreshOptions struct {
	Credentials               catalog.Credentials
	CachePath                 string
	DisableCredentialFallback bool
}

// RefreshProviderWithOptions fetches one provider using explicit host-owned
// credentials and cache state when supplied.
func RefreshProviderWithOptions(ctx context.Context, providerID string, opts ProviderRefreshOptions) (*catalog.RefreshResult, error) {
	runMu.Lock()
	defer runMu.Unlock()
	return refreshProvider(ctx, providerID, opts)
}

func refreshProvider(ctx context.Context, providerID string, opts ProviderRefreshOptions) (*catalog.RefreshResult, error) {
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return nil, fmt.Errorf("catalog discover: unknown provider %q", providerID)
	}
	if spec.LiveFetcherKey == "" {
		return nil, fmt.Errorf("catalog discover: provider %q has no live model list API", providerID)
	}

	cachePath := opts.CachePath
	if cachePath == "" {
		cachePath = catalog.DefaultCachePath()
	}
	var base *catalog.Catalog
	source := "cache"
	if compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
		CachePath: cachePath,
	}); err == nil && compiled != nil && compiled.Catalog != nil {
		base = compiled.Catalog
	} else if compiled, ok := catalog.LoadValidCatalogCache(cachePath); ok && compiled.Catalog != nil {
		base = compiled.Catalog
	} else {
		bootstrap := catalog.BootstrapCatalog()
		base = &bootstrap
		source = catalog.BootstrapSource()
	}
	catalog.EnsureDeploymentEnvFallbacks(base)
	catalog.EnsureCredentialRegistryInCatalog(base)

	env := opts.Credentials.Env()
	if len(env) == 0 && !opts.DisableCredentialFallback {
		env = graycoderoutercfg.DiscoveryCredentials(ctx).Env()
	}
	env = registry.ScopedProviderEnv(spec, env)
	if !registry.CredentialPresent(spec, env) {
		return nil, fmt.Errorf("catalog discover: no credentials for provider %q", providerID)
	}

	entries, err := live.Fetch(spec.LiveFetcherKey, env)
	if err != nil {
		return nil, fmt.Errorf("catalog discover: live fetch %q: %w", providerID, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("catalog discover: live API returned no models for %q", providerID)
	}

	generatedAt := time.Now().UTC().Truncate(time.Second)
	cat := catalog.SeedCatalog()
	cat.SchemaVersion = catalog.CatalogSchemaVersion
	cat.GeneratedAt = generatedAt
	cat.StaleAfter = generatedAt.Add(catalog.LiveStaleDuration)
	catalog.EnsureDeploymentEnvFallbacks(&cat)
	catalog.EnsureCredentialRegistryInCatalog(&cat)
	cat.Provenance = &catalog.Provenance{Source: "live-providers", ObservedAt: generatedAt}

	provID := spec.ProviderID
	if _, ok := cat.Providers[provID]; !ok {
		cat.Providers[provID] = catalog.Provider{ID: provID, Name: provID}
	}
	depID := spec.DeploymentID
	if _, ok := cat.Deployments[depID]; !ok {
		cat.Deployments[depID] = catalog.Deployment{
			ID:                    depID,
			Name:                  provID,
			ProviderID:            provID,
			APIProtocolID:         spec.ProtocolID,
			AdapterConstructor:    spec.AdapterID,
			NativeModelIDSource:   catalog.NativeModelIDDiscovered,
			ModelMappingsRequired: false,
		}
	}

	for _, entry := range entries {
		entryID := entry.ID
		if entryID == "" {
			continue
		}
		name := entry.DisplayName
		if name == "" {
			name = entryID
		}
		canonicalID := entryID
		if !strings.Contains(entryID, "/") {
			canonicalID = provID + "/" + entryID
		}
		cat.Models[canonicalID] = catalog.Model{
			ID:            canonicalID,
			ProviderID:    provID,
			Name:          name,
			ContextWindow: entry.ContextWindow,
			MaxOutput:     entry.MaxOutput,
		}
		cat.Aliases[entryID] = canonicalID
		offeringID := depID + ":" + entryID
		cat.Offerings = append(cat.Offerings, catalog.ModelOffering{
			ID:               offeringID,
			CanonicalModelID: canonicalID,
			DeploymentID:     depID,
			NativeModelID:    entryID,
			Capabilities:     catalog.CapabilitySetFromEntry(entry),
			Pricing:          catalog.PricingFromEntry(entry),
			LiveMetadata:     entry.RawJSON,
		})
	}

	// Replace the selected deployment's topology as well as its offerings so a
	// stale cache cannot retain an older adapter or protocol definition.
	delete(base.Deployments, depID)
	base = MergeCatalogWithPolicy(base, &cat, MergePolicy{
		PreferLive:                 true,
		PreferLiveProviders:        []string{catalog.CanonicalProviderID(spec.ProviderID)},
		ReplaceDeploymentOfferings: []string{spec.DeploymentID},
	})
	catalog.PruneUnreferencedDeployments(base)
	now := time.Now().UTC().Truncate(time.Second)
	base.GeneratedAt = now
	base.StaleAfter = now.Add(catalog.LiveStaleDuration)
	if base.Provenance == nil {
		base.Provenance = &catalog.Provenance{}
	}
	source = appendSourceSuffix(source, "providers")
	base.Provenance.Source = source
	base.Provenance.ObservedAt = now

	if err := catalog.WriteCatalogCache(cachePath, base); err != nil {
		return nil, fmt.Errorf("catalog discover: write cache: %w", err)
	}
	compiled, err := catalog.CompileCatalog(base)
	if err != nil {
		return nil, fmt.Errorf("catalog discover: compile: %w", err)
	}
	return &catalog.RefreshResult{
		Compiled:  compiled,
		CachePath: cachePath,
		Source:    source,
		Refreshed: true,
		LiveProviders: []catalog.LiveProviderEnrichment{{
			Provider:   spec.LiveCatalogKey,
			ModelCount: len(entries),
		}},
		StaleAfter: base.StaleAfter,
	}, nil
}
