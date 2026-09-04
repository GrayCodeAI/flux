package catalog

import (
	"sort"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// DeploymentIDForLiveCatalogKey maps a live fetch catalog key to a deployment ID.
func DeploymentIDForLiveCatalogKey(catalogKey string) string {
	for _, spec := range registry.All() {
		if spec.LiveCatalogKey == catalogKey {
			return spec.DeploymentID
		}
	}
	return ""
}

// FirstModelForProvider returns the first canonical model ID for a provider from compiled catalog.
func FirstModelForProvider(compiled *CompiledCatalog, providerID string) string {
	if compiled == nil {
		return ""
	}
	providerID = normalizeLiveProviderID(providerID)
	var ids []string
	for id, model := range compiled.ModelsByID {
		if normalizeLiveProviderID(model.ProviderID) == providerID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func normalizeLiveProviderID(providerID string) string {
	switch providerID {
	case "azure":
		return "openai"
	case "bedrock":
		return "anthropic"
	case "vertex":
		return "google"
	}
	return CanonicalProviderID(providerID)
}
