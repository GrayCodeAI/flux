package config

import (
	"github.com/GrayCodeAI/eyrie/catalog/zai"
)

// ResolveZAIOpenAIBase resolves the OpenAI-compat base for a Z.AI gateway id.
func ResolveZAIOpenAIBase(providerID string, cfg *ProviderConfig) (string, error) {
	plan, ok := zai.PlanForProvider(providerID)
	if !ok {
		return "", nil
	}
	var override string
	var regionStr string
	switch plan {
	case zai.PlanGeneral:
		if cfg != nil {
			override = cfg.ZAIBaseURL
			regionStr = cfg.ZAIRegion
		}
	case zai.PlanCoding:
		if cfg != nil {
			override = cfg.ZAICodingBaseURL
			regionStr = cfg.ZAICodingRegion
		}
	}
	region, _ := zai.NormalizeRegion(regionStr)
	return zai.ResolveOpenAIBase(plan, region, override)
}

// ResolveZAIAnthropicBase resolves the Anthropic-compat base for a Z.AI gateway config.
func ResolveZAIAnthropicBase(cfg *ProviderConfig) string {
	regionStr := ""
	if cfg != nil {
		regionStr = cfg.ZAICodingRegion
		if regionStr == "" {
			regionStr = cfg.ZAIRegion
		}
	}
	region, _ := zai.NormalizeRegion(regionStr)
	return zai.ResolveAnthropicBase(region)
}

// IsZAIProvider reports whether id is a Z.AI setup gateway (payg or coding).
func IsZAIProvider(providerID string) bool {
	_, ok := zai.PlanForProvider(providerID)
	return ok
}
