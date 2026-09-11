package config

import (
	"sort"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// DeploymentConfigFromEnv builds deployment credentials from catalog env_fallbacks and env values.
func DeploymentConfigFromEnv(dep catalog.Deployment, env map[string]string) DeploymentConfig {
	var dc DeploymentConfig
	for _, fb := range dep.EnvFallbacks {
		val := firstEnvFromMap(env, fb.Env)
		switch fb.Field {
		case "api_key":
			dc.APIKey = firstNonEmpty(dc.APIKey, val)
		case "base_url":
			dc.BaseURL = firstNonEmpty(dc.BaseURL, val)
		case "endpoint":
			dc.Endpoint = firstNonEmpty(dc.Endpoint, val)
		case "api_version":
			dc.APIVersion = firstNonEmpty(dc.APIVersion, val)
		case "project_id":
			dc.ProjectID = firstNonEmpty(dc.ProjectID, val)
		case "region":
			dc.Region = firstNonEmpty(dc.Region, val)
		case "token":
			dc.Token = firstNonEmpty(dc.Token, val)
		case "access_key_id":
			dc.AccessKeyID = firstNonEmpty(dc.AccessKeyID, val)
		case "secret_access_key":
			dc.SecretAccessKey = firstNonEmpty(dc.SecretAccessKey, val)
		case "session_token":
			dc.SessionToken = firstNonEmpty(dc.SessionToken, val)
		}
	}
	return dc
}

// DeploymentConfigured reports whether env supplies enough credentials for this deployment.
func DeploymentConfigured(deploymentID string, dep catalog.Deployment, dc DeploymentConfig) bool {
	if dep.ModelMappingsRequired && len(dc.ModelMappings) == 0 {
		return false
	}
	switch deploymentID {
	case "ollama-local":
		return dc.BaseURL != ""
	case "openai-azure":
		return dc.APIKey != "" && dc.Endpoint != ""
	case "xiaomi_mimo_token_plan-direct":
		return dc.APIKey != "" && dc.BaseURL != ""
	default:
		return deploymentHasLiveCredentials(deploymentID, dc)
	}
}

func deploymentHasLiveCredentials(deploymentID string, dc DeploymentConfig) bool {
	switch deploymentID {
	case "anthropic-bedrock":
		return dc.AccessKeyID != "" && dc.SecretAccessKey != "" && dc.Region != ""
	case "anthropic-vertex", "gemini-vertex":
		return dc.ProjectID != "" && dc.Region != "" &&
			(dc.Token != "" || dc.APIKey != "")
	default:
		return dc.APIKey != "" || dc.Token != "" ||
			dc.AccessKeyID != ""
	}
}

func firstEnvFromMap(env map[string]string, keys []string) string {
	for _, k := range keys {
		if v := env[k]; v != "" {
			return v
		}
	}
	return ""
}

// SyncProviderConfigFromCatalog merges catalog + env into provider.json deployments and routing.
func SyncProviderConfigFromCatalog(compiled *catalog.CompiledCatalog, env map[string]string) *ProviderConfig {
	cfg := LoadProviderConfig("")
	if cfg == nil {
		cfg = &ProviderConfig{}
	}
	if env == nil {
		env = map[string]string{}
	}
	deployments := map[string]DeploymentConfig{}
	if compiled != nil && compiled.Catalog != nil {
		ids := make([]string, 0, len(compiled.Catalog.Deployments))
		for id := range compiled.Catalog.Deployments {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			dep := compiled.Catalog.Deployments[id]
			dc := DeploymentConfigFromEnv(dep, env)
			if DeploymentConfigured(id, dep, dc) {
				deployments[id] = SanitizeDeploymentConfigForDisk(dc)
			}
		}
	}
	cfg.Deployments = deployments
	cfg.ConfigVersion = 2
	cfg.Routing = BuildRoutingPolicyFromDeployments(deployments)
	return cfg
}
