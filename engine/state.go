package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

type importedCredential struct {
	account  string
	previous string
	hadValue bool
}

func (e *Engine) importProviderConfigSecrets(ctx context.Context, cfg config.ProviderConfig) ([]importedCredential, error) {
	var writes []importedCredential
	secrets, err := config.ProviderConfigSecrets(cfg)
	if err != nil {
		return nil, err
	}
	envKeys := make([]string, 0, len(secrets))
	for envKey := range secrets {
		envKeys = append(envKeys, envKey)
	}
	sort.Strings(envKeys)
	for _, envKey := range envKeys {
		secret := secrets[envKey]
		account := credentials.AccountForEnv(envKey)
		previous, err := e.secretStore.Get(ctx, account)
		if err != nil && !errors.Is(err, credentials.ErrNotFound) {
			return nil, errors.Join(err, e.rollbackImportedCredentials(ctx, writes))
		}
		if strings.TrimSpace(previous) != "" && !config.LooksLikePlaceholderSecret(previous) {
			continue
		}
		write := importedCredential{account: account, previous: previous, hadValue: strings.TrimSpace(previous) != ""}
		if err := e.secretStore.Set(ctx, account, secret); err != nil {
			return nil, errors.Join(err, e.rollbackImportedCredentials(ctx, writes))
		}
		writes = append(writes, write)
	}
	return writes, nil
}

func (e *Engine) rollbackImportedCredentials(ctx context.Context, writes []importedCredential) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(nonNilContext(ctx)), 5*time.Second)
	defer cancel()
	var rollbackErrors []error
	for i := len(writes) - 1; i >= 0; i-- {
		var err error
		if writes[i].hadValue {
			err = e.secretStore.Set(cleanupCtx, writes[i].account, writes[i].previous)
		} else {
			err = e.secretStore.Delete(cleanupCtx, writes[i].account)
		}
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback credential account %q: %w", writes[i].account, err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func (e *Engine) saveProviderConfig(ctx context.Context, cfg *config.ProviderConfig) error {
	if cfg == nil {
		return nil
	}
	writes, err := e.importProviderConfigSecrets(nonNilContext(ctx), *cfg)
	if err != nil {
		return err
	}
	sanitized := config.SanitizeProviderConfigForDisk(*cfg)
	if err := writeProviderConfigAtomic(e.providerConfigPath, &sanitized); err != nil {
		return errors.Join(err, e.rollbackImportedCredentials(ctx, writes))
	}
	return nil
}

func (e *Engine) loadRuntimeState(ctx context.Context) (*catalog.CompiledCatalog, *config.ProviderConfig, error) {
	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
	if err != nil {
		return nil, nil, &Error{Code: ErrorCatalogUnavailable, Operation: "load_state", Message: err.Error(), Cause: err}
	}
	persisted, err := e.loadProviderConfigStrict()
	if err != nil {
		return nil, nil, &Error{Code: ErrorInternal, Operation: "load_state", Message: err.Error(), Cause: err}
	}
	cfg := *persisted
	cfg.Deployments = buildDeployments(compiled, persisted.Deployments, e.discoveryCredentialsFromConfig(ctx, compiled, persisted).Env())
	if cfg.Routing == nil {
		cfg.Routing = config.BuildRoutingPolicyFromDeployments(cfg.Deployments)
	}
	if len(cfg.Deployments) > 0 && cfg.ConfigVersion < 2 {
		cfg.ConfigVersion = 2
	}
	return compiled, &cfg, nil
}

func (e *Engine) credentialEnv(ctx context.Context, compiled *catalog.CompiledCatalog) map[string]string {
	out := make(map[string]string)
	if compiled == nil {
		return out
	}
	for _, envKey := range catalog.DiscoveryEnvKeysFromCatalog(compiled) {
		secret, err := e.secretStore.Get(ctx, credentials.AccountForEnv(envKey))
		if err == nil && strings.TrimSpace(secret) != "" && !config.LooksLikePlaceholderSecret(secret) {
			out[envKey] = secret
		}
	}
	for _, spec := range registry.All() {
		primary := strings.TrimSpace(spec.CredentialEnv)
		if primary == "" || strings.TrimSpace(out[primary]) != "" {
			continue
		}
		for _, alias := range registry.CredentialAliases(spec.ProviderID) {
			secret, err := e.secretStore.Get(ctx, credentials.AccountForEnv(alias))
			if err == nil && strings.TrimSpace(secret) != "" && !config.LooksLikePlaceholderSecret(secret) {
				out[primary] = secret
				break
			}
		}
	}
	return out
}

func (e *Engine) discoveryCredentials(ctx context.Context, compiled *catalog.CompiledCatalog) (catalog.Credentials, error) {
	cfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return catalog.Credentials{}, err
	}
	return e.discoveryCredentialsFromConfig(ctx, compiled, cfg), nil
}

func (e *Engine) discoveryCredentialsFromConfig(ctx context.Context, compiled *catalog.CompiledCatalog, cfg *config.ProviderConfig) catalog.Credentials {
	return config.DiscoveryCredentialsFromState(
		e.credentialEnv(ctx, compiled),
		cfg,
	)
}

func buildDeployments(compiled *catalog.CompiledCatalog, persisted map[string]config.DeploymentConfig, env map[string]string) map[string]config.DeploymentConfig {
	out := make(map[string]config.DeploymentConfig)
	if compiled == nil || compiled.Catalog == nil {
		for id, deployment := range persisted {
			out[id] = deployment
		}
		return out
	}
	for id, deployment := range persisted {
		if _, known := compiled.Catalog.Deployments[id]; !known {
			out[id] = deployment
		}
	}
	for id, deployment := range compiled.Catalog.Deployments {
		derived := config.DeploymentConfigFromEnv(deployment, env)
		if existing, ok := persisted[id]; ok {
			derived = mergeDeployment(existing, derived)
		}
		if config.DeploymentConfigured(id, deployment, derived) {
			out[id] = derived
		}
	}
	return out
}

// mergeDeployment keeps only non-secret routing fields from disk while filling
// credential fields from the injected store. Secret-bearing provider state is
// never accepted as a runtime credential source.
func mergeDeployment(persisted, derived config.DeploymentConfig) config.DeploymentConfig {
	out := config.SanitizeDeploymentConfigForDisk(persisted)
	if derived.APIKey != "" {
		out.APIKey = derived.APIKey
	}
	if derived.BaseURL != "" {
		out.BaseURL = derived.BaseURL
	}
	if derived.Endpoint != "" {
		out.Endpoint = derived.Endpoint
	}
	if derived.APIVersion != "" {
		out.APIVersion = derived.APIVersion
	}
	if derived.ProjectID != "" {
		out.ProjectID = derived.ProjectID
	}
	if derived.Region != "" {
		out.Region = derived.Region
	}
	if derived.Token != "" {
		out.Token = derived.Token
	}
	if derived.AccessKeyID != "" {
		out.AccessKeyID = derived.AccessKeyID
	}
	if derived.SecretAccessKey != "" {
		out.SecretAccessKey = derived.SecretAccessKey
	}
	if derived.SessionToken != "" {
		out.SessionToken = derived.SessionToken
	}
	if len(derived.ModelMappings) > 0 {
		out.ModelMappings = derived.ModelMappings
	}
	return out
}
