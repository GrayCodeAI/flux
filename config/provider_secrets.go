package config

import (
	"fmt"
	"sort"
	"strings"
)

// ProviderConfigContainsSecrets reports whether legacy provider state contains
// credential material. It never returns or formats credential values.
func ProviderConfigContainsSecrets(cfg ProviderConfig) bool {
	for _, secret := range providerConfigSecrets(cfg) {
		if strings.TrimSpace(secret) != "" {
			return true
		}
	}
	for _, deployment := range cfg.Deployments {
		if deploymentContainsSecrets(deployment) {
			return true
		}
	}
	return false
}

// SanitizeProviderConfigForDisk removes every historical credential field
// while preserving provider selection, routing, endpoints, and model metadata.
func SanitizeProviderConfigForDisk(cfg ProviderConfig) ProviderConfig {
	cfg.AnthropicAPIKey = ""
	cfg.GrokAPIKey = ""
	cfg.XAIAPIKey = ""
	cfg.OpenAIAPIKey = ""
	cfg.CanopyWaveAPIKey = ""
	cfg.DeepSeekAPIKey = ""
	cfg.ZAIAPIKey = ""
	cfg.ZAICodingAPIKey = ""
	cfg.OpenRouterAPIKey = ""
	cfg.GeminiAPIKey = ""
	cfg.OpenCodeGoAPIKey = ""
	cfg.MoonshotAPIKey = ""
	cfg.XiaomiMimoPaygAPIKey = ""
	cfg.XiaomiMimoTokenPlanAPIKey = ""
	cfg.MiniMaxTokenPlanAPIKey = ""
	cfg.MiniMaxPaygAPIKey = ""
	cfg.PoolsideAPIKey = ""
	cfg.GroqAPIKey = ""
	cfg.ClinePassAPIKey = ""
	if cfg.Deployments != nil {
		deployments := make(map[string]DeploymentConfig, len(cfg.Deployments))
		for id, deployment := range cfg.Deployments {
			deployments[id] = SanitizeDeploymentConfigForDisk(deployment)
		}
		cfg.Deployments = deployments
	}
	return cfg
}

// LegacyProviderSecrets maps historical provider.json credential fields to
// their canonical secret-store environment names. Placeholder values are
// omitted. Deployment credentials take precedence over older top-level fields.
func LegacyProviderSecrets(cfg ProviderConfig) map[string]string {
	out, _ := LegacyProviderSecretsStrict(cfg)
	return out
}

// LegacyProviderSecretsStrict returns every effective legacy credential or an
// error naming fields that cannot be represented by the canonical secret
// store. Callers must not sanitize provider state when this returns an error.
func LegacyProviderSecretsStrict(cfg ProviderConfig) (map[string]string, error) {
	out := map[string]string{}
	put := func(envKey, secret string) {
		secret = strings.TrimSpace(secret)
		if envKey != "" && secret != "" && !LooksLikePlaceholderSecret(secret) {
			out[envKey] = secret
		}
	}
	put("ANTHROPIC_API_KEY", cfg.AnthropicAPIKey)
	put("XAI_API_KEY", firstNonEmpty(cfg.XAIAPIKey, cfg.GrokAPIKey))
	put("OPENAI_API_KEY", cfg.OpenAIAPIKey)
	put("CANOPYWAVE_API_KEY", cfg.CanopyWaveAPIKey)
	put("DEEPSEEK_API_KEY", cfg.DeepSeekAPIKey)
	put("ZAI_API_KEY", cfg.ZAIAPIKey)
	put("ZAI_CODING_API_KEY", cfg.ZAICodingAPIKey)
	put("OPENROUTER_API_KEY", cfg.OpenRouterAPIKey)
	put("GEMINI_API_KEY", cfg.GeminiAPIKey)
	put("OPENCODEGO_API_KEY", cfg.OpenCodeGoAPIKey)
	put("MOONSHOT_API_KEY", cfg.MoonshotAPIKey)
	put("XIAOMI_MIMO_PAYG_API_KEY", cfg.XiaomiMimoPaygAPIKey)
	put("XIAOMI_MIMO_TOKEN_PLAN_API_KEY", cfg.XiaomiMimoTokenPlanAPIKey)
	put("MINIMAX_TOKEN_PLAN_API_KEY", cfg.MiniMaxTokenPlanAPIKey)
	put("MINIMAX_PAYG_API_KEY", cfg.MiniMaxPaygAPIKey)
	put("POOLSIDE_API_KEY", cfg.PoolsideAPIKey)
	put("GROQ_API_KEY", cfg.GroqAPIKey)
	put("CLINE_API_KEY", cfg.ClinePassAPIKey)

	deploymentIDs := make([]string, 0, len(cfg.Deployments))
	for id := range cfg.Deployments {
		deploymentIDs = append(deploymentIDs, id)
	}
	sort.Strings(deploymentIDs)
	for _, id := range deploymentIDs {
		deployment := cfg.Deployments[id]
		switch id {
		case "anthropic-bedrock":
			put("AWS_ACCESS_KEY_ID", firstNonEmpty(deployment.AccessKeyID, deployment.APIKey))
			put("AWS_SECRET_ACCESS_KEY", firstNonEmpty(deployment.SecretAccessKey, deployment.Token))
			put("AWS_SESSION_TOKEN", deployment.SessionToken)
		case "anthropic-vertex", "gemini-vertex":
			if strings.TrimSpace(deployment.AccessKeyID) != "" || strings.TrimSpace(deployment.SecretAccessKey) != "" || strings.TrimSpace(deployment.SessionToken) != "" {
				return nil, fmt.Errorf("provider deployment %q contains unsupported credential fields", id)
			}
			put("VERTEX_ACCESS_TOKEN", firstNonEmpty(deployment.Token, deployment.APIKey))
		default:
			envKey := legacyDeploymentCredentialEnv(id)
			if envKey == "" && deploymentContainsSecrets(deployment) {
				return nil, fmt.Errorf("provider deployment %q has no safe credential mapping", id)
			}
			if strings.TrimSpace(deployment.Token) != "" || strings.TrimSpace(deployment.SecretAccessKey) != "" ||
				strings.TrimSpace(deployment.AccessKeyID) != "" || strings.TrimSpace(deployment.SessionToken) != "" {
				return nil, fmt.Errorf("provider deployment %q contains unsupported credential fields", id)
			}
			put(envKey, deployment.APIKey)
		}
	}
	return out, nil
}

func legacyDeploymentCredentialEnv(deploymentID string) string {
	return map[string]string{
		"anthropic-direct":              "ANTHROPIC_API_KEY",
		"openai-direct":                 "OPENAI_API_KEY",
		"openai-azure":                  "AZURE_OPENAI_API_KEY",
		"grok-direct":                   "XAI_API_KEY",
		"gemini-direct":                 "GEMINI_API_KEY",
		"openrouter":                    "OPENROUTER_API_KEY",
		"canopywave":                    "CANOPYWAVE_API_KEY",
		"deepseek-direct":               "DEEPSEEK_API_KEY",
		"poolside":                      "POOLSIDE_API_KEY",
		"groq-direct":                   "GROQ_API_KEY",
		"clinepass":                     "CLINE_API_KEY",
		"zai_payg-direct":               "ZAI_API_KEY",
		"zai_coding-direct":             "ZAI_CODING_API_KEY",
		"opencodego":                    "OPENCODEGO_API_KEY",
		"kimi-direct":                   "MOONSHOT_API_KEY",
		"xiaomi_mimo_payg-direct":       "XIAOMI_MIMO_PAYG_API_KEY",
		"xiaomi_mimo_token_plan-direct": "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
		"minimax_token_plan-direct":     "MINIMAX_TOKEN_PLAN_API_KEY",
		"minimax_payg-direct":           "MINIMAX_PAYG_API_KEY",
		"ollama-local":                  "OLLAMA_API_KEY",
	}[deploymentID]
}

func providerConfigSecrets(cfg ProviderConfig) []string {
	return []string{
		cfg.AnthropicAPIKey, cfg.GrokAPIKey, cfg.XAIAPIKey, cfg.OpenAIAPIKey,
		cfg.CanopyWaveAPIKey, cfg.DeepSeekAPIKey, cfg.ZAIAPIKey, cfg.ZAICodingAPIKey,
		cfg.OpenRouterAPIKey, cfg.GeminiAPIKey, cfg.OpenCodeGoAPIKey, cfg.MoonshotAPIKey,
		cfg.XiaomiMimoPaygAPIKey, cfg.XiaomiMimoTokenPlanAPIKey,
		cfg.MiniMaxTokenPlanAPIKey, cfg.MiniMaxPaygAPIKey,
		cfg.PoolsideAPIKey, cfg.GroqAPIKey, cfg.ClinePassAPIKey,
	}
}

func deploymentContainsSecrets(deployment DeploymentConfig) bool {
	return strings.TrimSpace(deployment.APIKey) != "" || strings.TrimSpace(deployment.Token) != "" ||
		strings.TrimSpace(deployment.SecretAccessKey) != "" || strings.TrimSpace(deployment.AccessKeyID) != "" ||
		strings.TrimSpace(deployment.SessionToken) != ""
}
