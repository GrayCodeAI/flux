package catalog

import (
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// EnsureCredentialRegistryInCatalog merges registry providers/deployments into catalog v1.
func EnsureCredentialRegistryInCatalog(c *Catalog) {
	if c == nil {
		return
	}
	if c.Providers == nil {
		c.Providers = map[string]Provider{}
	}
	if c.Deployments == nil {
		c.Deployments = map[string]Deployment{}
	}
	for _, spec := range registry.DefaultRegistry.All() {
		pid := CanonicalProviderID(spec.ProviderID)
		if c.Providers[pid].ID == "" {
			c.Providers[pid] = Provider{ID: pid, Name: spec.DisplayName}
		}
		if c.Deployments[spec.DeploymentID].ID == "" {
			c.Deployments[spec.DeploymentID] = Deployment{
				ID:                  spec.DeploymentID,
				Name:                spec.DisplayName,
				ProviderID:          pid,
				APIProtocolID:       spec.ProtocolID,
				AdapterConstructor:  spec.AdapterID,
				NativeModelIDSource: NativeModelIDDiscovered,
			}
		}
	}
	EnsureDeploymentEnvFallbacks(c)
}

// SpecByEnvVar returns the registry ProviderSpec for the given credential env var.
func SpecByEnvVar(env string) (registry.ProviderSpec, bool) {
	return registry.DefaultRegistry.GetByEnv(env)
}

// SpecByProviderID returns the registry ProviderSpec for the given provider ID or catalog alias.
func SpecByProviderID(id string) (registry.ProviderSpec, bool) {
	id = strings.TrimSpace(id)
	if s, ok := registry.DefaultRegistry.Get(id); ok {
		return s, true
	}
	if alt := catalogProviderIDToRegistry(id); alt != id {
		if s, ok := registry.DefaultRegistry.Get(alt); ok {
			return s, true
		}
	}
	return registry.ProviderSpec{}, false
}

func catalogProviderIDToRegistry(id string) string {
	switch strings.TrimSpace(id) {
	case "google":
		return "gemini"
	case "xai":
		return "grok"
	default:
		return id
	}
}

// ProviderDisplayName returns UI label from registry.
func ProviderDisplayName(providerID string) string {
	return registry.DisplayName(providerID)
}
