package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/catalog/zai"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/runtime"
	"github.com/GrayCodeAI/eyrie/setup"
)

// SecretStoreName is safe presentation metadata for host UIs.
func SecretStoreName() string { return credentials.PlatformSecretStoreName() }

// SetSecretStoreServiceName overrides the OS secret-store service name (default
// "eyrie"). Hosts call this once at startup so existing credentials filed under
// their product name (e.g. "hawk") stay readable.
func SetSecretStoreServiceName(name string) { credentials.SetServiceName(name) }

// -- Test-fixture re‑exports -------------------------------------------------
// These let host test code inject credential fixtures through the engine
// boundary instead of importing eyrie/credentials directly.

// SetDefaultStore replaces the process-wide credential store (for tests).
var SetDefaultStore = credentials.SetDefaultStore

// DefaultStore returns the process-wide credential store (for tests).
var DefaultStore = credentials.DefaultStore

// MapStore is the in-memory credential store for tests (alias).
type MapStore = credentials.MapStore

// AccountForEnv returns the keychain account name for an env var.
func AccountForEnv(envVar string) string { return credentials.AccountForEnv(envVar) }

// HasSecret reports whether a secret exists for an env var (for tests).
func HasSecret(ctx context.Context, envKey string) bool { return credentials.HasSecret(ctx, envKey) }

// CredentialStorageReport is safe to print and never includes secrets.
type CredentialStorageReport struct {
	PlatformStore string `json:"platform_store"`
	Writable      bool   `json:"writable"`
	Detail        string `json:"detail,omitempty"`
	Formatted     string `json:"formatted,omitempty"`
}

func CredentialStorage(ctx context.Context) CredentialStorageReport {
	ctx = nonNilContext(ctx)
	report := credentials.StorageReportFor(ctx)
	return CredentialStorageReport{
		PlatformStore: report.PlatformStore, Writable: report.KeychainWritable, Detail: report.KeychainDetail,
		Formatted: credentials.FormatStorageReport(report),
	}
}

func (e *Engine) credentialStorage(ctx context.Context) CredentialStorageReport {
	if e.defaultSecretStore {
		return CredentialStorage(ctx)
	}
	return CredentialStorageReport{
		PlatformStore: "host-injected", Writable: false,
		Detail:    "host-injected credential store configured (writability not probed)",
		Formatted: "Credential storage:\n  platform store: host-injected\n  writability: not probed",
	}
}

func (e *Engine) StatePaths() StatePaths {
	return StatePaths{Catalog: e.catalogPath, ProviderConfig: e.providerConfigPath}
}

func MigrateEnvFileCredentials(ctx context.Context) (int, error) {
	return credentials.MigrateEnvFileCredentials(nonNilContext(ctx))
}

// HasCredentialEnv reports presence without exposing the credential value.
func (e *Engine) HasCredentialEnv(ctx context.Context, envVar string) bool {
	return e.hasCredential(nonNilContext(ctx), []string{envVar})
}

// SaveCredentialEnv validates and stores a legacy env-addressed credential.
// New callers should prefer SaveCredential(providerID, secret).
func (e *Engine) SaveCredentialEnv(ctx context.Context, envVar, secret string) error {
	envVar, secret = strings.TrimSpace(envVar), strings.TrimSpace(secret)
	if envVar == "" || secret == "" {
		return invalid("save_credential_env", "eyrie engine: credential env and value are required")
	}
	if err := config.ValidateCredentialSecret(envVar, secret); err != nil {
		return &Error{Code: ErrorInvalidRequest, Operation: "save_credential_env", Message: err.Error(), Cause: err}
	}
	if err := e.secretStore.Set(nonNilContext(ctx), credentials.AccountForEnv(envVar), secret); err != nil {
		return &Error{Code: ErrorInternal, Operation: "save_credential_env", Message: err.Error(), Cause: err}
	}
	return nil
}

func (e *Engine) CredentialEnvKeys(providerID string) []string {
	if gateway, ok := e.customGateway(providerID); ok {
		if gateway.CredentialEnv == "" {
			return nil
		}
		return []string{gateway.CredentialEnv}
	}
	spec, ok := registry.SpecByProviderID(NormalizeProviderID(providerID))
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, envVar := range append(append([]string{spec.CredentialEnv}, spec.CredentialEnvFallbacks...), spec.CredentialAliases...) {
		envVar = strings.TrimSpace(envVar)
		if envVar != "" && !seen[envVar] {
			seen[envVar] = true
			out = append(out, envVar)
		}
	}
	return out
}

// GatewayRegion returns normalized region presentation for regional gateways.
func (e *Engine) GatewayRegion(providerID string) (label string, required bool) {
	cfg := config.LoadProviderConfig(e.providerConfigPath)
	return gatewayRegionFromConfig(providerID, cfg)
}

func gatewayRegionFromConfig(providerID string, cfg *config.ProviderConfig) (label string, required bool) {
	providerID = NormalizeProviderID(providerID)
	switch providerID {
	case runtime.GatewayXiaomiTokenPlan:
		if cfg == nil {
			return "", true
		}
		region, err := xiaomi.NormalizeRegion(cfg.XiaomiMimoTokenPlanRegion)
		return string(region), err != nil
	case "zai_coding":
		if cfg == nil {
			return "", true
		}
		region, err := zai.NormalizeRegion(cfg.ZAICodingRegion)
		return string(region), err != nil
	default:
		return "", false
	}
}

// SetGatewayRegion persists regional gateway configuration at the Engine path.
func (e *Engine) SetGatewayRegion(ctx context.Context, providerID, value string) error {
	ctx = nonNilContext(ctx)
	providerID = NormalizeProviderID(providerID)
	unlock := lockProviderStatePath(e.providerConfigPath)
	defer unlock()
	cfg, err := e.loadProviderConfigStrict()
	if err != nil {
		return &Error{Code: ErrorInternal, Operation: "set_gateway_region", Provider: providerID, Message: err.Error(), Cause: err}
	}
	switch providerID {
	case runtime.GatewayXiaomiTokenPlan:
		region, err := xiaomi.NormalizeRegion(value)
		if err != nil {
			return err
		}
		cfg.XiaomiMimoTokenPlanRegion = string(region)
		if base, err := config.ResolveXiaomiOpenAIBase(providerID, cfg); err == nil {
			cfg.XiaomiMimoTokenPlanBaseURL = base
		}
	case "zai_payg", "zai_coding":
		region, err := zai.NormalizeRegion(value)
		if err != nil {
			return err
		}
		if providerID == "zai_coding" {
			cfg.ZAICodingRegion = string(region)
			if base, err := zai.ResolveOpenAIBase(zai.PlanCoding, region, cfg.ZAICodingBaseURL); err == nil {
				cfg.ZAICodingBaseURL = base
			}
		} else {
			cfg.ZAIRegion = string(region)
			if base, err := zai.ResolveOpenAIBase(zai.PlanGeneral, region, cfg.ZAIBaseURL); err == nil {
				cfg.ZAIBaseURL = base
			}
		}
	default:
		return invalid("set_gateway_region", "eyrie engine: gateway does not support regions")
	}
	if err := e.saveProviderConfig(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// ApplyGatewayEnvironment derives process-local regional base URLs.
// Deprecated: invocation-scoped hosts must use Engine state directly and must
// not mutate process environment. This remains only for legacy callers.
func (e *Engine) ApplyGatewayEnvironment(_ context.Context, providerID string) {
	cfg := config.LoadProviderConfig(e.providerConfigPath)
	if cfg == nil {
		return
	}
	switch NormalizeProviderID(providerID) {
	case runtime.GatewayXiaomiTokenPlan:
		if cfg.XiaomiMimoTokenPlanRegion != "" {
			_ = os.Setenv(config.EnvXiaomiTokenPlanRegion, cfg.XiaomiMimoTokenPlanRegion)
		}
		if base, err := config.ResolveXiaomiOpenAIBase(runtime.GatewayXiaomiTokenPlan, cfg); err == nil && base != "" {
			_ = os.Setenv(config.EnvXiaomiTokenPlanBaseURL, base)
		}
	case "zai_payg":
		if cfg.ZAIRegion != "" {
			_ = os.Setenv("ZAI_REGION", cfg.ZAIRegion)
		}
		if region, err := zai.NormalizeRegion(cfg.ZAIRegion); err == nil {
			if base, err := zai.ResolveOpenAIBase(zai.PlanGeneral, region, cfg.ZAIBaseURL); err == nil && base != "" {
				_ = os.Setenv("ZAI_BASE_URL", base)
			}
		}
	case "zai_coding":
		if cfg.ZAICodingRegion != "" {
			_ = os.Setenv("ZAI_CODING_REGION", cfg.ZAICodingRegion)
		}
		if region, err := zai.NormalizeRegion(cfg.ZAICodingRegion); err == nil {
			if base, err := zai.ResolveOpenAIBase(zai.PlanCoding, region, cfg.ZAICodingBaseURL); err == nil && base != "" {
				_ = os.Setenv("ZAI_CODING_BASE_URL", base)
			}
		}
	}
}

func (e *Engine) CatalogHealth(ctx context.Context) CatalogHealth {
	health := CatalogHealth{Path: e.catalogPath}
	exists, modified, size, err := catalog.CacheInfo(e.catalogPath)
	health.Exists, health.ModifiedAt, health.Size = exists, modified, size
	if err != nil {
		health.Error = err.Error()
		return health
	}
	if !exists {
		return health
	}
	compiled, err := catalog.LoadCatalog(nonNilContext(ctx), catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
	if err != nil {
		health.Error = err.Error()
		return health
	}
	health.Models = len(compiled.ModelsByID)
	health.Deployments = len(compiled.DeploymentsByID)
	health.Offerings = len(compiled.OfferingsByID)
	if compiled.Catalog != nil {
		health.StaleAfter = compiled.Catalog.StaleAfter
		health.Stale = !compiled.Catalog.StaleAfter.IsZero() && time.Now().UTC().After(compiled.Catalog.StaleAfter)
		if compiled.Catalog.Provenance != nil {
			health.Source = compiled.Catalog.Provenance.Source
		}
	}
	return health
}

func (e *Engine) CanonicalModel(ctx context.Context, modelID string) string {
	if _, ok := e.customGatewayForModel(modelID); ok {
		return strings.TrimSpace(modelID)
	}
	compiled, err := e.policyCatalog(ctx)
	if err == nil {
		if canonical := catalog.ResolveModel(compiled, modelID); canonical != "" {
			return canonical
		}
	}
	return strings.TrimSpace(modelID)
}

func (e *Engine) GatewayForModel(ctx context.Context, modelID string) string {
	if gateway, ok := e.customGatewayForModel(modelID); ok {
		return gateway.ID
	}
	compiled, _ := e.policyCatalog(ctx)
	return NormalizeProviderID(catalog.GatewayForModel(compiled, modelID))
}

func (e *Engine) DeploymentRoutingEnabled(override *bool) bool {
	if override != nil {
		return *override
	}
	cfg, err := e.loadProviderConfigStrict()
	return err == nil && setup.DeploymentRoutingFromState(cfg)
}

func (e *Engine) DeploymentStatus(ctx context.Context, activeModel string) (string, error) {
	// Compatibility formatter remains Eyrie-owned while setup internals migrate
	// to injected paths. Host code receives presentation text only.
	report, err := setup.DeploymentStatusFromPaths(nonNilContext(ctx), activeModel, e.providerConfigPath, e.catalogPath)
	if err != nil {
		return "", err
	}
	return setup.FormatStatus(report), nil
}

func (e *Engine) DeploymentSummary(ctx context.Context, activeModel string) (DeploymentSummary, error) {
	report, err := setup.DeploymentStatusFromPaths(nonNilContext(ctx), activeModel, e.providerConfigPath, e.catalogPath)
	if err != nil {
		return DeploymentSummary{}, err
	}
	return DeploymentSummary{
		RoutingSource: report.RoutingSource, RoutingStages: report.RoutingStages,
		Formatted: setup.FormatStatus(report),
	}, nil
}

func (e *Engine) RoutingPreview(ctx context.Context, modelID string) (string, error) {
	return setup.RoutingPreviewFromPaths(nonNilContext(ctx), modelID, e.providerConfigPath, e.catalogPath)
}

// FormatSetupError returns a safe provider setup hint.
func FormatSetupError(providerID string, err error) string {
	if err == nil {
		return ""
	}
	return runtime.FormatSetupError(providerID, err).Error()
}

// CredentialGuidance returns provider-specific, non-secret setup guidance.
func CredentialGuidance(providerID, secret string) string {
	switch NormalizeProviderID(providerID) {
	case runtime.GatewayXiaomiTokenPlan:
		return xiaomi.KeyMismatchHint(xiaomi.BillingTokenPlan, secret)
	case xiaomi.ProviderPayAsYouGo:
		return xiaomi.KeyMismatchHint(xiaomi.BillingPayAsYouGo, secret)
	default:
		return ""
	}
}

func (e *Engine) DefaultProviderFilter(ctx context.Context) string {
	selection := e.EffectiveSelection(ctx, SelectionOptions{})
	return selection.Provider
}

func (e *Engine) Preflight(ctx context.Context) PreflightReport {
	return e.PreflightWithOptions(ctx, PreflightOptions{})
}

func (e *Engine) PreflightWithOptions(ctx context.Context, opts PreflightOptions) PreflightReport {
	ctx = nonNilContext(ctx)
	var checks []PreflightCheck
	cfg, stateErr := e.loadProviderConfigStrict()
	if stateErr != nil {
		checks = append(checks, PreflightCheck{Name: "provider_state", Status: CheckFail, Detail: stateErr.Error()})
		cfg = &config.ProviderConfig{}
	} else {
		checks = append(checks, PreflightCheck{Name: "provider_state", Status: CheckOK, Detail: e.providerConfigPath})
	}
	providerID := NormalizeProviderID(config.ActiveProvider(cfg))
	activeModel := strings.TrimSpace(config.ActiveModel(cfg))
	customGateway, custom := e.customGateway(providerID)

	var compiled *catalog.CompiledCatalog
	if custom {
		checks = append(checks, PreflightCheck{Name: "catalog", Status: CheckOK, Detail: "not required for selected custom gateway"})
	} else {
		health := e.CatalogHealth(ctx)
		if health.Error != "" || health.Models == 0 {
			checks = append(checks, PreflightCheck{Name: "catalog", Status: CheckFail, Detail: fmt.Sprintf("catalog unavailable at %s", health.Path)})
		} else {
			var err error
			compiled, err = catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{CachePath: e.catalogPath, RequireCache: true})
			if err != nil {
				checks = append(checks, PreflightCheck{Name: "catalog", Status: CheckFail, Detail: err.Error()})
			} else {
				checks = append(checks, PreflightCheck{Name: "catalog", Status: CheckOK, Detail: fmt.Sprintf("%d models cached at %s", health.Models, health.Path)})
			}
		}
	}
	storage := e.credentialStorage(ctx)
	status := CheckWarn
	if storage.Writable {
		status = CheckOK
	}
	checks = append(checks, PreflightCheck{Name: "credentials_store", Status: status, Detail: storage.Detail})

	credentialReady := false
	credentialDetail := "no active provider selected"
	if custom {
		credentialReady = customGateway.CredentialEnv == "" || e.hasCredential(ctx, []string{customGateway.CredentialEnv})
		credentialDetail = providerID + " credential is not configured"
		if credentialReady {
			credentialDetail = providerID + " credential is configured"
		}
	} else if providerID != "" && compiled != nil && stateErr == nil {
		if spec, ok := registry.SpecByProviderID(providerID); ok {
			env := e.discoveryCredentialsFromConfig(ctx, compiled, cfg).Env()
			deployments := buildDeployments(compiled, cfg.Deployments, env)
			if deployment, ok := deployments[spec.DeploymentID]; ok {
				_, credentialReady = setup.ProviderForDeploymentFromState(spec.DeploymentID, deployment, cfg)
			}
			credentialDetail = providerID + " deployment is not fully configured"
			if credentialReady {
				credentialDetail = providerID + " deployment is configured"
			}
		} else {
			credentialDetail = "unknown active provider " + providerID
		}
	}
	credentialStatus := CheckFail
	if credentialReady {
		credentialStatus = CheckOK
	}
	checks = append(checks, PreflightCheck{Name: "credentials", Status: credentialStatus, Detail: credentialDetail})

	modelDetail := activeModel
	switch {
	case activeModel == "":
		checks = append(checks, PreflightCheck{Name: "model", Status: CheckFail, Detail: "no model selected"})
	case custom:
		checks = append(checks, PreflightCheck{Name: "model", Status: CheckOK, Detail: activeModel})
	case compiled != nil && providerModelAvailable(compiled, providerID, activeModel):
		checks = append(checks, PreflightCheck{Name: "model", Status: CheckOK, Detail: activeModel})
	default:
		checks = append(checks, PreflightCheck{Name: "model", Status: CheckFail, Detail: modelDetail + " is not available through " + providerID})
	}
	localReady := true
	for _, check := range checks {
		if check.Status == CheckFail {
			localReady = false
		}
	}
	liveVerified := false
	switch {
	case !opts.VerifyLive:
		checks = append(checks, PreflightCheck{Name: "provider_live", Status: CheckWarn, Detail: "not checked (local preflight only)"})
	case !localReady:
		checks = append(checks, PreflightCheck{Name: "provider_live", Status: CheckWarn, Detail: "skipped until local setup is complete"})
	default:
		var err error
		if custom {
			err = e.probeCustomGateway(ctx, customGateway, "")
		} else {
			var liveModels []Model
			liveModels, err = e.ListLiveModels(ctx, providerID)
			if err == nil && !modelListContains(liveModels, activeModel) {
				err = &Error{
					Code: ErrorModelUnavailable, Operation: "preflight", Provider: providerID, Model: activeModel,
					Message: fmt.Sprintf("eyrie engine: selected model %q is not present in %s live model list", activeModel, providerID),
				}
			}
		}
		if err != nil {
			checks = append(checks, PreflightCheck{Name: "provider_live", Status: CheckFail, Detail: fmt.Sprintf("%s verification failed: %v", providerID, err)})
		} else {
			liveVerified = true
			checks = append(checks, PreflightCheck{Name: "provider_live", Status: CheckOK, Detail: providerID + " connectivity and authentication verified"})
		}
	}
	ready := localReady
	if opts.VerifyLive && !liveVerified {
		ready = false
	}
	return PreflightReport{Ready: ready, LiveVerified: liveVerified, Checks: checks}
}

func modelListContains(models []Model, modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	for _, model := range models {
		if strings.TrimSpace(model.ID) == modelID || strings.TrimSpace(model.CanonicalID) == modelID {
			return true
		}
	}
	return false
}

func providerModelAvailable(compiled *catalog.CompiledCatalog, providerID, modelID string) bool {
	if compiled == nil || providerID == "" || strings.TrimSpace(modelID) == "" {
		return false
	}
	if _, ok := catalog.CanonicalModelForProviderNative(compiled, providerID, modelID); ok {
		return true
	}
	canonical, ok := compiled.CanonicalModelForAliasOrID(modelID)
	if !ok {
		canonical = strings.TrimSpace(modelID)
	}
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return false
	}
	for _, offering := range compiled.OfferingsByDeployment[spec.DeploymentID] {
		if offering.CanonicalModelID == canonical || offering.NativeModelID == modelID {
			return true
		}
	}
	return false
}

func FormatPreflight(report PreflightReport) string {
	var b strings.Builder
	switch {
	case report.Ready && report.LiveVerified:
		b.WriteString("Preflight: ready to chat (live verified)\n")
	case report.Ready:
		b.WriteString("Preflight: locally ready to chat\n")
	default:
		b.WriteString("Preflight: setup incomplete\n")
	}
	for _, check := range report.Checks {
		icon := "+"
		switch check.Status {
		case CheckWarn:
			icon = "!"
		case CheckFail:
			icon = "x"
		}
		fmt.Fprintf(&b, "  %s %s: %s\n", icon, check.Name, check.Detail)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ProviderStateSecurityStatus inspects provider.json without returning values.
func (e *Engine) ProviderStateSecurityStatus() ProviderStateSecurity {
	status := ProviderStateSecurity{Path: e.providerConfigPath}
	cfg, err := config.LoadProviderConfigWithError(e.providerConfigPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	if cfg == nil {
		return status
	}
	if config.ProviderConfigContainsSecrets(*cfg) {
		status.HasSecrets = true
		status.Detail = "provider state has credential fields on disk"
		return status
	}
	return status
}

// MigrateProviderSecrets imports credential fields persisted in provider
// state into the Engine's secret store, then atomically strips them from
// provider.json.
func (e *Engine) MigrateProviderSecrets() error {
	return e.MigrateProviderSecretsContext(context.Background())
}

func (e *Engine) MigrateProviderSecretsContext(ctx context.Context) error {
	ctx = nonNilContext(ctx)
	path := e.providerConfigPath
	unlock := lockProviderStatePath(path)
	defer unlock()
	marker := path + ".secrets-migrated"
	cfgState, err := config.LoadProviderConfigWithError(path)
	if err != nil {
		return err
	}
	if cfgState == nil {
		return nil
	}
	cfg := *cfgState
	writes, err := e.importProviderConfigSecrets(ctx, cfg)
	if err != nil {
		return &Error{Code: ErrorInternal, Operation: "migrate_provider_secrets", Message: "eyrie engine: could not import provider credentials", Cause: err}
	}
	sanitized := config.SanitizeProviderConfigForDisk(cfg)
	if err := writeProviderConfigAtomic(path, &sanitized); err != nil {
		return errors.Join(err, e.rollbackImportedCredentials(ctx, writes))
	}
	if err := writeBytesAtomic(marker, []byte("ok\n")); err != nil {
		return err
	}
	return nil
}

func writeProviderConfigAtomic(path string, cfg *config.ProviderConfig) (err error) {
	return config.SaveProviderConfig(cfg, path)
}

func writeBytesAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provider-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
