//nolint:errcheck
package client

import (
	"sort"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// staticProviderNames is a snapshot of every provider in the static runtime
// maps, captured at init() time. Tests that mutate the maps at runtime (e.g.
// TestDynamicProvider_OptIn_Registers calling RegisterDynamicProvider) would
// otherwise corrupt a naive len()-based drift check, so we always compare
// against this snapshot.
var staticProviderNames map[string]bool

func init() {
	staticProviderNames = make(map[string]bool, len(CoreProviders)+len(OpenAICompatibleProviders))
	for k := range CoreProviders {
		staticProviderNames[k] = true
	}
	for k := range OpenAICompatibleProviders {
		staticProviderNames[k] = true
	}
}

// TestProviderRegistry_NoDriftFromCatalog guards against future drift between
// the static runtime provider maps (CoreProviders, OpenAICompatibleProviders)
// and the authoritative spec list in catalog/registry. If a new ProviderSpec
// is added to catalog/registry/providers.go without a corresponding static
// runtime entry, or vice versa, the test fails and the PR is blocked.
//
// This regression test was introduced for H11 (provider-registry drift):
// minimax_token_plan and minimax_payg were present in catalog/registry but
// missing from OpenAICompatibleProviders, causing NewOpenAICompatibleClient
// to mis-route callers to NewOpenAIClient with BaseURL="" (4xx).
func TestProviderRegistry_NoDriftFromCatalog(t *testing.T) {
	t.Parallel()
	specs := registry.All()
	specNames := make(map[string]bool, len(specs))
	for _, s := range specs {
		specNames[s.ProviderID] = true
	}

	// Every spec must be present in the static runtime snapshot.
	var missingInRuntime []string
	for _, s := range specs {
		if !staticProviderNames[s.ProviderID] {
			missingInRuntime = append(missingInRuntime, s.ProviderID)
		}
	}
	if len(missingInRuntime) > 0 {
		sort.Strings(missingInRuntime)
		t.Fatalf(
			"provider-registry drift: %d spec(s) in catalog/registry/providers.go "+
				"are missing from the static runtime maps: %v",
			len(missingInRuntime), missingInRuntime,
		)
	}

	// Every static runtime entry must be present in catalog/registry.
	var staleInRuntime []string
	for name := range staticProviderNames {
		if !specNames[name] {
			staleInRuntime = append(staleInRuntime, name)
		}
	}
	if len(staleInRuntime) > 0 {
		sort.Strings(staleInRuntime)
		t.Fatalf(
			"provider-registry drift: %d provider(s) in client/provider_registry.go "+
				"(static) are missing from catalog/registry/providers.go: %v",
			len(staleInRuntime), staleInRuntime,
		)
	}
}
