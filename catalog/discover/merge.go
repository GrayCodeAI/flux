package discover

import (
	"maps"
	"slices"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
)

// MergePolicy controls catalog merge behavior when enriching from live APIs.
type MergePolicy struct {
	PreferLive                 bool
	PreferLiveProviders        []string
	ReplaceDeploymentOfferings []string
}

func (p MergePolicy) preferLiveForProvider(providerID string) bool {
	if len(p.PreferLiveProviders) == 0 {
		return p.PreferLive
	}
	providerID = strings.TrimSpace(providerID)
	return providerID != "" && slices.Contains(p.PreferLiveProviders, providerID)
}

// MergeCatalog merges models, offerings, providers, deployments, and aliases from src into dst.
func MergeCatalog(dst, src *catalog.Catalog) *catalog.Catalog {
	return MergeCatalogWithPolicy(dst, src, MergePolicy{})
}

// MergeCatalogWithPolicy merges with live replacement for prefer-live providers.
func MergeCatalogWithPolicy(dst, src *catalog.Catalog, policy MergePolicy) *catalog.Catalog {
	if dst == nil {
		return src
	}
	if src == nil {
		return dst
	}
	if dst.Providers == nil {
		dst.Providers = map[string]catalog.Provider{}
	}
	for id, p := range src.Providers {
		if dst.Providers[id].ID == "" {
			dst.Providers[id] = p
		}
	}
	if dst.Protocols == nil {
		dst.Protocols = map[string]catalog.Protocol{}
	}
	for id, p := range src.Protocols {
		if dst.Protocols[id].ID == "" {
			dst.Protocols[id] = p
		}
	}
	if dst.Deployments == nil {
		dst.Deployments = map[string]catalog.Deployment{}
	}
	for id, d := range src.Deployments {
		if dst.Deployments[id].ID == "" {
			dst.Deployments[id] = d
		}
	}
	if dst.Models == nil {
		dst.Models = map[string]catalog.Model{}
	}
	if len(policy.ReplaceDeploymentOfferings) > 0 {
		remove := map[string]bool{}
		for _, dep := range policy.ReplaceDeploymentOfferings {
			if dep = strings.TrimSpace(dep); dep != "" {
				remove[dep] = true
			}
		}
		if len(remove) > 0 {
			filtered := dst.Offerings[:0]
			for _, o := range dst.Offerings {
				if !remove[o.DeploymentID] {
					filtered = append(filtered, o)
				}
			}
			dst.Offerings = filtered
		}
	}
	for id, m := range src.Models {
		if _, ok := dst.Models[id]; ok {
			if policy.preferLiveForProvider(m.ProviderID) {
				dst.Models[id] = m
			}
			continue
		}
		// Key is absent here (the ok branch above continues), so the model is
		// always new — assign unconditionally.
		dst.Models[id] = m
	}
	seen := map[string]int{}
	for i, o := range dst.Offerings {
		seen[o.ID] = i
	}
	for _, o := range src.Offerings {
		if o.ID == "" {
			continue
		}
		if idx, ok := seen[o.ID]; ok {
			if provID := providerIDForOffering(dst, o); policy.preferLiveForProvider(provID) {
				dst.Offerings[idx] = mergeOfferingV1(dst.Offerings[idx], o, provID, policy)
			}
			continue
		}
		seen[o.ID] = len(dst.Offerings)
		dst.Offerings = append(dst.Offerings, o)
	}
	if dst.Aliases == nil {
		dst.Aliases = map[string]string{}
	}
	for alias, canonical := range src.Aliases {
		if dst.Aliases[alias] == "" {
			dst.Aliases[alias] = canonical
		}
	}
	return dst
}

func providerIDForOffering(c *catalog.Catalog, offering catalog.ModelOffering) string {
	if c == nil {
		return ""
	}
	model, ok := c.Models[offering.CanonicalModelID]
	if ok {
		return model.ProviderID
	}
	return ""
}

func mergeOfferingV1(existing, live catalog.ModelOffering, providerID string, policy MergePolicy) catalog.ModelOffering {
	if strings.TrimSpace(live.CanonicalModelID) != "" {
		existing.CanonicalModelID = live.CanonicalModelID
	}
	if strings.TrimSpace(live.DeploymentID) != "" {
		existing.DeploymentID = live.DeploymentID
	}
	if strings.TrimSpace(live.NativeModelID) != "" {
		existing.NativeModelID = live.NativeModelID
	}
	existing.Capabilities = mergeCapabilities(existing.Capabilities, live.Capabilities)
	if policy.preferLiveForProvider(providerID) || shouldReplacePricing(existing.Pricing, live.Pricing) {
		existing.Pricing = live.Pricing
	}
	if len(live.LiveMetadata) > 0 {
		existing.LiveMetadata = live.LiveMetadata
	}
	if live.Provenance != nil {
		existing.Provenance = live.Provenance
	}
	return existing
}

func mergeCapabilities(existing, live catalog.CapabilitySet) catalog.CapabilitySet {
	if len(live.ServerTools) > 0 {
		if existing.ServerTools == nil {
			existing.ServerTools = map[string]catalog.CapabilityState{}
		}
		maps.Copy(existing.ServerTools, live.ServerTools)
	}
	if live.FunctionCalling != "" {
		existing.FunctionCalling = live.FunctionCalling
	}
	if live.ExplicitThinkingBudget != "" {
		existing.ExplicitThinkingBudget = live.ExplicitThinkingBudget
	}
	if live.AdaptiveThinking != "" {
		existing.AdaptiveThinking = live.AdaptiveThinking
	}
	if live.Effort != "" {
		existing.Effort = live.Effort
	}
	if live.StructuredOutput != "" {
		existing.StructuredOutput = live.StructuredOutput
	}
	if live.CodeExecution != "" {
		existing.CodeExecution = live.CodeExecution
	}
	if live.Citations != "" {
		existing.Citations = live.Citations
	}
	if live.PDFInput != "" {
		existing.PDFInput = live.PDFInput
	}
	if live.ImageInput != "" {
		existing.ImageInput = live.ImageInput
	}
	if live.MaxInputTokens > 0 {
		existing.MaxInputTokens = live.MaxInputTokens
	}
	if live.MaxOutputTokens > 0 {
		existing.MaxOutputTokens = live.MaxOutputTokens
	}
	if len(live.ThinkingTypes) > 0 {
		existing.ThinkingTypes = live.ThinkingTypes
	}
	if len(live.EffortLevels) > 0 {
		existing.EffortLevels = live.EffortLevels
	}
	return existing
}

func shouldReplacePricing(existing, live catalog.Pricing) bool {
	if live.Status == "" {
		return false
	}
	if existing.Status == catalog.PricingUnknown && live.Status != catalog.PricingUnknown {
		return true
	}
	if existing.Status == catalog.PricingPartial && live.Status == catalog.PricingKnown {
		return true
	}
	if len(live.RatesPer1M) > 0 {
		return true
	}
	return existing.Status == ""
}
