package catalog

import (
	"sort"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
)

// ModelCostTier is a relative cost band for model selection.
type ModelCostTier int

const (
	// CostTierCheap is appropriate for short, low-risk, or summarization work.
	CostTierCheap ModelCostTier = iota
	// CostTierMid is the default balanced cost band.
	CostTierMid
	// CostTierExpensive is appropriate for complex planning or generation work.
	CostTierExpensive
)

// ModelRole identifies the purpose of a model in a multi-model workflow.
type ModelRole string

const (
	ModelRolePlanner  ModelRole = "planner"
	ModelRoleCoder    ModelRole = "coder"
	ModelRoleReviewer ModelRole = "reviewer"
	ModelRoleCommit   ModelRole = "commit"
)

// ModelRoleAssignments maps workflow roles to concrete model IDs.
type ModelRoleAssignments struct {
	Planner  string `json:"planner,omitempty"`
	Coder    string `json:"coder,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// ProviderDefaultModel returns the provider's first catalog model.
func ProviderDefaultModel(compiled *CompiledCatalog, provider, fallback string) string {
	models := ModelEntriesForProvider(compiled, provider)
	if len(models) == 0 {
		return fallback
	}
	if model := strings.TrimSpace(models[0].ID); model != "" {
		return model
	}
	return fallback
}

// PreferredProviderModel returns the preferred model for provider and tier.
func PreferredProviderModel(compiled *CompiledCatalog, provider string, tier ModelTier, fallback string) string {
	switch tier {
	case TierHaiku:
		return CheapestModelForProvider(compiled, provider, fallback)
	case TierOpus:
		return MostExpensiveModelForProvider(compiled, provider, fallback)
	default:
		return middleModelForProvider(compiled, provider, fallback)
	}
}

// PreferredModelsForTier returns unique preferred models for a tier, starting
// with primaryProvider and then following the registry chat preference order.
func PreferredModelsForTier(compiled *CompiledCatalog, primaryProvider string, tier ModelTier, limit int) []string {
	seenProviders := map[string]bool{}
	seenModels := map[string]bool{}
	providers := make([]string, 0, len(registry.ChatProviderPreferenceOrder())+1)
	if primary := CanonicalProviderID(strings.TrimSpace(primaryProvider)); primary != "" {
		providers = append(providers, primary)
	}
	providers = append(providers, registry.ChatProviderPreferenceOrder()...)
	providers = append(providers, AllModelProviders(compiled)...)

	models := make([]string, 0, len(providers))
	for _, provider := range providers {
		provider = CanonicalProviderID(strings.TrimSpace(provider))
		if provider == "" || seenProviders[provider] {
			continue
		}
		seenProviders[provider] = true
		model := PreferredProviderModel(compiled, provider, tier, "")
		if model == "" || seenModels[model] {
			continue
		}
		seenModels[model] = true
		models = append(models, model)
		if limit > 0 && len(models) >= limit {
			return models
		}
	}
	return models
}

// CheapestModelForProvider returns the lowest known input-priced model for a provider.
func CheapestModelForProvider(compiled *CompiledCatalog, provider, fallback string) string {
	models := ModelEntriesForProvider(compiled, provider)
	if len(models) == 0 {
		return fallback
	}
	best := models[0]
	foundPriced := false
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		if model.InputPricePer1M <= 0 {
			continue
		}
		if !foundPriced || model.InputPricePer1M < best.InputPricePer1M {
			best = model
			foundPriced = true
		}
	}
	if id := strings.TrimSpace(best.ID); id != "" {
		return id
	}
	return fallback
}

// MostExpensiveModelForProvider returns the highest known input-priced model for a provider.
func MostExpensiveModelForProvider(compiled *CompiledCatalog, provider, fallback string) string {
	models := ModelEntriesForProvider(compiled, provider)
	if len(models) == 0 {
		return fallback
	}
	best := models[0]
	for _, model := range models[1:] {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		if model.InputPricePer1M > best.InputPricePer1M {
			best = model
		}
	}
	if id := strings.TrimSpace(best.ID); id != "" {
		return id
	}
	return fallback
}

// ModelCostTierOf resolves a model's cost tier from catalog family and pricing data.
func ModelCostTierOf(compiled *CompiledCatalog, modelName string) ModelCostTier {
	if tier, ok := costTierFromCatalogFamily(compiled, modelName); ok {
		return mapModelTierToCostTier(tier)
	}
	if tier, ok := costTierFromCatalogPricing(compiled, modelName); ok {
		return tier
	}
	return costTierFromModelName(modelName)
}

// ProviderForModel returns the canonical owner provider for modelName.
func ProviderForModel(compiled *CompiledCatalog, modelName string) string {
	if compiled == nil {
		return ""
	}
	canonical := strings.TrimSpace(modelName)
	if resolved, ok := compiled.CanonicalModelForAliasOrID(canonical); ok {
		canonical = resolved
	}
	model := compiled.ModelsByID[canonical]
	return CanonicalProviderID(model.ProviderID)
}

// AllModelProviders lists canonical model owner providers in the catalog.
func AllModelProviders(compiled *CompiledCatalog) []string {
	if compiled == nil {
		return nil
	}
	seen := map[string]bool{}
	providers := make([]string, 0, len(compiled.ModelsByID))
	for _, model := range compiled.ModelsByID {
		provider := CanonicalProviderID(model.ProviderID)
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

// DefaultModelRolesV1 uses the primary model for interactive roles and the cheapest
// same-provider model for commit/summarization work when catalog data is available.
func DefaultModelRolesV1(compiled *CompiledCatalog, primaryModel string) ModelRoleAssignments {
	primaryModel = strings.TrimSpace(primaryModel)
	commit := primaryModel
	if provider := ProviderForModel(compiled, primaryModel); provider != "" {
		commit = CheapestModelForProvider(compiled, provider, primaryModel)
	}
	return ModelRoleAssignments{
		Planner:  primaryModel,
		Coder:    primaryModel,
		Reviewer: primaryModel,
		Commit:   commit,
	}
}

// ModelForRoleV1 resolves a role assignment with coder then catalog fallback.
func ModelForRoleV1(compiled *CompiledCatalog, roles ModelRoleAssignments, role ModelRole) string {
	var model string
	switch role {
	case ModelRolePlanner:
		model = roles.Planner
	case ModelRoleCoder:
		model = roles.Coder
	case ModelRoleReviewer:
		model = roles.Reviewer
	case ModelRoleCommit:
		model = roles.Commit
	}
	if strings.TrimSpace(model) != "" {
		return model
	}
	if strings.TrimSpace(roles.Coder) != "" {
		return roles.Coder
	}
	return PrimaryModel(compiled)
}

// PrimaryModel returns a stable best-effort model from chat-preferred providers.
func PrimaryModel(compiled *CompiledCatalog) string {
	for _, provider := range registry.ChatProviderPreferenceOrder() {
		if model := ProviderDefaultModel(compiled, provider, ""); model != "" {
			return model
		}
	}
	for _, provider := range AllModelProviders(compiled) {
		if model := ProviderDefaultModel(compiled, provider, ""); model != "" {
			return model
		}
	}
	return ""
}

func middleModelForProvider(compiled *CompiledCatalog, provider, fallback string) string {
	models := ModelEntriesForProvider(compiled, provider)
	if len(models) == 0 {
		return fallback
	}
	priced := make([]ModelCatalogEntry, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) != "" && model.InputPricePer1M > 0 {
			priced = append(priced, model)
		}
	}
	if len(priced) == 0 {
		return ProviderDefaultModel(compiled, provider, fallback)
	}
	sort.SliceStable(priced, func(i, j int) bool {
		if priced[i].InputPricePer1M == priced[j].InputPricePer1M {
			return priced[i].ID < priced[j].ID
		}
		return priced[i].InputPricePer1M < priced[j].InputPricePer1M
	})
	return priced[len(priced)/2].ID
}

func costTierFromCatalogFamily(compiled *CompiledCatalog, modelName string) (ModelTier, bool) {
	if compiled == nil {
		return "", false
	}
	canonical := strings.TrimSpace(modelName)
	if resolved, ok := compiled.CanonicalModelForAliasOrID(canonical); ok {
		canonical = resolved
	}
	model := compiled.ModelsByID[canonical]
	if model.ID == "" {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(model.Family)) {
	case "haiku", "cheap", "lite", "flash", "mini":
		return TierHaiku, true
	case "opus", "pro", "max", "heavy", "ultra":
		return TierOpus, true
	case "sonnet", "standard", "balanced", "medium":
		return TierSonnet, true
	default:
		return "", false
	}
}

func costTierFromCatalogPricing(compiled *CompiledCatalog, modelName string) (ModelCostTier, bool) {
	if compiled == nil {
		return 0, false
	}
	canonical := strings.TrimSpace(modelName)
	if resolved, ok := compiled.CanonicalModelForAliasOrID(canonical); ok {
		canonical = resolved
	}
	model := compiled.ModelsByID[canonical]
	if model.ID == "" {
		return 0, false
	}
	offering := firstOfferingForModel(compiled, canonical)
	price := offering.Pricing.RatesPer1M["input_tokens"]
	if price <= 0 {
		return 0, false
	}
	models := ModelEntriesForProvider(compiled, model.ProviderID)
	prices := make([]float64, 0, len(models))
	seen := map[float64]bool{}
	for _, candidate := range models {
		if candidate.InputPricePer1M <= 0 || seen[candidate.InputPricePer1M] {
			continue
		}
		seen[candidate.InputPricePer1M] = true
		prices = append(prices, candidate.InputPricePer1M)
	}
	if len(prices) < 2 {
		return 0, false
	}
	sort.Float64s(prices)
	switch {
	case price <= prices[0]:
		return CostTierCheap, true
	case price >= prices[len(prices)-1]:
		return CostTierExpensive, true
	default:
		return CostTierMid, true
	}
}

func mapModelTierToCostTier(tier ModelTier) ModelCostTier {
	switch tier {
	case TierHaiku:
		return CostTierCheap
	case TierOpus:
		return CostTierExpensive
	default:
		return CostTierMid
	}
}

func costTierFromModelName(modelName string) ModelCostTier {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	for _, pattern := range cheapModelNamePatterns {
		if strings.Contains(lower, pattern) {
			return CostTierCheap
		}
	}
	for _, pattern := range expensiveModelNamePatterns {
		if strings.Contains(lower, pattern) {
			return CostTierExpensive
		}
	}
	return CostTierMid
}

var (
	cheapModelNamePatterns     = []string{"haiku", "mini", "flash", "lite", "nano", "micro", "small", "tiny"}
	expensiveModelNamePatterns = []string{"opus", "pro", "max", "ultra", "heavy", "large", "o1", "o3"}
)
