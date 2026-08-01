package registry

import (
	"sort"
	"strings"
)

// CredentialSpec is paste-key / local setup metadata derived from ProviderSpec.
type CredentialSpec struct {
	ProviderID   string
	DisplayName  string
	DeploymentID string
	EnvVar       string
	ProbeKind    string
	ProbeBaseURL string
	RequiresKey  bool
	SortOrder    int
}

// SpecByProviderID finds a provider spec by id (accepts registry ids and catalog aliases like google→gemini).
func SpecByProviderID(id string) (ProviderSpec, bool) {
	return DefaultRegistry.Get(id)
}

func registryIDFromCatalogProvider(id string) string {
	switch strings.TrimSpace(id) {
	case "google":
		return "gemini"
	case "xai":
		return "grok"
	default:
		return id
	}
}

// SpecByEnvVar finds spec by primary credential env var.
func SpecByEnvVar(env string) (ProviderSpec, bool) {
	return DefaultRegistry.GetByEnv(env)
}

// SpecByDeploymentID finds a provider spec by its deployment id.
func SpecByDeploymentID(deploymentID string) (ProviderSpec, bool) {
	for _, spec := range DefaultRegistry.All() {
		if spec.DeploymentID == deploymentID {
			return spec, true
		}
	}
	return ProviderSpec{}, false
}

// DisplayName returns the UI label for a provider id.
func DisplayName(providerID string) string {
	if s, ok := SpecByProviderID(providerID); ok {
		return s.DisplayName
	}
	return providerID
}

// ChatProviderPreferenceOrder returns provider ids ordered by chat/runtime preference.
func ChatProviderPreferenceOrder() []string {
	specs := DefaultRegistry.All()
	sort.Slice(specs, func(i, j int) bool {
		left := specs[i].ChatPreference
		right := specs[j].ChatPreference
		if left == 0 {
			left = specs[i].SortOrder + 10_000
		}
		if right == 0 {
			right = specs[j].SortOrder + 10_000
		}
		if left != right {
			return left < right
		}
		return specs[i].ProviderID < specs[j].ProviderID
	})
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.ProviderID != "" {
			out = append(out, spec.ProviderID)
		}
	}
	return out
}

// RuntimeProfileKey returns the config runtime-profile key for a provider.
func RuntimeProfileKey(providerID string) string {
	if spec, ok := SpecByProviderID(providerID); ok {
		return strings.TrimSpace(spec.RuntimeProfileKey)
	}
	return ""
}

// CredentialAliases returns compatibility env var names for providerID.
func CredentialAliases(providerID string) []string {
	spec, ok := SpecByProviderID(providerID)
	if !ok || len(spec.CredentialAliases) == 0 {
		return nil
	}
	out := make([]string, 0, len(spec.CredentialAliases))
	for _, env := range spec.CredentialAliases {
		if trimmed := strings.TrimSpace(env); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// CredentialEnvPreparedProviders returns providers that need config-derived env before discovery.
func CredentialEnvPreparedProviders() []string {
	var out []string
	for _, spec := range DefaultRegistry.All() {
		if spec.PrepareCredentialEnv && spec.ProviderID != "" {
			out = append(out, spec.ProviderID)
		}
	}
	sort.Strings(out)
	return out
}

// CredentialRegistry derives credential rows from provider specs.
func CredentialRegistry() []CredentialSpec {
	specs := DefaultRegistry.All()
	out := make([]CredentialSpec, len(specs))
	for i, s := range specs {
		out[i] = CredentialSpec{
			ProviderID:   s.ProviderID,
			DisplayName:  s.DisplayName,
			DeploymentID: s.DeploymentID,
			EnvVar:       s.CredentialEnv,
			ProbeKind:    string(s.ProbeKind),
			ProbeBaseURL: s.ProbeBaseURL,
			RequiresKey:  s.RequiresKey,
			SortOrder:    s.SortOrder,
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}

// DeploymentEnvFallbacks derives seed env_fallbacks for registered deployments.
func DeploymentEnvFallbacks() map[string][]EnvFallback {
	return DefaultRegistry.DeploymentEnvFallbacks()
}

// LiveFetcherKeys returns live catalog provider keys that have fetchers.
func LiveFetcherKeys() []string {
	return DefaultRegistry.LiveFetcherKeys()
}

// LiveCatalogKeyForFetcher maps fetcher registry key to legacy catalog Providers map key.
func LiveCatalogKeyForFetcher(fetcherKey string) string {
	for _, s := range DefaultRegistry.All() {
		if s.LiveFetcherKey == fetcherKey {
			return s.LiveCatalogKey
		}
	}
	return fetcherKey
}

// CredentialPresent reports whether env satisfies this provider's discovery requirements.
func CredentialPresent(spec ProviderSpec, env map[string]string) bool {
	if spec.RequiresKey {
		if strings.TrimSpace(env[spec.CredentialEnv]) != "" {
			return true
		}
		for _, key := range spec.CredentialEnvFallbacks {
			if strings.TrimSpace(env[key]) != "" {
				return true
			}
		}
		for _, key := range spec.CredentialAliases {
			if strings.TrimSpace(env[key]) != "" {
				return true
			}
		}
		return false
	}
	return strings.TrimSpace(env[spec.CredentialEnv]) != ""
}

// ScopedProviderEnv returns only credential aliases and routing metadata used
// by one provider. Provider-specific fetchers must never receive unrelated
// credentials from the host's full secret snapshot.
func ScopedProviderEnv(spec ProviderSpec, env map[string]string) map[string]string {
	allowed := map[string]bool{}
	add := func(keys ...string) {
		for _, key := range keys {
			if key = strings.TrimSpace(key); key != "" {
				allowed[key] = true
			}
		}
	}
	add(spec.CredentialEnv)
	add(spec.CredentialEnvFallbacks...)
	add(spec.CredentialAliases...)
	add(spec.BaseURLEnv...)
	switch spec.ProviderID {
	case "azure":
		add("AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_API_VERSION", "AZURE_OPENAI_DEPLOYMENT")
	case "bedrock":
		add("AWS_REGION", "AWS_DEFAULT_REGION")
	case "vertex":
		add("VERTEX_PROJECT_ID", "VERTEX_REGION")
	case "xiaomi_mimo_token_plan":
		add("XIAOMI_MIMO_TOKEN_PLAN_REGION", "XIAOMI_MIMO_PLATFORM_MODELS_URL")
	case "xiaomi_mimo_payg":
		add("XIAOMI_MIMO_PLATFORM_MODELS_URL")
	case "zai_payg":
		add("ZAI_REGION")
	case "zai_coding":
		add("ZAI_CODING_REGION")
	}
	out := make(map[string]string, len(allowed))
	for key := range allowed {
		if value := strings.TrimSpace(env[key]); value != "" {
			out[key] = value
		}
	}
	if strings.TrimSpace(out[spec.CredentialEnv]) == "" {
		for _, alias := range spec.CredentialAliases {
			if value := strings.TrimSpace(out[alias]); value != "" {
				out[spec.CredentialEnv] = value
				break
			}
		}
	}
	return out
}

// SpecForLiveFetcher returns the provider spec for a live fetcher key.
func SpecForLiveFetcher(fetcherKey string) (ProviderSpec, bool) {
	return DefaultRegistry.GetForLiveFetcher(fetcherKey)
}
