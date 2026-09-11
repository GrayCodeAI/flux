package catalog

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// ModelEntriesForProvider lists models from a compiled v1 catalog for one provider.
// New models appear here automatically when the eyrie catalog is updated — hosts must not hardcode IDs.
func ModelEntriesForProvider(compiled *CompiledCatalog, provider string) []ModelCatalogEntry {
	if compiled == nil {
		return nil
	}
	provider = CanonicalProviderID(provider)
	if provider == "" {
		return nil
	}
	if spec, ok := registry.SpecByProviderID(provider); ok {
		return modelEntriesForDeployment(compiled, spec.DeploymentID)
	}
	return modelEntriesByProviderID(compiled, provider)
}

func modelEntriesByProviderID(compiled *CompiledCatalog, provider string) []ModelCatalogEntry {
	seen := map[string]bool{}
	var out []ModelCatalogEntry
	ids := make([]string, 0, len(compiled.ModelsByID))
	for id, model := range compiled.ModelsByID {
		if CanonicalProviderID(model.ProviderID) == provider {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := modelEntryFromOffering(compiled.ModelsByID[id], firstOfferingForModel(compiled, id))
		if entry.ID == "" || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// CanonicalModelForProviderNative maps a picker native id to the canonical model for that provider's
// deployment, without using global catalog aliases (e.g. mimo-v2.5-pro → xiaomi, not opencodego).
func CanonicalModelForProviderNative(compiled *CompiledCatalog, providerID, modelID string) (string, bool) {
	if compiled == nil {
		return "", false
	}
	modelID = strings.TrimSpace(modelID)
	providerID = CanonicalProviderID(providerID)
	if modelID == "" || providerID == "" {
		return "", false
	}
	if model, ok := compiled.ModelsByID[modelID]; ok && CanonicalProviderID(model.ProviderID) == providerID {
		return modelID, true
	}
	if spec, ok := registry.SpecByProviderID(providerID); ok {
		for _, offering := range compiled.OfferingsByDeployment[spec.DeploymentID] {
			if strings.TrimSpace(offering.NativeModelID) == modelID {
				return offering.CanonicalModelID, true
			}
		}
	}
	return "", false
}

func modelEntriesForDeployment(compiled *CompiledCatalog, deploymentID string) []ModelCatalogEntry {
	if compiled == nil || deploymentID == "" {
		return nil
	}
	offerings := compiled.OfferingsByDeployment[deploymentID]
	sort.SliceStable(offerings, func(i, j int) bool {
		return offerings[i].NativeModelID < offerings[j].NativeModelID
	})
	seen := map[string]bool{}
	var out []ModelCatalogEntry
	for _, offering := range offerings {
		model, ok := compiled.ModelsByID[offering.CanonicalModelID]
		if !ok {
			continue
		}
		entry := modelEntryFromOffering(model, offering)
		if entry.ID == "" || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		out = append(out, entry)
	}
	return out
}

func modelEntryFromOffering(model Model, offering ModelOffering) ModelCatalogEntry {
	id := strings.TrimSpace(model.ID)
	if native := strings.TrimSpace(offering.NativeModelID); native != "" {
		id = native
	}
	inPrice, outPrice := 0.0, 0.0
	if offering.Pricing.RatesPer1M != nil {
		inPrice = offering.Pricing.RatesPer1M["input_tokens"]
		outPrice = offering.Pricing.RatesPer1M["output_tokens"]
	}
	return ModelCatalogEntry{
		ID:               id,
		DisplayName:      DisplayModelLabel(id, model.Name),
		Description:      descriptionFromLiveMetadata(offering.LiveMetadata),
		Owner:            DisplayModelOwner(modelOwnerFromOffering(offering), id, offering.LiveMetadata),
		ContextWindow:    model.ContextWindow,
		MaxOutput:        model.MaxOutput,
		InputPricePer1M:  inPrice,
		OutputPricePer1M: outPrice,
		ServerTools:      serverToolsFromOffering(offering),
		LiveMetadata:     offering.LiveMetadata,
	}
}

func descriptionFromLiveMetadata(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var meta struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Description)
}

func modelOwnerFromOffering(offering ModelOffering) string {
	if o := ownerFromLiveMetadata(offering.LiveMetadata); o != "" {
		return o
	}
	return ownerFromModelID(offering.NativeModelID)
}

func serverToolsFromOffering(offering ModelOffering) []string {
	if offering.Capabilities.ServerTools == nil {
		return nil
	}
	var out []string
	for tool, state := range offering.Capabilities.ServerTools {
		if state == CapabilitySupported && strings.TrimSpace(tool) != "" {
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}

func firstOfferingForModel(compiled *CompiledCatalog, canonicalModelID string) ModelOffering {
	offerings := compiled.OfferingsByCanonicalModel[canonicalModelID]
	if len(offerings) == 0 {
		return ModelOffering{}
	}
	sort.SliceStable(offerings, func(i, j int) bool {
		return offerings[i].DeploymentID < offerings[j].DeploymentID
	})
	return offerings[0]
}
