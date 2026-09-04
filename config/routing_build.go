package config

import (
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// BuildRoutingPolicyFromDeployments builds deployment routing from configured deployments.
// Hawk should not author routing rules — consume this JSON from graycode-router only.
func BuildRoutingPolicyFromDeployments(deployments map[string]DeploymentConfig) *RoutingPolicy {
	if len(deployments) == 0 {
		return &RoutingPolicy{}
	}
	policy := &RoutingPolicy{
		Providers: map[string][]RoutingStage{},
		Models:    map[string][]RoutingStage{},
	}
	if _, ok := deployments["openrouter"]; ok {
		policy.Default = []RoutingStage{{
			Deployments: []DeploymentChoice{{DeploymentID: "openrouter", Weight: 100}},
			Retries:     1,
		}}
	}
	if stages := openAIProviderStages(deployments); len(stages) > 0 {
		policy.Providers["openai"] = stages
	}
	if stages := agnesProviderStages(deployments); len(stages) > 0 {
		policy.Providers["agnes"] = stages
	}
	if stages := longcatProviderStages(deployments); len(stages) > 0 {
		policy.Providers["longcat"] = stages
	}
	if stages := anthropicProviderStages(deployments); len(stages) > 0 {
		policy.Providers["anthropic"] = stages
	}
	if stages := googleProviderStages(deployments); len(stages) > 0 {
		policy.Providers["google"] = stages
	}
	if stages := grokProviderStages(deployments); len(stages) > 0 {
		policy.Providers["xai"] = stages
	}
	for id := range deployments {
		provider := deploymentOwnerProviderID(id)
		if provider == "" || policy.Providers[provider] != nil {
			continue
		}
		policy.Providers[provider] = singleDeploymentStages(id, 1)
	}
	return policy
}

func openAIProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	_, direct := deployments["openai-direct"]
	_, azure := deployments["openai-azure"]
	var stages []RoutingStage
	switch {
	case direct && azure:
		stages = append(stages, RoutingStage{
			Deployments: []DeploymentChoice{
				{DeploymentID: "openai-direct", Weight: 70},
				{DeploymentID: "openai-azure", Weight: 30},
			},
			Retries: 1,
		})
	case direct:
		stages = append(stages, singleDeploymentStages("openai-direct", 1)...)
	case azure:
		stages = append(stages, singleDeploymentStages("openai-azure", 1)...)
	default:
		return nil
	}
	return stages
}

func anthropicProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	var stages []RoutingStage
	if _, ok := deployments["anthropic-bedrock"]; ok {
		stages = append(stages, RoutingStage{
			Deployments: []DeploymentChoice{{DeploymentID: "anthropic-bedrock", Weight: 100}},
			Retries:     2,
		})
	}
	if _, ok := deployments["anthropic-direct"]; ok {
		stages = append(stages, RoutingStage{
			Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}},
			Retries:     1,
		})
	}
	if len(stages) == 0 {
		if _, ok := deployments["anthropic-vertex"]; ok {
			stages = append(stages, singleDeploymentStages("anthropic-vertex", 1)...)
		}
	}
	if len(stages) == 0 {
		return nil
	}
	return stages
}

func agnesProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	if _, ok := deployments["agnes-direct"]; !ok {
		return nil
	}
	// Agnes is OpenAI-compatible only: one deployment, one protocol.
	return singleDeploymentStages("agnes-direct", 1)
}

func longcatProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	if _, ok := deployments["longcat-direct"]; !ok {
		return nil
	}
	// Single OpenAI-compatible endpoint only (longcat-direct).
	// Official LongCat also documents /anthropic; hawk does not require it when OpenAI works.
	return singleDeploymentStages("longcat-direct", 1)
}

func googleProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	if _, ok := deployments["gemini-direct"]; ok {
		return singleDeploymentStages("gemini-direct", 1)
	}
	if _, ok := deployments["gemini-vertex"]; ok {
		return singleDeploymentStages("gemini-vertex", 1)
	}
	return nil
}

func grokProviderStages(deployments map[string]DeploymentConfig) []RoutingStage {
	if _, ok := deployments["grok-direct"]; ok {
		return singleDeploymentStages("grok-direct", 1)
	}
	return nil
}

func deploymentOwnerProviderID(deploymentID string) string {
	switch deploymentID {
	case "anthropic-direct", "anthropic-bedrock", "anthropic-vertex":
		return "anthropic"
	case "openai-direct", "openai-azure":
		return "openai"
	case "gemini-direct", "gemini-vertex":
		return "google"
	case "grok-direct":
		return "xai"
	case "xiaomi_mimo_payg-direct", "xiaomi_mimo-direct":
		return "xiaomi_mimo_payg"
	default:
		if spec, ok := registry.SpecByDeploymentID(deploymentID); ok {
			return spec.ProviderID
		}
		return ""
	}
}

func singleDeploymentStages(deploymentID string, retries int) []RoutingStage {
	if retries <= 0 {
		retries = 1
	}
	return []RoutingStage{{
		Deployments: []DeploymentChoice{{DeploymentID: deploymentID, Weight: 100}},
		Retries:     retries,
	}}
}
