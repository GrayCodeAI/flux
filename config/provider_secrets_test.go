package config

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

func TestSanitizeProviderConfigForDiskRemovesTypedAndDeploymentSecrets(t *testing.T) {
	original := ProviderConfig{
		OpenAIAPIKey: "sk-typed-value-1234567890", AnthropicAPIKey: "sk-ant-typed-value-1234567890",
		OpenAIBaseURL: "https://example.test", ActiveModel: "custom/model",
		Deployments: map[string]DeploymentConfig{
			"openai-direct": {APIKey: "sk-deployment-value-1234567890", BaseURL: "https://deployment.test"},
		},
	}
	if !ProviderConfigContainsSecrets(original) {
		t.Fatal("expected typed provider state to contain secrets")
	}
	sanitized := SanitizeProviderConfigForDisk(original)
	if ProviderConfigContainsSecrets(sanitized) {
		t.Fatalf("sanitized provider state still contains secrets: %+v", sanitized)
	}
	if sanitized.OpenAIBaseURL != original.OpenAIBaseURL || sanitized.ActiveModel != original.ActiveModel ||
		sanitized.Deployments["openai-direct"].BaseURL != "https://deployment.test" {
		t.Fatalf("sanitization lost routing metadata: %+v", sanitized)
	}
	if original.OpenAIAPIKey == "" || original.Deployments["openai-direct"].APIKey == "" {
		t.Fatal("sanitization mutated its input")
	}
}

func TestSanitizeProviderConfigForDiskClearsEveryTypedField(t *testing.T) {
	cfg := ProviderConfig{}
	for _, field := range providerCredentialFields {
		cfg = *populateField(&cfg, field, "sk-clear-check-1234567890")
	}
	if !ProviderConfigContainsSecrets(cfg) {
		t.Fatal("expected every typed credential field to be detected")
	}
	sanitized := SanitizeProviderConfigForDisk(cfg)
	for _, field := range providerCredentialFields {
		if got := field.value(&sanitized); strings.TrimSpace(got) != "" {
			t.Fatalf("field %s not cleared, got %q", field.label, got)
		}
	}
}

func TestProviderConfigSecretsMapsTopLevelAndDeploymentFields(t *testing.T) {
	cfg := ProviderConfig{
		OpenAIAPIKey: "sk-top-level-1234567890",
		Deployments: map[string]DeploymentConfig{
			"openai-direct":     {APIKey: "sk-deployment-value-1234567890"},
			"anthropic-bedrock": {AccessKeyID: "AKIAEXAMPLE12345678", SecretAccessKey: "aws-secret-value-long-enough", SessionToken: "aws-session-token-long-enough"},
		},
	}
	secrets, err := ProviderConfigSecrets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if secrets["OPENAI_API_KEY"] != "sk-deployment-value-1234567890" ||
		secrets["AWS_ACCESS_KEY_ID"] != "AKIAEXAMPLE12345678" ||
		secrets["AWS_SECRET_ACCESS_KEY"] != "aws-secret-value-long-enough" ||
		secrets["AWS_SESSION_TOKEN"] != "aws-session-token-long-enough" {
		t.Fatalf("ProviderConfigSecrets() = %#v", secrets)
	}
}

func TestProviderConfigSecretsMapsEveryRegisteredDeployment(t *testing.T) {
	for _, spec := range registry.All() {
		deploymentID := strings.TrimSpace(spec.DeploymentID)
		if deploymentID == "" {
			continue
		}
		t.Run(deploymentID, func(t *testing.T) {
			wantEnv := strings.TrimSpace(spec.CredentialEnv)
			if runtime := strings.TrimSpace(spec.RuntimeCredentialEnv); runtime != "" {
				wantEnv = runtime
			}
			deployment := DeploymentConfig{APIKey: "sk-registry-value-1234567890"}
			if wantEnv == "AWS_SECRET_ACCESS_KEY" {
				deployment = DeploymentConfig{SecretAccessKey: "sk-registry-value-1234567890"}
			}
			cfg := ProviderConfig{Deployments: map[string]DeploymentConfig{deploymentID: deployment}}
			secrets, err := ProviderConfigSecrets(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got := secrets[wantEnv]; got != "sk-registry-value-1234567890" {
				t.Fatalf("deployment env %q mismatch: got %q, want %q", wantEnv, got, "sk-registry-value-1234567890")
			}
		})
	}
}

func TestProviderConfigSecretsRejectsUnregisteredDeploymentFields(t *testing.T) {
	_, err := ProviderConfigSecrets(ProviderConfig{Deployments: map[string]DeploymentConfig{
		"future-provider": {APIKey: "future-secret-1234567890"},
	}})
	if err == nil {
		t.Fatal("unregistered future deployment credential was accepted")
	}
}

func TestProviderConfigSecretsMapsBedrockCompatibilityFields(t *testing.T) {
	secrets, err := ProviderConfigSecrets(ProviderConfig{Deployments: map[string]DeploymentConfig{
		"anthropic-bedrock": {APIKey: "AKIACOMPAT123456789", Token: "compat-secret-1234567890"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if secrets["AWS_ACCESS_KEY_ID"] != "AKIACOMPAT123456789" || secrets["AWS_SECRET_ACCESS_KEY"] != "compat-secret-1234567890" {
		t.Fatalf("Bedrock compatibility secrets = %#v", secrets)
	}
}

func TestProviderConfigSecretsMapsVertexDeployments(t *testing.T) {
	for _, id := range []string{"gemini-vertex", "anthropic-vertex"} {
		t.Run(id, func(t *testing.T) {
			secrets, err := ProviderConfigSecrets(ProviderConfig{Deployments: map[string]DeploymentConfig{
				id: {Token: "vertex-token-1234567890"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if got := secrets["VERTEX_ACCESS_TOKEN"]; got != "vertex-token-1234567890" {
				t.Fatalf("VERTEX_ACCESS_TOKEN mismatch: got %q, want %q", got, "vertex-token-1234567890")
			}
		})
	}
}

func TestProviderConfigSecretsRejectsAmbiguousFieldsOnDirectDeployment(t *testing.T) {
	for name, deployment := range map[string]DeploymentConfig{
		"token":         {Token: "ambiguous-token-1234567890"},
		"secret_access": {SecretAccessKey: "ambiguous-secret-1234567890"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ProviderConfigSecrets(ProviderConfig{Deployments: map[string]DeploymentConfig{
				"openai-direct": deployment,
			}})
			if err == nil {
				t.Fatal("ambiguous credential field was silently remapped")
			}
		})
	}
}

func TestProviderConfigSecretsDeploymentTakesPrecedenceOverTopLevel(t *testing.T) {
	secrets, err := ProviderConfigSecrets(ProviderConfig{
		OpenAIAPIKey: "sk-top-level-1234567890",
		Deployments: map[string]DeploymentConfig{
			"openai-direct": {APIKey: "sk-deployment-value-1234567890"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := secrets["OPENAI_API_KEY"]; got != "sk-deployment-value-1234567890" {
		t.Fatalf("OPENAI_API_KEY mismatch: got %q, want %q", got, "sk-deployment-value-1234567890")
	}
}

func TestProviderConfigSecretsMapsTypedFieldForEveryRequiresKeyDeployment(t *testing.T) {
	for _, spec := range registry.All() {
		if !spec.RequiresKey || strings.TrimSpace(spec.DeploymentID) == "" {
			continue
		}
		t.Run(spec.ProviderID, func(t *testing.T) {
			wantEnv := strings.TrimSpace(spec.CredentialEnv)
			deployment := DeploymentConfig{APIKey: "sk-typed-deployment-1234567890"}
			if wantEnv == "AWS_SECRET_ACCESS_KEY" {
				deployment = DeploymentConfig{SecretAccessKey: "sk-typed-deployment-1234567890"}
			}
			cfg := ProviderConfig{Deployments: map[string]DeploymentConfig{
				spec.DeploymentID: deployment,
			}}
			secrets, err := ProviderConfigSecrets(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got := secrets[wantEnv]; got != "sk-typed-deployment-1234567890" {
				t.Fatalf("env %q mismatch: got %q, want %q", wantEnv, got, "sk-typed-deployment-1234567890")
			}
		})
	}
}

func TestProviderCredentialFieldsAlignWithRegistry(t *testing.T) {
	referenced := map[string]bool{}
	for _, spec := range registry.All() {
		referenced[strings.TrimSpace(spec.CredentialEnv)] = true
		referenced[strings.TrimSpace(spec.RuntimeCredentialEnv)] = true
		for _, env := range spec.CredentialEnvFallbacks {
			referenced[strings.TrimSpace(env)] = true
		}
		for _, env := range spec.CredentialAliases {
			referenced[strings.TrimSpace(env)] = true
		}
	}
	for _, field := range providerCredentialFields {
		if !referenced[field.env] {
			t.Fatalf("typed field %s env %q is not referenced by any registry provider spec", field.label, field.env)
		}
	}
}

func TestProviderConfigContainsSecretsRejectsOnlyUnmappedDeploymentValues(t *testing.T) {
	if ProviderConfigContainsSecrets(ProviderConfig{}) {
		t.Fatal("empty provider state reported secrets")
	}
	if !ProviderConfigContainsSecrets(ProviderConfig{Deployments: map[string]DeploymentConfig{
		"future-provider": {APIKey: "future-secret-1234567890"},
	}}) {
		t.Fatal("unmapped deployment credential was not counted as a secret")
	}
}

func populateField(cfg *ProviderConfig, field providerCredentialField, value string) *ProviderConfig {
	field.clear(cfg)
	switch field.label {
	case "anthropic_api_key":
		cfg.AnthropicAPIKey = value
	case "grok_api_key":
		cfg.GrokAPIKey = value
	case "xai_api_key":
		cfg.XAIAPIKey = value
	case "openai_api_key":
		cfg.OpenAIAPIKey = value
	case "canopywave_api_key":
		cfg.CanopyWaveAPIKey = value
	case "deepseek_api_key":
		cfg.DeepSeekAPIKey = value
	case "zai_api_key":
		cfg.ZAIAPIKey = value
	case "zai_coding_api_key":
		cfg.ZAICodingAPIKey = value
	case "openrouter_api_key":
		cfg.OpenRouterAPIKey = value
	case "gemini_api_key":
		cfg.GeminiAPIKey = value
	case "opencodego_api_key":
		cfg.OpenCodeGoAPIKey = value
	case "moonshot_api_key":
		cfg.MoonshotAPIKey = value
	case "xiaomi_mimo_payg_api_key":
		cfg.XiaomiMimoPaygAPIKey = value
	case "xiaomi_mimo_token_plan_api_key":
		cfg.XiaomiMimoTokenPlanAPIKey = value
	case "minimax_token_plan_api_key":
		cfg.MiniMaxTokenPlanAPIKey = value
	case "minimax_payg_api_key":
		cfg.MiniMaxPaygAPIKey = value
	case "poolside_api_key":
		cfg.PoolsideAPIKey = value
	case "groq_api_key":
		cfg.GroqAPIKey = value
	case "clinepass_api_key":
		cfg.ClinePassAPIKey = value
	case "stepfun_api_key":
		cfg.StepFunAPIKey = value
	case "concentrate_api_key":
		cfg.ConcentrateAPIKey = value
	case "opengateway_api_key":
		cfg.OpenGatewayAPIKey = value
	case "agnes_api_key":
		cfg.AgnesAPIKey = value
	}
	return cfg
}
