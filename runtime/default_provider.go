package runtime

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
)

// DefaultModelProviderFilter returns the catalog provider id to use when listing models
// with no explicit provider (e.g. /config model picker after paste-key).
// Order: provider.json default → first configured deployment (stable sort by id).
func DefaultModelProviderFilter(ctx context.Context) string {
	rt, err := Load(ctx)
	if err != nil || rt == nil {
		return ""
	}
	if rt.Provider != nil {
		if p := config.DefaultProviderFromConfig(rt.Provider); p != "" {
			return catalog.CanonicalProviderID(p)
		}
	}
	rows, err := rt.DeploymentRows()
	if err != nil || len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for _, row := range rows {
		if row.Configured {
			if p := catalog.CanonicalProviderID(row.ProviderID); p != "" {
				return p
			}
		}
	}
	return ""
}

// PreferredProvider returns the runtime-owned provider choice when a host has
// not pinned one explicitly. Active selection wins first, then inferred model
// ownership, then configured providers ordered by runtime preference, and
// finally credential detection as a last resort.
func PreferredProvider(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	return preferredProviderWithState(ctx, newSelectionRuntimeState(ctx))
}

func preferredProviderWithState(ctx context.Context, state *selectionRuntimeState) string {
	if provider := normalizeRuntimeProviderID(ActiveProvider(ctx)); provider != "" && providerConfiguredWithState(state, provider) {
		return provider
	}
	if model := ActiveModel(ctx); model != "" {
		if provider := inferProviderForModel(ctx, model); provider != "" && providerConfiguredWithState(state, provider) {
			return provider
		}
	}
	if provider := preferredConfiguredProviderWithState(state); provider != "" {
		return provider
	}
	return preferredDetectedProviderWithState(state)
}

func preferredConfiguredProviderWithState(state *selectionRuntimeState) string {
	compiled := state.compiledCatalog()
	if compiled == nil || compiled.Catalog == nil {
		return ""
	}
	env := state.env()
	configured := make(map[string]struct{}, len(compiled.Catalog.Deployments))
	for id, dep := range compiled.Catalog.Deployments {
		dc := config.DeploymentConfigFromEnv(dep, env)
		if !config.DeploymentConfigured(id, dep, dc) {
			continue
		}
		if provider := catalog.CanonicalProviderID(dep.ProviderID); provider != "" {
			configured[provider] = struct{}{}
		}
	}
	for _, provider := range registry.ChatProviderPreferenceOrder() {
		if _, ok := configured[provider]; ok {
			return provider
		}
	}

	ordered := make([]string, 0, len(configured))
	for provider := range configured {
		ordered = append(ordered, provider)
	}
	sort.Strings(ordered)
	if len(ordered) == 0 {
		return ""
	}
	return ordered[0]
}

func preferredDetectedProviderWithState(state *selectionRuntimeState) string {
	env := state.env()
	for _, provider := range registry.ChatProviderPreferenceOrder() {
		profile, ok := runtimeProfileForProvider(provider)
		if !ok {
			continue
		}
		ready := true
		for _, envKey := range profile.DetectionEnv {
			if strings.TrimSpace(env[envKey]) == "" && runtimeEnvValue(envKey) == "" {
				ready = false
				break
			}
		}
		if ready {
			return provider
		}
	}
	return ""
}

func runtimeProfileForProvider(provider string) (config.RuntimeProviderProfile, bool) {
	key := registry.RuntimeProfileKey(provider)
	if key == "" {
		return config.RuntimeProviderProfile{}, false
	}
	return config.RuntimeProfileByKey(key)
}

func runtimeEnvValue(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if value := credentials.LookupSecret(ctx, key); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(key))
}
