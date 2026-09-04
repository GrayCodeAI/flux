package setup

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/router"
)

// StatusReport summarizes deployment routing readiness (graycode-router provider status).
type StatusReport struct {
	DeploymentRouting  bool
	ProviderConfig     string
	ConfigVersion      int
	Configured         []string
	CatalogCache       string
	CatalogExists      bool
	CatalogModified    time.Time
	CatalogStale       bool
	CatalogModels      int
	CatalogDeployments int
	CatalogOfferings   int
	ActiveModel        string
	RoutingSource      string
	RoutingStages      int
}

// DeploymentStatus builds a status report for CLI and agent diagnostics.
func DeploymentStatus(ctx context.Context, activeModel string) (StatusReport, error) {
	providerPath, err := config.GetProviderConfigPath()
	if err != nil {
		return StatusReport{}, err
	}
	return deploymentStatusFromPaths(ctx, activeModel, providerPath, catalog.DefaultCachePath(), true)
}

// DeploymentStatusFromPaths builds a status report from host-owned state.
// Empty paths retain the process defaults for backwards compatibility.
func DeploymentStatusFromPaths(ctx context.Context, activeModel, providerConfigPath, catalogCachePath string) (StatusReport, error) {
	return deploymentStatusFromPaths(ctx, activeModel, providerConfigPath, catalogCachePath, false)
}

func deploymentStatusFromPaths(ctx context.Context, activeModel, providerConfigPath, catalogCachePath string, allowAmbient bool) (StatusReport, error) {
	if strings.TrimSpace(providerConfigPath) == "" {
		providerConfigPath, _ = config.GetProviderConfigPath()
	}
	if strings.TrimSpace(catalogCachePath) == "" {
		catalogCachePath = catalog.DefaultCachePath()
	}
	cfg, err := config.LoadProviderConfigWithError(providerConfigPath)
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{
		ProviderConfig: providerConfigPath,
		ActiveModel:    strings.TrimSpace(activeModel),
	}
	if cfg != nil {
		report.ConfigVersion = cfg.ConfigVersion
	}
	report.DeploymentRouting = DeploymentRoutingFromState(cfg)
	configured := explicitDeployments(cfg)
	if allowAmbient {
		report.DeploymentRouting = UseDeploymentRouting(cfg)
		configured = ConfiguredDeployments(cfg)
	}

	for id := range configured {
		report.Configured = append(report.Configured, id)
	}
	sortStrings(report.Configured)

	report.CatalogCache = catalogCachePath
	if exists, mod, _, err := catalog.CacheInfo(report.CatalogCache); err == nil && exists {
		report.CatalogExists = true
		report.CatalogModified = mod
	}

	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
		CachePath: report.CatalogCache,
	})
	if err != nil {
		return report, err
	}
	report.CatalogModels = len(compiled.ModelsByID)
	report.CatalogDeployments = len(compiled.DeploymentsByID)
	report.CatalogOfferings = len(compiled.OfferingsByID)
	report.CatalogStale = time.Now().UTC().After(compiled.Catalog.StaleAfter)

	if report.ActiveModel != "" && cfg != nil {
		policy := RouterRoutingPolicy(cfg.Routing)
		res := router.ResolveRouting(report.ActiveModel, compiled, policy)
		report.RoutingSource = res.Source
		report.RoutingStages = len(res.Stages)
		if res.CanonicalModelID != "" {
			report.ActiveModel = res.CanonicalModelID
		}
	}
	return report, nil
}

// FormatStatus renders StatusReport for terminal output.
func FormatStatus(report StatusReport) string {
	var b strings.Builder
	b.WriteString("Deployment routing: ")
	if report.DeploymentRouting {
		b.WriteString("enabled\n")
	} else {
		b.WriteString("disabled (single-route GraycodeRouter transport)\n")
	}
	fmt.Fprintf(&b, "Provider config: %s", report.ProviderConfig)
	if report.ConfigVersion > 0 {
		fmt.Fprintf(&b, " (v%d)", report.ConfigVersion)
	}
	b.WriteString("\n")
	if len(report.Configured) > 0 {
		b.WriteString("Configured deployments: " + strings.Join(report.Configured, ", ") + "\n")
	} else {
		b.WriteString("Configured deployments: none (set API keys or deployments in provider.json)\n")
	}
	fmt.Fprintf(&b, "Catalog cache: %s\n", report.CatalogCache)
	if report.CatalogExists {
		age := time.Since(report.CatalogModified).Truncate(time.Second)
		fmt.Fprintf(&b, "  cached: yes (modified %s ago, %d models, %d deployments, %d offerings)\n",
			age, report.CatalogModels, report.CatalogDeployments, report.CatalogOfferings)
	} else {
		fmt.Fprintf(&b, "  cached: no (using embedded catalog: %d models)\n", report.CatalogModels)
	}
	if report.CatalogStale {
		b.WriteString("  stale: yes — hawk refreshes automatically; use `hawk models refresh` or `/refresh-model-catalog` for a manual run\n")
	}
	if report.ActiveModel != "" {
		fmt.Fprintf(&b, "Active canonical model: %s\n", report.ActiveModel)
		if report.RoutingSource != "" {
			fmt.Fprintf(&b, "Routing: %s (%d stages)\n", report.RoutingSource, report.RoutingStages)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RoutingPreview returns JSON describing effective routing for a model ID.
func RoutingPreview(ctx context.Context, model string) (string, error) {
	providerPath, err := config.GetProviderConfigPath()
	if err != nil {
		return "", err
	}
	return RoutingPreviewFromPaths(ctx, model, providerPath, catalog.DefaultCachePath())
}

// RoutingPreviewFromPaths resolves routing from host-owned state paths.
// Empty paths retain the process defaults for backwards compatibility.
func RoutingPreviewFromPaths(ctx context.Context, model, providerConfigPath, catalogCachePath string) (string, error) {
	if strings.TrimSpace(providerConfigPath) == "" {
		providerConfigPath, _ = config.GetProviderConfigPath()
	}
	if strings.TrimSpace(catalogCachePath) == "" {
		catalogCachePath = catalog.DefaultCachePath()
	}
	cfg, err := config.LoadProviderConfigWithError(providerConfigPath)
	if err != nil {
		return "", err
	}
	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
		CachePath: catalogCachePath,
	})
	if err != nil {
		return "", err
	}
	policy := RouterRoutingPolicy(nil)
	if cfg != nil {
		policy = RouterRoutingPolicy(cfg.Routing)
	}
	return router.RoutingPreviewJSON(model, compiled, policy)
}

// SaveProviderConfigV2 writes migrated provider config when upgraded in memory.
func SaveProviderConfigV2(cfg *config.ProviderConfig) error {
	if cfg == nil {
		return nil
	}
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil
	}
	return config.SaveProviderConfig(cfg, path)
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
