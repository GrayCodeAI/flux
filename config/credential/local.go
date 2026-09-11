package credential

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// CommitLocalCredential validates and probes a no-key provider (e.g. Ollama base URL).
func CommitLocalCredential(ctx context.Context, inference CredentialInference, value string) error {
	value = strings.TrimSpace(value)
	envKey := strings.TrimSpace(inference.EnvVar)
	if value == "" || envKey == "" {
		return fmt.Errorf("commit local credential: value and env var required")
	}
	spec, ok := registry.DefaultRegistry.Get(inference.ProviderID)
	if !ok || spec.RequiresKey {
		return fmt.Errorf("commit local credential: %s is not a local provider", inference.ProviderID)
	}
	value = normalizeLocalCredentialValue(inference.ProviderID, value)
	if err := validateLocalCredentialValue(envKey, value); err != nil {
		return err
	}
	return ProbeLocalCredential(ctx, envKey, value)
}

func normalizeLocalCredentialValue(providerID, value string) string {
	switch catalog.CanonicalProviderID(providerID) {
	case "ollama":
		value = NormalizeOllamaOpenAIBaseURL(value)
		if value == "" {
			return OllamaDefaultBaseURL
		}
		return value
	default:
		return value
	}
}

func validateLocalCredentialValue(envKey, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", envKey)
	}
	if strings.Contains(strings.ToUpper(envKey), "BASE_URL") {
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid base URL for %s", envKey)
		}
		return nil
	}
	if LooksLikePlaceholderSecret(value) {
		return fmt.Errorf("%s looks like a placeholder", envKey)
	}
	return nil
}

// ProbeLocalCredential verifies a local provider endpoint when configured.
func ProbeLocalCredential(ctx context.Context, envKey, value string) error {
	envKey = strings.TrimSpace(envKey)
	value = strings.TrimSpace(value)
	if envKey == "" || value == "" {
		return fmt.Errorf("local credential probe: env var and value required")
	}
	spec, ok := registry.DefaultRegistry.GetByEnv(envKey)
	if !ok {
		return nil
	}
	switch spec.ProbeKind {
	case registry.ProbeOllama:
		err := probeOllama(ctx, value)
		if err != nil {
			return FormatOllamaConnectError(err)
		}
		return nil
	default:
		return nil
	}
}
