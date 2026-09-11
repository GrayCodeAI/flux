package router

import (
	"encoding/json"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// RoutingResolution describes which routing policy matched a canonical model.
type RoutingResolution struct {
	RequestedModel   string         `json:"requested_model"`
	CanonicalModelID string         `json:"canonical_model_id"`
	Source           string         `json:"source"` // models, providers, provider_alias, default, automatic, unresolved
	Stages           []RoutingStage `json:"stages"`
}

// ResolveRouting previews routing stages without calling provider APIs.
func ResolveRouting(requested string, compiled *catalog.CompiledCatalog, policy RoutingPolicy) RoutingResolution {
	res := RoutingResolution{
		RequestedModel: requested,
		Stages:         nil,
	}
	if requested == "" {
		res.Source = "unresolved"
		return res
	}
	canonical := resolveCanonicalModelID(requested, compiled)
	res.CanonicalModelID = canonical
	if canonical == "" {
		res.Source = "unresolved"
		return res
	}
	stages, source := resolveRoutingStages(canonical, compiled, policy)
	res.Stages = stages
	res.Source = source
	return res
}

// RoutingPreviewJSON returns indented JSON for CLI / config UI.
func RoutingPreviewJSON(requested string, compiled *catalog.CompiledCatalog, policy RoutingPolicy) (string, error) {
	res := ResolveRouting(requested, compiled, policy)
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func resolveCanonicalModelID(requested string, compiled *catalog.CompiledCatalog) string {
	return catalog.ResolveModel(compiled, requested)
}

func resolveRoutingStages(canonicalModelID string, compiled *catalog.CompiledCatalog, policy RoutingPolicy) ([]RoutingStage, string) {
	if stages, ok := policy.Models[canonicalModelID]; ok && len(stages) > 0 {
		return cloneRoutingStages(stages), "models"
	}
	providerID := ownerProviderID(canonicalModelID)
	if compiled != nil {
		if model := compiled.ModelsByID[canonicalModelID]; model.ID != "" {
			providerID = model.ProviderID
		}
	}
	if providerID != "" {
		if stages, ok := policy.Providers[providerID]; ok && len(stages) > 0 {
			return cloneRoutingStages(stages), "providers"
		}
		for key, stages := range policy.Providers {
			if catalog.CanonicalProviderID(key) == providerID && len(stages) > 0 {
				return cloneRoutingStages(stages), "provider_alias"
			}
		}
	}
	if len(policy.Default) > 0 {
		return cloneRoutingStages(policy.Default), "default"
	}
	return automaticPreviewStages(canonicalModelID, compiled), "automatic"
}

func automaticPreviewStages(canonicalModelID string, compiled *catalog.CompiledCatalog) []RoutingStage {
	if compiled == nil {
		return nil
	}
	seen := map[string]bool{}
	var choices []DeploymentChoice
	for deploymentID := range compiled.DeploymentsByID {
		if seen[deploymentID] {
			continue
		}
		if _, ok := compiled.OfferingForDeployment(canonicalModelID, deploymentID); ok {
			choices = append(choices, DeploymentChoice{DeploymentID: deploymentID, Weight: 100})
			seen[deploymentID] = true
			continue
		}
		for _, tmpl := range compiled.TemplatesByCanonicalModel[canonicalModelID] {
			if tmpl.DeploymentID == deploymentID {
				choices = append(choices, DeploymentChoice{DeploymentID: deploymentID, Weight: 100})
				seen[deploymentID] = true
				break
			}
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return []RoutingStage{{Deployments: choices}}
}

// RoutingStagesFor exposes the active route for a canonical model on a live router.
func (r *DeploymentRouter) RoutingStagesFor(canonicalModelID string) []RoutingStage {
	if r == nil {
		return nil
	}
	return r.routeFor(canonicalModelID)
}
