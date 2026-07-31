package config

import (
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
)

const (
	EnvXiaomiPaygAPIKey       = "XIAOMI_MIMO_PAYG_API_KEY"       // #nosec G101 -- environment variable name string, not a secret value
	EnvXiaomiTokenPlanAPIKey  = "XIAOMI_MIMO_TOKEN_PLAN_API_KEY" // #nosec G101 -- environment variable name string, not a secret value
	EnvXiaomiPaygBaseURL      = "XIAOMI_MIMO_PAYG_BASE_URL"
	EnvXiaomiTokenPlanBaseURL = "XIAOMI_MIMO_TOKEN_PLAN_BASE_URL" // #nosec G101 -- environment variable name string, not a secret value
	EnvXiaomiTokenPlanRegion  = "XIAOMI_MIMO_TOKEN_PLAN_REGION"   // #nosec G101 -- environment variable name string, not a secret value
)

// XiaomiTokenPlanRegionFromConfig reads persisted Token Plan cluster from provider.json.
func XiaomiTokenPlanRegionFromConfig(cfg *ProviderConfig) xiaomi.Region {
	if cfg == nil {
		return ""
	}
	r, _ := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
	return r
}

// ResolveXiaomiOpenAIBase resolves the OpenAI-compat base for a MiMo gateway id.
func ResolveXiaomiOpenAIBase(providerID string, cfg *ProviderConfig) (string, error) {
	billing, ok := xiaomi.BillingForProvider(providerID)
	if !ok {
		return "", nil
	}
	var override string
	var region xiaomi.Region
	switch billing {
	case xiaomi.BillingPayAsYouGo:
		if cfg != nil {
			override = cfg.XiaomiMimoPaygBaseURL
		}
	case xiaomi.BillingTokenPlan:
		if cfg != nil {
			override = cfg.XiaomiMimoTokenPlanBaseURL
			region = XiaomiTokenPlanRegionFromConfig(cfg)
		}
	}
	return xiaomi.ResolveOpenAIBasePreferRegion(billing, region, override)
}

// ResolveXiaomiAnthropicBase resolves the Anthropic-compat base for a MiMo gateway id.
func ResolveXiaomiAnthropicBase(providerID string, cfg *ProviderConfig) (string, error) {
	openAIBase, err := ResolveXiaomiOpenAIBase(providerID, cfg)
	if err != nil {
		return "", err
	}
	// Anthropic base strips the /v1 suffix; same host, different protocol path.
	return strings.TrimSuffix(strings.TrimRight(openAIBase, "/"), "/v1"), nil
}

// IsXiaomiMimoProvider reports whether id is a MiMo setup gateway (payg or token plan).
func IsXiaomiMimoProvider(providerID string) bool {
	_, ok := xiaomi.BillingForProvider(providerID)
	return ok
}
