package catalog

import (
	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// extraDeploymentEnvFallbacks are env fallbacks for deployments that have no
// corresponding ProviderSpec (proxy/gateway deployments like Bedrock, Vertex, Azure).
// All provider-spec-derived deployments come from registry.DeploymentEnvFallbacks.
var extraDeploymentEnvFallbacks = map[string][]EnvFallback{
	"anthropic-bedrock": {
		{Field: "access_key_id", Env: []string{"AWS_ACCESS_KEY_ID"}},
		{Field: "secret_access_key", Env: []string{"AWS_SECRET_ACCESS_KEY"}},
		{Field: "session_token", Env: []string{"AWS_SESSION_TOKEN"}},
		{Field: "region", Env: []string{"AWS_REGION", "AWS_DEFAULT_REGION"}},
	},
	"anthropic-vertex": {
		{Field: "project_id", Env: []string{"VERTEX_PROJECT_ID"}},
		{Field: "region", Env: []string{"VERTEX_REGION"}},
		{Field: "token", Env: []string{"VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN"}},
	},
	"openai-azure": {
		{Field: "api_key", Env: []string{"AZURE_OPENAI_API_KEY", "OPENAI_API_KEY"}},
		{Field: "endpoint", Env: []string{"AZURE_OPENAI_ENDPOINT"}},
		{Field: "api_version", Env: []string{"AZURE_OPENAI_API_VERSION"}},
	},
	"gemini-vertex": {
		{Field: "project_id", Env: []string{"VERTEX_PROJECT_ID"}},
		{Field: "region", Env: []string{"VERTEX_REGION"}},
		{Field: "token", Env: []string{"VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN"}},
	},
}

// DefaultDeploymentEnvFallbacks seeds env_fallbacks per deployment until the published catalog includes them.
var DefaultDeploymentEnvFallbacks = func() map[string][]EnvFallback {
	result := make(map[string][]EnvFallback, len(registry.DeploymentEnvFallbacks())+len(extraDeploymentEnvFallbacks))
	for id, fbs := range registry.DeploymentEnvFallbacks() {
		var converted []EnvFallback
		for _, fb := range fbs {
			converted = append(converted, EnvFallback{Field: fb.Field, Env: fb.Env})
		}
		result[id] = converted
	}
	for id, fbs := range extraDeploymentEnvFallbacks {
		if _, ok := result[id]; !ok {
			result[id] = fbs
		}
	}
	return result
}()

// EnsureDeploymentEnvFallbacks fills missing env_fallbacks from the embedded seed.
// Published catalogs with env_fallbacks set are left unchanged.
func EnsureDeploymentEnvFallbacks(c *Catalog) {
	if c == nil || c.Deployments == nil {
		return
	}
	for id, dep := range c.Deployments {
		if len(dep.EnvFallbacks) > 0 {
			continue
		}
		if fb, ok := DefaultDeploymentEnvFallbacks[id]; ok {
			dep.EnvFallbacks = fb
			c.Deployments[id] = dep
		}
	}
}

// DiscoveryEnvKeysFromCatalog returns env var names needed for catalog discovery
// (API keys, base URLs) from deployment env_fallbacks in the compiled catalog.
func DiscoveryEnvKeysFromCatalog(compiled *CompiledCatalog) []string {
	if compiled == nil || compiled.Catalog == nil {
		return nil
	}
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		keys = append(keys, k)
	}
	for _, dep := range compiled.Catalog.Deployments {
		for _, fb := range dep.EnvFallbacks {
			for _, env := range fb.Env {
				add(env)
			}
		}
	}
	return keys
}

// EnvVarsForDeployment returns env var names for a deployment ID from the seed catalog.
func EnvVarsForDeployment(deploymentID string) []string {
	fb, ok := DefaultDeploymentEnvFallbacks[deploymentID]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range fb {
		for _, env := range f.Env {
			if env != "" && !seen[env] {
				seen[env] = true
				out = append(out, env)
			}
		}
	}
	return out
}
