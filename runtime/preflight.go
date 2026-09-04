package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	graycoderoutercfg "github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

// PreflightStatus is ok, warn, or fail.
type PreflightStatus string

const (
	PreflightOK   PreflightStatus = "ok"
	PreflightWarn PreflightStatus = "warn"
	PreflightFail PreflightStatus = "fail"
)

// PreflightCheck is one readiness row.
type PreflightCheck struct {
	Name   string          `json:"name"`
	Status PreflightStatus `json:"status"`
	Detail string          `json:"detail"`
}

// PreflightReport summarizes whether hawk can chat.
type PreflightReport struct {
	Ready  bool             `json:"ready"`
	Checks []PreflightCheck `json:"checks"`
}

// Preflight evaluates catalog, credentials, model selection, and optional live model access.
func Preflight(ctx context.Context) PreflightReport {
	if ctx == nil {
		ctx = context.Background()
	}
	var checks []PreflightCheck

	// Catalog cache
	cachePath := catalog.DefaultCachePath()
	exists, _, size, _ := catalog.CacheInfo(cachePath)
	if !exists || size == 0 {
		checks = append(checks, PreflightCheck{
			Name: "catalog", Status: PreflightWarn,
			Detail: "model catalog cache missing — hawk will discover on /config or refresh automatically",
		})
	} else {
		compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
			CachePath: cachePath, RequireCache: false,
		})
		nModels := 0
		if compiled != nil {
			nModels = len(compiled.ModelsByID)
		}
		if err != nil || nModels == 0 {
			checks = append(checks, PreflightCheck{
				Name: "catalog", Status: PreflightWarn,
				Detail: fmt.Sprintf("catalog at %s unreadable or empty", cachePath),
			})
		} else {
			stale := ""
			if compiled.Catalog != nil && !compiled.Catalog.StaleAfter.IsZero() {
				stale = fmt.Sprintf(", stale after %s", compiled.Catalog.StaleAfter.UTC().Format("2006-01-02"))
			}
			checks = append(checks, PreflightCheck{
				Name: "catalog", Status: PreflightOK,
				Detail: fmt.Sprintf("%d models cached at %s%s", nModels, cachePath, stale),
			})
		}
	}

	// Credential store
	if ok, detail := credentials.StorageStatus(ctx); ok {
		checks = append(checks, PreflightCheck{Name: "credentials_store", Status: PreflightOK, Detail: detail})
	} else {
		checks = append(checks, PreflightCheck{Name: "credentials_store", Status: PreflightWarn, Detail: detail})
	}

	// Keychain write (warn only when OS store unavailable)
	if ok, detail := credentials.KeychainWriteAvailable(ctx); ok {
		checks = append(checks, PreflightCheck{Name: "keychain", Status: PreflightOK, Detail: detail})
	} else {
		checks = append(checks, PreflightCheck{Name: "keychain", Status: PreflightWarn, Detail: detail})
	}

	// Provider credentials configured
	hasCreds := graycoderoutercfg.HasAnyConfiguredDeployment(ctx)
	if !hasCreds {
		checks = append(checks, PreflightCheck{
			Name: "credentials", Status: PreflightFail,
			Detail: "no provider credentials — run /config and paste an API key or configure Ollama",
		})
	} else {
		checks = append(checks, PreflightCheck{
			Name: "credentials", Status: PreflightOK,
			Detail: "at least one provider credential is configured in " + credentials.PlatformSecretStoreName(),
		})
	}

	// Model selection
	model := strings.TrimSpace(ActiveModel(ctx))
	if model == "" {
		checks = append(checks, PreflightCheck{
			Name: "model", Status: PreflightFail,
			Detail: "no model selected — run /config and pick a model",
		})
	} else {
		checks = append(checks, PreflightCheck{
			Name: "model", Status: PreflightOK,
			Detail: model,
		})
	}

	// Live model reachability for active provider (best effort)
	provider := strings.TrimSpace(ActiveProvider(ctx))
	if provider == "" && model != "" {
		provider = inferProviderForModel(ctx, model)
	}
	if provider != "" && hasCreds {
		entries, err := ListModels(ctx, ListModelsOpts{ProviderID: provider, Source: ListSourceAuto})
		switch {
		case err != nil:
			checks = append(checks, PreflightCheck{
				Name: "models_live", Status: PreflightWarn,
				Detail: FormatSetupError(provider, err).Error(),
			})
		case len(entries) == 0:
			checks = append(checks, PreflightCheck{
				Name: "models_live", Status: PreflightWarn,
				Detail: fmt.Sprintf("no models listed for provider %q", provider),
			})
		default:
			checks = append(checks, PreflightCheck{
				Name: "models_live", Status: PreflightOK,
				Detail: fmt.Sprintf("%d models available for %q", len(entries), provider),
			})
		}
	}

	ready := true
	for _, c := range checks {
		if c.Status == PreflightFail {
			ready = false
			break
		}
	}
	return PreflightReport{Ready: ready, Checks: checks}
}

// FormatPreflightReport returns human-readable preflight output.
func FormatPreflightReport(r PreflightReport) string {
	var b strings.Builder
	if r.Ready {
		b.WriteString("Preflight: ready to chat\n")
	} else {
		b.WriteString("Preflight: setup incomplete\n")
	}
	for _, c := range r.Checks {
		icon := "+"
		switch c.Status {
		case PreflightWarn:
			icon = "!"
		case PreflightFail:
			icon = "x"
		}
		fmt.Fprintf(&b, "  %s %s: %s\n", icon, c.Name, c.Detail)
	}
	return strings.TrimRight(b.String(), "\n")
}
