package catalog

import (
	"fmt"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog/live"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// FetchLiveProviderCatalog discovers models from all registered live provider APIs
// that have API keys available in env. Returns a catalog with enriched model listings
// and a list of per-provider fetch statuses.
func FetchLiveProviderCatalog(env map[string]string) (Catalog, []LiveProviderEnrichment) {
	cat := SeedCatalog()
	EnsureDeploymentEnvFallbacks(&cat)
	EnsureCredentialRegistryInCatalog(&cat)

	var enrichment []LiveProviderEnrichment
	for _, fetcherKey := range registry.LiveFetcherKeys() {
		spec, ok := registry.SpecForLiveFetcher(fetcherKey)
		if !ok {
			continue
		}
		catalogKey := registry.LiveCatalogKeyForFetcher(fetcherKey)
		start := time.Now()
		scopedEnv := registry.ScopedProviderEnv(spec, env)
		if !registry.CredentialPresent(spec, scopedEnv) {
			reason := "skipped (no API key)"
			if !spec.RequiresKey {
				reason = "skipped (no base URL)"
			}
			enrichment = append(enrichment, LiveProviderEnrichment{Provider: catalogKey, Error: reason, DurationMs: time.Since(start).Milliseconds()})
			continue
		}
		models, err := live.Fetch(fetcherKey, scopedEnv)
		elapsed := time.Since(start)
		duration := elapsed.Milliseconds()
		if err != nil {
			enrichment = append(enrichment, LiveProviderEnrichment{Provider: catalogKey, Error: err.Error(), DurationMs: duration})
			continue
		}
		if len(models) == 0 {
			enrichment = append(enrichment, LiveProviderEnrichment{Provider: catalogKey, Error: "no models returned", DurationMs: duration})
			continue
		}

		providerID := spec.ProviderID
		if _, ok := cat.Providers[providerID]; !ok {
			cat.Providers[providerID] = Provider{ID: providerID, Name: providerID}
		}

		deploymentID := spec.DeploymentID
		if _, ok := cat.Deployments[deploymentID]; !ok {
			cat.Deployments[deploymentID] = Deployment{
				ID:                    deploymentID,
				Name:                  providerID,
				ProviderID:            providerID,
				APIProtocolID:         "openai-chat-completions",
				AdapterConstructor:    "openai",
				NativeModelIDSource:   NativeModelIDDiscovered,
				ModelMappingsRequired: false,
			}
		}

		for _, entry := range models {
			entryID := entry.ID
			if entryID == "" {
				continue
			}
			name := entry.DisplayName
			if name == "" {
				name = entryID
			}

			canonicalID := canonicalModelIDForLiveEntry(providerID, entry)

			cat.Models[canonicalID] = Model{
				ID:            canonicalID,
				ProviderID:    providerID,
				Name:          name,
				ContextWindow: entry.ContextWindow,
				MaxOutput:     entry.MaxOutput,
			}
			cat.Aliases[entryID] = canonicalID

			offeringID := deploymentID + ":" + entryID
			cat.Offerings = append(cat.Offerings, ModelOffering{
				ID:               offeringID,
				CanonicalModelID: canonicalID,
				DeploymentID:     deploymentID,
				NativeModelID:    entryID,
				Capabilities:     CapabilitySetFromEntry(entry),
				Pricing:          PricingFromEntry(entry),
				LiveMetadata:     entry.RawJSON,
			})
		}

		enrichment = append(enrichment, LiveProviderEnrichment{Provider: catalogKey, ModelCount: len(models), DurationMs: duration})
	}
	return cat, enrichment
}

// canonicalModelIDForLiveEntry qualifies ownerless native IDs with the
// provider while preserving owner-qualified IDs. Gateway-priced IDs retain the
// gateway prefix so provider-specific pricing does not collide with a direct
// provider offering for the same underlying model.
func canonicalModelIDForLiveEntry(providerID string, entry live.Entry) string {
	if !hasSlash(entry.ID) {
		return providerID + "/" + entry.ID
	}
	owner, _, hasOwner := splitOwner(entry.ID)
	if hasOwner && owner == canonicalProviderID(providerID) {
		return entry.ID
	}
	if hasInputPricing(entry.RawJSON) {
		return providerID + "/" + entry.ID
	}
	return entry.ID
}

// FetchLiveModelEntriesForProvider lists models from one provider's live API with full JSON metadata.
func FetchLiveModelEntriesForProvider(env map[string]string, providerID string) ([]ModelCatalogEntry, error) {
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return nil, fmt.Errorf("catalog: unknown provider %q", providerID)
	}
	if spec.LiveFetcherKey == "" {
		return nil, fmt.Errorf("catalog: provider %q has no live model list API", providerID)
	}
	env = registry.ScopedProviderEnv(spec, env)
	if !spec.PublicModelCatalog && !registry.CredentialPresent(spec, env) {
		return nil, fmt.Errorf("catalog: set %s for %s", spec.CredentialEnv, providerID)
	}
	entries, err := live.Fetch(spec.LiveFetcherKey, env)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("catalog: live API returned no models for %q", providerID)
	}
	return LiveEntriesToCatalog(entries), nil
}

// LiveDiscoverableDeploymentIDs returns provider IDs that have live model-list APIs.
func LiveDiscoverableDeploymentIDs() []string {
	return registry.LiveFetcherKeys()
}
