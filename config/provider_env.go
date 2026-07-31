package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// ProviderConfig mirrors the Hawk provider.json file.
type ProviderConfig struct {
	ConfigVersion              int                         `json:"config_version,omitempty"`
	Version                    string                      `json:"_version,omitempty"`
	ActiveProvider             string                      `json:"active_provider,omitempty"`
	AnthropicAPIKey            string                      `json:"anthropic_api_key,omitempty"`
	GrokAPIKey                 string                      `json:"grok_api_key,omitempty"`
	XAIAPIKey                  string                      `json:"xai_api_key,omitempty"`
	OpenAIAPIKey               string                      `json:"openai_api_key,omitempty"`
	CanopyWaveAPIKey           string                      `json:"canopywave_api_key,omitempty"`
	DeepSeekAPIKey             string                      `json:"deepseek_api_key,omitempty"`
	ZAIAPIKey                  string                      `json:"zai_api_key,omitempty"`
	ZAICodingAPIKey            string                      `json:"zai_coding_api_key,omitempty"`
	OpenRouterAPIKey           string                      `json:"openrouter_api_key,omitempty"`
	GeminiAPIKey               string                      `json:"gemini_api_key,omitempty"`
	OllamaBaseURL              string                      `json:"ollama_base_url,omitempty"`
	OpenCodeGoAPIKey           string                      `json:"opencodego_api_key,omitempty"`
	MoonshotAPIKey             string                      `json:"moonshot_api_key,omitempty"`
	XiaomiMimoPaygAPIKey       string                      `json:"xiaomi_mimo_payg_api_key,omitempty"`
	XiaomiMimoTokenPlanAPIKey  string                      `json:"xiaomi_mimo_token_plan_api_key,omitempty"`
	MiniMaxTokenPlanAPIKey     string                      `json:"minimax_token_plan_api_key,omitempty"`
	MiniMaxPaygAPIKey          string                      `json:"minimax_payg_api_key,omitempty"`
	AnthropicBaseURL           string                      `json:"anthropic_base_url,omitempty"`
	CanopyWaveBaseURL          string                      `json:"canopywave_base_url,omitempty"`
	DeepSeekBaseURL            string                      `json:"deepseek_base_url,omitempty"`
	ZAIBaseURL                 string                      `json:"zai_base_url,omitempty"`
	ZAICodingBaseURL           string                      `json:"zai_coding_base_url,omitempty"`
	ZAIRegion                  string                      `json:"zai_region,omitempty"`
	ZAICodingRegion            string                      `json:"zai_coding_region,omitempty"`
	GrokBaseURL                string                      `json:"grok_base_url,omitempty"`
	XAIBaseURL                 string                      `json:"xai_base_url,omitempty"`
	OpenAIBaseURL              string                      `json:"openai_base_url,omitempty"`
	OpenRouterBaseURL          string                      `json:"openrouter_base_url,omitempty"`
	GeminiBaseURL              string                      `json:"gemini_base_url,omitempty"`
	OpenCodeGoBaseURL          string                      `json:"opencodego_base_url,omitempty"`
	MoonshotBaseURL            string                      `json:"moonshot_base_url,omitempty"`
	XiaomiBaseURL              string                      `json:"xiaomi_mimo_base_url,omitempty"`
	XiaomiMimoPaygBaseURL      string                      `json:"xiaomi_mimo_payg_base_url,omitempty"`
	XiaomiMimoTokenPlanBaseURL string                      `json:"xiaomi_mimo_token_plan_base_url,omitempty"`
	XiaomiMimoTokenPlanRegion  string                      `json:"xiaomi_mimo_token_plan_region,omitempty"`
	MiniMaxTokenPlanBaseURL    string                      `json:"minimax_token_plan_base_url,omitempty"`
	MiniMaxPaygBaseURL         string                      `json:"minimax_payg_base_url,omitempty"`
	PoolsideAPIKey             string                      `json:"poolside_api_key,omitempty"`
	PoolsideBaseURL            string                      `json:"poolside_base_url,omitempty"`
	PoolsideModel              string                      `json:"poolside_model,omitempty"`
	GroqAPIKey                 string                      `json:"groq_api_key,omitempty"`
	GroqBaseURL                string                      `json:"groq_base_url,omitempty"`
	GroqModel                  string                      `json:"groq_model,omitempty"`
	ClinePassAPIKey            string                      `json:"clinepass_api_key,omitempty"`
	ClinePassBaseURL           string                      `json:"clinepass_base_url,omitempty"`
	ClinePassModel             string                      `json:"clinepass_model,omitempty"`
	MiniMaxModel               string                      `json:"minimax_model,omitempty"`
	AnthropicModel             string                      `json:"anthropic_model,omitempty"`
	OpenAIModel                string                      `json:"openai_model,omitempty"`
	CanopyWaveModel            string                      `json:"canopywave_model,omitempty"`
	ConcentrateAPIKey          string                      `json:"concentrate_api_key,omitempty"`
	ConcentrateBaseURL         string                      `json:"concentrate_base_url,omitempty"`
	ConcentrateModel           string                      `json:"concentrate_model,omitempty"`
	AgnesAPIKey                string                      `json:"agnes_api_key,omitempty"`
	AgnesBaseURL               string                      `json:"agnes_base_url,omitempty"`
	AgnesModel                 string                      `json:"agnes_model,omitempty"`
	DeepSeekModel              string                      `json:"deepseek_model,omitempty"`
	ZAIModel                   string                      `json:"zai_model,omitempty"`
	GrokModel                  string                      `json:"grok_model,omitempty"`
	XAIModel                   string                      `json:"xai_model,omitempty"`
	OpenRouterModel            string                      `json:"openrouter_model,omitempty"`
	GeminiModel                string                      `json:"gemini_model,omitempty"`
	OllamaModel                string                      `json:"ollama_model,omitempty"`
	OpenCodeGoModel            string                      `json:"opencodego_model,omitempty"`
	MoonshotModel              string                      `json:"moonshot_model,omitempty"`
	XiaomiModel                string                      `json:"xiaomi_mimo_model,omitempty"`
	ActiveModel                string                      `json:"active_model,omitempty"`
	ExplorationModel           string                      `json:"exploration_model,omitempty"`
	AnthropicVersion           string                      `json:"anthropic_version,omitempty"`
	Deployments                map[string]DeploymentConfig `json:"deployments,omitempty"`
	Routing                    *RoutingPolicy              `json:"routing,omitempty"`
}

// providerConfigJSON accepts the historical "version" spelling while keeping
// ProviderConfig's serialized form canonical ("_version"). Keeping the alias
// out of ProviderConfig also prevents new writes from reintroducing it.
type providerConfigJSON struct {
	ProviderConfig
	LegacyVersion string `json:"version,omitempty"`
}

type DeploymentConfig struct {
	APIKey          string            `json:"api_key,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	Endpoint        string            `json:"endpoint,omitempty"`
	APIVersion      string            `json:"api_version,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"`
	Region          string            `json:"region,omitempty"`
	Token           string            `json:"token,omitempty"`
	AccessKeyID     string            `json:"access_key_id,omitempty"`
	SecretAccessKey string            `json:"secret_access_key,omitempty"`
	SessionToken    string            `json:"session_token,omitempty"`
	ModelMappings   map[string]string `json:"model_mappings,omitempty"`

	// OIDC keyless auth (opt-in). When RoleARN (Bedrock) or WIFAudience (Vertex)
	// is set — or EYRIE_OIDC=1 — and the process runs in GitHub Actions, the
	// deployment obtains short-lived credentials via OIDC instead of stored
	// secrets. Empty by default; the non-OIDC path is unchanged.
	RoleARN             string `json:"role_arn,omitempty"`
	WIFAudience         string `json:"wif_audience,omitempty"`
	ServiceAccountEmail string `json:"service_account_email,omitempty"`
}

// NOTE: RoutingPolicy, RoutingStage, DeploymentChoice are intentionally duplicated
// in both config and router packages to avoid a circular import (config→router→client→config).
// The setup.RouterRoutingPolicy() converter in setup/deployment.go bridges the two.
// Keep these structs in sync with router/deployment_router.go.

type RoutingPolicy struct {
	Default   []RoutingStage            `json:"default,omitempty"`
	Providers map[string][]RoutingStage `json:"providers,omitempty"`
	Models    map[string][]RoutingStage `json:"models,omitempty"`
}

type RoutingStage struct {
	Deployments []DeploymentChoice `json:"deployments"`
	Retries     int                `json:"retries,omitempty"`
}

type DeploymentChoice struct {
	DeploymentID string `json:"deployment_id"`
	Weight       int    `json:"weight"`
}

// providerFieldMap defines which config fields map to each provider.
type providerFieldMap struct {
	APIKeys func(c *ProviderConfig) []string
	Models  func(c *ProviderConfig) []string
	BaseURL func(c *ProviderConfig) string
}

var providerFields = map[string]providerFieldMap{
	ProviderAnthropic: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.AnthropicAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.AnthropicModel} },
		BaseURL: func(c *ProviderConfig) string { return c.AnthropicBaseURL },
	},
	ProviderOpenAI: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.OpenAIAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.OpenAIModel} },
		BaseURL: func(c *ProviderConfig) string { return c.OpenAIBaseURL },
	},
	ProviderCanopyWave: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.CanopyWaveAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.CanopyWaveModel} },
		BaseURL: func(c *ProviderConfig) string { return c.CanopyWaveBaseURL },
	},
	ProviderConcentrate: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.ConcentrateAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.ConcentrateModel} },
		BaseURL: func(c *ProviderConfig) string { return c.ConcentrateBaseURL },
	},
	ProviderDeepSeek: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.DeepSeekAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.DeepSeekModel} },
		BaseURL: func(c *ProviderConfig) string { return c.DeepSeekBaseURL },
	},
	ProviderZAIPayg: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.ZAIAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.ZAIModel} },
		BaseURL: func(c *ProviderConfig) string { return c.ZAIBaseURL },
	},
	ProviderZAICoding: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.ZAICodingAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.ZAIModel} },
		BaseURL: func(c *ProviderConfig) string { return c.ZAICodingBaseURL },
	},
	ProviderOpenRouter: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.OpenRouterAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.OpenRouterModel} },
		BaseURL: func(c *ProviderConfig) string { return c.OpenRouterBaseURL },
	},
	ProviderGrok: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.GrokAPIKey, c.XAIAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.GrokModel, c.XAIModel} },
		BaseURL: func(c *ProviderConfig) string { return firstNonEmpty(c.GrokBaseURL, c.XAIBaseURL) },
	},
	ProviderPoolside: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.PoolsideAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.PoolsideModel} },
		BaseURL: func(c *ProviderConfig) string { return c.PoolsideBaseURL },
	},
	ProviderGroq: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.GroqAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.GroqModel} },
		BaseURL: func(c *ProviderConfig) string { return c.GroqBaseURL },
	},
	ProviderClinePass: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.ClinePassAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.ClinePassModel} },
		BaseURL: func(c *ProviderConfig) string { return c.ClinePassBaseURL },
	},
	ProviderGemini: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.GeminiAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.GeminiModel} },
		BaseURL: func(c *ProviderConfig) string { return c.GeminiBaseURL },
	},
	ProviderOllama: {
		APIKeys: func(c *ProviderConfig) []string { return nil },
		Models:  func(c *ProviderConfig) []string { return []string{c.OllamaModel} },
		BaseURL: func(c *ProviderConfig) string { return c.OllamaBaseURL },
	},
	ProviderOpenCodeGo: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.OpenCodeGoAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.OpenCodeGoModel} },
		BaseURL: func(c *ProviderConfig) string { return c.OpenCodeGoBaseURL },
	},
	ProviderKimi: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.MoonshotAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.MoonshotModel} },
		BaseURL: func(c *ProviderConfig) string { return c.MoonshotBaseURL },
	},
	ProviderXiaomiMimoPayg: {
		APIKeys: func(c *ProviderConfig) []string {
			return []string{c.XiaomiMimoPaygAPIKey}
		},
		Models:  func(c *ProviderConfig) []string { return []string{c.XiaomiModel} },
		BaseURL: func(c *ProviderConfig) string { return firstNonEmpty(c.XiaomiMimoPaygBaseURL, c.XiaomiBaseURL) },
	},
	ProviderXiaomiMimoTokenPlan: {
		APIKeys: func(c *ProviderConfig) []string { return []string{c.XiaomiMimoTokenPlanAPIKey} },
		Models:  func(c *ProviderConfig) []string { return []string{c.XiaomiModel} },
		BaseURL: func(c *ProviderConfig) string { return c.XiaomiMimoTokenPlanBaseURL },
	},
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// AsNonEmptyString returns trimmed string or empty.
func AsNonEmptyString(v string) string {
	if t := strings.TrimSpace(v); t != "" {
		return t
	}
	return ""
}

// NormalizeOllamaOpenAIBaseURL ensures the URL ends with /v1.
func NormalizeOllamaOpenAIBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}

// SetEnvValue sets an env var if value is non-empty and overwrite is allowed.
func SetEnvValue(key, value string, overwrite bool) {
	if value == "" {
		return
	}
	if !overwrite && os.Getenv(key) != "" {
		return
	}
	_ = os.Setenv(key, value)
}

// ApplyOpenAICompatibleProvider sets env vars for an OpenAI-compatible provider.
func ApplyOpenAICompatibleProvider(prefix, apiKey, model, baseURL string, overwrite bool) {
	SetEnvValue(prefix+"_API_KEY", apiKey, overwrite)
	SetEnvValue(prefix+"_MODEL", model, overwrite)
	SetEnvValue(prefix+"_BASE_URL", baseURL, overwrite)
	SetEnvValue("OPENAI_API_KEY", apiKey, overwrite)
	SetEnvValue("OPENAI_MODEL", model, overwrite)
	SetEnvValue("OPENAI_BASE_URL", baseURL, overwrite)
}

// GetProviderModel returns the configured model for a provider.
func GetProviderModel(config *ProviderConfig, provider string) string {
	if f, ok := providerFields[provider]; ok {
		for _, m := range f.Models(config) {
			if v := AsNonEmptyString(m); v != "" {
				return v
			}
		}
	}
	return ""
}

// GetProviderAPIKey returns the configured API key for a provider.
func GetProviderAPIKey(config *ProviderConfig, provider string) string {
	if f, ok := providerFields[provider]; ok {
		for _, k := range f.APIKeys(config) {
			if v := AsNonEmptyString(k); v != "" {
				return v
			}
		}
	}
	return ""
}

// ValidateAPIKey validates an API key.
func ValidateAPIKey(apiKey, providerName string) string {
	if apiKey == "" {
		return providerName + " requires an API key"
	}
	if apiKey == "SUA_CHAVE" {
		return providerName + " API key cannot be placeholder value 'SUA_CHAVE'"
	}
	if len(apiKey) < 10 {
		return providerName + " API key appears invalid (too short)"
	}
	return ""
}

// ProviderDetectionOrder is the priority order for provider detection.
var ProviderDetectionOrder = APIProviderDetectionOrder

// GetProviderConfigDir returns the config directory path. It returns an error
// when no configuration directory can be resolved (e.g. unset HOME in a
// container) so callers degrade gracefully instead of panicking.
func GetProviderConfigDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("EYRIE_CONFIG_DIR")); d != "" {
		return d, nil
	}
	// HAWK_CONFIG_DIR is retained as a compatibility fallback for hosts that
	// predate Eyrie's host-neutral configuration namespace.
	if d := os.Getenv("HAWK_CONFIG_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err == nil && d != "" {
		// Host-neutral default. Embedders that migrate from a product-specific
		// directory should copy existing provider state to this path.
		return filepath.Join(d, "eyrie"), nil
	}
	return "", fmt.Errorf("eyrie provider config: user config directory unavailable")
}

// GetProviderConfigPath returns the full path to provider.json. It returns an
// error when the configuration directory cannot be resolved.
func GetProviderConfigPath() (string, error) {
	dir, err := GetProviderConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "provider.json"), nil
}

// LoadProviderConfig loads provider config from disk.
// Returns nil if file doesn't exist or the config dir cannot be resolved.
// Returns nil for corrupt JSON or permission issues (callers wanting detail
// should use LoadProviderConfigWithError).
func LoadProviderConfig(path string) *ProviderConfig {
	if path == "" {
		path, _ = GetProviderConfigPath()
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path defaults to GetProviderConfigPath() or is supplied by the local caller, not untrusted input
	if err != nil {
		return nil
	}
	var wire providerConfigJSON
	if json.Unmarshal(data, &wire) != nil {
		return nil
	}
	cfg, err := normalizeProviderConfigVersion(wire)
	if err != nil {
		return nil
	}
	return cfg
}

// LoadProviderConfigWithError loads provider config from disk with detailed error reporting.
// Returns (nil, nil) if file doesn't exist.
// Returns (nil, error) for corrupt JSON, missing config dir, or permission issues.
func LoadProviderConfigWithError(path string) (*ProviderConfig, error) {
	if path == "" {
		path, _ = GetProviderConfigPath()
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path defaults to GetProviderConfigPath() or is supplied by the local caller, not untrusted input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("eyrie: failed to read provider config at %s: %w", path, err)
	}
	var wire providerConfigJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("eyrie: corrupt provider config at %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("eyrie: corrupt provider config at %s: %w", path, err)
	}
	cfg, err := normalizeProviderConfigVersion(wire)
	if err != nil {
		return nil, fmt.Errorf("eyrie: corrupt provider config at %s: %w", path, err)
	}
	if cfg.Version != "" && cfg.Version != "1" {
		return nil, fmt.Errorf("eyrie: unsupported provider config version %q at %s", cfg.Version, path)
	}
	return cfg, nil
}

func normalizeProviderConfigVersion(wire providerConfigJSON) (*ProviderConfig, error) {
	canonical := strings.TrimSpace(wire.Version)
	legacy := strings.TrimSpace(wire.LegacyVersion)
	if canonical != "" && legacy != "" && canonical != legacy {
		return nil, fmt.Errorf("conflicting provider config versions %q and %q", canonical, legacy)
	}
	if canonical == "" {
		wire.Version = legacy
	} else {
		wire.Version = canonical
	}
	return &wire.ProviderConfig, nil
}

// SaveProviderConfig atomically persists non-secret provider state. Credential
// material belongs in the credential store; refusing it here makes plaintext
// provider.json writes impossible through the public config API.
func SaveProviderConfig(config *ProviderConfig, path string) (err error) {
	if config == nil {
		return fmt.Errorf("eyrie: provider config is nil")
	}
	if ProviderConfigContainsSecrets(*config) {
		return fmt.Errorf("eyrie: refusing to persist credential fields in provider config")
	}
	if config.Version != "" && config.Version != "1" {
		return fmt.Errorf("eyrie: refusing to persist unsupported provider config version %q", config.Version)
	}
	if path == "" {
		path, err = GetProviderConfigPath()
		if err != nil {
			return err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provider-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	// The rename is the commit point. Directory fsync is best-effort because a
	// post-commit error must not make callers roll back credentials after the
	// plaintext source has already been replaced by sanitized state.
	if dirHandle, openErr := os.Open(dir); openErr == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

// IsProviderConfigured checks if a provider has valid configuration.
func IsProviderConfigured(config *ProviderConfig, provider string) bool {
	if provider == ProviderOllama {
		return AsNonEmptyString(config.OllamaBaseURL) != ""
	}
	if depID := cloudDeploymentForProvider(provider); depID != "" {
		if dep, ok := config.Deployments[depID]; ok {
			return deploymentHasLiveCredentials(depID, dep)
		}
	}
	return GetProviderAPIKey(config, provider) != ""
}

func cloudDeploymentForProvider(provider string) string {
	switch provider {
	case ProviderAzure:
		return "openai-azure"
	case ProviderBedrock:
		return "anthropic-bedrock"
	case ProviderVertex:
		return "gemini-vertex"
	default:
		return ""
	}
}

// DefaultProviderFromConfig determines the default provider from config.
func DefaultProviderFromConfig(config *ProviderConfig) string {
	if config == nil {
		return ""
	}
	if ep := AsNonEmptyString(config.ActiveProvider); ep != "" {
		if IsProviderConfigured(config, ep) {
			return ep
		}
	}
	for _, p := range ProviderDetectionOrder {
		if IsProviderConfigured(config, p) {
			return p
		}
	}
	return ""
}

// GetProviderActiveModel gets the active model for a provider from config.
func GetProviderActiveModel(config *ProviderConfig, provider string) string {
	specific := GetProviderModel(config, provider)
	if specific != "" {
		return specific
	}
	// Check if any provider has a scoped model
	for _, p := range ProviderDetectionOrder {
		if GetProviderModel(config, p) != "" {
			return ""
		}
	}
	legacy := AsNonEmptyString(config.ActiveModel)
	if legacy == "" {
		return ""
	}
	if DefaultProviderFromConfig(config) == provider {
		return legacy
	}
	return ""
}

// ClearProviderRuntimeEnv clears all provider-related env vars.
func ClearProviderRuntimeEnv() {
	keys := []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_BASE_URL", "ANTHROPIC_VERSION",
		"OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_BASE_URL",
		"AZURE_OPENAI_API_KEY", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_API_VERSION", "AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_MODEL",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION", "BEDROCK_MODEL",
		"VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN", "VERTEX_PROJECT_ID", "VERTEX_REGION", "VERTEX_MODEL",
		"OPENROUTER_API_KEY", "OPENROUTER_MODEL", "OPENROUTER_BASE_URL",
		"CONCENTRATE_API_KEY", "CONCENTRATE_MODEL", "CONCENTRATE_BASE_URL",
		"CANOPYWAVE_API_KEY", "CANOPYWAVE_MODEL", "CANOPYWAVE_BASE_URL",
		"DEEPSEEK_API_KEY", "DEEPSEEK_MODEL", "DEEPSEEK_BASE_URL",
		"ZAI_API_KEY", "ZAI_CODING_API_KEY", "ZAI_MODEL", "ZAI_BASE_URL", "ZAI_CODING_BASE_URL", "ZAI_API_BASE",
		"ZAI_REGION", "ZAI_CODING_REGION",
		"XAI_API_KEY", "XAI_MODEL", "XAI_BASE_URL",
		"GEMINI_API_KEY", "GEMINI_MODEL", "GEMINI_BASE_URL",
		"OLLAMA_BASE_URL",
		"OPENCODEGO_API_KEY", "OPENCODEGO_MODEL", "OPENCODEGO_BASE_URL",
		"GROQ_API_KEY", "GROQ_MODEL", "GROQ_BASE_URL",
		"POOLSIDE_API_KEY", "POOLSIDE_MODEL", "POOLSIDE_BASE_URL",
		"CLINE_API_KEY", "CLINE_MODEL", "CLINE_API_BASE",
		"MOONSHOT_API_KEY", "MOONSHOT_MODEL", "MOONSHOT_BASE_URL",
		"XIAOMI_MIMO_PAYG_API_KEY", "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
		"XIAOMI_MIMO_TOKEN_PLAN_REGION", "XIAOMI_MODEL", "XIAOMI_BASE_URL",
		"XIAOMI_MIMO_PAYG_BASE_URL", "XIAOMI_MIMO_TOKEN_PLAN_BASE_URL",
	}
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

// collectEnvValue adds a key/value to the env map if value is non-empty and
// overwrite is allowed (or the key is not already set in the process env).
func collectEnvValue(env map[string]string, key, value string, overwrite bool) {
	if value == "" {
		return
	}
	if !overwrite && os.Getenv(key) != "" {
		return
	}
	env[key] = value
}

// collectOpenAICompatibleProvider adds env vars for an OpenAI-compatible provider to the map.
func collectOpenAICompatibleProvider(env map[string]string, prefix, apiKey, model, baseURL string, overwrite bool) {
	collectEnvValue(env, prefix+"_API_KEY", apiKey, overwrite)
	collectEnvValue(env, prefix+"_MODEL", model, overwrite)
	collectEnvValue(env, prefix+"_BASE_URL", baseURL, overwrite)
	collectEnvValue(env, "OPENAI_API_KEY", apiKey, overwrite)
	collectEnvValue(env, "OPENAI_MODEL", model, overwrite)
	collectEnvValue(env, "OPENAI_BASE_URL", baseURL, overwrite)
}

// ApplyProviderEnv computes the env vars for a specific provider and returns
// them as a map without modifying the process environment.
func ApplyProviderEnv(provider string, config *ProviderConfig, activeModel string, overwrite bool, cat *catalog.ModelCatalog) map[string]string {
	env := make(map[string]string)
	switch provider {
	case ProviderAnthropic:
		collectEnvValue(env, "ANTHROPIC_API_KEY", AsNonEmptyString(config.AnthropicAPIKey), overwrite)
		m := activeModel
		if m == "" {
			m = catalog.GetPreferredProviderModel("anthropic", catalog.TierSonnet, cat)
		}
		collectEnvValue(env, "ANTHROPIC_MODEL", m, overwrite)
		collectEnvValue(env, "ANTHROPIC_BASE_URL", AsNonEmptyString(config.AnthropicBaseURL), overwrite)
		collectEnvValue(env, "ANTHROPIC_VERSION", AsNonEmptyString(config.AnthropicVersion), overwrite)
	case ProviderOpenAI:
		collectEnvValue(env, "OPENAI_API_KEY", AsNonEmptyString(config.OpenAIAPIKey), overwrite)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("openai", cat)
		}
		collectEnvValue(env, "OPENAI_MODEL", m, overwrite)
		base := AsNonEmptyString(config.OpenAIBaseURL)
		if base == "" {
			base = DefaultOpenAIBaseURL
		}
		collectEnvValue(env, "OPENAI_BASE_URL", base, overwrite)
	case ProviderGemini:
		apiKey := AsNonEmptyString(config.GeminiAPIKey)
		base := firstNonEmpty(config.GeminiBaseURL, DefaultGeminiOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("gemini", cat)
		}
		collectOpenAICompatibleProvider(env, "GEMINI", apiKey, m, base, overwrite)
	case ProviderVertex:
		dep := config.Deployments["gemini-vertex"]
		token := firstNonEmpty(dep.Token, dep.APIKey)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("vertex", cat)
		}
		collectEnvValue(env, "VERTEX_ACCESS_TOKEN", token, overwrite)
		collectEnvValue(env, "VERTEX_PROJECT_ID", dep.ProjectID, overwrite)
		collectEnvValue(env, "VERTEX_REGION", dep.Region, overwrite)
		collectEnvValue(env, "VERTEX_MODEL", m, overwrite)
	case ProviderAzure:
		dep := config.Deployments["openai-azure"]
		m := activeModel
		if m == "" {
			m = firstNonEmptyDeploymentMapping(dep.ModelMappings)
		}
		collectEnvValue(env, "AZURE_OPENAI_API_KEY", dep.APIKey, overwrite)
		collectEnvValue(env, "AZURE_OPENAI_ENDPOINT", dep.Endpoint, overwrite)
		collectEnvValue(env, "AZURE_OPENAI_API_VERSION", dep.APIVersion, overwrite)
		collectEnvValue(env, "AZURE_OPENAI_DEPLOYMENT", m, overwrite)
	case ProviderBedrock:
		dep := config.Deployments["anthropic-bedrock"]
		m := activeModel
		if m == "" {
			m = firstNonEmptyDeploymentMapping(dep.ModelMappings)
		}
		collectEnvValue(env, "AWS_ACCESS_KEY_ID", dep.AccessKeyID, overwrite)
		collectEnvValue(env, "AWS_SECRET_ACCESS_KEY", dep.SecretAccessKey, overwrite)
		collectEnvValue(env, "AWS_SESSION_TOKEN", dep.SessionToken, overwrite)
		collectEnvValue(env, "AWS_REGION", dep.Region, overwrite)
		collectEnvValue(env, "BEDROCK_MODEL", m, overwrite)
	case ProviderGrok:
		apiKey := firstNonEmpty(config.XAIAPIKey, config.GrokAPIKey)
		base := firstNonEmpty(config.XAIBaseURL, config.GrokBaseURL, DefaultGrokOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("grok", cat)
		}
		collectEnvValue(env, "XAI_API_KEY", apiKey, overwrite)
		collectOpenAICompatibleProvider(env, "XAI", apiKey, m, base, overwrite)
	case ProviderPoolside:
		apiKey := AsNonEmptyString(config.PoolsideAPIKey)
		base := firstNonEmpty(config.PoolsideBaseURL, DefaultPoolsideOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("poolside", cat)
		}
		collectOpenAICompatibleProvider(env, "POOLSIDE", apiKey, m, base, overwrite)
	case ProviderGroq:
		apiKey := AsNonEmptyString(config.GroqAPIKey)
		base := firstNonEmpty(config.GroqBaseURL, DefaultGroqOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("groq", cat)
		}
		collectOpenAICompatibleProvider(env, "GROQ", apiKey, m, base, overwrite)
	case ProviderClinePass:
		apiKey := AsNonEmptyString(config.ClinePassAPIKey)
		base := firstNonEmpty(config.ClinePassBaseURL, DefaultClinePassOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("clinepass", cat)
		}
		collectOpenAICompatibleProvider(env, "CLINE", apiKey, m, base, overwrite)
	case ProviderCanopyWave:
		apiKey := AsNonEmptyString(config.CanopyWaveAPIKey)
		base := firstNonEmpty(config.CanopyWaveBaseURL, DefaultCanopyWaveOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("canopywave", cat)
		}
		collectOpenAICompatibleProvider(env, "CANOPYWAVE", apiKey, m, base, overwrite)
	case ProviderConcentrate:
		apiKey := AsNonEmptyString(config.ConcentrateAPIKey)
		base := firstNonEmpty(config.ConcentrateBaseURL, DefaultConcentrateOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("concentrate", cat)
		}
		collectOpenAICompatibleProvider(env, "CONCENTRATE", apiKey, m, base, overwrite)
	case ProviderDeepSeek:
		apiKey := AsNonEmptyString(config.DeepSeekAPIKey)
		base := firstNonEmpty(config.DeepSeekBaseURL, "https://api.deepseek.com/v1")
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("deepseek", cat)
		}
		collectOpenAICompatibleProvider(env, "DEEPSEEK", apiKey, m, base, overwrite)
	case ProviderZAIPayg:
		apiKey := AsNonEmptyString(config.ZAIAPIKey)
		base := firstNonEmpty(config.ZAIBaseURL, DefaultZAIOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("zai_payg", cat)
		}
		collectOpenAICompatibleProvider(env, "ZAI", apiKey, m, base, overwrite)
	case ProviderZAICoding:
		apiKey := AsNonEmptyString(config.ZAICodingAPIKey)
		base := firstNonEmpty(config.ZAICodingBaseURL, DefaultZAICodingOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("zai_coding", cat)
		}
		collectOpenAICompatibleProvider(env, "ZAI_CODING", apiKey, m, base, overwrite)
	case ProviderOpenRouter:
		apiKey := AsNonEmptyString(config.OpenRouterAPIKey)
		base := firstNonEmpty(config.OpenRouterBaseURL, DefaultOpenRouterOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("openrouter", cat)
		}
		collectOpenAICompatibleProvider(env, "OPENROUTER", apiKey, m, base, overwrite)
	case ProviderOllama:
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("ollama", cat)
		}
		collectEnvValue(env, "OPENAI_MODEL", m, overwrite)
		base := NormalizeOllamaOpenAIBaseURL(AsNonEmptyString(config.OllamaBaseURL))
		if base == "" {
			base = OllamaDefaultBaseURL
		}
		collectEnvValue(env, "OPENAI_BASE_URL", base, overwrite)
	case ProviderOpenCodeGo:
		apiKey := AsNonEmptyString(config.OpenCodeGoAPIKey)
		base := firstNonEmpty(config.OpenCodeGoBaseURL, DefaultOpenCodeGoBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("opencodego", cat)
		}
		collectOpenAICompatibleProvider(env, "OPENCODEGO", apiKey, m, base, overwrite)
	case ProviderKimi:
		apiKey := AsNonEmptyString(config.MoonshotAPIKey)
		base := firstNonEmpty(config.MoonshotBaseURL, DefaultKimiOpenAIBaseURL)
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("kimi", cat)
		}
		collectEnvValue(env, "MOONSHOT_API_KEY", apiKey, overwrite)
		collectOpenAICompatibleProvider(env, "MOONSHOT", apiKey, m, base, overwrite)
	case ProviderXiaomiMimoPayg:
		apiKey := config.XiaomiMimoPaygAPIKey
		base, _ := ResolveXiaomiOpenAIBase(ProviderXiaomiMimoPayg, config)
		if base == "" {
			base = DefaultXiaomiOpenAIBaseURL
		}
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("xiaomi_mimo_payg", cat)
		}
		collectEnvValue(env, EnvXiaomiPaygAPIKey, apiKey, overwrite)
		collectEnvValue(env, EnvXiaomiPaygBaseURL, base, overwrite)
		collectOpenAICompatibleProvider(env, "XIAOMI", apiKey, m, base, overwrite)
	case ProviderXiaomiMimoTokenPlan:
		apiKey := AsNonEmptyString(config.XiaomiMimoTokenPlanAPIKey)
		base, err := ResolveXiaomiOpenAIBase(ProviderXiaomiMimoTokenPlan, config)
		if err == nil && base != "" {
			collectEnvValue(env, EnvXiaomiTokenPlanBaseURL, base, overwrite)
		}
		if r := strings.TrimSpace(config.XiaomiMimoTokenPlanRegion); r != "" {
			collectEnvValue(env, EnvXiaomiTokenPlanRegion, r, overwrite)
		}
		m := activeModel
		if m == "" {
			m = catalog.GetProviderDefaultModel("xiaomi_mimo_token_plan", cat)
		}
		collectEnvValue(env, EnvXiaomiTokenPlanAPIKey, apiKey, overwrite)
		if base != "" {
			collectOpenAICompatibleProvider(env, "XIAOMI", apiKey, m, base, overwrite)
		}
	}
	return env
}

// ApplyProviderEnvToProcess applies the env vars for a specific provider
// directly to the process environment via os.Setenv.
func ApplyProviderEnvToProcess(provider string, config *ProviderConfig, activeModel string, overwrite bool, cat *catalog.ModelCatalog) {
	for k, v := range ApplyProviderEnv(provider, config, activeModel, overwrite, cat) {
		_ = os.Setenv(k, v)
	}
}

// ApplyProviderConfigToEnv applies the full provider config to env vars.
// Returns the detected provider or empty string.
func ApplyProviderConfigToEnv(config *ProviderConfig, overwrite bool, cat *catalog.ModelCatalog) string {
	if config == nil {
		config = LoadProviderConfig("")
	}
	if config == nil {
		return ""
	}
	provider := DefaultProviderFromConfig(config)
	if provider == "" {
		return ""
	}
	if overwrite {
		ClearProviderRuntimeEnv()
	}
	activeModel := GetProviderActiveModel(config, provider)
	SetEnvValue("GRAYCODE_SMALL_FAST_MODEL", AsNonEmptyString(config.ExplorationModel), overwrite)
	ApplyProviderEnvToProcess(provider, config, activeModel, overwrite, cat)
	return provider
}
