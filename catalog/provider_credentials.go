package catalog

import "strings"

// ProviderIDsFromCompiled lists provider IDs from catalog providers and deployments.
func ProviderIDsFromCompiled(compiled *CompiledCatalog) []string {
	if compiled == nil || compiled.Catalog == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = canonicalProviderID(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for id := range compiled.Catalog.Providers {
		add(id)
	}
	for _, dep := range compiled.Catalog.Deployments {
		add(dep.ProviderID)
	}
	return out
}

// PrimaryAPIKeyEnvForProvider returns the preferred API key env var for a provider.
func PrimaryAPIKeyEnvForProvider(compiled *CompiledCatalog, providerID string) string {
	providerID = canonicalProviderID(providerID)
	if providerID == "" || compiled == nil || compiled.Catalog == nil {
		return ""
	}
	preferred := []string{providerID + "-direct", providerID}
	for _, depID := range preferred {
		if env := apiKeyEnvFromDeployment(compiled.Catalog.Deployments[depID]); env != "" {
			return env
		}
	}
	for _, dep := range compiled.Catalog.Deployments {
		if canonicalProviderID(dep.ProviderID) != providerID {
			continue
		}
		if env := apiKeyEnvFromDeployment(dep); env != "" {
			return env
		}
	}
	return ""
}

func apiKeyEnvFromDeployment(dep Deployment) string {
	for _, fb := range dep.EnvFallbacks {
		if fb.Field == "api_key" && len(fb.Env) > 0 {
			return fb.Env[0]
		}
	}
	return ""
}

// CredentialStatusForProvider reports whether a provider needs an API key (local vs required).
// For set/empty status use hawk config.EnvKeyStatus or credentials.HasSecret — catalog does not read env.
func CredentialStatusForProvider(compiled *CompiledCatalog, providerID string) string {
	providerID = canonicalProviderID(providerID)
	if providerID == "" {
		return "empty"
	}
	envs := apiKeyEnvsForProvider(compiled, providerID)
	if len(envs) == 0 {
		return "local"
	}
	return "required"
}

// APIKeyEnvsForProvider lists API key env var names for a provider from deployment env_fallbacks.
func APIKeyEnvsForProvider(compiled *CompiledCatalog, providerID string) []string {
	return apiKeyEnvsForProvider(compiled, canonicalProviderID(providerID))
}

// PrimaryAPIKeyEnvForDeployment returns the primary API key env var for a deployment ID.
func PrimaryAPIKeyEnvForDeployment(compiled *CompiledCatalog, deploymentID string) string {
	if compiled != nil && compiled.Catalog != nil {
		if dep, ok := compiled.Catalog.Deployments[deploymentID]; ok {
			if env := apiKeyEnvFromDeployment(dep); env != "" {
				return env
			}
		}
	}
	for _, env := range EnvVarsForDeployment(deploymentID) {
		if strings.Contains(env, "API_KEY") || strings.Contains(env, "TOKEN") {
			return env
		}
	}
	return ""
}

func apiKeyEnvsForProvider(compiled *CompiledCatalog, providerID string) []string {
	if compiled == nil || compiled.Catalog == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, dep := range compiled.Catalog.Deployments {
		if canonicalProviderID(dep.ProviderID) != providerID {
			continue
		}
		for _, fb := range dep.EnvFallbacks {
			if fb.Field != "api_key" {
				continue
			}
			for _, env := range fb.Env {
				if env != "" && !seen[env] {
					seen[env] = true
					out = append(out, env)
				}
			}
		}
	}
	return out
}
