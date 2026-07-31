// Package xiaomi resolves Xiaomi MiMo API base URLs for pay-as-you-go and Token Plan.
package xiaomi

import (
	"fmt"
	"strings"
)

// Billing identifies pay-as-you-go vs Token Plan products (independent keys and hosts).
type Billing string

const (
	BillingPayAsYouGo Billing = "payg"
	BillingTokenPlan  Billing = "token_plan"
)

// Region is a Token Plan cluster (required for token_plan billing).
type Region string

const (
	RegionCN  Region = "cn"
	RegionSGP Region = "sgp"
	RegionAMS Region = "ams"
)

const (
	PayAsYouGoOpenAIBase   = "https://api.xiaomimimo.com/v1"
	TokenPlanCNOpenAIBase  = "https://token-plan-cn.xiaomimimo.com/v1"  // #nosec G101 -- public API base URL, not a secret value
	TokenPlanSGPOpenAIBase = "https://token-plan-sgp.xiaomimimo.com/v1" // #nosec G101 -- public API base URL, not a secret value
	TokenPlanAMSOpenAIBase = "https://token-plan-ams.xiaomimimo.com/v1" // #nosec G101 -- public API base URL, not a secret value
)

// ProviderPayAsYouGo is the registry / setup gateway id for pay-as-you-go.
const ProviderPayAsYouGo = "xiaomi_mimo_payg"

// ProviderTokenPlan is the registry / setup gateway id for Token Plan.
const ProviderTokenPlan = "xiaomi_mimo_token_plan" // #nosec G101 -- provider id string, not a secret value

// NormalizeRegion parses a region id (cn, sgp, ams).
func NormalizeRegion(region string) (Region, error) {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "cn", "china":
		return RegionCN, nil
	case "sgp", "singapore":
		return RegionSGP, nil
	case "ams", "eu", "europe":
		return RegionAMS, nil
	case "":
		return "", fmt.Errorf("xiaomi: token plan region required")
	default:
		return "", fmt.Errorf("xiaomi: unknown token plan region %q (use cn, sgp, or ams)", region)
	}
}

// BillingForProvider maps registry provider ids to billing mode.
func BillingForProvider(providerID string) (Billing, bool) {
	switch strings.TrimSpace(providerID) {
	case ProviderPayAsYouGo, "xiaomi_mimo":
		return BillingPayAsYouGo, true
	case ProviderTokenPlan:
		return BillingTokenPlan, true
	default:
		return "", false
	}
}

// ResolveOpenAIBasePreferRegion returns the OpenAI base URL. For Token Plan, a valid region
// wins over a stale persisted override (e.g. user switched cn → sgp but base URL was not cleared).
func ResolveOpenAIBasePreferRegion(billing Billing, region Region, override string) (string, error) {
	if billing != BillingTokenPlan || region == "" {
		return ResolveOpenAIBase(billing, region, override)
	}
	fromRegion, err := ResolveOpenAIBase(billing, region, "")
	if err != nil {
		return ResolveOpenAIBase(billing, region, override)
	}
	o := strings.TrimRight(strings.TrimSpace(override), "/")
	if o == "" || o == strings.TrimRight(fromRegion, "/") {
		return fromRegion, nil
	}
	return fromRegion, nil
}

// ResolveOpenAIBase returns the OpenAI-compatible base URL (with /v1 suffix).
func ResolveOpenAIBase(billing Billing, region Region, override string) (string, error) {
	if base := strings.TrimRight(strings.TrimSpace(override), "/"); base != "" {
		return base, nil
	}
	switch billing {
	case BillingPayAsYouGo:
		return PayAsYouGoOpenAIBase, nil
	case BillingTokenPlan:
		switch region {
		case RegionCN:
			return TokenPlanCNOpenAIBase, nil
		case RegionSGP:
			return TokenPlanSGPOpenAIBase, nil
		case RegionAMS:
			return TokenPlanAMSOpenAIBase, nil
		default:
			return "", fmt.Errorf("xiaomi: token plan region required (cn, sgp, ams)")
		}
	default:
		return "", fmt.Errorf("xiaomi: unknown billing mode")
	}
}

// KeyMismatchHint returns a user-facing hint when key shape may not match billing (never blocks save).
func KeyMismatchHint(billing Billing, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	switch billing {
	case BillingPayAsYouGo:
		if strings.HasPrefix(secret, "tp-") {
			return "This looks like a Token Plan key (tp-). Use Xiaomi (MiMo) — Token Plan gateway, or create a pay-as-you-go key at platform.xiaomimimo.com/console/api-keys."
		}
	case BillingTokenPlan:
		if strings.HasPrefix(secret, "sk-") {
			return "This looks like a pay-as-you-go key (sk-). Use Xiaomi (MiMo) — Pay-as-you-go gateway, or use your Token Plan key from plan-manage."
		}
	}
	return ""
}

// AppendKeyMismatchHint adds a billing/key-shape hint to probe or setup errors (never blocks save).
func AppendKeyMismatchHint(err error, providerID, secret string) error {
	if err == nil {
		return nil
	}
	billing, ok := BillingForProvider(providerID)
	if !ok {
		return err
	}
	if hint := KeyMismatchHint(billing, secret); hint != "" {
		return fmt.Errorf("%w · %s", err, hint)
	}
	return err
}
