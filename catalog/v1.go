package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog/live"
)

type CapabilityState string

const (
	CapabilitySupported   CapabilityState = "supported"
	CapabilityUnsupported CapabilityState = "unsupported"
	CapabilityUnknown     CapabilityState = "unknown"
)

type PricingStatus string

const (
	PricingKnown   PricingStatus = "known"
	PricingPartial PricingStatus = "partial"
	PricingUnknown PricingStatus = "unknown"
	PricingFree    PricingStatus = "free"
)

type NativeModelIDSource string

const (
	NativeModelIDCatalogKnown   NativeModelIDSource = "catalog_known"
	NativeModelIDDiscovered     NativeModelIDSource = "discovered"
	NativeModelIDUserConfigured NativeModelIDSource = "user_configured"
	NativeModelIDCatalogOrUser  NativeModelIDSource = "catalog_or_user_configured"
)

const (
	CatalogSchemaVersion = "model-catalog/v1"
	// SeedCatalogURL is the published model-catalog/v1 document.
	SeedCatalogURL = "https://langdag.com/model-catalog/v1/catalog.json"
	EnvCatalogURL  = "GRAYCODE_ROUTER_MODEL_CATALOG_URL"
	// LiveStaleDuration is how long a cache remains fresh after live provider APIs were merged.
	LiveStaleDuration = 24 * time.Hour
)

// Catalog separates model ownership from the API protocol and deployment used
// to call the model. It is intentionally data-only; adapters remain code.
type Catalog struct {
	SchemaVersion     string                  `json:"schema_version"`
	GeneratedAt       time.Time               `json:"generated_at"`
	StaleAfter        time.Time               `json:"stale_after"`
	Providers         map[string]Provider     `json:"providers"`
	Protocols         map[string]Protocol     `json:"api_protocols"`
	Deployments       map[string]Deployment   `json:"deployments"`
	Models            map[string]Model        `json:"models"`
	Offerings         []ModelOffering         `json:"offerings"`
	OfferingTemplates []ModelOfferingTemplate `json:"offering_templates,omitempty"`
	Aliases           map[string]string       `json:"aliases,omitempty"`
	Provenance        *Provenance             `json:"provenance,omitempty"`
}

type Provider struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	HomepageURL string      `json:"homepage_url,omitempty"`
	Aliases     []string    `json:"aliases,omitempty"`
	Provenance  *Provenance `json:"provenance,omitempty"`
}

type Protocol struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Provenance  *Provenance `json:"provenance,omitempty"`
}

type Deployment struct {
	ID                     string              `json:"id"`
	Name                   string              `json:"name"`
	ProviderID             string              `json:"provider_id"`
	APIProtocolID          string              `json:"api_protocol_id"`
	AdapterConstructor     string              `json:"adapter_constructor"`
	CredentialRequirements []Credential        `json:"credential_requirements,omitempty"`
	EnvFallbacks           []EnvFallback       `json:"env_fallbacks,omitempty"`
	NativeModelIDSource    NativeModelIDSource `json:"native_model_id_source"`
	ModelMappingsRequired  bool                `json:"model_mappings_required,omitempty"`
	Local                  bool                `json:"local,omitempty"`
	Provenance             *Provenance         `json:"provenance,omitempty"`
}

type Credential struct {
	Field    string `json:"field"`
	Secret   bool   `json:"secret,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type EnvFallback struct {
	Field string   `json:"field"`
	Env   []string `json:"env"`
}

type Model struct {
	ID            string      `json:"id"`
	ProviderID    string      `json:"provider_id"`
	Name          string      `json:"name"`
	Family        string      `json:"family,omitempty"`
	ContextWindow int         `json:"context_window,omitempty"`
	MaxOutput     int         `json:"max_output,omitempty"`
	Aliases       []string    `json:"aliases,omitempty"`
	Provenance    *Provenance `json:"provenance,omitempty"`
}

type ModelOffering struct {
	ID               string          `json:"id"`
	CanonicalModelID string          `json:"canonical_model_id"`
	DeploymentID     string          `json:"deployment_id"`
	NativeModelID    string          `json:"native_model_id"`
	Capabilities     CapabilitySet   `json:"capabilities,omitempty"`
	Pricing          Pricing         `json:"pricing"`
	LiveMetadata     json.RawMessage `json:"live_metadata,omitempty"`
	Provenance       *Provenance     `json:"provenance,omitempty"`
}

type ModelOfferingTemplate struct {
	ID                  string              `json:"id"`
	CanonicalModelID    string              `json:"canonical_model_id"`
	DeploymentID        string              `json:"deployment_id"`
	NativeModelIDSource NativeModelIDSource `json:"native_model_id_source"`
	MappingRequired     bool                `json:"mapping_required"`
	Capabilities        CapabilitySet       `json:"capabilities,omitempty"`
	Pricing             Pricing             `json:"pricing"`
	Provenance          *Provenance         `json:"provenance,omitempty"`
}

type CapabilitySet struct {
	ServerTools            map[string]CapabilityState `json:"server_tools,omitempty"`
	FunctionCalling        CapabilityState            `json:"function_calling,omitempty"`
	ExplicitThinkingBudget CapabilityState            `json:"explicit_thinking_budget,omitempty"`
	AdaptiveThinking       CapabilityState            `json:"adaptive_thinking,omitempty"`
	Effort                 CapabilityState            `json:"effort,omitempty"`
	StructuredOutput       CapabilityState            `json:"structured_output,omitempty"`
	CodeExecution          CapabilityState            `json:"code_execution,omitempty"`
	Citations              CapabilityState            `json:"citations,omitempty"`
	PDFInput               CapabilityState            `json:"pdf_input,omitempty"`
	ImageInput             CapabilityState            `json:"image_input,omitempty"`
	MaxInputTokens         int                        `json:"max_input_tokens,omitempty"`
	MaxOutputTokens        int                        `json:"max_output_tokens,omitempty"`
	ThinkingTypes          []string                   `json:"thinking_types,omitempty"`
	EffortLevels           []string                   `json:"effort_levels,omitempty"`
}

type Pricing struct {
	Status            PricingStatus      `json:"status"`
	Currency          string             `json:"currency,omitempty"`
	EffectiveAt       time.Time          `json:"effective_at,omitempty"`
	RatesPer1M        map[string]float64 `json:"rates_per_1m,omitempty"`
	MissingDimensions []string           `json:"missing_dimensions,omitempty"`
	Notes             []string           `json:"notes,omitempty"`
	Source            string             `json:"source,omitempty"`
}

// CapabilitySetFromEntry builds a CapabilitySet from a live entry's features.
func CapabilitySetFromEntry(e live.Entry) CapabilitySet {
	set := CapabilitySet{ServerTools: map[string]CapabilityState{}}
	for _, feat := range e.Features {
		switch strings.ToLower(strings.TrimSpace(feat)) {
		case "web_search":
			set.ServerTools[feat] = CapabilitySupported
		case "image_generation":
			set.ServerTools[feat] = CapabilitySupported
		case "code_interpreter":
			set.ServerTools[feat] = CapabilitySupported
		case "function_calling", "function-calling", "tools":
			set.FunctionCalling = CapabilitySupported
		case "thinking:enabled":
			set.ExplicitThinkingBudget = CapabilitySupported
			set.ThinkingTypes = appendUniqueString(set.ThinkingTypes, "enabled")
		case "thinking:adaptive":
			set.AdaptiveThinking = CapabilitySupported
			set.ThinkingTypes = appendUniqueString(set.ThinkingTypes, "adaptive")
		case "effort":
			set.Effort = CapabilitySupported
		case "structured_output":
			set.StructuredOutput = CapabilitySupported
		case "code_execution":
			set.CodeExecution = CapabilitySupported
		case "citations":
			set.Citations = CapabilitySupported
		case "pdf_input":
			set.PDFInput = CapabilitySupported
		case "image_input", "vision":
			set.ImageInput = CapabilitySupported
		}
	}
	if len(set.ServerTools) == 0 && len(e.Features) > 0 {
		for _, feat := range e.Features {
			switch strings.ToLower(strings.TrimSpace(feat)) {
			case "function_calling", "function-calling", "tools",
				"thinking:enabled", "thinking:adaptive", "effort",
				"structured_output", "code_execution", "citations",
				"pdf_input", "image_input", "vision":
				continue
			default:
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(feat)), "effort:") {
					continue
				}
				set.ServerTools[feat] = CapabilitySupported
			}
		}
	}
	if e.MaxOutput > 0 {
		set.MaxOutputTokens = e.MaxOutput
	}
	if e.ContextWindow > 0 {
		set.MaxInputTokens = e.ContextWindow
	} else if e.MaxInputTokens > 0 {
		set.MaxInputTokens = e.MaxInputTokens
	}
	if e.ThinkingEnabled {
		set.ExplicitThinkingBudget = CapabilitySupported
		set.ThinkingTypes = appendUniqueString(set.ThinkingTypes, "enabled")
	}
	if e.ThinkingAdaptive {
		set.AdaptiveThinking = CapabilitySupported
		set.ThinkingTypes = appendUniqueString(set.ThinkingTypes, "adaptive")
	}
	if e.EffortSupported {
		set.Effort = CapabilitySupported
	}
	if e.EffortLevels != "" {
		set.EffortLevels = strings.Split(e.EffortLevels, ",")
	}
	if e.StructuredOutput {
		set.StructuredOutput = CapabilitySupported
	}
	if e.CodeExecution {
		set.CodeExecution = CapabilitySupported
	}
	if e.CitationsSupported {
		set.Citations = CapabilitySupported
	}
	if e.PDFInput {
		set.PDFInput = CapabilitySupported
	}
	if e.ImageInput {
		set.ImageInput = CapabilitySupported
	}
	return set
}

func appendUniqueString(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

// PricingFromEntry builds a Pricing from a live entry's per-token rates.
func PricingFromEntry(e live.Entry) Pricing {
	in := e.InputPricePer1M
	out := e.OutputPricePer1M
	if in < 0 || out < 0 {
		return Pricing{Status: PricingUnknown, Currency: "USD", Source: "live"}
	}
	pricing := Pricing{
		Status:     PricingKnown,
		Currency:   "USD",
		Source:     "live",
		RatesPer1M: map[string]float64{"input_tokens": in, "output_tokens": out},
	}
	if in == 0 && out == 0 {
		pricing.Status = PricingFree
	}
	return pricing
}

type Provenance struct {
	Source     string    `json:"source"`
	SourceURL  string    `json:"source_url,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

type CatalogDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CompiledCatalog struct {
	Catalog                   *Catalog
	ProvidersByID             map[string]Provider
	ProtocolsByID             map[string]Protocol
	DeploymentsByID           map[string]Deployment
	ModelsByID                map[string]Model
	OfferingsByID             map[string]ModelOffering
	OfferingsByCanonicalModel map[string][]ModelOffering
	OfferingsByDeployment     map[string][]ModelOffering
	TemplatesByCanonicalModel map[string][]ModelOfferingTemplate
	Diagnostics               []CatalogDiagnostic
}

// CapabilitiesForModel returns the capability set for a model on a deployment.
func (c *CompiledCatalog) CapabilitiesForModel(modelID, deploymentID string) CapabilitySet {
	if c == nil {
		return CapabilitySet{}
	}
	if offerings, ok := c.OfferingsByCanonicalModel[modelID]; ok {
		for _, offering := range offerings {
			if deploymentID == "" || offering.DeploymentID == deploymentID {
				return offering.Capabilities
			}
		}
	}
	for _, offerings := range c.OfferingsByDeployment {
		for _, offering := range offerings {
			if offering.NativeModelID == modelID {
				return offering.Capabilities
			}
		}
	}
	return CapabilitySet{}
}

func (c *CompiledCatalog) CanonicalModelForAliasOrID(value string) (string, bool) {
	if c == nil || value == "" {
		return "", false
	}
	if c.ModelsByID[value].ID != "" {
		return value, true
	}
	if target := c.Catalog.Aliases[value]; c.ModelsByID[target].ID != "" {
		return target, true
	}
	for _, model := range c.ModelsByID {
		for _, alias := range model.Aliases {
			if alias == value {
				return model.ID, true
			}
		}
	}
	return "", false
}

// ResolveModel maps a model alias or native ID to its canonical catalog ID.
// It trims whitespace, handles nil catalogs, and falls back to the input
// itself when it looks like a provider-native ID (contains "/").
// This centralizes the alias-resolution pattern used across engine, runtime,
// and router packages, replacing per-caller trim + nil-check + fallback logic.
func ResolveModel(compiled *CompiledCatalog, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if compiled == nil {
		if strings.Contains(model, "/") {
			return model
		}
		return ""
	}
	if canonical, ok := compiled.CanonicalModelForAliasOrID(model); ok {
		return canonical
	}
	if strings.Contains(model, "/") {
		return model
	}
	return ""
}

func (c *CompiledCatalog) OfferingForDeployment(canonicalModelID, deploymentID string) (ModelOffering, bool) {
	if c == nil {
		return ModelOffering{}, false
	}
	for _, offering := range c.OfferingsByCanonicalModel[canonicalModelID] {
		if offering.DeploymentID == deploymentID {
			return offering, true
		}
	}
	return ModelOffering{}, false
}

func (c *CompiledCatalog) FirstModelForProvider(providerID string) (string, bool) {
	if c == nil {
		return "", false
	}
	providerID = canonicalProviderID(providerID)
	for modelID, model := range c.ModelsByID {
		if canonicalProviderID(model.ProviderID) == providerID && model.Name != "" {
			return modelID, true
		}
	}
	return "", false
}

// ModelIDsForProvider returns all model IDs for a given provider, sorted.
func (c *CompiledCatalog) ModelIDsForProvider(providerID string) []string {
	if c == nil {
		return nil
	}
	providerID = canonicalProviderID(providerID)
	var ids []string
	seen := map[string]bool{}
	for modelID, model := range c.ModelsByID {
		if canonicalProviderID(model.ProviderID) == providerID && model.Name != "" && !seen[modelID] {
			ids = append(ids, modelID)
			seen[modelID] = true
		}
	}
	sort.Strings(ids)
	return ids
}

func (c *CompiledCatalog) ProviderNames() []string {
	if c == nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	for _, p := range c.ProvidersByID {
		if p.Name != "" && !seen[p.ID] {
			names = append(names, p.ID)
			seen[p.ID] = true
		}
	}
	sort.Strings(names)
	return names
}

func SplitOfferingID(id string) (deploymentID, nativeModelID string, ok bool) {
	left, right, found := strings.Cut(id, ":")
	return left, right, found && left != "" && right != ""
}

// --- Catalog helpers ---

func ParseCatalog(data []byte) (*Catalog, error) {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("catalog: decode envelope: %w", err)
	}
	if envelope.SchemaVersion == "" {
		return nil, fmt.Errorf("catalog: legacy format is no longer supported")
	}
	var c Catalog
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("catalog: decode: %w", err)
	}
	return &c, ValidateCatalog(&c)
}

func ValidateCatalog(c *Catalog) error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if c == nil {
		return fmt.Errorf("catalog: nil catalog")
	}
	if c.SchemaVersion != CatalogSchemaVersion {
		add("schema_version must be %q", CatalogSchemaVersion)
	}
	if c.GeneratedAt.IsZero() {
		add("generated_at is required")
	}
	if c.StaleAfter.IsZero() {
		add("stale_after is required")
	} else if !c.GeneratedAt.IsZero() && !c.StaleAfter.After(c.GeneratedAt) {
		add("stale_after must be after generated_at")
	}
	if len(c.Providers) == 0 {
		add("providers is required")
	}
	if len(c.Protocols) == 0 {
		add("api_protocols is required")
	}
	if len(c.Deployments) == 0 {
		add("deployments is required")
	}
	if len(c.Models) == 0 && !IsBootstrapCatalog(c) {
		add("models is required")
	}
	if len(c.Offerings) == 0 && !IsBootstrapCatalog(c) {
		add("offerings is required")
	}
	for id, provider := range c.Providers {
		if id == "" || provider.ID != id || provider.Name == "" {
			add("provider %q must have matching non-empty id and name", id)
		}
	}
	for id, protocol := range c.Protocols {
		if id == "" || protocol.ID != id || protocol.Name == "" {
			add("api_protocol %q must have matching non-empty id and name", id)
		}
	}
	for id, deployment := range c.Deployments {
		if id == "" || deployment.ID != id || deployment.Name == "" {
			add("deployment %q must have matching non-empty id and name", id)
		}
		if c.Providers[deployment.ProviderID].ID == "" {
			add("deployment %q references unknown provider %q", id, deployment.ProviderID)
		}
		if c.Protocols[deployment.APIProtocolID].ID == "" {
			add("deployment %q references unknown api_protocol %q", id, deployment.APIProtocolID)
		}
		if deployment.AdapterConstructor == "" {
			add("deployment %q missing adapter_constructor", id)
		}
		if !validNativeModelIDSource(deployment.NativeModelIDSource) {
			add("deployment %q has invalid native_model_id_source %q", id, deployment.NativeModelIDSource)
		}
	}
	for id, model := range c.Models {
		if id == "" || model.ID != id || model.Name == "" {
			add("model %q must have matching non-empty id and name", id)
		}
		if !looksCanonicalModelID(id) {
			add("model %q is not an owner-qualified canonical model id", id)
		}
		if c.Providers[model.ProviderID].ID == "" {
			add("model %q references unknown provider %q", id, model.ProviderID)
		}
	}
	seenOfferings := map[string]bool{}
	for _, offering := range c.Offerings {
		if offering.ID == "" {
			add("offering missing id")
			continue
		}
		if seenOfferings[offering.ID] {
			add("offering %q is duplicated", offering.ID)
			continue
		}
		seenOfferings[offering.ID] = true
		deploymentID, nativeID, ok := SplitOfferingID(offering.ID)
		if !ok || deploymentID != offering.DeploymentID || nativeID != offering.NativeModelID {
			add("offering %q must be deployment_id:native_model_id", offering.ID)
		}
		if c.Models[offering.CanonicalModelID].ID == "" {
			add("offering %q references unknown model %q", offering.ID, offering.CanonicalModelID)
		}
		if c.Deployments[offering.DeploymentID].ID == "" {
			add("offering %q references unknown deployment %q", offering.ID, offering.DeploymentID)
		}
		validatePricing(&problems, offering.ID, offering.Pricing)
		validateCapabilities(&problems, offering.ID, offering.Capabilities)
	}
	for _, tmpl := range c.OfferingTemplates {
		if tmpl.ID == "" {
			add("offering_template missing id")
			continue
		}
		if !IsBootstrapCatalog(c) && c.Models[tmpl.CanonicalModelID].ID == "" {
			add("offering_template %q references unknown model %q", tmpl.ID, tmpl.CanonicalModelID)
		}
		deployment := c.Deployments[tmpl.DeploymentID]
		if deployment.ID == "" {
			add("offering_template %q references unknown deployment %q", tmpl.ID, tmpl.DeploymentID)
		} else if !deployment.ModelMappingsRequired {
			add("offering_template %q references deployment %q that does not require mappings", tmpl.ID, tmpl.DeploymentID)
		}
		if !tmpl.MappingRequired || tmpl.NativeModelIDSource != NativeModelIDUserConfigured {
			add("offering_template %q must require user-configured mappings", tmpl.ID)
		}
		validatePricing(&problems, tmpl.ID, tmpl.Pricing)
		validateCapabilities(&problems, tmpl.ID, tmpl.Capabilities)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("catalog validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func CompileCatalog(c *Catalog) (*CompiledCatalog, error) {
	EnsureDeploymentEnvFallbacks(c)
	SanitizePricing(c)
	if err := ValidateCatalog(c); err != nil {
		return nil, err
	}
	compiled := &CompiledCatalog{
		Catalog:                   c,
		ProvidersByID:             cloneMap(c.Providers),
		ProtocolsByID:             cloneMap(c.Protocols),
		DeploymentsByID:           cloneMap(c.Deployments),
		ModelsByID:                cloneMap(c.Models),
		OfferingsByID:             map[string]ModelOffering{},
		OfferingsByCanonicalModel: map[string][]ModelOffering{},
		OfferingsByDeployment:     map[string][]ModelOffering{},
		TemplatesByCanonicalModel: map[string][]ModelOfferingTemplate{},
	}
	if time.Now().UTC().After(c.StaleAfter) {
		compiled.Diagnostics = append(compiled.Diagnostics, CatalogDiagnostic{Code: "stale_catalog", Message: "catalog is stale"})
	}
	for _, offering := range c.Offerings {
		compiled.OfferingsByID[offering.ID] = offering
		compiled.OfferingsByCanonicalModel[offering.CanonicalModelID] = append(compiled.OfferingsByCanonicalModel[offering.CanonicalModelID], offering)
		compiled.OfferingsByDeployment[offering.DeploymentID] = append(compiled.OfferingsByDeployment[offering.DeploymentID], offering)
	}
	for _, tmpl := range c.OfferingTemplates {
		compiled.TemplatesByCanonicalModel[tmpl.CanonicalModelID] = append(compiled.TemplatesByCanonicalModel[tmpl.CanonicalModelID], tmpl)
	}
	return compiled, nil
}

func LoadCatalog(ctx context.Context, opts LoadCatalogOptions) (*CompiledCatalog, error) {
	if opts.CachePath == "" {
		opts.CachePath = DefaultCachePath()
	}
	if opts.RefreshRemote {
		remote, err := FetchRemoteCatalog(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("catalog remote: %w", err)
		}
		if opts.CachePath != "" {
			_ = WriteCatalogCache(opts.CachePath, remote)
		}
		return CompileCatalog(remote)
	}
	if compiled, ok := LoadValidCatalogCache(opts.CachePath); ok {
		return compiled, nil
	}
	if opts.RequireCache {
		return nil, fmt.Errorf("%w (%s missing or invalid; run: hawk models refresh)", ErrCatalogCacheRequired, opts.CachePath)
	}
	bootstrap := BootstrapCatalog()
	compiled, err := CompileCatalog(&bootstrap)
	if err != nil {
		return nil, err
	}
	compiled.Diagnostics = append(compiled.Diagnostics, CatalogDiagnostic{
		Code:    "bootstrap_only",
		Message: "no model catalog cache; run hawk models refresh or graycode-router catalog discover",
	})
	return compiled, nil
}

func LoadValidCatalogCache(cachePath string) (*CompiledCatalog, bool) {
	if cachePath == "" {
		return nil, false
	}
	data, err := os.ReadFile(cachePath) // #nosec G304 -- cachePath is an operator-supplied local cache file path, not untrusted input
	if err != nil {
		return nil, false
	}
	c, err := ParseCatalog(data)
	if err != nil {
		return nil, false
	}
	if IsBootstrapCatalog(c) || len(c.Models) == 0 {
		return nil, false
	}
	compiled, err := CompileCatalog(c)
	if err != nil {
		return nil, false
	}
	return compiled, true
}

// --- Remote catalog ---

type LoadCatalogOptions struct {
	CachePath     string
	RemoteURL     string
	RefreshRemote bool
	RequireCache  bool
	HTTPClient    *http.Client
	Timeout       time.Duration
}

func ResolvedRemoteCatalogURL(explicit string) string {
	if u := strings.TrimSpace(explicit); u != "" {
		return u
	}
	if u := strings.TrimSpace(os.Getenv(EnvCatalogURL)); u != "" {
		return u
	}
	return SeedCatalogURL
}

func FetchRemoteCatalog(ctx context.Context, opts LoadCatalogOptions) (*Catalog, error) {
	url := ResolvedRemoteCatalogURL(opts.RemoteURL)
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog: remote returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}
	return ParseCatalog(body)
}

func WriteCatalogCache(cachePath string, c *Catalog) error {
	if cachePath == "" {
		return nil
	}
	SanitizePricing(c)
	if err := ValidateCatalog(c); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return err
	}
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, 0x0a), 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, cachePath)
}
