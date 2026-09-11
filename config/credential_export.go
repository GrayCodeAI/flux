package config

import (
	"context"

	"github.com/GrayCodeAI/eyrie/config/credential"
)

func init() {
	credential.SetMimoProbeConfigLoader(mimoProbeConfigFromProvider)
}

// Credential types and helpers — implemented in config/credential.

type (
	CredentialProviderOption = credential.CredentialProviderOption
	CredentialResolveResult  = credential.CredentialResolveResult
	CredentialInference      = credential.CredentialInference
)

// FormatOllamaConnectError turns probe/network failures into actionable setup hints.
func FormatOllamaConnectError(err error) error {
	return credential.FormatOllamaConnectError(err)
}

// ValidateKeyFormat checks a pasted secret before any provider is chosen.
func ValidateKeyFormat(secret string) error {
	return credential.ValidateKeyFormat(secret)
}

// ListCredentialProviders returns all registry providers for setup UIs.
func ListCredentialProviders() []CredentialProviderOption {
	return credential.ListCredentialProviders()
}

// ResolveCredential validates format and lists registered providers (gateway must be chosen first).
func ResolveCredential(ctx context.Context, secret string) CredentialResolveResult {
	return credential.ResolveCredential(ctx, secret)
}

// InferenceForProvider returns save metadata for a gateway selected in setup UI.
func InferenceForProvider(providerID string) (CredentialInference, error) {
	return credential.InferenceForProvider(providerID)
}

// InferenceFromOption converts a provider picker row to persistence metadata.
func InferenceFromOption(opt CredentialProviderOption) CredentialInference {
	return credential.InferenceFromOption(opt)
}

// LocalCredentialInference returns setup metadata for no-key providers.
func LocalCredentialInference(providerID string) (CredentialInference, error) {
	return credential.LocalCredentialInference(providerID)
}

// InferCredentialsFromAPIKey is deprecated; use InferenceForProvider after gateway selection.
func InferCredentialsFromAPIKey(ctx context.Context, secret string) []CredentialInference {
	return credential.InferCredentialsFromAPIKey(ctx, secret)
}

// ValidateCredentialBeforeSave checks format without a live API probe.
func ValidateCredentialBeforeSave(inference CredentialInference, secret string) error {
	return credential.ValidateCredentialBeforeSave(inference, secret)
}

// PrepareCredentialForSave validates and returns the normalized value to persist.
func PrepareCredentialForSave(inference CredentialInference, secret string) (string, error) {
	return credential.PrepareCredentialForSave(inference, secret)
}

// CommitCredential validates and probes a credential before persistence.
func CommitCredential(ctx context.Context, inference CredentialInference, secret string) error {
	return credential.CommitCredential(ctx, inference, secret)
}

// CommitLocalCredential validates and probes a no-key provider.
func CommitLocalCredential(ctx context.Context, inference CredentialInference, value string) error {
	return credential.CommitLocalCredential(ctx, inference, value)
}

// ProbeCredential verifies a key against the provider API when a probe is configured.
func ProbeCredential(ctx context.Context, envKey, secret string) error {
	return credential.ProbeCredentialWithMimo(ctx, envKey, secret, mimoProbeConfigFromProvider())
}

// ProbeCredentialWithProviderConfig verifies a credential using only the
// supplied provider state for region/base routing. Host-facing callers should
// prefer this over ProbeCredential so an Engine never consults the
// process-default provider.json path while probing a scoped credential.
func ProbeCredentialWithProviderConfig(ctx context.Context, envKey, secret string, cfg *ProviderConfig) error {
	return credential.ProbeCredentialWithMimoStrict(ctx, envKey, secret, mimoProbeConfig(cfg))
}

func mimoProbeConfigFromProvider() credential.MimoProbeConfig {
	return mimoProbeConfig(LoadProviderConfig(""))
}

func mimoProbeConfig(cfg *ProviderConfig) credential.MimoProbeConfig {
	if cfg == nil {
		return credential.MimoProbeConfig{}
	}
	region := XiaomiTokenPlanRegionFromConfig(cfg)
	base, _ := ResolveXiaomiOpenAIBase(ProviderXiaomiMimoTokenPlan, cfg)
	return credential.MimoProbeConfig{
		TokenPlanRegion: string(region),
		TokenPlanBase:   base,
		PaygBase:        cfg.XiaomiMimoPaygBaseURL,
	}
}

// ProbeLocalCredential verifies a local provider endpoint when configured.
func ProbeLocalCredential(ctx context.Context, envKey, value string) error {
	return credential.ProbeLocalCredential(ctx, envKey, value)
}

// ProviderIDForEnv maps an env var to registry provider id.
func ProviderIDForEnv(envKey string) string {
	return credential.ProviderIDForEnv(envKey)
}

// LooksLikePlaceholderSecret detects obvious placeholder API keys.
func LooksLikePlaceholderSecret(secret string) bool {
	return credential.LooksLikePlaceholderSecret(secret)
}

// ValidateCredentialSecret validates env-specific secret shape.
func ValidateCredentialSecret(envKey, secret string) error {
	return credential.ValidateCredentialSecret(envKey, secret)
}
