package catalog

import (
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// IsSetupGateway reports whether id is a registered API-key gateway (not an aggregator owner slug).
func IsSetupGateway(providerID string) bool {
	providerID = CanonicalProviderID(providerID)
	if providerID == "" {
		return false
	}
	_, ok := registry.SpecByProviderID(providerID)
	return ok
}

// GatewayForModel returns the setup gateway that serves a model (e.g. openrouter for openrouter/auto).
func GatewayForModel(compiled *CompiledCatalog, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if prefix, _, ok := strings.Cut(modelID, "/"); ok && IsSetupGateway(prefix) {
		return CanonicalProviderID(prefix)
	}
	if compiled == nil {
		return ""
	}
	canonical, ok := compiled.CanonicalModelForAliasOrID(modelID)
	if !ok {
		return ""
	}
	for _, offering := range compiled.OfferingsByCanonicalModel[canonical] {
		dep, ok := compiled.DeploymentsByID[offering.DeploymentID]
		if !ok {
			continue
		}
		gw := CanonicalProviderID(dep.ProviderID)
		if IsSetupGateway(gw) {
			return gw
		}
	}
	if m, ok := compiled.ModelsByID[canonical]; ok {
		gw := CanonicalProviderID(m.ProviderID)
		if IsSetupGateway(gw) {
			return gw
		}
	}
	return ""
}
