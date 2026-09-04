package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Provider is hawk's hawk-owned view of the provider engine: a composition of
// the role interfaces below. It is the single integration surface — hawk never
// holds an *graycoderouterengine.Engine, and graycode-router never imports hawk/internal.
//
// Callers that need only a subset depend on the relevant role interface
// directly (e.g. session_factory depends only on Generator), keeping the
// declared dependency precise and the test stub small.
type Provider interface {
	Generator
	ModelCatalog
	CredentialManager
	SelectionManager
	GatewayInspector
	CatalogMaintenance
	NativeCompactor
}

// Generator is the chat transport facet: the only part the ChatClient path uses.
//
// Stream returns a pull-based EventStreamer (the host engine facade contract).
// Lower-level channel-based streaming still uses StreamResult on the client
// transport layer; that type is intentionally not part of this host port.
type Generator interface {
	Generate(ctx context.Context, req GenerateRequest) (*GraycodeRouterResponse, error)
	Stream(ctx context.Context, req GenerateRequest) (EventStreamer, error)
}

// EventStreamer is the pull-based host stream contract used by the engine facade.
// Next must not be called concurrently. Close is idempotent.
type EventStreamer interface {
	Next() bool
	Event() GraycodeRouterStreamEvent
	Err() error
	Close() error
}

// GenerateRequest is the normalized generation request.
type GenerateRequest struct {
	Messages     []GraycodeRouterMessage
	SystemPrompt string
	Tools        []GraycodeRouterTool
	Requirements Requirements
	Preference   Preference
	Limits       Limits
	Metadata     Metadata
	Temperature  *float64
	OutputSchema string
	Options      GenerationOptions
}

// Intent expresses a host's semantic preference without naming a provider.
type Intent string

const (
	IntentFast       Intent = "fast"
	IntentBalanced   Intent = "balanced"
	IntentReasoning  Intent = "reasoning"
	IntentEconomical Intent = "economical"
)

// Requirements declare what the request needs from the engine.
type Requirements struct {
	Streaming      bool
	Tools          bool
	Vision         bool
	StructuredJSON bool
	Reasoning      bool
	MinimumContext int `json:"minimum_context,omitempty"`
}

// Preference declares the preferred provider/model.
type Preference struct {
	Intent            Intent  `json:"intent,omitempty"`
	PreferredProvider string  `json:"-"`
	PreferredModelID  string  `json:"preferred_model_id,omitempty"`
	AllowFallback     bool    `json:"allow_fallback,omitempty"`
	MaximumCostUSD    float64 `json:"maximum_cost_usd,omitempty"`
}

// Limits declares output limits.
type Limits struct {
	MaxOutputTokens      int           `json:"max_output_tokens,omitempty"`
	MaxContinuations     int           `json:"max_continuations,omitempty"`
	MaxTotalOutputTokens int           `json:"max_total_output_tokens,omitempty"`
	Timeout              time.Duration `json:"timeout,omitempty"`
}

// Metadata carries request-scoped metadata.
type Metadata struct {
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// GenerationOptions holds provider-specific generation knobs.
type GenerationOptions struct {
	EnableCaching        bool
	ReasoningEffort      string
	ThinkingBudgetTokens int
	ThinkingMode         string
	ThinkingDisplay      string
	ThinkingEnabled      *bool
	// GLMThinkingEnabled is a deprecated alias of ThinkingEnabled (Z.AI-era).
	GLMThinkingEnabled *bool
	VirtualKeyID       string
	KimiContextCacheID string
	KimiCacheResetTTL  bool
	TopP               *float64
	TopK               *int
	StopSequences      []string
	ToolChoice         *ToolChoiceOption
	ServiceTier        string
	OutputEffort       string
	PresencePenalty    *float64
	FrequencyPenalty   *float64
	N                  *int
	LogProbs           *bool
	TopLogProbs        *int
	Seed               *int
	Store              *bool
	Metadata           map[string]string
	Modalities         []string
	AudioConfig        string
	Prediction         string
	WebSearchOptions   string
}

// ModelCatalog is the model-discovery facet (used by routing + config).
type ModelCatalog interface {
	ListModels(ctx context.Context, providerID string, refresh bool) ([]Model, error)
	ListLiveModels(ctx context.Context, providerID string) ([]Model, error)
	ListPublicModels(ctx context.Context, providerID string) ([]Model, error)
	ModelInfo(ctx context.Context, modelID string) (Model, bool, error)
	ModelProviders(ctx context.Context) ([]string, error)
	DefaultModel(ctx context.Context, provider, fallback string) string
	PreferredModel(ctx context.Context, provider string, class ModelClass, fallback string) string
	PreferredModels(ctx context.Context, primaryProvider string, class ModelClass, limit int) []string
	ModelClassOf(ctx context.Context, modelID string) ModelClass
	ProviderForModel(ctx context.Context, modelID string) string
	PrimaryModel(ctx context.Context) string
	ModelNames(ctx context.Context) []string
	Catalog(ctx context.Context) (CatalogSnapshot, error)
}

// Model is the product-facing view of model metadata.
type Model struct {
	ID               string          `json:"id"`
	ProviderID       string          `json:"provider_id"`
	CanonicalID      string          `json:"canonical_id,omitempty"`
	DisplayName      string          `json:"display_name"`
	Description      string          `json:"description,omitempty"`
	Owner            string          `json:"owner,omitempty"`
	GatewayID        string          `json:"gateway_id,omitempty"`
	ContextWindow    int             `json:"context_window,omitempty"`
	MaxOutputTokens  int             `json:"max_output_tokens,omitempty"`
	InputPricePer1M  float64         `json:"input_price_per_1m,omitempty"`
	OutputPricePer1M float64         `json:"output_price_per_1m,omitempty"`
	PriceKnown       bool            `json:"price_known"`
	Capabilities     []string        `json:"capabilities,omitempty"`
	Source           string          `json:"source,omitempty"`
	LiveMetadata     json.RawMessage `json:"live_metadata,omitempty"`

	// SupportsThinkingToggle indicates this model's provider wire protocol
	// honors the ThinkingEnabled toggle. Populated by the engine catalog.
	SupportsThinkingToggle bool `json:"supports_thinking_toggle,omitempty"`
	// DefaultThinkingEnabled is the provider-default thinking state for this
	// model when no explicit user preference is set. nil means no provider
	// default (fall through to global settings).
	DefaultThinkingEnabled *bool `json:"default_thinking_enabled,omitempty"`
}

// ModelClass is a provider-neutral relative model cost/capability band.
type ModelClass string

const (
	ModelClassEconomical ModelClass = "economical"
	ModelClassBalanced   ModelClass = "balanced"
	ModelClassPremium    ModelClass = "premium"
)

// CatalogSnapshot is an immutable, point-in-time host-facing view of a loaded
// model catalog. It is the canonical definition; graycode-router's engine.CatalogSnapshot
// is a type alias to this so a single struct crosses the host boundary.
type CatalogSnapshot struct {
	Models    []Model   `json:"models"`
	CachePath string    `json:"cache_path,omitempty"`
	RemoteURL string    `json:"remote_url,omitempty"`
	Stale     bool      `json:"stale,omitempty"`
	LoadedAt  time.Time `json:"loaded_at"`
}

// CredentialManager is the key/credential facet (config only).
type CredentialManager interface {
	SaveCredential(ctx context.Context, providerID, secret string) (CredentialStatus, error)
	RemoveCredential(ctx context.Context, providerID string) error
	CredentialStatus(ctx context.Context, providerID string) (CredentialStatus, error)
	SaveCredentialEnv(ctx context.Context, envVar, secret string) error
	HasCredentialEnv(ctx context.Context, envVar string) bool
	CredentialEnvKeys(providerID string) []string
	ResolveCredential(ctx context.Context, secret string) CredentialResolution
	CredentialProviders(context.Context) []CredentialProviderOption
	ApplyCredentials(ctx context.Context, providerID string) (CatalogSnapshot, error)
}

// CredentialStatus reports whether a provider's credential is configured.
type CredentialStatus struct {
	Configured          bool   `json:"configured"`
	ProviderID          string `json:"provider_id,omitempty"`
	EnvironmentVariable string `json:"environment_variable,omitempty"`
	EnvironmentConflict bool   `json:"environment_conflict,omitempty"`
	Verified            bool   `json:"verified,omitempty"`
	Masked              string `json:"masked,omitempty"`
	EnvVar              string `json:"env_var,omitempty"`
}

// CredentialResolution is the result of validating a pasted API key.
type CredentialResolution struct {
	FormatOK                bool   `json:"format_ok"`
	FormatError             string `json:"format_error,omitempty"`
	Providers               []CredentialProviderOption
	ProbeDisambiguationUsed bool `json:"probe_disambiguation_used,omitempty"`
}

// CredentialProviderOption is one provider row in the key picker.
type CredentialProviderOption struct {
	ProviderID   string `json:"provider_id"`
	DeploymentID string `json:"deployment_id,omitempty"`
	EnvVar       string `json:"env_var"`
	DisplayName  string `json:"display_name"`
	Inferred     bool   `json:"inferred,omitempty"`
	RequiresKey  bool   `json:"requires_key"`
	Rank         int    `json:"rank"`
}

// SelectionManager is the get/set selection facet (config only).
type SelectionManager interface {
	ActiveSelection(ctx context.Context) ResolvedRoute
	EffectiveSelection(ctx context.Context, opts SelectionOptions) Selection
	SetActiveProvider(ctx context.Context, provider string) error
	SetActiveModel(ctx context.Context, modelID string) error
	SetSelection(ctx context.Context, provider, modelID string) error
	ClearSelection(ctx context.Context) error
}

// Selection is the effective provider/model pair.
type Selection struct {
	Provider                string `json:"provider"`
	Model                   string `json:"model"`
	HasConfiguredDeployment bool   `json:"has_configured_deployment"`
	DeploymentRouting       bool   `json:"deployment_routing"`
}

// SelectionOptions controls effective-selection resolution.
type SelectionOptions struct {
	ProviderOverride          string
	ModelOverride             string
	DeploymentRoutingOverride *bool
}

// GatewayInspector is the gateway/deployment facet (config only).
type GatewayInspector interface {
	GatewayDefinitions() []Gateway
	Gateways(ctx context.Context) []Gateway
	GatewayRegion(providerID string) (label string, required bool)
	SetGatewayRegion(ctx context.Context, providerID, value string) error
	GatewayForModel(ctx context.Context, modelID string) string
	CanonicalModel(ctx context.Context, modelID string) string
	DeploymentRoutingEnabled(override *bool) bool
	DeploymentStatus(ctx context.Context, activeModel string) (string, error)
	DeploymentSummary(ctx context.Context, activeModel string) (DeploymentSummary, error)
	RoutingPreview(ctx context.Context, modelID string) (string, error)
}

// GatewayRegionOption describes one selectable region for a regional gateway.
type GatewayRegionOption struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
	Endpoint    string `json:"endpoint,omitempty"`
}

// Gateway is a provider/gateway descriptor.
type Gateway struct {
	ID                    string                `json:"id"`
	DisplayName           string                `json:"display_name"`
	DeploymentID          string                `json:"deployment_id,omitempty"`
	CredentialEnv         string                `json:"credential_env"`
	RequiresKey           bool                  `json:"requires_key"`
	SortOrder             int                   `json:"sort_order"`
	ChatPreference        int                   `json:"chat_preference"`
	SupportsLiveDiscovery bool                  `json:"supports_live_discovery"`
	CredentialConfigured  bool                  `json:"credential_configured"`
	DeploymentConfigured  bool                  `json:"deployment_configured"`
	ModelCount            int                   `json:"model_count"`
	RegionLabel           string                `json:"region_label,omitempty"`
	RegionRequired        bool                  `json:"region_required"`
	RegionOptions         []GatewayRegionOption `json:"region_options,omitempty"`
	DNSHost               string                `json:"dns_host,omitempty"`
	Active                bool                  `json:"active"`
}

// DeploymentSummary summarizes deployment routing for a model (host-facing).
type DeploymentSummary struct {
	RoutingSource string `json:"routing_source,omitempty"`
	RoutingStages int    `json:"routing_stages,omitempty"`
	Formatted     string `json:"formatted"`
}

// CatalogMaintenance is the refresh/preflight/security facet (config only).
type CatalogMaintenance interface {
	RefreshCatalog(ctx context.Context, providerID string) (CatalogSnapshot, error)
	CatalogHealth(ctx context.Context) CatalogHealth
	StatePaths() StatePaths
	DefaultProviderFilter(ctx context.Context) string
	Preflight(ctx context.Context) PreflightReport
	PreflightWithOptions(ctx context.Context, opts PreflightOptions) PreflightReport
	ProviderStateSecurityStatus() ProviderStateSecurity
	MigrateProviderSecrets() error
	MigrateProviderSecretsContext(ctx context.Context) error
}

// CatalogHealth reports the health of the model catalog (host-facing).
type CatalogHealth struct {
	Path        string    `json:"path"`
	Exists      bool      `json:"exists"`
	ModifiedAt  time.Time `json:"modified_at,omitempty"`
	Size        int64     `json:"size,omitempty"`
	Models      int       `json:"models,omitempty"`
	Deployments int       `json:"deployments,omitempty"`
	Offerings   int       `json:"offerings,omitempty"`
	Stale       bool      `json:"stale,omitempty"`
	StaleAfter  time.Time `json:"stale_after,omitempty"`
	Source      string    `json:"source,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// StatePaths is the on-disk location of catalog + provider config.
type StatePaths struct {
	Catalog        string `json:"catalog"`
	ProviderConfig string `json:"provider_config"`
}

// PreflightOptions configures a preflight check.
type PreflightOptions struct {
	VerifyLive bool `json:"verify_live,omitempty"`
}

// PreflightReport is the result of a preflight check.
type PreflightReport struct {
	Ready        bool             `json:"ready"`
	LiveVerified bool             `json:"live_verified"`
	Checks       []PreflightCheck `json:"checks"`
}

// CheckStatus is a preflight check status string.
type CheckStatus string

// Common preflight check statuses.
const (
	CheckOK   CheckStatus = "ok"
	CheckFail CheckStatus = "fail"
	CheckWarn CheckStatus = "warn"
)

// PreflightCheck is one readiness check.
type PreflightCheck struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

// ProviderStateSecurity reports security state of provider config.
type ProviderStateSecurity struct {
	Path       string `json:"path"`
	HasSecrets bool   `json:"has_secrets"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
}

// NativeCompactor is the provider-native-compaction facet.
type NativeCompactor interface {
	SupportsNativeCompaction(ctx context.Context, provider, model string) bool
	CompactNative(ctx context.Context, req NativeCompactionRequest) (string, error)
}

// NativeCompactionRequest is a native-compaction request.
type NativeCompactionRequest struct {
	Provider        string
	Model           string
	Messages        []GraycodeRouterMessage
	ContextWindow   int
	ThresholdPct    int
	MaxOutputTokens int
}
