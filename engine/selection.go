package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/runtime"
)

// NormalizeProviderID resolves provider aliases to GraycodeRouter's runtime identifier.
func NormalizeProviderID(providerID string) string {
	return runtime.NormalizeProviderID(providerID)
}

// EffectiveSelection resolves persisted state plus optional host overrides
// using this Engine's injected catalog, provider config, and credential store.
func (e *Engine) EffectiveSelection(ctx context.Context, opts SelectionOptions) Selection {
	ctx = nonNilContext(ctx)
	active := e.ActiveSelection(ctx)
	provider := NormalizeProviderID(active.Provider)
	model := strings.TrimSpace(active.Model)
	hasProviderOverride := false
	if override := NormalizeProviderID(opts.ProviderOverride); override != "" {
		if override != provider && strings.TrimSpace(opts.ModelOverride) == "" {
			model = ""
		}
		provider = override
		hasProviderOverride = true
	}
	if override := strings.TrimSpace(opts.ModelOverride); override != "" {
		model = override
	}

	compiled, _ := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath})
	if provider == "" && model != "" && compiled != nil {
		provider = NormalizeProviderID(catalog.GatewayForModel(compiled, model))
		if provider == "" {
			provider = NormalizeProviderID(catalog.ProviderForModel(compiled, model))
		}
	}

	gateways := e.Gateways(ctx)
	hasConfigured := false
	configured := make(map[string]bool, len(gateways))
	for _, gateway := range gateways {
		ready := gateway.DeploymentConfigured
		configured[NormalizeProviderID(gateway.ID)] = ready
		if ready {
			hasConfigured = true
		}
	}
	if provider == "" && hasConfigured {
		provider = preferredConfiguredGateway(gateways, configured)
	}
	if provider != "" && hasConfigured && !configured[provider] && !hasProviderOverride {
		provider = preferredConfiguredGateway(gateways, configured)
		model = ""
	}
	if provider != "" && model == "" {
		if models, err := e.ListModels(ctx, provider, false); err == nil && len(models) > 0 {
			model = models[0].ID
		}
	}
	if _, custom := e.customGateway(provider); !custom && compiled != nil && model != "" {
		if canonical, ok := catalog.CanonicalModelForProviderNative(compiled, provider, model); ok {
			model = canonical
		} else if canonical, ok := compiled.CanonicalModelForAliasOrID(model); ok {
			model = canonical
		}
	}

	routing := active.DeploymentRouting
	if opts.DeploymentRoutingOverride != nil {
		routing = *opts.DeploymentRoutingOverride
	}
	return Selection{
		Provider: provider, Model: model,
		HasConfiguredDeployment: hasConfigured,
		DeploymentRouting:       routing,
	}
}

func preferredConfiguredGateway(gateways []Gateway, configured map[string]bool) string {
	bestID, bestRank := "", int(^uint(0)>>1)
	for _, gateway := range gateways {
		providerID := NormalizeProviderID(gateway.ID)
		if !configured[providerID] {
			continue
		}
		rank := gateway.ChatPreference
		if rank <= 0 {
			rank = gateway.SortOrder + 10_000
		}
		if rank < bestRank || (rank == bestRank && (bestID == "" || providerID < bestID)) {
			bestID, bestRank = providerID, rank
		}
	}
	return bestID
}

// ActiveSelection reads the persisted host selection from this Engine's state.
func (e *Engine) ActiveSelection(ctx context.Context) Route {
	_ = nonNilContext(ctx)
	cfg := config.LoadProviderConfig(e.providerConfigPath)
	return Route{
		Provider:          NormalizeProviderID(config.ActiveProvider(cfg)),
		Model:             config.ActiveModel(cfg),
		DeploymentRouting: cfg != nil && (cfg.ConfigVersion >= 2 || len(cfg.Deployments) > 0 || cfg.Routing != nil),
	}
}

// SetActiveProvider persists a provider choice without requiring a model.
func (e *Engine) SetActiveProvider(ctx context.Context, providerID string) error {
	ctx = nonNilContext(ctx)
	providerID = NormalizeProviderID(providerID)
	if providerID == "" {
		return invalid("set_active_provider", "graycode-router engine: provider id is required")
	}
	unlock := lockProviderStatePath(e.providerConfigPath)
	defer unlock()
	cfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return &Error{Code: ErrorInternal, Operation: "set_active_provider", Provider: providerID, Message: err.Error(), Cause: err}
	}
	config.SetActiveProvider(cfg, providerID)
	if err := e.saveProviderConfig(ctx, cfg); err != nil {
		return &Error{Code: ErrorInternal, Operation: "set_active_provider", Provider: providerID, Message: err.Error(), Cause: err}
	}
	return nil
}

// SetActiveModel persists a model choice and infers its serving provider from
// the configured catalog.
func (e *Engine) SetActiveModel(ctx context.Context, modelID string) error {
	return e.SetSelection(ctx, "", modelID)
}

// SetSelection persists the host/user's provider and model choice in this
// Engine's configured state path. It does not persist credentials.
func (e *Engine) SetSelection(ctx context.Context, providerID, modelID string) error {
	ctx = nonNilContext(ctx)
	providerID = NormalizeProviderID(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return invalid("set_selection", "graycode-router engine: model id is required")
	}
	unlock := lockProviderStatePath(e.providerConfigPath)
	defer unlock()
	cfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return &Error{Code: ErrorInternal, Operation: "set_selection", Provider: providerID, Model: modelID, Message: err.Error(), Cause: err}
	}
	activeProvider := NormalizeProviderID(config.ActiveProvider(cfg))
	_, custom := e.customGateway(providerID)
	if providerID == "" {
		if gateway, ok := e.customGatewayForModel(modelID); ok {
			providerID, custom = gateway.ID, true
		} else if _, ok := e.customGateway(activeProvider); ok {
			providerID, custom = activeProvider, true
		}
	}
	if !custom {
		compiled, catalogErr := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
		if catalogErr != nil {
			return &Error{Code: ErrorCatalogUnavailable, Operation: "set_selection", Provider: providerID, Model: modelID, Message: catalogErr.Error(), Cause: catalogErr}
		}
		if providerID != "" {
			canonical, ok := canonicalSelectionModel(compiled, providerID, modelID)
			if !ok {
				return selectionModelUnavailable(providerID, modelID)
			}
			modelID = canonical
		} else {
			if activeProvider != "" {
				if canonical, ok := canonicalSelectionModel(compiled, activeProvider, modelID); ok {
					providerID, modelID = activeProvider, canonical
				}
			}
			if providerID == "" {
				canonical, ok := compiled.CanonicalModelForAliasOrID(modelID)
				if !ok {
					return selectionModelUnavailable("", modelID)
				}
				modelID = canonical
				providerID = NormalizeProviderID(catalog.GatewayForModel(compiled, modelID))
				if providerID == "" {
					providerID = NormalizeProviderID(catalog.ProviderForModel(compiled, modelID))
				}
				if providerID == "" {
					return selectionModelUnavailable("", modelID)
				}
			}
		}
	}
	config.SetProviderModel(cfg, providerID, modelID)
	if err := e.saveProviderConfig(ctx, cfg); err != nil {
		return &Error{Code: ErrorInternal, Operation: "set_selection", Provider: providerID, Model: modelID, Message: err.Error(), Cause: err}
	}
	return nil
}

func canonicalSelectionModel(compiled *catalog.CompiledCatalog, providerID, modelID string) (string, bool) {
	if canonical, ok := catalog.CanonicalModelForProviderNative(compiled, providerID, modelID); ok {
		return canonical, true
	}
	canonical, ok := compiled.CanonicalModelForAliasOrID(modelID)
	if !ok {
		return "", false
	}
	if !providerModelAvailable(compiled, providerID, canonical) {
		return "", false
	}
	return canonical, true
}

func selectionModelUnavailable(providerID, modelID string) error {
	message := fmt.Sprintf("graycode-router engine: model %q is not available", modelID)
	if providerID != "" {
		message = fmt.Sprintf("graycode-router engine: model %q is not available through %q", modelID, providerID)
	}
	return &Error{Code: ErrorModelUnavailable, Operation: "set_selection", Provider: providerID, Model: modelID, Message: message}
}

// ClearSelection removes active provider/model state without modifying
// credentials, deployments, or routing.
func (e *Engine) ClearSelection(ctx context.Context) error {
	ctx = nonNilContext(ctx)
	unlock := lockProviderStatePath(e.providerConfigPath)
	defer unlock()
	cfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return &Error{Code: ErrorInternal, Operation: "clear_selection", Message: err.Error(), Cause: err}
	}
	config.ClearActiveSelection(cfg)
	if err := e.saveProviderConfig(ctx, cfg); err != nil {
		return &Error{Code: ErrorInternal, Operation: "clear_selection", Message: err.Error(), Cause: err}
	}
	return nil
}
