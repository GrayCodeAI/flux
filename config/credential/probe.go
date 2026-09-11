package credential

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/internal/probehttp"
)

const credentialProbeTimeout = 8 * time.Second

// MimoProbeConfig carries Token Plan / payg routing from provider.json for probes.
type MimoProbeConfig struct {
	TokenPlanRegion string
	TokenPlanBase   string
	PaygBase        string
}

var mimoProbeConfigLoader func() MimoProbeConfig

// SetMimoProbeConfigLoader supplies MiMo region/base from host config (e.g. provider.json).
func SetMimoProbeConfigLoader(fn func() MimoProbeConfig) {
	mimoProbeConfigLoader = fn
}

func loadMimoProbeConfig() MimoProbeConfig {
	if mimoProbeConfigLoader != nil {
		return mimoProbeConfigLoader()
	}
	return MimoProbeConfig{}
}

// ProbeCredential verifies a key against the provider API when a probe is configured.
func ProbeCredential(ctx context.Context, envKey, secret string) error {
	return ProbeCredentialWithMimo(ctx, envKey, secret, loadMimoProbeConfig())
}

// ProbeCredentialWithMimo is like ProbeCredential but accepts persisted MiMo routing fields.
func ProbeCredentialWithMimo(ctx context.Context, envKey, secret string, mimo MimoProbeConfig) error {
	return probeCredentialWithMimo(ctx, envKey, secret, mimo, true)
}

// ProbeCredentialWithMimoStrict probes with explicitly supplied routing only.
// It never reads process environment for MiMo base URLs or regions.
func ProbeCredentialWithMimoStrict(ctx context.Context, envKey, secret string, mimo MimoProbeConfig) error {
	return probeCredentialWithMimo(ctx, envKey, secret, mimo, false)
}

func probeCredentialWithMimo(ctx context.Context, envKey, secret string, mimo MimoProbeConfig, allowAmbient bool) error {
	secret = strings.TrimSpace(secret)
	envKey = strings.TrimSpace(envKey)
	if secret == "" || envKey == "" {
		return fmt.Errorf("credential probe: key and env var required")
	}
	spec, ok := registry.DefaultRegistry.GetByEnv(envKey)
	if !ok {
		return nil
	}
	if spec.ProbeKind == "" || spec.ProbeKind == registry.ProbeNone {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, credentialProbeTimeout)
	defer cancel()

	switch spec.ProbeKind {
	case registry.ProbeOpenAIModels:
		baseURL := spec.ProbeBaseURL
		if _, ok := xiaomi.BillingForProvider(spec.ProviderID); ok {
			baseURL = resolveMimoProbeBaseURL(spec, mimo, allowAmbient)
			if baseURL == "" {
				return fmt.Errorf("credential probe: configure Token Plan region (cn, sgp, or ams) before probing")
			}
			return xiaomi.ProbeOpenAIModels(ctx, baseURL, secret)
		}
		return probeOpenAIModels(ctx, baseURL, secret)
	case registry.ProbeAnthropic:
		return probeAnthropic(ctx, secret)
	case registry.ProbeGemini:
		return probeGemini(ctx, secret)
	case registry.ProbeOllama:
		err := probeOllama(ctx, secret)
		if err != nil {
			return FormatOllamaConnectError(err)
		}
		return nil
	default:
		return nil
	}
}

func probeOllama(ctx context.Context, baseURL string) error {
	models, err := catalog.FetchOllamaModels(map[string]string{"OLLAMA_BASE_URL": baseURL})
	if err != nil {
		return FormatOllamaConnectError(err)
	}
	if len(models) == 0 {
		return errOllamaNoModels
	}
	return nil
}

func resolveMimoProbeBaseURL(spec registry.ProviderSpec, mimo MimoProbeConfig, allowAmbient bool) string {
	billing, _ := xiaomi.BillingForProvider(spec.ProviderID)
	override := ""
	var region xiaomi.Region
	switch billing {
	case xiaomi.BillingTokenPlan:
		override = strings.TrimSpace(mimo.TokenPlanBase)
		if override == "" && allowAmbient {
			override = strings.TrimSpace(os.Getenv("XIAOMI_MIMO_TOKEN_PLAN_BASE_URL"))
		}
		region, _ = xiaomi.NormalizeRegion(mimo.TokenPlanRegion)
		if region == "" && allowAmbient {
			region, _ = xiaomi.NormalizeRegion(os.Getenv("XIAOMI_MIMO_TOKEN_PLAN_REGION"))
		}
	default:
		override = strings.TrimSpace(mimo.PaygBase)
		if override == "" && allowAmbient {
			override = strings.TrimSpace(os.Getenv("XIAOMI_MIMO_PAYG_BASE_URL"))
		}
	}
	base, err := xiaomi.ResolveOpenAIBasePreferRegion(billing, region, override)
	if err != nil {
		return strings.TrimSpace(spec.ProbeBaseURL)
	}
	return base
}

func probeOpenAIModels(ctx context.Context, baseURL, secret string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("credential probe: missing base URL")
	}
	status, _, err := probehttp.DoGet(ctx, baseURL+"/models", map[string]string{
		"Authorization": "Bearer " + secret,
	})
	if err != nil {
		return fmt.Errorf("credential probe: network error: %w", err)
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return probehttp.ProbeError(status)
}

func probeAnthropic(ctx context.Context, secret string) error {
	status, _, err := probehttp.DoGet(ctx, "https://api.anthropic.com/v1/models", map[string]string{
		"x-api-key":         secret,
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return fmt.Errorf("credential probe: network error: %w", err)
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return probehttp.ProbeError(status)
}

func probeGemini(ctx context.Context, secret string) error {
	status, _, err := probehttp.DoGet(ctx, "https://generativelanguage.googleapis.com/v1beta/models", map[string]string{
		"x-goog-api-key": secret,
	})
	if err != nil {
		return fmt.Errorf("credential probe: network error: %w", err)
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return probehttp.ProbeError(status)
}
