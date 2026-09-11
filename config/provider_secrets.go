package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// providerCredentialField maps one ProviderConfig field to the canonical
// secret-store environment variable for its provider. The env var must match
// the provider's registry CredentialEnv so that setup, discovery, and the
// credential store agree on a single key per provider.
type providerCredentialField struct {
	label string // json field name, used in diagnostics
	env   string // canonical secret-store environment variable
	value func(*ProviderConfig) string
	clear func(*ProviderConfig)
}

// providerCredentialFields is the single registry of typed credential fields
// on ProviderConfig. Sanitization, secret detection, and store import all
// derive from this table.
var providerCredentialFields = []providerCredentialField{
	{
		label: "anthropic_api_key", env: "ANTHROPIC_API_KEY",
		value: func(c *ProviderConfig) string { return c.AnthropicAPIKey },
		clear: func(c *ProviderConfig) { c.AnthropicAPIKey = "" },
	},
	{
		label: "grok_api_key", env: "XAI_API_KEY",
		value: func(c *ProviderConfig) string { return c.GrokAPIKey },
		clear: func(c *ProviderConfig) { c.GrokAPIKey = "" },
	},
	{
		label: "xai_api_key", env: "XAI_API_KEY",
		value: func(c *ProviderConfig) string { return c.XAIAPIKey },
		clear: func(c *ProviderConfig) { c.XAIAPIKey = "" },
	},
	{
		label: "openai_api_key", env: "OPENAI_API_KEY",
		value: func(c *ProviderConfig) string { return c.OpenAIAPIKey },
		clear: func(c *ProviderConfig) { c.OpenAIAPIKey = "" },
	},
	{
		label: "canopywave_api_key", env: "CANOPYWAVE_API_KEY",
		value: func(c *ProviderConfig) string { return c.CanopyWaveAPIKey },
		clear: func(c *ProviderConfig) { c.CanopyWaveAPIKey = "" },
	},
	{
		label: "deepseek_api_key", env: "DEEPSEEK_API_KEY",
		value: func(c *ProviderConfig) string { return c.DeepSeekAPIKey },
		clear: func(c *ProviderConfig) { c.DeepSeekAPIKey = "" },
	},
	{
		label: "zai_api_key", env: "ZAI_API_KEY",
		value: func(c *ProviderConfig) string { return c.ZAIAPIKey },
		clear: func(c *ProviderConfig) { c.ZAIAPIKey = "" },
	},
	{
		label: "zai_coding_api_key", env: "ZAI_CODING_API_KEY",
		value: func(c *ProviderConfig) string { return c.ZAICodingAPIKey },
		clear: func(c *ProviderConfig) { c.ZAICodingAPIKey = "" },
	},
	{
		label: "openrouter_api_key", env: "OPENROUTER_API_KEY",
		value: func(c *ProviderConfig) string { return c.OpenRouterAPIKey },
		clear: func(c *ProviderConfig) { c.OpenRouterAPIKey = "" },
	},
	{
		label: "gemini_api_key", env: "GEMINI_API_KEY",
		value: func(c *ProviderConfig) string { return c.GeminiAPIKey },
		clear: func(c *ProviderConfig) { c.GeminiAPIKey = "" },
	},
	{
		label: "opencodego_api_key", env: "OPENCODEGO_API_KEY",
		value: func(c *ProviderConfig) string { return c.OpenCodeGoAPIKey },
		clear: func(c *ProviderConfig) { c.OpenCodeGoAPIKey = "" },
	},
	{
		label: "moonshot_api_key", env: "MOONSHOT_API_KEY",
		value: func(c *ProviderConfig) string { return c.MoonshotAPIKey },
		clear: func(c *ProviderConfig) { c.MoonshotAPIKey = "" },
	},
	{
		label: "xiaomi_mimo_payg_api_key", env: "XIAOMI_MIMO_PAYG_API_KEY",
		value: func(c *ProviderConfig) string { return c.XiaomiMimoPaygAPIKey },
		clear: func(c *ProviderConfig) { c.XiaomiMimoPaygAPIKey = "" },
	},
	{
		label: "xiaomi_mimo_token_plan_api_key", env: "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
		value: func(c *ProviderConfig) string { return c.XiaomiMimoTokenPlanAPIKey },
		clear: func(c *ProviderConfig) { c.XiaomiMimoTokenPlanAPIKey = "" },
	},
	{
		label: "minimax_token_plan_api_key", env: "MINIMAX_TOKEN_PLAN_API_KEY",
		value: func(c *ProviderConfig) string { return c.MiniMaxTokenPlanAPIKey },
		clear: func(c *ProviderConfig) { c.MiniMaxTokenPlanAPIKey = "" },
	},
	{
		label: "minimax_payg_api_key", env: "MINIMAX_PAYG_API_KEY",
		value: func(c *ProviderConfig) string { return c.MiniMaxPaygAPIKey },
		clear: func(c *ProviderConfig) { c.MiniMaxPaygAPIKey = "" },
	},
	{
		label: "poolside_api_key", env: "POOLSIDE_API_KEY",
		value: func(c *ProviderConfig) string { return c.PoolsideAPIKey },
		clear: func(c *ProviderConfig) { c.PoolsideAPIKey = "" },
	},
	{
		label: "groq_api_key", env: "GROQ_API_KEY",
		value: func(c *ProviderConfig) string { return c.GroqAPIKey },
		clear: func(c *ProviderConfig) { c.GroqAPIKey = "" },
	},
	{
		label: "clinepass_api_key", env: "CLINE_API_KEY",
		value: func(c *ProviderConfig) string { return c.ClinePassAPIKey },
		clear: func(c *ProviderConfig) { c.ClinePassAPIKey = "" },
	},
	{
		label: "stepfun_api_key", env: "STEP_API_KEY",
		value: func(c *ProviderConfig) string { return c.StepFunAPIKey },
		clear: func(c *ProviderConfig) { c.StepFunAPIKey = "" },
	},
	{
		label: "concentrate_api_key", env: "CONCENTRATE_API_KEY",
		value: func(c *ProviderConfig) string { return c.ConcentrateAPIKey },
		clear: func(c *ProviderConfig) { c.ConcentrateAPIKey = "" },
	},
	{
		label: "opengateway_api_key", env: "OPENGATEWAY_API_KEY",
		value: func(c *ProviderConfig) string { return c.OpenGatewayAPIKey },
		clear: func(c *ProviderConfig) { c.OpenGatewayAPIKey = "" },
	},
	{
		label: "agnes_api_key", env: "AGNES_API_KEY",
		value: func(c *ProviderConfig) string { return c.AgnesAPIKey },
		clear: func(c *ProviderConfig) { c.AgnesAPIKey = "" },
	},
	{
		label: "fireworks_api_key", env: "FIREWORKS_API_KEY",
		value: func(c *ProviderConfig) string { return c.FireworksAPIKey },
		clear: func(c *ProviderConfig) { c.FireworksAPIKey = "" },
	},
}

// ProviderConfigContainsSecrets reports whether provider state contains
// credential material, including unrepresentable fields on unregistered
// deployments. It never returns or formats credential values.
func ProviderConfigContainsSecrets(cfg ProviderConfig) bool {
	for _, deployment := range cfg.Deployments {
		if deploymentContainsSecrets(deployment) {
			return true
		}
	}
	secrets, _ := ProviderConfigSecrets(cfg)
	return len(secrets) > 0
}

// SanitizeProviderConfigForDisk removes every credential field while
// preserving provider selection, routing, endpoints, and model metadata.
func SanitizeProviderConfigForDisk(cfg ProviderConfig) ProviderConfig {
	for _, field := range providerCredentialFields {
		field.clear(&cfg)
	}
	if cfg.Deployments != nil {
		deployments := make(map[string]DeploymentConfig, len(cfg.Deployments))
		for id, deployment := range cfg.Deployments {
			deployments[id] = SanitizeDeploymentConfigForDisk(deployment)
		}
		cfg.Deployments = deployments
	}
	return cfg
}

// ProviderConfigSecrets maps every credential stored in provider state to its
// canonical secret-store environment variable. Placeholder values are omitted.
// Deployment credentials take precedence over older top-level fields. It
// returns an error naming fields that cannot be represented by the canonical
// secret store; callers must not sanitize provider state on error.
func ProviderConfigSecrets(cfg ProviderConfig) (map[string]string, error) {
	out := map[string]string{}
	put := func(envKey, secret string) {
		secret = strings.TrimSpace(secret)
		if envKey != "" && secret != "" && !LooksLikePlaceholderSecret(secret) {
			out[envKey] = secret
		}
	}
	for _, field := range providerCredentialFields {
		put(field.env, field.value(&cfg))
	}
	deploymentIDs := make([]string, 0, len(cfg.Deployments))
	for id := range cfg.Deployments {
		deploymentIDs = append(deploymentIDs, id)
	}
	sort.Strings(deploymentIDs)
	for _, id := range deploymentIDs {
		deployment := cfg.Deployments[id]
		secrets, err := providerDeploymentSecrets(id, deployment)
		if err != nil {
			return nil, err
		}
		for envKey, secret := range secrets {
			out[envKey] = secret
		}
	}
	return out, nil
}

// providerDeploymentSecrets maps one deployment's credential fields to
// canonical secret-store env vars. Credential shapes are enforced per
// deployment: single-key providers accept only APIKey, while Bedrock
// (AWS_*) and Vertex (VERTEX_ACCESS_TOKEN) accept their native shapes.
func providerDeploymentSecrets(id string, deployment DeploymentConfig) (map[string]string, error) {
	out := map[string]string{}
	put := func(envKey, secret string) {
		secret = strings.TrimSpace(secret)
		if envKey != "" && secret != "" && !LooksLikePlaceholderSecret(secret) {
			out[envKey] = secret
		}
	}
	hasAmbiguousFields := strings.TrimSpace(deployment.Token) != "" ||
		strings.TrimSpace(deployment.AccessKeyID) != "" ||
		strings.TrimSpace(deployment.SecretAccessKey) != "" ||
		strings.TrimSpace(deployment.SessionToken) != ""
	switch id {
	case "anthropic-bedrock":
		put("AWS_ACCESS_KEY_ID", firstNonEmpty(deployment.AccessKeyID, deployment.APIKey))
		put("AWS_SECRET_ACCESS_KEY", firstNonEmpty(deployment.SecretAccessKey, deployment.Token))
		put("AWS_SESSION_TOKEN", deployment.SessionToken)
		return out, nil
	case "gemini-vertex", "anthropic-vertex":
		if strings.TrimSpace(deployment.AccessKeyID) != "" ||
			strings.TrimSpace(deployment.SecretAccessKey) != "" ||
			strings.TrimSpace(deployment.SessionToken) != "" {
			return nil, fmt.Errorf("provider deployment %q contains unsupported credential fields", id)
		}
		put("VERTEX_ACCESS_TOKEN", firstNonEmpty(deployment.Token, deployment.APIKey))
		return out, nil
	}
	envKey, known := credentialEnvForDeployment(id)
	if !known && deploymentContainsSecrets(deployment) {
		return nil, fmt.Errorf("provider deployment %q has no safe credential mapping", id)
	}
	if hasAmbiguousFields {
		return nil, fmt.Errorf("provider deployment %q contains unsupported credential fields", id)
	}
	put(envKey, deployment.APIKey)
	return out, nil
}

// credentialEnvForDeployment resolves the canonical secret-store env var for
// a deployment from the provider registry. New providers register their
// deployment and credential env in the registry; this function needs no
// per-provider changes.
func credentialEnvForDeployment(deploymentID string) (string, bool) {
	// anthropic-vertex predates the registry's vertex deployment id.
	if deploymentID == "anthropic-vertex" {
		deploymentID = "gemini-vertex"
	}
	for _, spec := range registry.All() {
		if spec.DeploymentID != deploymentID {
			continue
		}
		if env := strings.TrimSpace(spec.RuntimeCredentialEnv); env != "" {
			return env, true
		}
		return strings.TrimSpace(spec.CredentialEnv), true
	}
	return "", false
}

func deploymentContainsSecrets(deployment DeploymentConfig) bool {
	return strings.TrimSpace(deployment.APIKey) != "" || strings.TrimSpace(deployment.Token) != "" ||
		strings.TrimSpace(deployment.SecretAccessKey) != "" || strings.TrimSpace(deployment.AccessKeyID) != "" ||
		strings.TrimSpace(deployment.SessionToken) != ""
}

// providerBaseURLEnv maps flat provider.json base-url fields to the env var
// names declared in each provider's registry spec.
var providerBaseURLEnv = map[string]func(*ProviderConfig) string{
	"ANTHROPIC_BASE_URL":  func(c *ProviderConfig) string { return c.AnthropicBaseURL },
	"OPENAI_BASE_URL":     func(c *ProviderConfig) string { return c.OpenAIBaseURL },
	"OPENAI_API_BASE":     func(c *ProviderConfig) string { return c.OpenAIBaseURL },
	"GEMINI_BASE_URL":     func(c *ProviderConfig) string { return c.GeminiBaseURL },
	"DEEPSEEK_BASE_URL":   func(c *ProviderConfig) string { return c.DeepSeekBaseURL },
	"XAI_BASE_URL":        func(c *ProviderConfig) string { return firstNonEmpty(c.XAIBaseURL, c.GrokBaseURL) },
	"MOONSHOT_BASE_URL":   func(c *ProviderConfig) string { return c.MoonshotBaseURL },
	"ZAI_BASE_URL":        func(c *ProviderConfig) string { return c.ZAIBaseURL },
	"ZAI_API_BASE":        func(c *ProviderConfig) string { return c.ZAIBaseURL },
	"ZAI_CODING_BASE_URL": func(c *ProviderConfig) string { return c.ZAICodingBaseURL },
	"XIAOMI_MIMO_TOKEN_PLAN_BASE_URL": func(c *ProviderConfig) string {
		return c.XiaomiMimoTokenPlanBaseURL
	},
	"XIAOMI_MIMO_PAYG_BASE_URL": func(c *ProviderConfig) string { return c.XiaomiMimoPaygBaseURL },
	"XIAOMI_BASE_URL":           func(c *ProviderConfig) string { return c.XiaomiBaseURL },
	"MINIMAX_TOKEN_PLAN_BASE_URL": func(c *ProviderConfig) string {
		return c.MiniMaxTokenPlanBaseURL
	},
	"MINIMAX_PAYG_BASE_URL": func(c *ProviderConfig) string { return c.MiniMaxPaygBaseURL },
	"MINIMAX_BASE_URL":      func(c *ProviderConfig) string { return c.MiniMaxPaygBaseURL },
	"OPENROUTER_BASE_URL":   func(c *ProviderConfig) string { return c.OpenRouterBaseURL },
	"CONCENTRATE_BASE_URL":  func(c *ProviderConfig) string { return c.ConcentrateBaseURL },
	"OPENGATEWAY_BASE_URL":  func(c *ProviderConfig) string { return c.OpenGatewayBaseURL },
	"STEP_BASE_URL":         func(c *ProviderConfig) string { return c.StepFunBaseURL },
	"AGNES_BASE_URL":        func(c *ProviderConfig) string { return c.AgnesBaseURL },
	"FIREWORKS_BASE_URL":    func(c *ProviderConfig) string { return c.FireworksBaseURL },
	"CANOPYWAVE_BASE_URL":   func(c *ProviderConfig) string { return c.CanopyWaveBaseURL },
	"POOLSIDE_BASE_URL":     func(c *ProviderConfig) string { return c.PoolsideBaseURL },
	"GROQ_BASE_URL":         func(c *ProviderConfig) string { return c.GroqBaseURL },
	"CLINE_API_BASE":        func(c *ProviderConfig) string { return c.ClinePassBaseURL },
	"OPENCODEGO_BASE_URL":   func(c *ProviderConfig) string { return c.OpenCodeGoBaseURL },
	"OLLAMA_BASE_URL":       func(c *ProviderConfig) string { return c.OllamaBaseURL },
}

// DeploymentConfigFromProviderState builds a deployment for provider from flat
// provider.json fields, resolving canonical credential and base-url env var
// names from the provider registry. Providers without flat fields return an
// empty deployment.
func DeploymentConfigFromProviderState(cfg *ProviderConfig, provider string) DeploymentConfig {
	if cfg == nil {
		return DeploymentConfig{}
	}
	spec, ok := registry.SpecByProviderID(provider)
	if !ok {
		return DeploymentConfig{}
	}
	credentialEnvs := []string{strings.TrimSpace(spec.CredentialEnv)}
	if runtime := strings.TrimSpace(spec.RuntimeCredentialEnv); runtime != "" {
		credentialEnvs = append(credentialEnvs, runtime)
	}
	out := DeploymentConfig{}
	for _, field := range providerCredentialFields {
		for _, env := range credentialEnvs {
			if field.env != env || out.APIKey != "" {
				continue
			}
			if value := AsNonEmptyString(field.value(cfg)); value != "" {
				out.APIKey = value
			}
		}
	}
	for _, baseEnv := range spec.BaseURLEnv {
		if get := providerBaseURLEnv[baseEnv]; get != nil {
			if value := AsNonEmptyString(get(cfg)); value != "" {
				out.BaseURL = value
				break
			}
		}
	}
	if provider == ProviderXiaomiMimoTokenPlan {
		if base, err := ResolveXiaomiOpenAIBase(provider, cfg); err == nil && base != "" {
			out.BaseURL = base
		}
	}
	return out
}
