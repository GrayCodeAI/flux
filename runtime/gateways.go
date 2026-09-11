package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/catalog/zai"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/setup"
)

const (
	GatewayXiaomiTokenPlan = "xiaomi_mimo_token_plan" // #nosec G101 -- constant name, not a secret value
	gatewayZAIPayg         = "zai_payg"
	gatewayZAICoding       = "zai_coding"
)

// GatewayStatusOpts controls runtime-owned gateway summaries for host setup UIs.
type GatewayStatusOpts struct {
	ActiveProvider string
	ActiveModel    string
}

// GatewayStatus is one setup-gateway row for host UIs.
type GatewayStatus struct {
	ID                      string `json:"id"`
	DisplayName             string `json:"display_name"`
	HasStoredCredential     bool   `json:"has_stored_credential"`
	HasConfiguredDeployment bool   `json:"has_configured_deployment"`
	ModelCount              int    `json:"model_count"`
	Active                  bool   `json:"active"`
	RegionLabel             string `json:"region_label,omitempty"`
	RegionRequired          bool   `json:"region_required,omitempty"`
}

// SetupGateways returns the registry-backed gateway ids users configure in setup UIs.
func SetupGateways() []string {
	specs := registry.CredentialRegistry()
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.ProviderID != "" {
			out = append(out, spec.ProviderID)
		}
	}
	return out
}

// SetupGatewayID canonicalizes a host-facing setup-gateway id through runtime rules.
func SetupGatewayID(provider string) string {
	provider = normalizeRuntimeProviderID(provider)
	if catalog.IsSetupGateway(provider) {
		return provider
	}
	return provider
}

// CatalogProviderID canonicalizes a host-facing provider into the catalog-facing id.
// Setup gateways stay on their registry ids; provider aliases like gemini/grok map to
// their catalog owners google/xai.
func CatalogProviderID(provider string) string {
	if gw := SetupGatewayID(provider); catalog.IsSetupGateway(gw) {
		return gw
	}
	switch normalizeRuntimeProviderID(provider) {
	case "gemini":
		return "google"
	case "grok":
		return "xai"
	default:
		return normalizeRuntimeProviderID(provider)
	}
}

// SetupGatewayCredentialEnv returns the primary credential env var for a setup gateway.
func SetupGatewayCredentialEnv(providerID string) string {
	spec, ok := registry.SpecByProviderID(SetupGatewayID(providerID))
	if !ok || !spec.RequiresKey {
		return ""
	}
	return strings.TrimSpace(spec.CredentialEnv)
}

// IsSetupGateway reports whether the provider id resolves to a registered setup gateway.
func IsSetupGateway(providerID string) bool {
	return catalog.IsSetupGateway(SetupGatewayID(providerID))
}

// GatewayDisplayName returns the registry display label for a setup gateway.
func GatewayDisplayName(providerID string) string {
	providerID = normalizeRuntimeProviderID(providerID)
	if name := registry.DisplayName(providerID); name != providerID {
		return name
	}
	return providerID
}

// ActiveGateway returns the selected setup gateway derived from active provider/model state.
func ActiveGateway(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	return activeGateway(ctx)
}

// GatewayStatuses returns runtime-owned setup-gateway summaries for host /config UIs.
func GatewayStatuses(ctx context.Context, opts GatewayStatusOpts) []GatewayStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	active := normalizedGatewaySelection(ctx, opts)
	rt, _ := Load(ctx)

	statuses := make([]GatewayStatus, 0)
	for _, providerID := range SetupGateways() {
		count := 0
		if rt != nil && rt.Catalog != nil {
			count = len(catalog.ModelEntriesForProvider(rt.Catalog, providerID))
		}
		statuses = append(statuses, GatewayStatus{
			ID:                      providerID,
			DisplayName:             GatewayDisplayName(providerID),
			HasStoredCredential:     HasStoredCredential(ctx, providerID),
			HasConfiguredDeployment: providerConfigured(ctx, providerID),
			ModelCount:              count,
			Active:                  providerID == active,
			RegionLabel:             GatewayRegionLabel(providerID),
			RegionRequired:          GatewayNeedsRegion(providerID),
		})
	}
	return statuses
}

func normalizedGatewaySelection(ctx context.Context, opts GatewayStatusOpts) string {
	if provider := normalizeRuntimeProviderID(opts.ActiveProvider); provider != "" {
		return provider
	}
	if model := strings.TrimSpace(opts.ActiveModel); model != "" {
		if gateway := inferProviderForModel(ctx, model); gateway != "" {
			return gateway
		}
	}
	return activeGateway(ctx)
}

// CachedModelCountForProvider returns the on-disk catalog model count for a gateway.
func CachedModelCountForProvider(ctx context.Context, provider string) int {
	provider = normalizeRuntimeProviderID(provider)
	if provider == "" {
		return 0
	}
	rt, err := Load(ctx)
	if err != nil || rt == nil || rt.Catalog == nil {
		return 0
	}
	return len(catalog.ModelEntriesForProvider(rt.Catalog, provider))
}

// ShouldClearSelectionAfterCredentialRemove reports whether removing a gateway key invalidates the active selection.
func ShouldClearSelectionAfterCredentialRemove(ctx context.Context, removedProvider string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	removedProvider = normalizeRuntimeProviderID(removedProvider)
	if !HasConfiguredDeployment(ctx) {
		return true
	}
	if gw := activeGateway(ctx); gw == removedProvider {
		return true
	}
	if m := strings.TrimSpace(ActiveModel(ctx)); m != "" && inferProviderForModel(ctx, m) == removedProvider {
		return true
	}
	return false
}

// DeploymentRoutingEnabled resolves runtime routing policy plus an optional host override.
func DeploymentRoutingEnabled(ctx context.Context, override *bool) bool {
	_ = ctx
	cfg := config.LoadProviderConfig("")
	return useDeploymentRouting(cfg, override)
}

// HasStoredCredential reports whether the OS secret store has a usable secret for a gateway.
func HasStoredCredential(ctx context.Context, providerID string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, envKey := range gatewayCredentialEnvKeys(providerID) {
		if credentials.HasSecret(ctx, envKey) {
			return true
		}
	}
	return false
}

// CredentialEnvKeys returns the credential env vars associated with a provider,
// including registry fallbacks and compatibility aliases.
func CredentialEnvKeys(providerID string) []string {
	return gatewayCredentialEnvKeys(providerID)
}

func gatewayCredentialEnvKeys(providerID string) []string {
	providerID = normalizeRuntimeProviderID(providerID)
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	add(spec.CredentialEnv)
	for _, env := range spec.CredentialEnvFallbacks {
		add(env)
	}
	for _, env := range registry.CredentialAliases(providerID) {
		add(env)
	}
	return out
}

// PrepareCredentialDiscovery applies runtime-owned gateway env derivations before probe/discovery.
func PrepareCredentialDiscovery(ctx context.Context) {
	for _, providerID := range registry.CredentialEnvPreparedProviders() {
		ApplyGatewayEnv(ctx, providerID)
	}
}

// ApplyGatewayEnv applies derived env settings from provider.json for gateways that need them.
func ApplyGatewayEnv(ctx context.Context, providerID string) {
	_ = ctx
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		return
	}
	switch normalizeRuntimeProviderID(providerID) {
	case GatewayXiaomiTokenPlan:
		if region := strings.TrimSpace(cfg.XiaomiMimoTokenPlanRegion); region != "" {
			_ = os.Setenv(config.EnvXiaomiTokenPlanRegion, region)
		}
		if base, err := config.ResolveXiaomiOpenAIBase(GatewayXiaomiTokenPlan, cfg); err == nil && base != "" {
			_ = os.Setenv(config.EnvXiaomiTokenPlanBaseURL, base)
		}
	case gatewayZAIPayg:
		if region := strings.TrimSpace(cfg.ZAIRegion); region != "" {
			_ = os.Setenv("ZAI_REGION", region)
			norm, _ := zai.NormalizeRegion(region)
			if base, err := zai.ResolveOpenAIBase(zai.PlanGeneral, norm, cfg.ZAIBaseURL); err == nil && base != "" {
				_ = os.Setenv("ZAI_BASE_URL", base)
			}
		}
	case gatewayZAICoding:
		if region := strings.TrimSpace(cfg.ZAICodingRegion); region != "" {
			_ = os.Setenv("ZAI_CODING_REGION", region)
			norm, _ := zai.NormalizeRegion(region)
			if base, err := zai.ResolveOpenAIBase(zai.PlanCoding, norm, cfg.ZAICodingBaseURL); err == nil && base != "" {
				_ = os.Setenv("ZAI_CODING_BASE_URL", base)
			}
		}
	}
}

// GatewayNeedsRegion reports whether a gateway still requires a region selection.
func GatewayNeedsRegion(providerID string) bool {
	cfg := config.LoadProviderConfig("")
	switch normalizeRuntimeProviderID(providerID) {
	case GatewayXiaomiTokenPlan:
		if cfg == nil {
			return true
		}
		_, err := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
		return err != nil
	case gatewayZAICoding:
		if cfg == nil {
			return true
		}
		_, err := zai.NormalizeRegion(cfg.ZAICodingRegion)
		return err != nil
	default:
		return false
	}
}

// GatewayRegionLabel returns the saved normalized region for gateways that require one.
func GatewayRegionLabel(providerID string) string {
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		return ""
	}
	switch normalizeRuntimeProviderID(providerID) {
	case GatewayXiaomiTokenPlan:
		region, err := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
		if err != nil {
			return ""
		}
		return string(region)
	case gatewayZAICoding:
		region, err := zai.NormalizeRegion(cfg.ZAICodingRegion)
		if err != nil {
			return ""
		}
		return string(region)
	default:
		return ""
	}
}

// SetGatewayRegion persists a normalized gateway region and updates derived env/base-url state.
func SetGatewayRegion(providerID, region string) error {
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		cfg = &config.ProviderConfig{}
	}
	switch normalizeRuntimeProviderID(providerID) {
	case GatewayXiaomiTokenPlan:
		normalized, err := xiaomi.NormalizeRegion(region)
		if err != nil {
			return err
		}
		cfg.XiaomiMimoTokenPlanRegion = string(normalized)
		if base, err := config.ResolveXiaomiOpenAIBase(GatewayXiaomiTokenPlan, cfg); err == nil && base != "" {
			cfg.XiaomiMimoTokenPlanBaseURL = base
		}
		if err := config.SaveProviderConfig(cfg, ""); err != nil {
			return err
		}
		ApplyGatewayEnv(context.Background(), GatewayXiaomiTokenPlan)
		return nil
	case gatewayZAIPayg, gatewayZAICoding:
		normalized, err := zai.NormalizeRegion(region)
		if err != nil {
			return err
		}
		if normalizeRuntimeProviderID(providerID) == gatewayZAICoding {
			cfg.ZAICodingRegion = string(normalized)
			if base, err := zai.ResolveOpenAIBase(zai.PlanCoding, normalized, cfg.ZAICodingBaseURL); err == nil && base != "" {
				cfg.ZAICodingBaseURL = base
			}
		} else {
			cfg.ZAIRegion = string(normalized)
			if base, err := zai.ResolveOpenAIBase(zai.PlanGeneral, normalized, cfg.ZAIBaseURL); err == nil && base != "" {
				cfg.ZAIBaseURL = base
			}
		}
		if err := config.SaveProviderConfig(cfg, ""); err != nil {
			return err
		}
		ApplyGatewayEnv(context.Background(), providerID)
		return nil
	default:
		return fmt.Errorf("runtime: gateway %q does not use a selectable region", providerID)
	}
}

// ApplyCredentialsForProvider refreshes runtime catalog/provider state for one gateway after saving credentials.
func ApplyCredentialsForProvider(ctx context.Context, providerID string) (*setup.ApplyCredentialsResult, error) {
	PrepareCredentialDiscovery(ctx)
	return setup.ApplyCredentialsForProvider(ctx, normalizeRuntimeProviderID(providerID), config.DiscoveryCredentials(ctx))
}

// RefreshGatewayCatalog fetches live models for one gateway and updates the cached catalog.
func RefreshGatewayCatalog(ctx context.Context, providerID string) (string, error) {
	PrepareCredentialDiscovery(ctx)
	providerID = normalizeRuntimeProviderID(providerID)
	result, err := setup.DiscoverProviderCatalog(ctx, providerID, config.DiscoveryCredentials(ctx))
	if err != nil {
		return "", err
	}
	count := 0
	if result.Compiled != nil {
		count = len(catalog.ModelEntriesForProvider(result.Compiled, providerID))
	}
	return fmt.Sprintf("Refreshed %s (%d models)", providerID, count), nil
}

// FormatApplyCredentialsSummary summarizes provider apply results for host UIs.
func FormatApplyCredentialsSummary(result *setup.ApplyCredentialsResult) string {
	if result == nil || result.Catalog == nil || result.Catalog.Compiled == nil {
		return "Eyrie credentials applied"
	}
	models := len(result.Catalog.Compiled.ModelsByID)
	deployments := 0
	if result.ProviderConfig != nil {
		deployments = len(result.ProviderConfig.Deployments)
	}
	return fmt.Sprintf(
		"Eyrie: %d models, %d deployments configured, routing updated -> %s",
		models, deployments, result.ProviderConfigPath,
	)
}
