package setup

import (
	"sort"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// ModelUI is one selectable model for host /config UIs.
type ModelUI struct {
	CanonicalID string `json:"canonical_id"`
	DisplayName string `json:"display_name"`
	Source      string `json:"source,omitempty"` // "remote", "live", or "remote+live"
}

// ProviderUI is a provider and its models after credential apply.
type ProviderUI struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Models      []ModelUI `json:"models"`
}

// SetupUI is JSON-safe metadata returned to hawk (no secrets).
type SetupUI struct {
	Providers []ProviderUI `json:"providers"`
}

// BuildSetupUI lists models per provider from the compiled catalog.
// If providerFilter is non-empty, only that provider is included.
func BuildSetupUI(compiled *catalog.CompiledCatalog, providerFilter string) *SetupUI {
	if compiled == nil {
		return &SetupUI{}
	}
	providerFilter = strings.TrimSpace(providerFilter)
	seenProv := map[string]bool{}
	var providerIDs []string
	for _, model := range compiled.ModelsByID {
		pid := catalog.CanonicalProviderID(model.ProviderID)
		if pid == "" || seenProv[pid] {
			continue
		}
		if providerFilter != "" && pid != catalog.CanonicalProviderID(providerFilter) {
			continue
		}
		seenProv[pid] = true
		providerIDs = append(providerIDs, pid)
	}
	sort.Strings(providerIDs)

	ui := &SetupUI{Providers: make([]ProviderUI, 0, len(providerIDs))}
	for _, pid := range providerIDs {
		pu := ProviderUI{
			ID:          pid,
			DisplayName: displayNameForProvider(pid),
			Models:      modelsForProvider(compiled, pid),
		}
		if len(pu.Models) == 0 {
			continue
		}
		ui.Providers = append(ui.Providers, pu)
	}
	return ui
}

func displayNameForProvider(pid string) string {
	if name := catalog.ProviderDisplayName(pid); name != pid {
		return name
	}
	switch pid {
	case "google":
		return registry.DisplayName("gemini")
	case "xai":
		return registry.DisplayName("grok")
	default:
		return pid
	}
}

func modelsForProvider(compiled *catalog.CompiledCatalog, providerID string) []ModelUI {
	var ids []string
	for id, model := range compiled.ModelsByID {
		if catalog.CanonicalProviderID(model.ProviderID) == providerID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]ModelUI, 0, len(ids))
	for _, id := range ids {
		model := compiled.ModelsByID[id]
		label := strings.TrimSpace(model.Name)
		if label == "" {
			label = id
			if i := strings.LastIndex(id, "/"); i >= 0 {
				label = id[i+1:]
			}
		}
		out = append(out, ModelUI{
			CanonicalID: id,
			DisplayName: label,
			Source:      modelProvenanceSource(model),
		})
	}
	return out
}

func modelProvenanceSource(model catalog.Model) string {
	if model.Provenance == nil || model.Provenance.Source == "" {
		return ""
	}
	return model.Provenance.Source
}

// ProviderIDForDeployment resolves catalog provider id for a deployment id.
func ProviderIDForDeployment(compiled *catalog.CompiledCatalog, deploymentID string) string {
	if compiled == nil || compiled.DeploymentsByID == nil {
		return ""
	}
	if dep, ok := compiled.DeploymentsByID[deploymentID]; ok {
		return catalog.CanonicalProviderID(dep.ProviderID)
	}
	return ""
}
