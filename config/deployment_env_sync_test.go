package config

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func TestBuildRoutingPolicyFromDeployments_OpenAI(t *testing.T) {
	t.Parallel()
	deployments := map[string]DeploymentConfig{
		"openai-direct": {APIKey: "sk-test"},
		"openai-azure":  {APIKey: "az-test"},
		"openrouter":    {APIKey: "or-test"},
	}
	policy := BuildRoutingPolicyFromDeployments(deployments)
	if len(policy.Providers["openai"]) < 2 {
		t.Fatalf("expected openai stages with openrouter fallback, got %+v", policy.Providers["openai"])
	}
	primary := policy.Providers["openai"][0]
	if len(primary.Deployments) != 2 {
		t.Fatalf("expected weighted openai stage, got %+v", primary.Deployments)
	}
}

func TestBuildRoutingPolicyFromDeployments_ZAI(t *testing.T) {
	t.Parallel()
	deployments := map[string]DeploymentConfig{
		"zai_payg-direct": {APIKey: "zai-test"},
	}
	policy := BuildRoutingPolicyFromDeployments(deployments)
	if len(policy.Providers["zai_payg"]) == 0 {
		t.Fatalf("expected zai_payg routing, got %+v", policy.Providers)
	}
	if policy.Providers["zai_payg"][0].Deployments[0].DeploymentID != "zai_payg-direct" {
		t.Fatalf("expected zai_payg-direct deployment, got %+v", policy.Providers["zai_payg"])
	}
}

func TestBuildRoutingPolicyFromDeployments_CanopyWave(t *testing.T) {
	t.Parallel()
	deployments := map[string]DeploymentConfig{
		"canopywave": {APIKey: "cw-test"},
		"openrouter": {APIKey: "or-test"},
	}
	policy := BuildRoutingPolicyFromDeployments(deployments)
	if len(policy.Providers["canopywave"]) == 0 {
		t.Fatalf("expected canopywave routing, got %+v", policy.Providers)
	}
	if policy.Providers["canopywave"][0].Deployments[0].DeploymentID != "canopywave" {
		t.Fatalf("expected canopywave deployment, got %+v", policy.Providers["canopywave"])
	}
	if len(policy.Providers["zai_payg"]) != 0 {
		t.Fatalf("zai_payg should not own canopywave routing, got %+v", policy.Providers["zai_payg"])
	}
}

func TestBuildRoutingPolicyFromDeployments_AgnesAndLongCatSingleEndpoint(t *testing.T) {
	t.Parallel()
	deployments := map[string]DeploymentConfig{
		"agnes-direct":     {},
		"longcat-direct":   {},
		"anthropic-direct": {},
		"openrouter":       {},
	}
	policy := BuildRoutingPolicyFromDeployments(deployments)
	for _, provider := range []string{"agnes", "longcat"} {
		stages := policy.Providers[provider]
		if len(stages) != 1 {
			t.Fatalf("%s: want 1 stage, got %+v", provider, stages)
		}
		if len(stages[0].Deployments) != 1 || stages[0].Deployments[0].DeploymentID != provider+"-direct" {
			t.Fatalf("%s: want single %s-direct deployment, got %+v", provider, provider, stages[0].Deployments)
		}
	}
}

func TestSyncProviderConfigFromCatalog(t *testing.T) {
	t.Parallel()
	bootstrap := catalog.BootstrapCatalog()
	compiled, err := catalog.CompileCatalog(&bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-test-key-12345"}
	cfg := SyncProviderConfigFromCatalog(compiled, env)
	if _, ok := cfg.Deployments["anthropic-direct"]; !ok {
		t.Fatal("expected anthropic-direct deployment from env")
	}
	if cfg.Deployments["anthropic-direct"].APIKey != "" {
		t.Fatal("expected sanitized deployment without api_key on disk")
	}
	if cfg.Routing == nil || len(cfg.Routing.Providers["anthropic"]) == 0 {
		t.Fatal("expected anthropic routing")
	}
}
