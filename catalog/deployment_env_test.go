package catalog

import (
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

func TestDefaultDeploymentEnvFallbacks_HasAllProviderDeployments(t *testing.T) {
	t.Parallel()
	fbs := DefaultDeploymentEnvFallbacks
	for _, spec := range registry.All() {
		if _, ok := fbs[spec.DeploymentID]; !ok {
			t.Errorf("missing deployment %q in DefaultDeploymentEnvFallbacks", spec.DeploymentID)
		}
	}
}

func TestDefaultDeploymentEnvFallbacks_ExtraDeployments(t *testing.T) {
	t.Parallel()
	fbs := DefaultDeploymentEnvFallbacks
	extras := []string{"anthropic-bedrock", "anthropic-vertex", "openai-azure", "gemini-vertex"}
	for _, id := range extras {
		if _, ok := fbs[id]; !ok {
			t.Errorf("missing extra deployment %q in DefaultDeploymentEnvFallbacks", id)
		}
	}
}

func TestDefaultDeploymentEnvFallbacks_GrokHasXAIAPIKey(t *testing.T) {
	t.Parallel()
	fbs := DefaultDeploymentEnvFallbacks
	grok, ok := fbs["grok-direct"]
	if !ok {
		t.Fatal("grok-direct not found in env fallbacks")
	}
	hasXAI := false
	for _, fb := range grok {
		if fb.Field == "api_key" {
			for _, env := range fb.Env {
				if env == "XAI_API_KEY" {
					hasXAI = true
				}
			}
		}
	}
	if !hasXAI {
		t.Error("grok-direct api_key fallbacks should include XAI_API_KEY")
	}
}

func TestDefaultDeploymentEnvFallbacks_ZAIHasZAIAPIBase(t *testing.T) {
	t.Parallel()
	fbs := DefaultDeploymentEnvFallbacks
	zai, ok := fbs["zai_payg-direct"]
	if !ok {
		t.Fatal("zai_payg-direct not found in env fallbacks")
	}
	hasZAIAPIBase := false
	for _, fb := range zai {
		if fb.Field == "base_url" {
			for _, env := range fb.Env {
				if env == "ZAI_API_BASE" {
					hasZAIAPIBase = true
				}
			}
		}
	}
	if !hasZAIAPIBase {
		t.Error("zai_payg-direct base_url fallbacks should include ZAI_API_BASE")
	}
}

func TestEnsureDeploymentEnvFallbacks(t *testing.T) {
	t.Parallel()
	c := &Catalog{
		Deployments: map[string]Deployment{
			"anthropic-direct": {ID: "anthropic-direct"},
			"unknown-dep":      {ID: "unknown-dep"},
		},
	}
	EnsureDeploymentEnvFallbacks(c)
	dep, ok := c.Deployments["anthropic-direct"]
	if !ok {
		t.Fatal("anthropic-direct should exist")
	}
	if len(dep.EnvFallbacks) == 0 {
		t.Error("anthropic-direct should have env fallbacks after EnsureDeploymentEnvFallbacks")
	}
	unknown, ok := c.Deployments["unknown-dep"]
	if !ok {
		t.Fatal("unknown-dep should exist")
	}
	if len(unknown.EnvFallbacks) != 0 {
		t.Error("unknown-dep should have no env fallbacks")
	}
}

func TestEnvVarsForDeployment(t *testing.T) {
	t.Parallel()
	vars := EnvVarsForDeployment("anthropic-direct")
	if len(vars) == 0 {
		t.Fatal("anthropic-direct should have env vars")
	}
	found := false
	for _, v := range vars {
		if v == "ANTHROPIC_API_KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("anthropic-direct env vars should include ANTHROPIC_API_KEY")
	}

	vars = EnvVarsForDeployment("nonexistent")
	if vars != nil {
		t.Error("nonexistent deployment should return nil")
	}
}
