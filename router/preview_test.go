package router

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func TestResolveRoutingModelOverride(t *testing.T) {
	t.Parallel()
	c := catalog.SeedCatalog()
	compiled, err := catalog.CompileCatalog(&c)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	policy := RoutingPolicy{
		Models: map[string][]RoutingStage{
			"openai/gpt-4.1-2025-04-14": {{
				Deployments: []DeploymentChoice{{DeploymentID: "openai-azure", Weight: 100}},
				Retries:     2,
			}},
		},
		Providers: map[string][]RoutingStage{
			"openai": {{
				Deployments: []DeploymentChoice{{DeploymentID: "openai-direct", Weight: 100}},
			}},
		},
	}
	res := ResolveRouting("openai/gpt-4.1-2025-04-14", compiled, policy)
	if res.Source != "models" {
		t.Fatalf("source = %q, want models", res.Source)
	}
	if len(res.Stages) != 1 || res.Stages[0].Deployments[0].DeploymentID != "openai-azure" {
		t.Fatalf("unexpected stages: %#v", res.Stages)
	}
}

func TestResolveRoutingProviderFallback(t *testing.T) {
	t.Parallel()
	c := catalog.SeedCatalog()
	compiled, err := catalog.CompileCatalog(&c)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	policy := RoutingPolicy{
		Providers: map[string][]RoutingStage{
			"anthropic": {{
				Deployments: []DeploymentChoice{
					{DeploymentID: "anthropic-direct", Weight: 70},
					{DeploymentID: "anthropic-bedrock", Weight: 30},
				},
				Retries: 1,
			}},
		},
	}
	res := ResolveRouting("anthropic/claude-sonnet-4-6", compiled, policy)
	if res.Source != "providers" {
		t.Fatalf("source = %q, want providers", res.Source)
	}
	if len(res.Stages[0].Deployments) != 2 {
		t.Fatalf("expected 2 deployment choices, got %#v", res.Stages[0].Deployments)
	}
}
