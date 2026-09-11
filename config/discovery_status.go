package config

import (
	"context"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// DiscoveryEnvMap returns merged credential env (keychain + provider.json routing) for status UI.
func DiscoveryEnvMap(ctx context.Context) map[string]string {
	return DiscoveryCredentials(ctx).Env()
}

// HasAnyConfiguredDeployment reports whether catalog env + keychain satisfy any deployment.
func HasAnyConfiguredDeployment(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	env := DiscoveryEnvMap(ctx)
	compiled, err := catalog.LoadCatalogForDiscovery(ctx)
	if err != nil || compiled == nil || compiled.Catalog == nil {
		return anyNonemptyCredentialEnv(env)
	}
	for id, dep := range compiled.Catalog.Deployments {
		dc := DeploymentConfigFromEnv(dep, env)
		if DeploymentConfigured(id, dep, dc) {
			return true
		}
	}
	return false
}

func anyNonemptyCredentialEnv(env map[string]string) bool {
	for _, v := range env {
		if !LooksLikePlaceholderSecret(v) {
			return true
		}
	}
	return false
}
