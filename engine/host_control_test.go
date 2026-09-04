package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestCatalogHealthMissingCacheDoesNotUseBootstrapFallback(t *testing.T) {
	eng := newHostControlTestEngine(t)

	health := eng.CatalogHealth(context.Background())
	if health.Exists {
		t.Fatalf("missing cache reported as existing: %+v", health)
	}
	if health.Error != "" || health.Models != 0 || health.Source != "" || health.Stale {
		t.Fatalf("missing cache inherited bootstrap health: %+v", health)
	}
}

func TestCatalogHealthRejectsInvalidAndBootstrapCaches(t *testing.T) {
	tests := []struct {
		name  string
		write func(*testing.T, string)
	}{
		{
			name: "empty file",
			write: func(t *testing.T, path string) {
				writeHostControlTestFile(t, path, nil)
			},
		},
		{
			name: "corrupt JSON",
			write: func(t *testing.T, path string) {
				writeHostControlTestFile(t, path, []byte("{not-json"))
			},
		},
		{
			name: "empty model catalog",
			write: func(t *testing.T, path string) {
				seed := catalog.SeedCatalog()
				seed.Models = nil
				seed.Offerings = nil
				data, err := json.Marshal(seed)
				if err != nil {
					t.Fatal(err)
				}
				writeHostControlTestFile(t, path, data)
			},
		},
		{
			name: "bootstrap catalog",
			write: func(t *testing.T, path string) {
				bootstrap := catalog.BootstrapCatalog()
				if err := catalog.WriteCatalogCache(path, &bootstrap); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "zero stale-after",
			write: func(t *testing.T, path string) {
				seed := catalog.SeedCatalog()
				seed.StaleAfter = time.Time{}
				data, err := json.Marshal(seed)
				if err != nil {
					t.Fatal(err)
				}
				writeHostControlTestFile(t, path, data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newHostControlTestEngine(t)
			tt.write(t, eng.catalogPath)

			health := eng.CatalogHealth(context.Background())
			if !health.Exists {
				t.Fatalf("on-disk cache reported as missing: %+v", health)
			}
			if health.Error == "" {
				t.Fatalf("invalid cache reported healthy: %+v", health)
			}
			if health.Models != 0 || health.Source != "" || health.Stale || !health.StaleAfter.IsZero() {
				t.Fatalf("invalid cache inherited compiled/bootstrap metadata: %+v", health)
			}
		})
	}
}

func TestCatalogHealthReportsValidCacheWithoutFalseStaleness(t *testing.T) {
	eng := newHostControlTestEngine(t)
	seed := catalog.SeedCatalog()
	seed.StaleAfter = time.Now().UTC().Add(time.Hour)
	if err := catalog.WriteCatalogCache(eng.catalogPath, &seed); err != nil {
		t.Fatal(err)
	}

	health := eng.CatalogHealth(context.Background())
	if health.Error != "" || !health.Exists || health.Models == 0 {
		t.Fatalf("valid cache reported unhealthy: %+v", health)
	}
	if health.Stale {
		t.Fatalf("future stale-after reported stale: %+v", health)
	}
	if health.Source != "test" {
		t.Fatalf("source = %q, want test", health.Source)
	}
}

func TestPreflightRequiresPersistedModelEvenWhenEffectiveSelectionCanChooseOne(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(eng.catalogPath, &seed); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-injected-secret"); err != nil {
		t.Fatal(err)
	}

	effective := eng.EffectiveSelection(ctx, SelectionOptions{})
	if strings.TrimSpace(effective.Model) == "" {
		t.Fatal("test setup did not produce an automatic effective model")
	}
	report := eng.Preflight(ctx)
	if report.Ready {
		t.Fatalf("preflight accepted an automatic model without persisted user selection: %+v", report)
	}
	assertPreflightCheck(t, report, "model", CheckFail, "no model selected")
}

func TestPreflightAcceptsPersistedModelSelection(t *testing.T) {
	ctx := context.Background()
	store := &credentials.MapStore{}
	eng, err := New(Options{SecretStore: store, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	seed := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(eng.catalogPath, &seed); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("OPENAI_API_KEY"), "sk-injected-secret"); err != nil {
		t.Fatal(err)
	}
	if err := eng.SetSelection(ctx, "openai", catalog.GPT_4o); err != nil {
		t.Fatal(err)
	}

	report := eng.Preflight(ctx)
	if !report.Ready {
		t.Fatalf("persisted, credentialed model was not ready: %+v", report)
	}
	assertPreflightCheck(t, report, "model", CheckOK, "openai/gpt-4o")
}

func newHostControlTestEngine(t *testing.T) *Engine {
	t.Helper()
	eng, err := New(Options{SecretStore: &credentials.MapStore{}, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

func writeHostControlTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPreflightCheck(t *testing.T, report PreflightReport, name string, status CheckStatus, detail string) {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			if check.Status != status || check.Detail != detail {
				t.Fatalf("%s check = %+v, want status=%q detail=%q", name, check, status, detail)
			}
			return
		}
	}
	t.Fatalf("missing %q check in %+v", name, report)
}
