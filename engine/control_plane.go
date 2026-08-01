package engine

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	llm "github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// ResolveCredential validates credential input and returns safe provider
// choices. Eyrie does not retain or return the supplied secret.
func (e *Engine) ResolveCredential(ctx context.Context, secret string) CredentialResolution {
	resolved := config.ResolveCredential(nonNilContext(ctx), secret)
	out := CredentialResolution{
		FormatOK: resolved.FormatOK, FormatError: resolved.FormatError,
		ProbeDisambiguationUsed: resolved.ProbeDisambiguationUsed,
		Providers:               make([]CredentialProvider, len(resolved.Providers)),
	}
	for i, provider := range resolved.Providers {
		out.Providers[i] = CredentialProvider{
			ProviderID: provider.ProviderID, DeploymentID: provider.DeploymentID,
			EnvVar: provider.EnvVar, DisplayName: provider.DisplayName,
			RequiresKey: provider.RequiresKey, Rank: provider.Rank,
		}
	}
	if out.FormatOK {
		for _, gateway := range e.GatewayDefinitions() {
			if _, custom := e.customGateway(gateway.ID); !custom || !gateway.RequiresKey {
				continue
			}
			out.Providers = append(out.Providers, CredentialProvider{
				ProviderID: gateway.ID, EnvVar: gateway.CredentialEnv,
				DisplayName: gateway.DisplayName, RequiresKey: true, Rank: gateway.SortOrder,
			})
		}
	}
	return out
}

// CredentialProviders lists safe provider choices for setup UIs.
func (e *Engine) CredentialProviders(context.Context) []CredentialProvider {
	providers := config.ListCredentialProviders()
	out := make([]CredentialProvider, len(providers))
	for i, provider := range providers {
		out[i] = CredentialProvider{
			ProviderID: provider.ProviderID, DeploymentID: provider.DeploymentID,
			EnvVar: provider.EnvVar, DisplayName: provider.DisplayName,
			RequiresKey: provider.RequiresKey, Rank: provider.Rank,
		}
	}
	for _, gateway := range e.GatewayDefinitions() {
		if _, custom := e.customGateway(gateway.ID); !custom {
			continue
		}
		out = append(out, CredentialProvider{
			ProviderID: gateway.ID, EnvVar: gateway.CredentialEnv,
			DisplayName: gateway.DisplayName, RequiresKey: gateway.RequiresKey, Rank: gateway.SortOrder,
		})
	}
	return out
}

// RegisteredGatewayCount returns the first-class provider count from the
// provider registry. Hosts derive provider counts from Eyrie instead of
// hard-coding them, so new providers require no host changes.
func RegisteredGatewayCount() int {
	return len(registry.CredentialRegistry())
}

// DefaultThinkingDisabled reports whether a provider defaults thinking OFF when unset.
func (e *Engine) DefaultThinkingDisabled(providerID string) bool {
	return DefaultThinkingDisabled(providerID)
}

// ThinkingToggleSupported reports whether a provider's wire protocol honors thinking toggles.
func (e *Engine) ThinkingToggleSupported(providerID string) bool {
	return ThinkingToggleSupported(providerID)
}

// DefaultThinkingDisabled reports whether a provider defaults thinking OFF when unset.
func DefaultThinkingDisabled(providerID string) bool {
	spec, ok := registry.SpecByProviderID(providerID)
	return ok && spec.DefaultThinkingDisabled
}

// ThinkingToggleSupported reports whether a provider's wire protocol honors thinking toggles.
func ThinkingToggleSupported(providerID string) bool {
	spec, ok := registry.SpecByProviderID(providerID)
	return ok && spec.ThinkingToggleSupported
}

// GatewayDefinitions returns pure registry/custom metadata in setup UI order.
// It does not read credentials, provider state, or the model catalog.
func (e *Engine) GatewayDefinitions() []Gateway {
	specs := registry.CredentialRegistry()
	out := make([]Gateway, 0, len(specs)+len(e.customGateways))
	for _, spec := range specs {
		providerSpec, _ := registry.SpecByProviderID(spec.ProviderID)
		gw := Gateway{
			ID: spec.ProviderID, DisplayName: spec.DisplayName,
			DeploymentID: spec.DeploymentID, CredentialEnv: spec.EnvVar,
			RequiresKey: spec.RequiresKey, SortOrder: spec.SortOrder, ChatPreference: providerSpec.ChatPreference,
			SupportsLiveDiscovery: strings.TrimSpace(providerSpec.LiveFetcherKey) != "",
			DNSHost:               providerSpec.DNSHost,
		}
		if len(providerSpec.RegionOptions) > 0 {
			gw.RegionOptions = make([]llm.GatewayRegionOption, len(providerSpec.RegionOptions))
			for i, ro := range providerSpec.RegionOptions {
				gw.RegionOptions[i] = llm.GatewayRegionOption{Value: ro.Value, DisplayName: ro.DisplayName, Endpoint: ro.Endpoint}
			}
		}
		out = append(out, gw)
	}
	for _, gateway := range e.customGateways {
		out = append(out, Gateway{
			ID: gateway.ID, DisplayName: gateway.DisplayName,
			CredentialEnv: gateway.CredentialEnv, RequiresKey: gateway.CredentialEnv != "",
			ModelCount: boolInt(gateway.DefaultModel != ""), SortOrder: gateway.SortOrder, ChatPreference: gateway.ChatPreference,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Gateways returns safe configuration status using this Engine's injected
// credential store, catalog path, and provider state path.
func (e *Engine) Gateways(ctx context.Context) []Gateway {
	ctx = nonNilContext(ctx)
	selection := e.ActiveSelection(ctx)
	providerConfig := config.LoadProviderConfig(e.providerConfigPath)
	compiled, _ := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath})
	configuredDeployments := map[string]config.DeploymentConfig{}
	if compiled != nil {
		var persisted map[string]config.DeploymentConfig
		if providerConfig != nil {
			persisted = providerConfig.Deployments
		}
		configuredDeployments = buildDeployments(compiled, persisted, e.discoveryCredentialsFromConfig(ctx, compiled, providerConfig).Env())
	}

	definitions := e.GatewayDefinitions()
	out := make([]Gateway, 0, len(definitions))
	for _, definition := range definitions {
		if custom, ok := e.customGateway(definition.ID); ok {
			configured := custom.CredentialEnv == "" || e.hasCredential(ctx, []string{custom.CredentialEnv})
			definition.CredentialConfigured = configured
			definition.DeploymentConfigured = configured
			definition.Active = NormalizeProviderID(selection.Provider) == custom.ID
			out = append(out, definition)
			continue
		}
		providerSpec, _ := registry.SpecByProviderID(definition.ID)
		envVars := append([]string{definition.CredentialEnv}, providerSpec.CredentialEnvFallbacks...)
		envVars = append(envVars, providerSpec.CredentialAliases...)
		configured := e.hasCredential(ctx, envVars)
		_, deploymentConfigured := configuredDeployments[definition.DeploymentID]
		modelCount := 0
		if compiled != nil {
			modelCount = len(catalog.ModelEntriesForProvider(compiled, definition.ID))
		}
		definition.CredentialConfigured = configured
		definition.DeploymentConfigured = deploymentConfigured
		definition.ModelCount = modelCount
		definition.Active = NormalizeProviderID(selection.Provider) == NormalizeProviderID(definition.ID)
		definition.RegionLabel, definition.RegionRequired = gatewayRegionFromConfig(definition.ID, providerConfig)
		out = append(out, definition)
	}
	return out
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (e *Engine) hasCredential(ctx context.Context, envVars []string) bool {
	for _, envVar := range envVars {
		envVar = strings.TrimSpace(envVar)
		if envVar == "" {
			continue
		}
		secret, err := e.secretStore.Get(ctx, credentials.AccountForEnv(envVar))
		if err == nil && strings.TrimSpace(secret) != "" && !config.LooksLikePlaceholderSecret(secret) {
			return true
		}
	}
	return false
}

func maskedCredential(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return strings.Repeat("•", len(secret))
	}
	return strings.Repeat("•", 8) + secret[len(secret)-4:]
}

func (e *Engine) credentialValue(ctx context.Context, envVar string) (string, error) {
	secret, err := e.secretStore.Get(nonNilContext(ctx), credentials.AccountForEnv(envVar))
	if errors.Is(err, credentials.ErrNotFound) {
		return "", nil
	}
	return secret, err
}
