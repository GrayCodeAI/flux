package discover

import (
	"context"
	"fmt"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	graycoderoutercfg "github.com/GrayCodeAI/graycode-router/config"
)

func appendSourceSuffix(source, suffix string) string {
	if source == "embedded" || source == catalog.BootstrapSource() || source == "cache-fallback" {
		if source == "cache-fallback" {
			return "cache-fallback+" + suffix
		}
		return suffix
	}
	return source + "+" + suffix
}

// liveDeploymentIDs returns the set of deployment IDs referenced by
// the offerings in a live-discovered catalog, so the caller can
// replace them in the base catalog.
func liveDeploymentIDs(v1Catalog catalog.Catalog) []string {
	var ids []string
	seen := map[string]bool{}
	for _, o := range v1Catalog.Offerings {
		if o.DeploymentID != "" && !seen[o.DeploymentID] {
			seen[o.DeploymentID] = true
			ids = append(ids, o.DeploymentID)
		}
	}
	return ids
}

// Options configures catalog discovery: published catalog + live provider APIs via API keys.
type Options struct {
	catalog.LoadCatalogOptions
	Credentials               catalog.Credentials
	DisableCredentialFallback bool
}

// Run loads the deployment-aware catalog, optionally refreshes the remote catalog,
// merges live provider model lists when API keys are supplied, writes the cache,
// and returns the compiled catalog.
func Run(ctx context.Context, opts Options) (*catalog.RefreshResult, error) {
	runMu.Lock()
	defer runMu.Unlock()
	return run(ctx, opts)
}

func run(ctx context.Context, opts Options) (*catalog.RefreshResult, error) {
	if opts.CachePath == "" {
		opts.CachePath = catalog.DefaultCachePath()
	}

	var base *catalog.Catalog
	source := "embedded"
	refreshed := false
	remoteRefreshed := false
	var liveProviders []catalog.LiveProviderEnrichment

	if opts.RefreshRemote {
		loadOpts := opts.LoadCatalogOptions
		loadOpts.RemoteURL = catalog.ResolvedRemoteCatalogURL(opts.RemoteURL)
		remote, err := catalog.FetchRemoteCatalog(ctx, loadOpts)
		if err != nil {
			if compiled, ok := catalog.LoadValidCatalogCache(opts.CachePath); ok && compiled.Catalog != nil {
				base = compiled.Catalog
				source = "cache-fallback"
			} else {
				bootstrap := catalog.BootstrapCatalog()
				base = &bootstrap
				source = catalog.BootstrapSource()
			}
		} else {
			base = remote
			source = "remote"
			refreshed = true
			remoteRefreshed = true
			opts.RemoteURL = loadOpts.RemoteURL
		}
	} else {
		compiled, err := catalog.LoadCatalog(ctx, opts.LoadCatalogOptions)
		if err != nil {
			return nil, fmt.Errorf("catalog discover: load: %w", err)
		}
		base = compiled.Catalog
		if base != nil && base.Provenance != nil && base.Provenance.Source != "" {
			source = base.Provenance.Source
		}
	}

	if base == nil {
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
	if len(env) > 0 {
		v1Catalog, enrichment := catalog.FetchLiveProviderCatalog(env)
		liveProviders = enrichment
		if liveEnrichmentSucceeded(enrichment) {
			base = MergeCatalogWithPolicy(base, &v1Catalog, MergePolicy{
				PreferLive:                 true,
				ReplaceDeploymentOfferings: liveDeploymentIDs(v1Catalog),
			})
			now := time.Now().UTC().Truncate(time.Second)
			base.GeneratedAt = now
			base.StaleAfter = now.Add(catalog.LiveStaleDuration)
			if base.Provenance == nil {
				base.Provenance = &catalog.Provenance{}
			}
			source = appendSourceSuffix(source, "providers")
			base.Provenance.Source = source
			base.Provenance.ObservedAt = now
		}
	}

	catalog.PruneUnreferencedDeployments(base)
	if err := catalog.WriteCatalogCache(opts.CachePath, base); err != nil {
		return nil, fmt.Errorf("catalog discover: write cache: %w", err)
	}
	compiled, err := catalog.CompileCatalog(base)
	if err != nil {
		return nil, fmt.Errorf("catalog discover: compile: %w", err)
	}
	if len(compiled.ModelsByID) == 0 {
		return nil, fmt.Errorf("catalog discover: no models available (remote fetch and live APIs returned nothing; check network and API keys)")
	}
	return &catalog.RefreshResult{
		Compiled:        compiled,
		CachePath:       opts.CachePath,
		Source:          source,
		RemoteURL:       opts.RemoteURL,
		Refreshed:       refreshed || liveEnrichmentSucceeded(liveProviders),
		RemoteRefreshed: remoteRefreshed,
		LiveProviders:   liveProviders,
		StaleAfter:      base.StaleAfter,
	}, nil
}

func liveEnrichmentSucceeded(enrichment []catalog.LiveProviderEnrichment) bool {
	for _, provider := range enrichment {
		if provider.Error == "" && provider.ModelCount > 0 {
			return true
		}
	}
	return false
}
