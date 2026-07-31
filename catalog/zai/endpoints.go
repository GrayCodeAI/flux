// Package zai resolves Z.AI (Zhipu GLM) API base URLs for General (pay-as-you-go)
// and Coding Plan subscriptions across International vs China regions.
// Hawk uses the OpenAI-compatible surface only.
package zai

import (
	"fmt"
	"strings"
)

// Plan identifies the billing product.
type Plan string

const (
	PlanGeneral Plan = "general"
	PlanCoding  Plan = "coding"
)

// Region identifies International (api.z.ai) vs China (bigmodel.cn family).
type Region string

const (
	RegionInternational Region = "international"
	RegionChina         Region = "cn"
)

// OpenAI-compatible bases (primary for /models and chat/completions).
const (
	GeneralInternationalOpenAIBase = "https://api.z.ai/api/paas/v4"
	CodingInternationalOpenAIBase  = "https://api.z.ai/api/coding/paas/v4"

	GeneralChinaOpenAIBase = "https://open.bigmodel.cn/api/paas/v4"
	CodingChinaOpenAIBase  = "https://open.bigmodel.cn/api/coding/paas/v4"
)

// Provider IDs.
const (
	ProviderGeneral = "zai_payg"
	ProviderCoding  = "zai_coding"
)

// NormalizeRegion accepts common names for the two regions.
func NormalizeRegion(region string) (Region, error) {
	r := strings.ToLower(strings.TrimSpace(region))
	switch r {
	case "", "global", "intl", "international":
		return RegionInternational, nil
	case "cn", "china", "chinese":
		return RegionChina, nil
	default:
		return "", fmt.Errorf("zai: unknown region %q (use international or cn)", region)
	}
}

// PlanForProvider maps gateway ID to plan.
func PlanForProvider(providerID string) (Plan, bool) {
	switch strings.TrimSpace(providerID) {
	case ProviderGeneral:
		return PlanGeneral, true
	case ProviderCoding:
		return PlanCoding, true
	default:
		return "", false
	}
}

// ResolveOpenAIBase returns the correct OpenAI-compat base for the plan + region.
// override wins (ZAI_BASE_URL / ZAI_CODING_BASE_URL).
func ResolveOpenAIBase(plan Plan, region Region, override string) (string, error) {
	if base := strings.TrimRight(strings.TrimSpace(override), "/"); base != "" {
		return base, nil
	}
	switch plan {
	case PlanGeneral:
		if region == RegionChina {
			return GeneralChinaOpenAIBase, nil
		}
		return GeneralInternationalOpenAIBase, nil
	case PlanCoding:
		if region == RegionChina {
			return CodingChinaOpenAIBase, nil
		}
		return CodingInternationalOpenAIBase, nil
	default:
		return "", fmt.Errorf("zai: unknown plan %q", plan)
	}
}

// KeyMismatchHint (kept for future key prefix detection).
func KeyMismatchHint(plan Plan, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if plan == PlanCoding {
		return "Using Z.AI — Coding Plan gateway. Make sure your key comes from a GLM Coding Plan subscription at z.ai."
	}
	return ""
}

func AppendKeyMismatchHint(err error, providerID, secret string) error {
	if err == nil {
		return nil
	}
	plan, ok := PlanForProvider(providerID)
	if !ok {
		return err
	}
	if hint := KeyMismatchHint(plan, secret); hint != "" {
		return fmt.Errorf("%w · %s", err, hint)
	}
	return err
}
