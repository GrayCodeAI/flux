package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/setup"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// ContractVersion is the compatibility version of the host-facing API.
const ContractVersion = "2"

// Options supplies host-owned dependencies. A zero value uses Eyrie's safe
// defaults. Product-specific paths are deliberately not inferred here.
type Options struct {
	SecretStore        credentials.Store
	StateDir           string
	CatalogPath        string
	ProviderConfigPath string
	// RemoteCatalogURL is the trusted published catalog source used by full
	// refreshes. Empty selects Eyrie's compiled-in HTTPS catalog URL, never a
	// process-environment override.
	RemoteCatalogURL string
	// CustomGateways is snapshotted per Engine. A non-nil empty slice
	// explicitly declares that the host has no custom gateways.
	CustomGateways []CustomGateway
	// UseRegisteredCustomGateways opts into the deprecated process-global
	// RegisterCustomGateway registry when CustomGateways is nil.
	UseRegisteredCustomGateways bool
	// EnableRateLimiting wraps resolved transports with an adaptive rate
	// limiter that backs off when approaching provider limits. Off by default.
	EnableRateLimiting bool
	// RateLimitConfig configures the adaptive rate limiter. Zero value uses
	// sensible defaults (10% threshold, 10s max delay).
	RateLimitConfig client.AdaptiveRateLimitConfig
	// EnableCaching wraps resolved transports with a semantic response cache.
	// Only caches deterministic requests (temperature <= threshold). Off by
	// default.
	EnableCaching bool
	// CacheConfig configures the response cache. Zero value uses sensible
	// defaults (5min TTL, 100 entries, 0.5 temperature threshold).
	CacheConfig client.CacheConfig
}

// Engine is Eyrie's narrow host facade. It is safe for concurrent use when
// the configured SecretStore is safe for concurrent use.
type Engine struct {
	secretStore        credentials.Store
	defaultSecretStore bool
	catalogPath        string
	providerConfigPath string
	remoteCatalogURL   string
	customGateways     map[string]CustomGateway
	resolveTransport   func(context.Context, Route) (client.Provider, error)
	enableRateLimiting bool
	rateLimitConfig    client.AdaptiveRateLimitConfig
	enableCaching      bool
	cacheConfig        client.CacheConfig
}

// New constructs a host-facing Eyrie engine.
func New(opts Options) (*Engine, error) {
	usesDefaultStore := opts.SecretStore == nil
	store := opts.SecretStore
	if store == nil {
		store = credentials.DefaultStore()
	}
	if store == nil {
		return nil, &Error{Code: ErrorInternal, Operation: "new", Message: "eyrie engine: credential store unavailable"}
	}
	stateDir := strings.TrimSpace(opts.StateDir)
	catalogPath := strings.TrimSpace(opts.CatalogPath)
	providerPath := strings.TrimSpace(opts.ProviderConfigPath)
	if catalogPath == "" && stateDir != "" {
		catalogPath = filepath.Join(stateDir, "model_catalog.json")
	}
	if providerPath == "" && stateDir != "" {
		providerPath = filepath.Join(stateDir, "provider.json")
	}
	if catalogPath == "" {
		catalogPath = catalog.DefaultCachePath()
	}
	if providerPath == "" {
		resolvedProviderPath, err := config.GetProviderConfigPath()
		if err != nil {
			return nil, &Error{Code: ErrorInternal, Operation: "new", Message: err.Error(), Cause: err}
		}
		providerPath = resolvedProviderPath
	}
	remoteCatalogURL := strings.TrimSpace(opts.RemoteCatalogURL)
	if remoteCatalogURL == "" {
		remoteCatalogURL = catalog.SeedCatalogURL
	}
	customGateways, err := customGatewaysForOptions(opts.CustomGateways, opts.UseRegisteredCustomGateways)
	if err != nil {
		return nil, err
	}
	engine := &Engine{
		secretStore: store, defaultSecretStore: usesDefaultStore,
		catalogPath: catalogPath, providerConfigPath: providerPath, remoteCatalogURL: remoteCatalogURL,
		customGateways:     customGateways,
		enableRateLimiting: opts.EnableRateLimiting,
		rateLimitConfig:    opts.RateLimitConfig,
		enableCaching:      opts.EnableCaching,
		cacheConfig:        opts.CacheConfig,
	}
	engine.resolveTransport = engine.defaultTransport
	migrateLegacyProviderConfigOnce.Do(migrateLegacyProviderConfig)
	return engine, nil
}

// migrateLegacyProviderConfig copies a provider.json left in the old
// product-specific "hawk" config dir into the new host-neutral "eyrie" dir the
// first time an engine starts after the rename. Without this, upgrading users
// silently lose their active provider/model selection, deployments, and routing
// (hawk starts as if unconfigured and they must re-run /config).
//
// The copy only happens when the eyrie-dir provider.json does not yet exist, so
// it is a one-time, idempotent migration that never overwrites newer state.
var migrateLegacyProviderConfigOnce sync.Once

func migrateLegacyProviderConfig() {
	// Resolve the target dir from the same source eyrie reads provider.json
	// from. Honors EYRIE_CONFIG_DIR and the HAWK_CONFIG_DIR compat fallback,
	// instead of hard-coding the default user-config dir.
	resolvedDir, err := config.GetProviderConfigDir()
	if err != nil || resolvedDir == "" {
		return
	}
	userDir, err := os.UserConfigDir()
	if err != nil || userDir == "" {
		return
	}
	// The legacy "hawk" subdir lived in the user-config root. If a custom
	// EYRIE_CONFIG_DIR is in use, there is no legacy "hawk" subdir to migrate
	// from; skip.
	legacyDir := filepath.Join(userDir, "hawk")
	// Copy legacy <legacyDir>/<name> → <resolvedDir>/<name> the first time an
	// engine starts after the rename, for each file that does not yet exist
	// in the destination. One-time, idempotent, never overwrites newer state.
	// The .tmp+rename pair makes the write atomic, so a parallel engine
	// starting in the same instant cannot observe a half-written file.
	for _, name := range []string{"provider.json", "categories.json"} {
		dest := filepath.Join(resolvedDir, name)
		if _, err := os.Stat(dest); err == nil {
			continue // already present (fresh install or previously migrated)
		}
		legacyPath := filepath.Join(legacyDir, name)
		data, readErr := os.ReadFile(legacyPath)
		if readErr != nil {
			continue // no legacy file to migrate; nothing to do
		}
		if err := os.MkdirAll(resolvedDir, 0o700); err != nil {
			slog.Warn("config: legacy migration mkdir failed",
				"dir", resolvedDir, "name", name, "error", err)
			continue
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			slog.Warn("config: legacy migration write failed",
				"path", tmp, "name", name, "error", err)
			continue
		}
		if err := os.Rename(tmp, dest); err != nil {
			slog.Warn("config: legacy migration rename failed",
				"from", tmp, "to", dest, "name", name, "error", err)
			_ = os.Remove(tmp)
		}
	}
}

// SelectionRequest asks Eyrie to resolve a concrete provider/model route.
type SelectionRequest struct {
	Requirements Requirements
	Preference   Preference
}

// Resolve selects a concrete provider/model and verifies known catalog
// requirements before transport construction.
func (e *Engine) Resolve(ctx context.Context, req SelectionRequest) (Route, error) {
	ctx = nonNilContext(ctx)
	selection, err := e.resolveSelection(ctx, req)
	if err != nil {
		return Route{}, err
	}
	return selection, nil
}

// Generate performs a blocking generation through Eyrie's resolved transport.
func (e *Engine) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	ctx = nonNilContext(ctx)
	if err := validateGenerateRequest(req); err != nil {
		return nil, err
	}
	route, provider, err := e.resolveProvider(ctx, req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := requestContext(ctx, req.Limits.Timeout)
	defer cancel()
	resp, err := provider.Chat(callCtx, toClientMessages(req.Messages), toClientOptions(req, route, false))
	if err != nil {
		return nil, classify("generate", route, err)
	}
	return fromClientResponse(resp, route), nil
}

// Stream starts a normalized streaming generation. The returned stream must be
// closed by the caller. The concrete type is *Stream; the signature returns
// EventStreamer so *Engine satisfies llm.Provider / llm.Generator.
//
// When the concrete *Stream is nil, this returns a typed-nil EventStreamer
// (interface nil) so callers can check stream == nil reliably.
func (e *Engine) Stream(ctx context.Context, req GenerateRequest) (EventStreamer, error) {
	s, err := e.stream(ctx, req)
	if s == nil {
		return nil, err
	}
	return s, err
}

// stream is the concrete implementation used by Stream and by callers that need
// the *Stream type without a type assertion.
func (e *Engine) stream(ctx context.Context, req GenerateRequest) (*Stream, error) {
	ctx = nonNilContext(ctx)
	if err := validateGenerateRequest(req); err != nil {
		return nil, err
	}
	route, provider, err := e.resolveProvider(ctx, req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := requestContext(ctx, req.Limits.Timeout)
	source, err := streamWithContinuation(callCtx, provider, toClientMessages(req.Messages), toClientOptions(req, route, true), req.Limits)
	if err != nil {
		cancel()
		return nil, classify("stream", route, err)
	}
	return newStream(callCtx, cancel, source, route), nil
}

// ListModels returns provider models through the stable facade.
func (e *Engine) ListModels(ctx context.Context, providerID string, refresh bool) ([]Model, error) {
	ctx = nonNilContext(ctx)
	if gateway, ok := e.customGateway(providerID); ok {
		if gateway.DefaultModel == "" {
			return nil, nil
		}
		return []Model{customGatewayModel(gateway)}, nil
	}
	var snapshot CatalogSnapshot
	var err error
	if refresh {
		snapshot, err = e.RefreshCatalog(ctx, providerID)
	} else {
		snapshot, err = e.Catalog(ctx)
	}
	if err != nil {
		return nil, &Error{Code: ErrorCatalogUnavailable, Operation: "list_models", Provider: providerID, Message: err.Error(), Cause: err}
	}
	requestedProvider := strings.TrimSpace(providerID)
	if requestedProvider == "" {
		return snapshot.Models, nil
	}
	compiled, loadErr := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
	if loadErr != nil {
		return nil, &Error{Code: ErrorCatalogUnavailable, Operation: "list_models", Provider: providerID, Message: loadErr.Error(), Cause: loadErr}
	}
	entries := catalog.ModelEntriesForProvider(compiled, requestedProvider)
	return modelsFromCatalogEntries(compiled, requestedProvider, entries, "cache", refresh, true), nil
}

func modelsFromCatalogEntries(compiled *catalog.CompiledCatalog, requestedProvider string, entries []catalog.ModelCatalogEntry, defaultSource string, metadataMarksLive, enrichCapabilities bool) []Model {
	gatewayID := NormalizeProviderID(requestedProvider)
	catalogProviderID := catalog.CanonicalProviderID(requestedProvider)
	out := make([]Model, 0, len(entries))
	for _, entry := range entries {
		capabilities := append([]string(nil), entry.ServerTools...)
		owner := catalogProviderID
		canonicalID := entry.ID
		if canonical, ok := catalog.CanonicalModelForProviderNative(compiled, requestedProvider, entry.ID); ok {
			canonicalID = canonical
			if enrichCapabilities {
				offering := offeringForProvider(compiled, requestedProvider, canonical, entry.ID)
				capabilities = capabilityNames(offering.Capabilities)
			}
			if resolvedOwner := catalog.ProviderForModel(compiled, canonical); resolvedOwner != "" {
				owner = resolvedOwner
			}
		}
		source := defaultSource
		if metadataMarksLive && len(entry.LiveMetadata) > 0 {
			source = "live"
		}
		out = append(out, Model{
			ID: entry.ID, CanonicalID: canonicalID, DisplayName: entry.DisplayName, Description: entry.Description,
			Owner: entry.Owner, ProviderID: owner, GatewayID: gatewayID,
			ContextWindow: entry.ContextWindow, MaxOutputTokens: entry.MaxOutput,
			InputPricePer1M: entry.InputPricePer1M, OutputPricePer1M: entry.OutputPricePer1M,
			PriceKnown:   modelPriceKnown(entry.ID, entry.DisplayName, entry.InputPricePer1M, entry.OutputPricePer1M, entry.ContextWindow),
			Capabilities: capabilities, Source: source,
			LiveMetadata: append([]byte(nil), entry.LiveMetadata...),
		})
	}
	return out
}

func modelPriceKnown(id, displayName string, input, output float64, contextWindow int) bool {
	if input > 0 || output > 0 {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(id) + " " + strings.TrimSpace(displayName))
	if strings.Contains(text, ":free") || strings.Contains(text, "/free") ||
		strings.HasSuffix(text, "-free") || strings.Contains(text, " free") {
		return true
	}
	return contextWindow > 0
}

func offeringForProvider(compiled *catalog.CompiledCatalog, providerID, canonicalID, nativeID string) catalog.ModelOffering {
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return firstOffering(compiled.OfferingsByCanonicalModel[canonicalID])
	}
	for _, offering := range compiled.OfferingsByDeployment[spec.DeploymentID] {
		if offering.CanonicalModelID == canonicalID && (nativeID == "" || offering.NativeModelID == nativeID) {
			return offering
		}
	}
	return firstOffering(compiled.OfferingsByCanonicalModel[canonicalID])
}

func (e *Engine) resolveProvider(ctx context.Context, req GenerateRequest) (Route, client.Provider, error) {
	route, err := e.resolveSelection(ctx, SelectionRequest{Requirements: req.Requirements, Preference: req.Preference})
	if err != nil {
		return Route{}, nil, err
	}
	provider, err := e.resolveTransport(ctx, route)
	if err != nil {
		return Route{}, nil, classify("resolve_transport", route, err)
	}
	// Guard against a transport that returns (nil, nil): resolveTransport is a
	// pluggable func field (custom gateways can override it), and a nil
	// provider paired with a nil error would nil-panic at the provider.Chat /
	// provider.StreamChat call sites. Surface it as a classified error instead.
	if provider == nil {
		return Route{}, nil, classify("resolve_transport", route,
			fmt.Errorf("resolved transport is nil for provider %q", route.Provider))
	}
	return route, provider, nil
}

func (e *Engine) defaultTransport(ctx context.Context, route Route) (client.Provider, error) {
	if provider, ok, err := e.customGatewayTransport(ctx, route); ok {
		return provider, err
	}
	compiled, cfg, err := e.loadRuntimeState(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := setup.DeploymentProviderFromState(cfg, compiled)
	if err != nil {
		return nil, err
	}
	// Wrap with opt-in middleware: rate limiting first (outermost), then cache.
	if e.enableRateLimiting {
		rlProvider, rlErr := client.NewAdaptiveRateLimitProvider(provider, e.rateLimitConfig)
		if rlErr == nil {
			provider = rlProvider
		}
		// On error, proceed without rate limiting rather than failing the request.
	}
	if e.enableCaching {
		provider = client.NewCachedProvider(provider, e.cacheConfig)
	}
	return provider, nil
}

func (e *Engine) resolveSelection(ctx context.Context, req SelectionRequest) (Route, error) {
	if route, ok, err := e.resolveCustomSelection(req); ok {
		return route, err
	}
	compiled, cfg, err := e.loadRuntimeState(ctx)
	if err != nil {
		return Route{}, err
	}
	selection := Route{
		Provider:          strings.TrimSpace(req.Preference.PreferredProvider),
		Model:             strings.TrimSpace(req.Preference.PreferredModelID),
		DeploymentRouting: true,
	}
	if selection.Provider == "" {
		selection.Provider = config.ActiveProvider(cfg)
	}
	if selection.Model == "" {
		selection.Model = config.ActiveModel(cfg)
	}
	if selection.Provider == "" && selection.Model != "" {
		selection.Provider = catalog.GatewayForModel(compiled, selection.Model)
		if selection.Provider == "" {
			selection.Provider = catalog.ProviderForModel(compiled, selection.Model)
		}
	}
	if strings.TrimSpace(selection.Model) != "" {
		err := validateRequirementsFromCatalog(compiled, selection.Model, req.Requirements)
		if err == nil {
			return selection, nil
		}
		// A user-selected exact model is a hard constraint unless fallback was
		// explicitly permitted. Persisted/default selections may be replaced by
		// a capability-compatible route.
		if req.Preference.PreferredModelID != "" && !req.Preference.AllowFallback {
			return Route{}, err
		}
	}
	modelID, providerID := selectCompatibleModel(compiled, req)
	if modelID == "" {
		return Route{}, &Error{Code: ErrorCapabilityMismatch, Operation: "resolve", Message: "eyrie engine: no catalog model satisfies the requested capabilities"}
	}
	selection.Model = modelID
	selection.Provider = providerID
	return selection, nil
}

type modelCandidate struct {
	model    catalog.Model
	offering catalog.ModelOffering
	provider string
	cost     float64
}

func selectCompatibleModel(compiled *catalog.CompiledCatalog, req SelectionRequest) (string, string) {
	if compiled == nil {
		return "", ""
	}
	preferredProvider := catalog.CanonicalProviderID(req.Preference.PreferredProvider)
	var candidates []modelCandidate
	for id, model := range compiled.ModelsByID {
		if req.Requirements.MinimumContext > 0 && model.ContextWindow < req.Requirements.MinimumContext {
			continue
		}
		if !offeringSupports(compiled, id, req.Requirements) {
			continue
		}
		provider := catalog.CanonicalProviderID(catalog.GatewayForModel(compiled, id))
		if provider == "" {
			provider = catalog.CanonicalProviderID(model.ProviderID)
		}
		if preferredProvider != "" && provider != preferredProvider {
			continue
		}
		offering := firstOffering(compiled.OfferingsByCanonicalModel[id])
		cost := offering.Pricing.RatesPer1M["input_tokens"] + offering.Pricing.RatesPer1M["output_tokens"]
		candidates = append(candidates, modelCandidate{model: model, offering: offering, provider: provider, cost: cost})
	}
	if len(candidates) == 0 {
		return "", ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		switch req.Preference.Intent {
		case llm.IntentEconomical:
			if a.cost != b.cost {
				return a.cost < b.cost
			}
		case llm.IntentReasoning:
			if a.model.ContextWindow != b.model.ContextWindow {
				return a.model.ContextWindow > b.model.ContextWindow
			}
		case llm.IntentFast:
			// Prefer smaller context windows as a latency proxy: models with
			// smaller context windows tend to have lower TTFT and higher
			// throughput. MaxOutput ascending is a secondary tiebreaker
			// (smaller output caps correlate with lighter models).
			if a.model.ContextWindow != b.model.ContextWindow {
				return a.model.ContextWindow < b.model.ContextWindow
			}
			if a.model.MaxOutput != b.model.MaxOutput {
				return a.model.MaxOutput < b.model.MaxOutput
			}
		}
		return a.model.ID < b.model.ID
	})
	return candidates[0].model.ID, candidates[0].provider
}

func validateGenerateRequest(req GenerateRequest) error {
	if len(req.Messages) == 0 {
		return invalid("generate", "eyrie engine: at least one message is required")
	}
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) == "" {
			return invalid("generate", "eyrie engine: every message requires a role")
		}
	}
	if req.Limits.MaxOutputTokens < 0 {
		return invalid("generate", "eyrie engine: max output tokens cannot be negative")
	}
	if req.Limits.MaxContinuations < 0 || req.Limits.MaxTotalOutputTokens < 0 {
		return invalid("generate", "eyrie engine: continuation limits cannot be negative")
	}
	if req.Requirements.MinimumContext < 0 {
		return invalid("generate", "eyrie engine: minimum context cannot be negative")
	}
	return nil
}

func validateRequirementsFromCatalog(compiled *catalog.CompiledCatalog, modelID string, req Requirements) error {
	if req.MinimumContext <= 0 && !req.Tools && !req.Vision && !req.StructuredJSON && !req.Reasoning {
		return nil
	}
	if compiled == nil {
		return &Error{Code: ErrorCatalogUnavailable, Operation: "validate_capabilities", Model: modelID, Message: "eyrie engine: catalog unavailable for capability validation"}
	}
	canonical, ok := compiled.CanonicalModelForAliasOrID(modelID)
	if !ok {
		canonical = modelID
	}
	model, ok := compiled.ModelsByID[canonical]
	if !ok {
		return &Error{Code: ErrorModelUnavailable, Operation: "validate_capabilities", Model: modelID, Message: fmt.Sprintf("eyrie engine: model %q is not in the catalog", modelID)}
	}
	if req.MinimumContext > 0 && model.ContextWindow < req.MinimumContext {
		return &Error{Code: ErrorCapabilityMismatch, Operation: "validate_capabilities", Model: canonical, Message: fmt.Sprintf("eyrie engine: model %q has context window %d, need at least %d", canonical, model.ContextWindow, req.MinimumContext)}
	}
	if req.Tools || req.Vision || req.StructuredJSON || req.Reasoning {
		if !offeringSupports(compiled, canonical, req) {
			return &Error{Code: ErrorCapabilityMismatch, Operation: "validate_capabilities", Model: canonical, Message: fmt.Sprintf("eyrie engine: model %q does not satisfy requested capabilities", canonical)}
		}
	}
	return nil
}

func offeringSupports(compiled *catalog.CompiledCatalog, modelID string, req Requirements) bool {
	offerings := compiled.OfferingsByCanonicalModel[modelID]
	if len(offerings) == 0 {
		return !req.Tools && !req.Vision && !req.StructuredJSON && !req.Reasoning
	}
	for _, offering := range offerings {
		caps := offering.Capabilities
		if req.Tools && caps.FunctionCalling != catalog.CapabilitySupported {
			continue
		}
		if req.Vision && caps.ImageInput != catalog.CapabilitySupported {
			continue
		}
		if req.StructuredJSON && caps.StructuredOutput != catalog.CapabilitySupported {
			continue
		}
		if req.Reasoning && caps.ExplicitThinkingBudget != catalog.CapabilitySupported && caps.AdaptiveThinking != catalog.CapabilitySupported && caps.Effort != catalog.CapabilitySupported {
			continue
		}
		return true
	}
	return false
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
