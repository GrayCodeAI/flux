package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// SaveCredential validates, stores, and probes a provider credential. The
// secret is persisted before the probe so a transient network failure does not
// discard user input. The returned error states that persistence occurred.
func (e *Engine) SaveCredential(ctx context.Context, providerID, secret string) (CredentialStatus, error) {
	ctx = nonNilContext(ctx)
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return CredentialStatus{}, invalid("save_credential", "eyrie engine: provider id is required")
	}
	if gateway, ok := e.customGateway(providerID); ok {
		return e.saveCustomGatewayCredential(ctx, gateway, secret)
	}
	providerCfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInternal, Operation: "save_credential", Provider: providerID, Message: err.Error(), Cause: err}
	}
	inference, err := config.InferenceForProvider(providerID)
	if err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInvalidRequest, Operation: "save_credential", Provider: providerID, Message: err.Error(), Cause: err}
	}
	prepared, err := config.PrepareCredentialForSave(inference, secret)
	if err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInvalidRequest, Operation: "save_credential", Provider: providerID, Message: err.Error(), Cause: err}
	}
	envKey := strings.TrimSpace(inference.EnvVar)
	if envKey == "" {
		return CredentialStatus{}, invalid("save_credential", "eyrie engine: provider has no credential target")
	}
	if err := e.secretStore.Set(ctx, credentials.AccountForEnv(envKey), prepared); err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInternal, Operation: "save_credential", Provider: providerID, Message: "eyrie engine: could not save credential", Cause: err}
	}
	status := CredentialStatus{ProviderID: providerID, EnvVar: envKey, Configured: true}
	if spec, ok := registry.DefaultRegistry.Get(providerID); ok && !spec.RequiresKey {
		if err := config.ProbeLocalCredential(ctx, envKey, prepared); err != nil {
			return status, &Error{Code: ErrorProviderUnavailable, Operation: "probe_credential", Provider: providerID, Message: fmt.Sprintf("%v (value saved in keychain)", err), Cause: err}
		}
		status.Verified = true
		return status, nil
	}
	if err := config.ProbeCredentialWithProviderConfig(ctx, envKey, prepared, providerCfg); err != nil {
		return status, &Error{Code: ErrorAuthentication, Operation: "probe_credential", Provider: providerID, Message: fmt.Sprintf("%v (key saved in keychain)", err), Cause: err}
	}
	status.Verified = true
	return status, nil
}

// RemoveCredential deletes a provider credential from the configured store.
func (e *Engine) RemoveCredential(ctx context.Context, providerID string) error {
	ctx = nonNilContext(ctx)
	if gateway, ok := e.customGateway(providerID); ok {
		if gateway.CredentialEnv == "" {
			return nil
		}
		if err := e.secretStore.Delete(ctx, credentials.AccountForEnv(gateway.CredentialEnv)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
			return &Error{Code: ErrorInternal, Operation: "remove_credential", Provider: gateway.ID, Message: "eyrie engine: could not remove credential", Cause: err}
		}
		return nil
	}
	if _, err := config.InferenceForProvider(strings.TrimSpace(providerID)); err != nil {
		return &Error{Code: ErrorInvalidRequest, Operation: "remove_credential", Provider: providerID, Message: err.Error(), Cause: err}
	}
	for _, envKey := range e.CredentialEnvKeys(providerID) {
		if err := e.secretStore.Delete(ctx, credentials.AccountForEnv(envKey)); err != nil && !errors.Is(err, credentials.ErrNotFound) {
			return &Error{Code: ErrorInternal, Operation: "remove_credential", Provider: providerID, Message: "eyrie engine: could not remove credential", Cause: err}
		}
	}
	return nil
}

// CredentialStatus reports whether a provider credential is configured.
func (e *Engine) CredentialStatus(ctx context.Context, providerID string) (CredentialStatus, error) {
	ctx = nonNilContext(ctx)
	providerID = strings.TrimSpace(providerID)
	if gateway, ok := e.customGateway(providerID); ok {
		if gateway.CredentialEnv == "" {
			return CredentialStatus{ProviderID: gateway.ID, Configured: true}, nil
		}
		secret, err := e.credentialValue(ctx, gateway.CredentialEnv)
		if err != nil {
			return CredentialStatus{}, &Error{Code: ErrorInternal, Operation: "credential_status", Provider: gateway.ID, Message: "eyrie engine: could not read credential status", Cause: err}
		}
		configured := strings.TrimSpace(secret) != "" && !config.LooksLikePlaceholderSecret(secret)
		return credentialStatusWithEnvironment(CredentialStatus{
			ProviderID: gateway.ID, EnvVar: gateway.CredentialEnv,
			Configured: configured, Masked: maskedCredentialIf(configured, secret),
		}, secret, gateway.CredentialEnv), nil
	}
	inference, err := config.InferenceForProvider(providerID)
	if err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInvalidRequest, Operation: "credential_status", Provider: providerID, Message: err.Error(), Cause: err}
	}
	for _, envKey := range e.CredentialEnvKeys(providerID) {
		secret, err := e.credentialValue(ctx, envKey)
		if err != nil {
			return CredentialStatus{}, &Error{Code: ErrorInternal, Operation: "credential_status", Provider: providerID, Message: "eyrie engine: could not read credential status", Cause: err}
		}
		if strings.TrimSpace(secret) != "" && !config.LooksLikePlaceholderSecret(secret) {
			return credentialStatusWithEnvironment(CredentialStatus{
				ProviderID: providerID, EnvVar: inference.EnvVar,
				Configured: true, Masked: maskedCredential(secret),
			}, secret, e.CredentialEnvKeys(providerID)...), nil
		}
	}
	return credentialStatusWithEnvironment(
		CredentialStatus{ProviderID: providerID, EnvVar: inference.EnvVar},
		"",
		e.CredentialEnvKeys(providerID)...,
	), nil
}

// credentialStatusWithEnvironment adds safe process-environment metadata to a
// credential status. Secret values are compared only as fixed-size digests and
// are never retained in or returned from the status DTO.
func credentialStatusWithEnvironment(status CredentialStatus, storedSecret string, envVars ...string) CredentialStatus {
	for _, envVar := range envVars {
		envVar = strings.TrimSpace(envVar)
		environmentSecret, ok := os.LookupEnv(envVar)
		environmentSecret = strings.TrimSpace(environmentSecret)
		if !ok || environmentSecret == "" {
			continue
		}
		status.EnvironmentVariable = envVar
		storedSecret = strings.TrimSpace(storedSecret)
		if storedSecret != "" {
			status.EnvironmentConflict = sha256.Sum256([]byte(environmentSecret)) != sha256.Sum256([]byte(storedSecret))
		}
		break
	}
	return status
}

func (e *Engine) saveCustomGatewayCredential(ctx context.Context, gateway CustomGateway, secret string) (CredentialStatus, error) {
	if gateway.CredentialEnv == "" {
		return CredentialStatus{ProviderID: gateway.ID, Configured: true, Verified: true}, nil
	}
	secret = strings.TrimSpace(secret)
	if err := config.ValidateCredentialSecret(gateway.CredentialEnv, secret); err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInvalidRequest, Operation: "save_credential", Provider: gateway.ID, Message: err.Error(), Cause: err}
	}
	if err := e.secretStore.Set(ctx, credentials.AccountForEnv(gateway.CredentialEnv), secret); err != nil {
		return CredentialStatus{}, &Error{Code: ErrorInternal, Operation: "save_credential", Provider: gateway.ID, Message: "eyrie engine: could not save credential", Cause: err}
	}
	status := CredentialStatus{ProviderID: gateway.ID, EnvVar: gateway.CredentialEnv, Configured: true}
	if err := e.probeCustomGateway(ctx, gateway, secret); err != nil {
		code := ErrorAuthentication
		var typed *Error
		if errors.As(err, &typed) && typed.Code != "" {
			code = typed.Code
		}
		return status, &Error{Code: code, Operation: "probe_credential", Provider: gateway.ID, Message: fmt.Sprintf("%v (key saved in keychain)", err), Cause: err}
	}
	status.Verified = true
	return status, nil
}

func maskedCredentialIf(configured bool, secret string) string {
	if !configured {
		return ""
	}
	return maskedCredential(secret)
}
