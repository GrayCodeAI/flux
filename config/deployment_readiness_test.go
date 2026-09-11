package config

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func TestDeploymentConfiguredMatchesStrictFactoryRequirements(t *testing.T) {
	tests := []struct {
		name string
		id   string
		dep  catalog.Deployment
		cfg  DeploymentConfig
		want bool
	}{
		{name: "azure missing endpoint", id: "openai-azure", cfg: DeploymentConfig{APIKey: "secret"}},
		{name: "azure missing mapping", id: "openai-azure", dep: catalog.Deployment{ModelMappingsRequired: true}, cfg: DeploymentConfig{APIKey: "secret", Endpoint: "https://azure.example.test"}},
		{name: "azure ready", id: "openai-azure", dep: catalog.Deployment{ModelMappingsRequired: true}, cfg: DeploymentConfig{APIKey: "secret", Endpoint: "https://azure.example.test", ModelMappings: map[string]string{"openai/gpt": "deployment"}}, want: true},
		{name: "bedrock missing region", id: "anthropic-bedrock", cfg: DeploymentConfig{AccessKeyID: "access", SecretAccessKey: "secret"}},
		{name: "bedrock ready", id: "anthropic-bedrock", cfg: DeploymentConfig{AccessKeyID: "access", SecretAccessKey: "secret", Region: "us-east-1"}, want: true},
		{name: "token plan missing routed base", id: "xiaomi_mimo_token_plan-direct", cfg: DeploymentConfig{APIKey: "secret"}},
		{name: "token plan ready", id: "xiaomi_mimo_token_plan-direct", cfg: DeploymentConfig{APIKey: "secret", BaseURL: "https://api.xiaomimimo.com/v1"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeploymentConfigured(tt.id, tt.dep, tt.cfg); got != tt.want {
				t.Fatalf("DeploymentConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
