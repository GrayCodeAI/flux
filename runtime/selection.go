package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/setup"
)

type selectionRuntimeState struct {
	ctx                     context.Context
	compiled                *catalog.CompiledCatalog
	compiledLoaded          bool
	discoveryEnv            map[string]string
	discoveryEnvLoaded      bool
	hasConfiguredDeployment bool
	hasConfiguredLoaded     bool
	providerConfiguredCache map[string]bool
}

func newSelectionRuntimeState(ctx context.Context) *selectionRuntimeState {
	if ctx == nil {
		ctx = context.Background()
	}
	return &selectionRuntimeState{
		ctx:                     ctx,
		providerConfiguredCache: make(map[string]bool),
	}
}

func (s *selectionRuntimeState) compiledCatalog() *catalog.CompiledCatalog {
	if s == nil {
		return nil
	}
	if s.compiledLoaded {
		return s.compiled
	}
	s.compiledLoaded = true
	compiled, err := catalog.LoadCatalogForDiscovery(s.ctx)
	if err == nil {
		s.compiled = compiled
	}
	return s.compiled
}

func (s *selectionRuntimeState) env() map[string]string {
	if s == nil {
		return nil
	}
	if s.discoveryEnvLoaded {
		return s.discoveryEnv
	}
	s.discoveryEnvLoaded = true
	s.discoveryEnv = config.DiscoveryEnvMap(s.ctx)
	return s.discoveryEnv
}

func (s *selectionRuntimeState) hasAnyConfiguredDeployment() bool {
	if s == nil {
		return false
	}
	if s.hasConfiguredLoaded {
		return s.hasConfiguredDeployment
	}
	s.hasConfiguredLoaded = true
	env := s.env()
	compiled := s.compiledCatalog()
	if compiled == nil || compiled.Catalog == nil {
		s.hasConfiguredDeployment = runtimeAnyNonemptyCredentialEnv(env)
		return s.hasConfiguredDeployment
	}
	for id, dep := range compiled.Catalog.Deployments {
		dc := config.DeploymentConfigFromEnv(dep, env)
		if config.DeploymentConfigured(id, dep, dc) {
			s.hasConfiguredDeployment = true
			return true
		}
	}
	return false
}

func runtimeAnyNonemptyCredentialEnv(env map[string]string) bool {
	for _, v := range env {
		if !config.LooksLikePlaceholderSecret(v) {
			return true
		}
	}
	return false
}

// ActiveModel returns the selected model from provider.json.
func ActiveModel(ctx context.Context) string {
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.ActiveModel)
}

// ActiveProvider returns the selected provider from provider.json.
func ActiveProvider(ctx context.Context) string {
	_ = ctx
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		return ""
	}
	return catalog.CanonicalProviderID(strings.TrimSpace(cfg.ActiveProvider))
}

// NormalizeProviderID resolves catalog aliases and host-facing variants to the
// runtime provider identifier used by GraycodeRouter adapters and setup gateways.
func NormalizeProviderID(provider string) string {
	return normalizeRuntimeProviderID(provider)
}

// ActiveProviderID canonicalizes a host-facing provider/gateway id through the
// runtime provider-id rules used by GraycodeRouter.
func ActiveProviderID(provider string) string {
	return NormalizeProviderID(provider)
}

// SelectionState is the engine-resolved provider/model selection for chat.
type SelectionState struct {
	Provider                string `json:"provider"`
	Model                   string `json:"model"`
	HasConfiguredDeployment bool   `json:"has_configured_deployment"`
	DeploymentRouting       bool   `json:"deployment_routing"`
}

// SelectionOpts supplies optional host overrides for provider/model selection.
// Empty overrides mean "use engine-persisted/default selection".
type SelectionOpts struct {
	ProviderOverride string
	ModelOverride    string
	// DeploymentRoutingOverride lets a host force the routing mode while the
	// migration to pure engine-owned policy is in progress.
	DeploymentRoutingOverride *bool
}

// HasConfiguredDeployment reports whether the engine has at least one usable deployment.
func HasConfiguredDeployment(ctx context.Context) bool {
	return config.HasAnyConfiguredDeployment(ctx)
}

// ResolveCanonicalModel maps aliases/native IDs to canonical catalog model IDs.
func ResolveCanonicalModel(ctx context.Context, model string) string {
	rt, err := Load(ctx)
	if err == nil && rt != nil {
		return catalog.ResolveModel(rt.Catalog, model)
	}
	// No runtime available — fall back to trimmed input if it looks native.
	model = strings.TrimSpace(model)
	if strings.Contains(model, "/") {
		return model
	}
	return model
}

// DefaultModelForProvider returns the preferred model for a provider using cache
// first, then live discovery when credentials are configured.
func DefaultModelForProvider(ctx context.Context, provider string) string {
	provider = normalizeRuntimeProviderID(provider)
	if provider == "" {
		return ""
	}
	if rt, err := Load(ctx); err == nil && rt != nil && rt.Catalog != nil {
		if id := catalog.FirstModelForProvider(rt.Catalog, provider); id != "" {
			return id
		}
	}
	if !providerConfigured(ctx, provider) {
		return ""
	}
	models, err := ListModels(ctx, ListModelsOpts{ProviderID: provider, Source: ListSourceAuto})
	if err == nil && len(models) > 0 {
		return strings.TrimSpace(models[0].ID)
	}
	return ""
}

// SyncSelectionWithCredentials clears stale persisted selection when the selected
// gateway no longer has usable credentials.
func SyncSelectionWithCredentials(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	syncSelectionWithCredentials(ctx, newSelectionRuntimeState(ctx))
}

func syncSelectionWithCredentials(ctx context.Context, state *selectionRuntimeState) {
	if !state.hasAnyConfiguredDeployment() {
		if strings.TrimSpace(ActiveModel(ctx)) != "" || strings.TrimSpace(ActiveProvider(ctx)) != "" {
			_ = ClearActiveSelection(ctx)
		}
		return
	}
	gateway := activeGateway(ctx)
	if gateway == "" {
		return
	}
	if !providerConfiguredWithState(state, gateway) {
		_ = ClearActiveSelection(ctx)
	}
}

// EffectiveSelection resolves the provider/model that chat should use after
// applying persisted selection, optional host overrides, credential-aware
// fallback, and optional canonicalization.
func EffectiveSelection(ctx context.Context, opts SelectionOpts) SelectionState {
	if ctx == nil {
		ctx = context.Background()
	}
	stateCache := newSelectionRuntimeState(ctx)
	syncSelectionWithCredentials(ctx, stateCache)
	cfg := config.LoadProviderConfig("")
	state := SelectionState{
		HasConfiguredDeployment: stateCache.hasAnyConfiguredDeployment(),
		DeploymentRouting:       useDeploymentRouting(cfg, opts.DeploymentRoutingOverride),
	}

	provider := normalizeRuntimeProviderID(ActiveProvider(ctx))
	if override := normalizeRuntimeProviderID(opts.ProviderOverride); override != "" {
		provider = override
	}

	model := strings.TrimSpace(ActiveModel(ctx))
	if override := strings.TrimSpace(opts.ModelOverride); override != "" {
		model = override
	}

	if provider == "" && model != "" {
		provider = inferProviderForModel(ctx, model)
	}

	if provider == "" {
		provider = preferredProviderWithState(ctx, stateCache)
	}

	if !state.HasConfiguredDeployment {
		state.Provider = provider
		state.Model = model
		return state
	}

	if provider != "" && !providerConfiguredWithState(stateCache, provider) {
		if detected := normalizeRuntimeProviderID(client.DetectProvider()); detected != "" && providerConfiguredWithState(stateCache, detected) {
			provider = detected
			model = ""
		}
	}

	if provider != "" && model == "" {
		model = DefaultModelForProvider(ctx, provider)
	}
	if state.DeploymentRouting && model != "" {
		model = ResolveCanonicalModel(ctx, model)
	}

	state.Provider = provider
	state.Model = model
	return state
}

// SetActiveModel persists the user's model choice to provider.json.
func SetActiveModel(ctx context.Context, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("runtime: model id required")
	}
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		cfg = &config.ProviderConfig{}
	}
	provider := inferProviderForModel(ctx, modelID)
	if provider == "" {
		provider = config.ActiveProvider(cfg)
	}
	if provider == "" {
		provider = config.DefaultProviderFromConfig(cfg)
	}
	config.SetProviderModel(cfg, provider, modelID)
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return err
	}
	return config.SaveProviderConfig(cfg, path)
}

// SetActiveProvider persists active_provider to provider.json.
func SetActiveProvider(ctx context.Context, provider string) error {
	_ = ctx
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("runtime: provider required")
	}
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		cfg = &config.ProviderConfig{}
	}
	config.SetActiveProvider(cfg, provider)
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return err
	}
	return config.SaveProviderConfig(cfg, path)
}

// ClearActiveSelection removes active provider/model from provider.json.
func ClearActiveSelection(ctx context.Context) error {
	_ = ctx
	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		return nil
	}
	config.ClearActiveSelection(cfg)
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return err
	}
	return config.SaveProviderConfig(cfg, path)
}

func inferProviderForModel(ctx context.Context, modelID string) string {
	rt, err := Load(ctx)
	if err != nil || rt == nil || rt.Catalog == nil {
		if prefix, _, ok := strings.Cut(strings.TrimSpace(modelID), "/"); ok && catalog.IsSetupGateway(prefix) {
			return normalizeRuntimeProviderID(prefix)
		}
		return ""
	}
	if gw := catalog.GatewayForModel(rt.Catalog, modelID); gw != "" {
		return normalizeRuntimeProviderID(gw)
	}
	return ""
}

func activeGateway(ctx context.Context) string {
	if provider := normalizeRuntimeProviderID(ActiveProvider(ctx)); catalog.IsSetupGateway(provider) {
		return provider
	}
	if model := strings.TrimSpace(ActiveModel(ctx)); model != "" {
		return inferProviderForModel(ctx, model)
	}
	return ""
}

func providerConfigured(ctx context.Context, provider string) bool {
	return providerConfiguredWithState(newSelectionRuntimeState(ctx), provider)
}

func providerConfiguredWithState(state *selectionRuntimeState, provider string) bool {
	provider = normalizeRuntimeProviderID(provider)
	if provider == "" {
		return false
	}
	if state != nil {
		if cached, ok := state.providerConfiguredCache[provider]; ok {
			return cached
		}
	}
	spec, ok := registry.SpecByProviderID(provider)
	if !ok {
		return false
	}
	var compiled *catalog.CompiledCatalog
	var env map[string]string
	if state != nil {
		compiled = state.compiledCatalog()
		env = state.env()
	}
	if compiled == nil || compiled.Catalog == nil {
		return false
	}
	dep, ok := compiled.Catalog.Deployments[spec.DeploymentID]
	if !ok {
		return false
	}
	dc := config.DeploymentConfigFromEnv(dep, env)
	configured := config.DeploymentConfigured(spec.DeploymentID, dep, dc)
	if state != nil {
		state.providerConfiguredCache[provider] = configured
	}
	return configured
}

func normalizeRuntimeProviderID(provider string) string {
	base := strings.ToLower(strings.TrimSpace(provider))
	if base == "" {
		return ""
	}
	switch base {
	case "z-ai-payg":
		base = "zai_payg"
	case "z-ai-coding":
		base = "zai_coding"
	case "xiaomi-mimo":
		base = "xiaomi_mimo_payg"
	case "xiaomi-mimo-payg":
		base = "xiaomi_mimo_payg"
	case "xiaomi-mimo-token-plan":
		base = "xiaomi_mimo_token_plan"
	}
	candidates := []string{
		base,
		strings.ReplaceAll(base, "-", "_"),
		strings.ReplaceAll(base, "_", "-"),
		catalog.CanonicalProviderID(base),
		catalog.CanonicalProviderID(strings.ReplaceAll(base, "-", "_")),
		catalog.CanonicalProviderID(strings.ReplaceAll(base, "_", "-")),
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if spec, ok := registry.SpecByProviderID(candidate); ok {
			return spec.ProviderID
		}
	}
	return catalog.CanonicalProviderID(strings.ReplaceAll(base, "-", "_"))
}

func useDeploymentRouting(cfg *config.ProviderConfig, override *bool) bool {
	if override != nil {
		return *override
	}
	return setup.UseDeploymentRouting(cfg)
}
