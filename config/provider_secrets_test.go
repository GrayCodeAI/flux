package config

import "testing"

func TestSanitizeProviderConfigForDiskRemovesLegacyAndDeploymentSecrets(t *testing.T) {
	original := ProviderConfig{
		OpenAIAPIKey: "sk-legacy", AnthropicAPIKey: "sk-ant-legacy",
		OpenAIBaseURL: "https://example.test", ActiveModel: "custom/model",
		Deployments: map[string]DeploymentConfig{
			"openai-direct": {APIKey: "sk-deployment", BaseURL: "https://deployment.test"},
		},
	}
	if !ProviderConfigContainsSecrets(original) {
		t.Fatal("expected legacy provider state to contain secrets")
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

func TestLegacyProviderSecretsMapsTopLevelAndDeploymentFields(t *testing.T) {
	cfg := ProviderConfig{
		OpenAIAPIKey: "sk-legacy-value-1234567890",
		Deployments: map[string]DeploymentConfig{
			"openai-direct":     {APIKey: "sk-current-value-1234567890"},
			"anthropic-bedrock": {AccessKeyID: "AKIAEXAMPLE12345678", SecretAccessKey: "aws-secret-value-long-enough", SessionToken: "aws-session-token-long-enough"},
		},
	}
	secrets := LegacyProviderSecrets(cfg)
	if secrets["OPENAI_API_KEY"] != "sk-current-value-1234567890" ||
		secrets["AWS_ACCESS_KEY_ID"] != "AKIAEXAMPLE12345678" ||
		secrets["AWS_SECRET_ACCESS_KEY"] != "aws-secret-value-long-enough" ||
		secrets["AWS_SESSION_TOKEN"] != "aws-session-token-long-enough" {
		t.Fatalf("LegacyProviderSecrets() = %#v", secrets)
	}
}

func TestLegacyProviderSecretsStrictRejectsUnmappedDeploymentFields(t *testing.T) {
	_, err := LegacyProviderSecretsStrict(ProviderConfig{Deployments: map[string]DeploymentConfig{
		"future-provider": {APIKey: "future-secret-1234567890"},
	}})
	if err == nil {
		t.Fatal("unmapped future deployment credential was accepted")
	}
}

func TestLegacyProviderSecretsStrictMapsBedrockCompatibilityFields(t *testing.T) {
	secrets, err := LegacyProviderSecretsStrict(ProviderConfig{Deployments: map[string]DeploymentConfig{
		"anthropic-bedrock": {APIKey: "AKIALEGACY123456789", Token: "legacy-secret-1234567890"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if secrets["AWS_ACCESS_KEY_ID"] != "AKIALEGACY123456789" || secrets["AWS_SECRET_ACCESS_KEY"] != "legacy-secret-1234567890" {
		t.Fatalf("Bedrock compatibility secrets = %#v", secrets)
	}
}

func TestLegacyProviderSecretsStrictRejectsAmbiguousFieldsOnDirectDeployment(t *testing.T) {
	for name, deployment := range map[string]DeploymentConfig{
		"token":         {Token: "ambiguous-token-1234567890"},
		"secret_access": {SecretAccessKey: "ambiguous-secret-1234567890"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LegacyProviderSecretsStrict(ProviderConfig{Deployments: map[string]DeploymentConfig{
				"openai-direct": deployment,
			}})
			if err == nil {
				t.Fatal("ambiguous credential field was silently remapped")
			}
		})
	}
}
