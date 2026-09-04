package credential

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// ValidateCredentialBeforeSave checks format/placeholders without a live API probe.
func ValidateCredentialBeforeSave(inference CredentialInference, secret string) error {
	secret = strings.TrimSpace(secret)
	envKey := strings.TrimSpace(inference.EnvVar)
	if secret == "" || envKey == "" {
		return fmt.Errorf("credential: key and env var required")
	}
	if spec, ok := registry.DefaultRegistry.Get(inference.ProviderID); ok && !spec.RequiresKey {
		return validateLocalCredentialValue(envKey, normalizeLocalCredentialValue(inference.ProviderID, secret))
	}
	if err := ValidateKeyFormat(secret); err != nil {
		return err
	}
	return ValidateCredentialSecret(envKey, secret)
}

// PrepareCredentialForSave returns the normalized secret to persist (e.g. Ollama base URL).
func PrepareCredentialForSave(inference CredentialInference, secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if err := ValidateCredentialBeforeSave(inference, secret); err != nil {
		return "", err
	}
	if spec, ok := registry.DefaultRegistry.Get(inference.ProviderID); ok && !spec.RequiresKey {
		return normalizeLocalCredentialValue(inference.ProviderID, secret), nil
	}
	return secret, nil
}

// CommitCredential validates and probes a credential before persistence (host calls before SetCredential).
func CommitCredential(ctx context.Context, inference CredentialInference, secret string) error {
	if err := ValidateCredentialBeforeSave(inference, secret); err != nil {
		return err
	}
	envKey := strings.TrimSpace(inference.EnvVar)
	if spec, ok := registry.DefaultRegistry.Get(inference.ProviderID); ok && !spec.RequiresKey {
		value := normalizeLocalCredentialValue(inference.ProviderID, secret)
		return ProbeLocalCredential(ctx, envKey, value)
	}
	return ProbeCredential(ctx, envKey, secret)
}

// ProviderIDForEnv maps an env var to registry provider id.
func ProviderIDForEnv(envKey string) string {
	if spec, ok := registry.DefaultRegistry.GetByEnv(strings.TrimSpace(envKey)); ok {
		return spec.ProviderID
	}
	return ""
}
