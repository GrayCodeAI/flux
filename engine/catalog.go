package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/discover"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/config"
)

// Catalog returns the current cached catalog through a stable snapshot.
func (e *Engine) Catalog(ctx context.Context) (CatalogSnapshot, error) {
	ctx = nonNilContext(ctx)
	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
	if err != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorCatalogUnavailable, Operation: "catalog", Message: err.Error(), Cause: err}
	}
	snapshot := snapshotFromCompiled(compiled)
	snapshot.CachePath = e.catalogPath
	return snapshot, nil
}

// RefreshCatalog refreshes one provider when providerID is non-empty, or all
// configured providers otherwise. Credentials are read from this Engine's
// injected store rather than a package-global store.
func (e *Engine) RefreshCatalog(ctx context.Context, providerID string) (CatalogSnapshot, error) {
	ctx = nonNilContext(ctx)
	compiled, _ := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath})
	creds, credentialErr := e.discoveryCredentials(ctx, compiled)
	if credentialErr != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorInternal, Operation: "refresh_catalog", Provider: providerID, Message: credentialErr.Error(), Cause: credentialErr}
	}
	var (
		result *catalog.RefreshResult
		err    error
	)
	if providerID = strings.TrimSpace(providerID); providerID != "" {
		spec, ok := registry.SpecByProviderID(providerID)
		if !ok {
			return CatalogSnapshot{}, &Error{Code: ErrorInvalidRequest, Operation: "refresh_catalog", Provider: providerID, Message: "eyrie engine: unknown provider"}
		}
		if strings.TrimSpace(spec.LiveFetcherKey) != "" {
			result, err = discover.RefreshProviderWithOptions(ctx, providerID, discover.ProviderRefreshOptions{
				Credentials: creds, CachePath: e.catalogPath, DisableCredentialFallback: true,
			})
		} else {
			result, err = discover.Run(ctx, discover.Options{
				LoadCatalogOptions: catalog.LoadCatalogOptions{CachePath: e.catalogPath, RefreshRemote: true, RemoteURL: e.remoteCatalogURL},
				Credentials:        creds, DisableCredentialFallback: true,
			})
		}
	} else {
		result, err = discover.Run(ctx, discover.Options{
			LoadCatalogOptions: catalog.LoadCatalogOptions{CachePath: e.catalogPath, RefreshRemote: true, RemoteURL: e.remoteCatalogURL},
			Credentials:        creds, DisableCredentialFallback: true,
		})
	}
	if err != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorCatalogUnavailable, Operation: "refresh_catalog", Provider: providerID, Message: err.Error(), Cause: err}
	}
	if result == nil || result.Compiled == nil {
		return CatalogSnapshot{}, &Error{Code: ErrorCatalogUnavailable, Operation: "refresh_catalog", Provider: providerID, Message: "eyrie engine: catalog refresh returned no compiled catalog"}
	}
	snapshot := snapshotFromCompiled(result.Compiled)
	snapshot.CachePath = e.catalogPath
	return snapshot, nil
}

// ApplyCredentials refreshes catalog state and persists sanitized deployment
// routing derived from the Engine's credential store. Secret fields are never
// written to provider.json.
func (e *Engine) ApplyCredentials(ctx context.Context, providerID string) (CatalogSnapshot, error) {
	ctx = nonNilContext(ctx)
	snapshot, err := e.RefreshCatalog(ctx, providerID)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
	if err != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorCatalogUnavailable, Operation: "apply_credentials", Provider: providerID, Message: err.Error(), Cause: err}
	}
	unlock := lockProviderStatePath(e.providerConfigPath)
	defer unlock()
	persisted, err := e.loadProviderConfigStrict()
	if err != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorInternal, Operation: "apply_credentials", Provider: providerID, Message: err.Error(), Cause: err}
	}
	deployments := buildDeployments(compiled, persisted.Deployments, e.discoveryCredentialsFromConfig(ctx, compiled, persisted).Env())
	persisted.ConfigVersion = 2
	persisted.Deployments = deployments
	persisted.Routing = config.BuildRoutingPolicyFromDeployments(deployments)
	if err := e.saveProviderConfig(ctx, persisted); err != nil {
		return CatalogSnapshot{}, &Error{Code: ErrorInternal, Operation: "apply_credentials", Provider: providerID, Message: err.Error(), Cause: err}
	}
	return snapshot, nil
}

func snapshotFromCompiled(compiled *catalog.CompiledCatalog) CatalogSnapshot {
	snapshot := CatalogSnapshot{CachePath: catalog.DefaultCachePath(), LoadedAt: time.Now().UTC()}
	if compiled == nil {
		return snapshot
	}
	ids := make([]string, 0, len(compiled.ModelsByID))
	for id := range compiled.ModelsByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		model := compiled.ModelsByID[id]
		offering := firstOffering(compiled.OfferingsByCanonicalModel[id])
		inputPrice, outputPrice := 0.0, 0.0
		if offering.Pricing.RatesPer1M != nil {
			inputPrice = offering.Pricing.RatesPer1M["input_tokens"]
			outputPrice = offering.Pricing.RatesPer1M["output_tokens"]
		}
		m := Model{
			ID: id, CanonicalID: id, DisplayName: catalog.DisplayModelLabel(id, model.Name),
			Description: model.Name,
			Owner:       catalog.DisplayModelOwner(model.ProviderID, id), ProviderID: model.ProviderID,
			GatewayID:     NormalizeProviderID(catalog.GatewayForModel(compiled, id)),
			ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutput,
			InputPricePer1M: inputPrice, OutputPricePer1M: outputPrice,
			PriceKnown:   modelPriceKnown(id, model.Name, inputPrice, outputPrice, model.ContextWindow),
			Capabilities: capabilityNames(offering.Capabilities), Source: "catalog",
			LiveMetadata: append([]byte(nil), offering.LiveMetadata...),
		}
		applyProviderThinkingDefaults(&m)
		snapshot.Models = append(snapshot.Models, m)
	}
	if compiled.Catalog != nil && !compiled.Catalog.StaleAfter.IsZero() {
		snapshot.Stale = time.Now().UTC().After(compiled.Catalog.StaleAfter)
	}
	return snapshot
}

// ListPublicModels returns provider-published model metadata that does not
// require credentials. It never mutates the Engine catalog cache.
func (e *Engine) ListPublicModels(ctx context.Context, providerID string) ([]Model, error) {
	return listPublicModels(nonNilContext(ctx), providerID, "")
}

// ListLiveModels queries one provider directly without reading from or writing
// to the catalog cache for its result set. Credentials and routing metadata
// come exclusively from this Engine's injected state.
func (e *Engine) ListLiveModels(ctx context.Context, providerID string) ([]Model, error) {
	ctx = nonNilContext(ctx)
	providerID = NormalizeProviderID(providerID)
	if providerID == "" {
		return nil, invalid("list_live_models", "eyrie engine: provider id is required")
	}
	if _, custom := e.customGateway(providerID); custom {
		return nil, invalid("list_live_models", "eyrie engine: custom gateway live model listing is not supported")
	}
	compiled, _ := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath})
	creds, err := e.discoveryCredentials(ctx, compiled)
	if err != nil {
		return nil, &Error{Code: ErrorInternal, Operation: "list_live_models", Provider: providerID, Message: err.Error(), Cause: err}
	}
	entries, err := catalog.FetchLiveModelEntriesForProvider(creds.Env(), providerID)
	if err != nil {
		return nil, &Error{Code: ErrorProviderUnavailable, Operation: "list_live_models", Provider: providerID, Message: err.Error(), Cause: err}
	}
	return modelsFromCatalogEntries(compiled, providerID, entries, "live", false, false), nil
}

func listPublicModels(ctx context.Context, providerID, catalogURL string) ([]Model, error) {
	gatewayID := NormalizeProviderID(providerID)
	switch gatewayID {
	case "xiaomi_mimo_payg", "xiaomi_mimo_token_plan":
	default:
		return nil, invalid("list_public_models", "eyrie engine: gateway has no public model metadata source")
	}
	index, err := xiaomi.FetchPlatformModelsIndex(ctx, catalogURL)
	if err != nil {
		return nil, &Error{Code: ErrorCatalogUnavailable, Operation: "list_public_models", Provider: gatewayID, Message: err.Error(), Cause: err}
	}
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Model, 0, len(ids))
	for _, id := range ids {
		model := index[id]
		m := Model{
			ID: id, CanonicalID: id, DisplayName: model.Name, Description: model.Description,
			Owner: "Xiaomi", ProviderID: "xiaomi", GatewayID: gatewayID,
			ContextWindow: model.ContextLength, MaxOutputTokens: model.MaxOutputLength,
			InputPricePer1M: model.InputPricePer1M, OutputPricePer1M: model.OutputPricePer1M,
			PriceKnown: model.InputPricePer1M > 0 || model.OutputPricePer1M > 0,
			Source:     "public", LiveMetadata: append([]byte(nil), model.Raw...),
		}
		applyProviderThinkingDefaults(&m)
		out = append(out, m)
	}
	return out, nil
}

func firstOffering(offerings []catalog.ModelOffering) catalog.ModelOffering {
	if len(offerings) == 0 {
		return catalog.ModelOffering{}
	}
	ordered := append([]catalog.ModelOffering(nil), offerings...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].DeploymentID < ordered[j].DeploymentID })
	return ordered[0]
}

func capabilityNames(caps catalog.CapabilitySet) []string {
	var out []string
	if caps.FunctionCalling == catalog.CapabilitySupported {
		out = append(out, "tools")
	}
	if caps.ImageInput == catalog.CapabilitySupported {
		out = append(out, "vision")
	}
	if caps.StructuredOutput == catalog.CapabilitySupported {
		out = append(out, "structured_json")
	}
	if caps.ExplicitThinkingBudget == catalog.CapabilitySupported || caps.AdaptiveThinking == catalog.CapabilitySupported || caps.Effort == catalog.CapabilitySupported {
		out = append(out, "reasoning")
	}
	sort.Strings(out)
	return out
}

// applyProviderThinkingDefaults enriches a Model with thinking metadata
// from the provider's registry spec. This lets hosts read Model fields
// instead of maintaining hardcoded provider-name switches.
func applyProviderThinkingDefaults(m *Model) {
	providerID := NormalizeProviderID(m.ProviderID)
	if providerID == "" {
		providerID = NormalizeProviderID(m.GatewayID)
	}
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return
	}
	if spec.ThinkingToggleSupported {
		m.SupportsThinkingToggle = true
	}
	if spec.DefaultThinkingDisabled {
		disabled := false
		m.DefaultThinkingEnabled = &disabled
	}
}
