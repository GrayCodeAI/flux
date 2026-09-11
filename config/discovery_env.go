package config

import (
	"context"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// DiscoveryCredentials loads API keys from the OS secret store (not process env or .env files),
// merged with non-secret routing from ~/.hawk/provider.json (e.g. MiMo Token Plan region/base URL).
func DiscoveryCredentials(ctx context.Context) catalog.Credentials {
	if ctx == nil {
		ctx = context.Background()
	}
	env := filterPlaceholderEnv(credentials.APIKeysMap(ctx, credentials.DefaultStore()))
	mergeDiscoveryEnvFromConfig(env, LoadProviderConfig(""), true)
	return catalog.Credentials{APIKeys: env}
}

// DiscoveryCredentialsFromState combines injected credential values with
// non-secret provider routing state. It never reads process-global paths,
// environment variables, or the default credential store.
func DiscoveryCredentialsFromState(apiKeys map[string]string, cfg *ProviderConfig) catalog.Credentials {
	env := filterPlaceholderEnv(cloneDiscoveryEnv(apiKeys))
	mergeDiscoveryEnvFromConfig(env, cfg, false)
	return catalog.Credentials{APIKeys: env}
}

// mergeDiscoveryEnvFromConfig adds non-secret provider state required for live
// catalog fetch. Credential fields in provider.json are deliberately ignored.
func mergeDiscoveryEnvFromConfig(env map[string]string, cfg *ProviderConfig, allowProcessEnv bool) {
	if env == nil {
		return
	}
	if cfg == nil {
		return
	}
	r := strings.TrimSpace(cfg.XiaomiMimoTokenPlanRegion)
	if r == "" && allowProcessEnv {
		r = strings.TrimSpace(os.Getenv(EnvXiaomiTokenPlanRegion))
	}
	if norm, err := xiaomi.NormalizeRegion(r); err == nil {
		env[EnvXiaomiTokenPlanRegion] = string(norm)
	}
	if base, err := ResolveXiaomiOpenAIBase(ProviderXiaomiMimoTokenPlan, cfg); err == nil && base != "" {
		env[EnvXiaomiTokenPlanBaseURL] = base
	}
	if paygBase := strings.TrimSpace(cfg.XiaomiMimoPaygBaseURL); paygBase != "" {
		env[EnvXiaomiPaygBaseURL] = paygBase
	}
	setDiscoveryEnv(env, "ANTHROPIC_BASE_URL", cfg.AnthropicBaseURL)
	setDiscoveryEnv(env, "OPENAI_BASE_URL", cfg.OpenAIBaseURL)
	setDiscoveryEnv(env, "CANOPYWAVE_BASE_URL", cfg.CanopyWaveBaseURL)
	setDiscoveryEnv(env, "DEEPSEEK_BASE_URL", cfg.DeepSeekBaseURL)
	setDiscoveryEnv(env, "ZAI_BASE_URL", cfg.ZAIBaseURL)
	setDiscoveryEnv(env, "ZAI_CODING_BASE_URL", cfg.ZAICodingBaseURL)
	setDiscoveryEnv(env, "ZAI_REGION", cfg.ZAIRegion)
	setDiscoveryEnv(env, "ZAI_CODING_REGION", cfg.ZAICodingRegion)
	setDiscoveryEnv(env, "XAI_BASE_URL", firstNonEmpty(cfg.GrokBaseURL, cfg.XAIBaseURL))
	setDiscoveryEnv(env, "OPENROUTER_BASE_URL", cfg.OpenRouterBaseURL)
	setDiscoveryEnv(env, "GEMINI_BASE_URL", cfg.GeminiBaseURL)
	setDiscoveryEnv(env, "OPENCODEGO_BASE_URL", cfg.OpenCodeGoBaseURL)
	setDiscoveryEnv(env, "MOONSHOT_BASE_URL", cfg.MoonshotBaseURL)
	setDiscoveryEnv(env, "MINIMAX_TOKEN_PLAN_BASE_URL", cfg.MiniMaxTokenPlanBaseURL)
	setDiscoveryEnv(env, "MINIMAX_PAYG_BASE_URL", cfg.MiniMaxPaygBaseURL)
	setDiscoveryEnv(env, "POOLSIDE_BASE_URL", cfg.PoolsideBaseURL)
	setDiscoveryEnv(env, "GROQ_BASE_URL", cfg.GroqBaseURL)
	setDiscoveryEnv(env, "CLINE_API_BASE", cfg.ClinePassBaseURL)
	setDiscoveryEnv(env, "FIREWORKS_BASE_URL", cfg.FireworksBaseURL)
	if ollamaBase := NormalizeOllamaOpenAIBaseURL(AsNonEmptyString(cfg.OllamaBaseURL)); ollamaBase != "" {
		env["OLLAMA_BASE_URL"] = ollamaBase
	}
	mergeDeploymentDiscoveryEnv(env, cfg.Deployments)
}

func mergeDeploymentDiscoveryEnv(env map[string]string, deployments map[string]DeploymentConfig) {
	for id, dep := range deployments {
		switch id {
		case "openai-azure":
			setDiscoveryEnv(env, "AZURE_OPENAI_ENDPOINT", dep.Endpoint)
			setDiscoveryEnv(env, "AZURE_OPENAI_API_VERSION", dep.APIVersion)
			setDiscoveryEnv(env, "AZURE_OPENAI_DEPLOYMENT", firstNonEmptyDeploymentMapping(dep.ModelMappings))
		case "anthropic-bedrock":
			setDiscoveryEnv(env, "AWS_REGION", dep.Region)
		case "anthropic-vertex", "gemini-vertex":
			setDiscoveryEnv(env, "VERTEX_PROJECT_ID", dep.ProjectID)
			setDiscoveryEnv(env, "VERTEX_REGION", dep.Region)
		}
		mergeDeploymentBaseURL(env, id, dep.BaseURL)
	}
}

func mergeDeploymentBaseURL(env map[string]string, deploymentID, baseURL string) {
	keys := map[string]string{
		"anthropic-direct":              "ANTHROPIC_BASE_URL",
		"openai-direct":                 "OPENAI_BASE_URL",
		"gemini-direct":                 "GEMINI_BASE_URL",
		"deepseek-direct":               "DEEPSEEK_BASE_URL",
		"grok-direct":                   "XAI_BASE_URL",
		"kimi-direct":                   "MOONSHOT_BASE_URL",
		"zai_coding-direct":             "ZAI_CODING_BASE_URL",
		"zai_payg-direct":               "ZAI_BASE_URL",
		"xiaomi_mimo_token_plan-direct": EnvXiaomiTokenPlanBaseURL,
		"xiaomi_mimo_payg-direct":       EnvXiaomiPaygBaseURL,
		"minimax_token_plan-direct":     "MINIMAX_TOKEN_PLAN_BASE_URL",
		"minimax_payg-direct":           "MINIMAX_PAYG_BASE_URL",
		"openrouter":                    "OPENROUTER_BASE_URL",
		"canopywave":                    "CANOPYWAVE_BASE_URL",
		"poolside":                      "POOLSIDE_BASE_URL",
		"groq-direct":                   "GROQ_BASE_URL",
		"clinepass":                     "CLINE_API_BASE",
		"opencodego":                    "OPENCODEGO_BASE_URL",
		"ollama-local":                  "OLLAMA_BASE_URL",
		"fireworks-direct":              "FIREWORKS_BASE_URL",
	}
	if key := keys[deploymentID]; key != "" {
		setDiscoveryEnv(env, key, baseURL)
	}
}

func cloneDiscoveryEnv(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func setDiscoveryEnv(env map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" || strings.TrimSpace(env[key]) != "" {
		return
	}
	env[key] = value
}

func firstNonEmptyDeploymentMapping(m map[string]string) string {
	for _, value := range m {
		if v := strings.TrimSpace(value); v != "" {
			return v
		}
	}
	return ""
}

func filterPlaceholderEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return env
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if strings.TrimSpace(v) != "" && !LooksLikePlaceholderSecret(v) {
			out[k] = v
		}
	}
	return out
}
