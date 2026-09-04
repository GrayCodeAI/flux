package setup

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/config"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestFormatStatus_DisabledRouting(t *testing.T) {
	report := StatusReport{
		DeploymentRouting: false,
		ProviderConfig:    "/home/user/.hawk/provider.json",
		ConfigVersion:     2,
		Configured:        []string{},
		CatalogCache:      "/home/user/.graycode-router/model_catalog.json",
		CatalogExists:     false,
		CatalogModels:     10,
	}
	out := FormatStatus(report)
	if !strings.Contains(out, "disabled (single-route GraycodeRouter transport)") {
		t.Fatal("expected engine-owned disabled-routing description")
	}
	if strings.Contains(out, "legacy provider client") {
		t.Fatal("status output still describes the retired host-side provider client")
	}
	if !strings.Contains(out, "none (set API keys or deployments in provider.json)") {
		t.Fatal("expected 'none' deployment message")
	}
	if !strings.Contains(out, "using embedded catalog: 10 models") {
		t.Fatal("expected embedded catalog message")
	}
}

func TestFormatStatus_EnabledRouting(t *testing.T) {
	report := StatusReport{
		DeploymentRouting:  true,
		ProviderConfig:     "/home/user/.hawk/provider.json",
		ConfigVersion:      2,
		Configured:         []string{"anthropic-direct", "openai-direct"},
		CatalogCache:       "/home/user/.graycode-router/model_catalog.json",
		CatalogExists:      true,
		CatalogModified:    time.Now().UTC().Add(-5 * time.Minute),
		CatalogModels:      50,
		CatalogDeployments: 3,
		CatalogOfferings:   100,
	}
	out := FormatStatus(report)
	if !strings.Contains(out, "enabled") {
		t.Fatal("expected 'enabled' in output")
	}
	if !strings.Contains(out, "anthropic-direct, openai-direct") {
		t.Fatal("expected deployment list in output")
	}
	if !strings.Contains(out, "50 models") {
		t.Fatal("expected model count in output")
	}
	if !strings.Contains(out, "3 deployments") {
		t.Fatal("expected deployment count in output")
	}
	if !strings.Contains(out, "100 offerings") {
		t.Fatal("expected offering count in output")
	}
}

func TestFormatStatus_StaleCatalog(t *testing.T) {
	report := StatusReport{
		DeploymentRouting: true,
		ProviderConfig:    "/home/user/.hawk/provider.json",
		CatalogCache:      "/home/user/.graycode-router/model_catalog.json",
		CatalogExists:     true,
		CatalogModified:   time.Now().UTC().Add(-1 * time.Hour),
		CatalogStale:      true,
	}
	out := FormatStatus(report)
	if !strings.Contains(out, "stale: yes") {
		t.Fatal("expected stale warning in output")
	}
}

func TestFormatStatus_ActiveModel(t *testing.T) {
	report := StatusReport{
		DeploymentRouting: true,
		ProviderConfig:    "/home/user/.hawk/provider.json",
		CatalogCache:      "/home/user/.graycode-router/model_catalog.json",
		ActiveModel:       "anthropic/claude-sonnet-4",
		RoutingSource:     "model",
		RoutingStages:     2,
	}
	out := FormatStatus(report)
	if !strings.Contains(out, "Active canonical model: anthropic/claude-sonnet-4") {
		t.Fatal("expected active model in output")
	}
	if !strings.Contains(out, "Routing: model (2 stages)") {
		t.Fatal("expected routing info in output")
	}
}

func TestFormatStatus_NoActiveModel(t *testing.T) {
	report := StatusReport{
		DeploymentRouting: true,
		ProviderConfig:    "/home/user/.hawk/provider.json",
		CatalogCache:      "/home/user/.graycode-router/model_catalog.json",
	}
	out := FormatStatus(report)
	if strings.Contains(out, "Active canonical model") {
		t.Fatal("should not contain active model when not set")
	}
}

func TestFormatStatus_ConfigVersion(t *testing.T) {
	report := StatusReport{
		DeploymentRouting: false,
		ProviderConfig:    "/home/user/.hawk/provider.json",
		ConfigVersion:     2,
	}
	out := FormatStatus(report)
	if !strings.Contains(out, "(v2)") {
		t.Fatal("expected version in output")
	}
}

func TestSortStrings(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"sorted", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"reverse", []string{"c", "b", "a"}, []string{"a", "b", "c"}},
		{"duplicates", []string{"b", "a", "b"}, []string{"a", "b", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortStrings(tt.input)
			if len(tt.input) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(tt.input), len(tt.want))
			}
			for i := range tt.want {
				if tt.input[i] != tt.want[i] {
					t.Fatalf("index %d = %q, want %q", i, tt.input[i], tt.want[i])
				}
			}
		})
	}
}

func TestSaveProviderConfigV2_Nil(t *testing.T) {
	// Should not panic.
	if err := SaveProviderConfigV2(nil); err != nil {
		t.Fatalf("expected nil error for nil config, got %v", err)
	}
}

func TestDeploymentStatusFromPathsIgnoresAmbientRoutingAndCredentials(t *testing.T) {
	dir := t.TempDir()
	providerPath := filepath.Join(dir, "provider.json")
	catalogPath := filepath.Join(dir, "model_catalog.json")
	if err := config.SaveProviderConfig(&config.ProviderConfig{}, providerPath); err != nil {
		t.Fatal(err)
	}
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(catalogPath, &seed); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_DEPLOYMENT_ROUTING", "true")
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "ambient-secret-1234567890"); err != nil {
		t.Fatal(err)
	}
	report, err := DeploymentStatusFromPaths(context.Background(), "", providerPath, catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.DeploymentRouting || len(report.Configured) != 0 {
		t.Fatalf("host-scoped status used ambient state: %+v", report)
	}
}
