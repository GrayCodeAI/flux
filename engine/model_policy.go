package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/llm"
)

func (e *Engine) policyCatalog(ctx context.Context) (*catalog.CompiledCatalog, error) {
	compiled, err := catalog.LoadCatalog(nonNilContext(ctx), catalog.LoadCatalogOptions{CachePath: e.catalogPath})
	if err != nil {
		return nil, &Error{Code: ErrorCatalogUnavailable, Operation: "model_policy", Message: err.Error(), Cause: err}
	}
	return compiled, nil
}

// ModelInfo returns normalized metadata for a model id or alias.
func (e *Engine) ModelInfo(ctx context.Context, modelID string) (Model, bool, error) {
	if gateway, ok := e.customGatewayForModel(modelID); ok {
		return customGatewayModel(gateway), true, nil
	}
	compiled, err := e.policyCatalog(ctx)
	if err != nil {
		return Model{}, false, err
	}
	canonical, ok := compiled.CanonicalModelForAliasOrID(strings.TrimSpace(modelID))
	if !ok {
		return Model{}, false, nil
	}
	for _, model := range snapshotFromCompiled(compiled).Models {
		if model.ID == canonical {
			return model, true, nil
		}
	}
	return Model{}, false, nil
}

// ModelProviders lists canonical catalog model owners and invocation-scoped
// custom gateway owners.
func (e *Engine) ModelProviders(ctx context.Context) ([]string, error) {
	compiled, err := e.policyCatalog(ctx)
	if err != nil && len(e.customGateways) == 0 {
		return nil, err
	}
	seen := map[string]bool{}
	catalogProviders := catalog.AllModelProviders(compiled)
	providers := make([]string, 0, len(e.customGateways)+len(catalogProviders))
	for _, provider := range catalogProviders {
		provider = strings.TrimSpace(provider)
		if provider != "" && !seen[provider] {
			seen[provider] = true
			providers = append(providers, provider)
		}
	}
	for _, gateway := range e.orderedCustomGateways() {
		if !seen[gateway.ID] {
			seen[gateway.ID] = true
			providers = append(providers, gateway.ID)
		}
	}
	sort.Strings(providers)
	return providers, nil
}

// DefaultModel returns the catalog default for a provider.
func (e *Engine) DefaultModel(ctx context.Context, provider, fallback string) string {
	if gateway, ok := e.customGateway(provider); ok {
		if gateway.DefaultModel != "" {
			return gateway.DefaultModel
		}
		return fallback
	}
	compiled, err := e.policyCatalog(ctx)
	if err != nil {
		return fallback
	}
	return catalog.ProviderDefaultModel(compiled, provider, fallback)
}

// PreferredModel returns a provider model in the requested relative class.
func (e *Engine) PreferredModel(ctx context.Context, provider string, class ModelClass, fallback string) string {
	if gateway, ok := e.customGateway(provider); ok {
		if gateway.DefaultModel != "" {
			return gateway.DefaultModel
		}
		return fallback
	}
	compiled, err := e.policyCatalog(ctx)
	if err != nil {
		return fallback
	}
	return catalog.PreferredProviderModel(compiled, provider, catalogTier(class), fallback)
}

// PreferredModels returns unique cross-provider candidates in preference order.
func (e *Engine) PreferredModels(ctx context.Context, primaryProvider string, class ModelClass, limit int) []string {
	compiled, err := e.policyCatalog(ctx)
	if err != nil {
		compiled = nil
	}
	seen := map[string]bool{}
	models := make([]string, 0, len(e.customGateways)+8)
	add := func(model string) bool {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return false
		}
		seen[model] = true
		models = append(models, model)
		return limit > 0 && len(models) >= limit
	}

	primaryCustom, isPrimaryCustom := e.customGateway(primaryProvider)
	if isPrimaryCustom && add(primaryCustom.DefaultModel) {
		return models
	}
	if !isPrimaryCustom {
		for _, model := range catalog.PreferredModelsForTier(compiled, primaryProvider, catalogTier(class), limit) {
			if add(model) {
				return models
			}
		}
	}
	for _, gateway := range e.orderedCustomGateways() {
		if isPrimaryCustom && gateway.ID == primaryCustom.ID {
			continue
		}
		if add(gateway.DefaultModel) {
			return models
		}
	}
	if isPrimaryCustom {
		for _, model := range catalog.PreferredModelsForTier(compiled, "", catalogTier(class), limit) {
			if add(model) {
				return models
			}
		}
	}
	return models
}

// ModelClassOf resolves a model's relative catalog cost band.
func (e *Engine) ModelClassOf(ctx context.Context, modelID string) ModelClass {
	compiled, _ := e.policyCatalog(ctx)
	switch catalog.ModelCostTierOf(compiled, modelID) {
	case catalog.CostTierCheap:
		return llm.ModelClassEconomical
	case catalog.CostTierExpensive:
		return llm.ModelClassPremium
	default:
		return llm.ModelClassBalanced
	}
}

// ProviderForModel returns the canonical catalog owner or invocation-scoped
// custom gateway owner for a model.
func (e *Engine) ProviderForModel(ctx context.Context, modelID string) string {
	if gateway, ok := e.customGatewayForModel(modelID); ok {
		return gateway.ID
	}
	compiled, _ := e.policyCatalog(ctx)
	return catalog.ProviderForModel(compiled, modelID)
}

// PrimaryModel returns GraycodeRouter's stable best-effort catalog primary model.
func (e *Engine) PrimaryModel(ctx context.Context) string {
	compiled, _ := e.policyCatalog(ctx)
	if model := catalog.PrimaryModel(compiled); model != "" {
		return model
	}
	for _, gateway := range e.orderedCustomGateways() {
		if gateway.DefaultModel != "" {
			return gateway.DefaultModel
		}
	}
	return ""
}

// ModelNames returns canonical ids, display names, and aliases for completion.
func (e *Engine) ModelNames(ctx context.Context) []string {
	compiled, err := e.policyCatalog(ctx)
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	for _, gateway := range e.orderedCustomGateways() {
		add(gateway.DefaultModel)
	}
	if err == nil && compiled != nil {
		for id, model := range compiled.ModelsByID {
			add(id)
			add(model.Name)
		}
		if compiled.Catalog != nil {
			for alias, canonical := range compiled.Catalog.Aliases {
				add(alias)
				add(canonical)
			}
		}
	}
	sort.Strings(out)
	return out
}

// orderedCustomGateways returns the immutable per-Engine gateway snapshot in
// routing preference order. Map iteration must never decide which custom
// gateway owns a model ID shared by multiple invocation-scoped gateways.
func (e *Engine) orderedCustomGateways() []CustomGateway {
	gateways := make([]CustomGateway, 0, len(e.customGateways))
	for _, gateway := range e.customGateways {
		gateways = append(gateways, cloneCustomGateway(gateway))
	}
	sort.SliceStable(gateways, func(i, j int) bool {
		if gateways[i].ChatPreference != gateways[j].ChatPreference {
			return gateways[i].ChatPreference < gateways[j].ChatPreference
		}
		if gateways[i].SortOrder != gateways[j].SortOrder {
			return gateways[i].SortOrder < gateways[j].SortOrder
		}
		return gateways[i].ID < gateways[j].ID
	})
	return gateways
}

func (e *Engine) customGatewayForModel(modelID string) (CustomGateway, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return CustomGateway{}, false
	}
	for _, gateway := range e.orderedCustomGateways() {
		if gateway.DefaultModel == modelID {
			return gateway, true
		}
	}
	return CustomGateway{}, false
}

func customGatewayModel(gateway CustomGateway) Model {
	m := Model{
		ID: gateway.DefaultModel, CanonicalID: gateway.DefaultModel, DisplayName: gateway.DefaultModel,
		Owner: gateway.DisplayName, ProviderID: gateway.ID, GatewayID: gateway.ID,
		ContextWindow: gateway.ContextWindow, Capabilities: customGatewayCapabilityNames(gateway), Source: "custom",
	}
	applyProviderThinkingDefaults(&m)
	return m
}

func catalogTier(class ModelClass) catalog.ModelTier {
	switch class {
	case llm.ModelClassEconomical:
		return catalog.TierHaiku
	case llm.ModelClassPremium:
		return catalog.TierOpus
	default:
		return catalog.TierSonnet
	}
}
