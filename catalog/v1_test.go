package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog/live"
)

func TestCatalogFromLegacyCompiles(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	compiled, err := CompileCatalog(&c)
	if err != nil {
		t.Fatalf("CompileCatalog failed: %v", err)
	}
	if compiled.ModelsByID["anthropic/claude-sonnet-4-6"].ID == "" {
		t.Fatal("expected canonical anthropic model")
	}
	offering, ok := compiled.OfferingForDeployment("anthropic/claude-sonnet-4-6", "anthropic-direct")
	if !ok {
		t.Fatal("expected anthropic direct offering")
	}
	if offering.NativeModelID != "claude-sonnet-4-6" {
		t.Fatalf("native model = %q, want claude-sonnet-4-6", offering.NativeModelID)
	}
	if canonical, ok := compiled.CanonicalModelForAliasOrID("claude-sonnet-4-6"); !ok || canonical != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("alias resolved to %q, %v", canonical, ok)
	}
}

func TestValidateCatalogRejectsBadReferences(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	c.Offerings = append(c.Offerings, ModelOffering{
		ID:               "missing:model",
		CanonicalModelID: "anthropic/claude-sonnet-4-6",
		DeploymentID:     "missing",
		NativeModelID:    "model",
		Pricing:          Pricing{Status: PricingUnknown},
	})
	if err := ValidateCatalog(&c); err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestLoadCatalogUsesValidCacheBeforeRemote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "catalog.json")
	c := SeedCatalog()
	c.SourceForTest("cache")
	if err := WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	compiled, err := LoadCatalog(context.Background(), LoadCatalogOptions{
		CachePath: cachePath,
		RemoteURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}
	if compiled.Catalog.Provenance == nil || compiled.Catalog.Provenance.Source != "cache" {
		t.Fatalf("expected cached catalog, got %#v", compiled.Catalog.Provenance)
	}
	if calls != 0 {
		t.Fatalf("remote called despite valid cache")
	}
}

func TestLoadCatalogRefreshRemoteOverridesValidCache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "catalog.json")
	cached := SeedCatalog()
	cached.SourceForTest("cache")
	if err := WriteCatalogCache(cachePath, &cached); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	remote := SeedCatalog()
	remote.SourceForTest("remote")
	data, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	compiled, err := LoadCatalog(context.Background(), LoadCatalogOptions{
		CachePath:     cachePath,
		RemoteURL:     srv.URL,
		RefreshRemote: true,
	})
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}
	if compiled.Catalog.Provenance == nil || compiled.Catalog.Provenance.Source != "remote" {
		t.Fatalf("expected remote catalog, got %#v", compiled.Catalog.Provenance)
	}
	if calls != 1 {
		t.Fatalf("remote calls = %d, want 1", calls)
	}
}

func TestLoadCatalogRejectsInvalidRemote(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "missing.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":"model-catalog/v1"}`))
	}))
	defer srv.Close()
	_, err := LoadCatalog(context.Background(), LoadCatalogOptions{
		CachePath:     cachePath,
		RemoteURL:     srv.URL,
		RefreshRemote: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid remote catalog")
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("invalid remote should not write cache, stat err=%v", err)
	}
}

func TestFetchRemoteCatalogStrictValidation(t *testing.T) {
	t.Parallel()
	c := SeedCatalog()
	c.GeneratedAt = time.Now().UTC()
	c.StaleAfter = c.GeneratedAt.Add(time.Hour)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	fetched, err := FetchRemoteCatalog(context.Background(), LoadCatalogOptions{RemoteURL: srv.URL})
	if err != nil {
		t.Fatalf("FetchRemoteCatalog failed: %v", err)
	}
	if fetched.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schema version = %q", fetched.SchemaVersion)
	}
}

func (c *Catalog) SourceForTest(source string) {
	if c.Provenance == nil {
		c.Provenance = &Provenance{}
	}
	c.Provenance.Source = source
}

func TestCapabilitySetFromLegacy_AnthropicFeatures(t *testing.T) {
	t.Parallel()
	entry := ModelCatalogEntry{
		ID:            "claude-sonnet-4-6",
		ContextWindow: 1000000,
		MaxOutput:     128000,
		ServerTools: []string{
			"thinking:enabled",
			"thinking:adaptive",
			"effort",
			"effort:low,medium,high,xhigh,max",
			"structured_output",
			"code_execution",
			"citations",
			"pdf_input",
			"image_input",
			"tools",
		},
	}
	set := capabilitySetFromLegacy(entry)

	if set.ExplicitThinkingBudget != CapabilitySupported {
		t.Errorf("ExplicitThinkingBudget = %q, want supported", set.ExplicitThinkingBudget)
	}
	if set.AdaptiveThinking != CapabilitySupported {
		t.Errorf("AdaptiveThinking = %q, want supported", set.AdaptiveThinking)
	}
	if set.Effort != CapabilitySupported {
		t.Errorf("Effort = %q, want supported", set.Effort)
	}
	if set.StructuredOutput != CapabilitySupported {
		t.Errorf("StructuredOutput = %q, want supported", set.StructuredOutput)
	}
	if set.CodeExecution != CapabilitySupported {
		t.Errorf("CodeExecution = %q, want supported", set.CodeExecution)
	}
	if set.Citations != CapabilitySupported {
		t.Errorf("Citations = %q, want supported", set.Citations)
	}
	if set.PDFInput != CapabilitySupported {
		t.Errorf("PDFInput = %q, want supported", set.PDFInput)
	}
	if set.ImageInput != CapabilitySupported {
		t.Errorf("ImageInput = %q, want supported", set.ImageInput)
	}
	if set.FunctionCalling != CapabilitySupported {
		t.Errorf("FunctionCalling = %q, want supported", set.FunctionCalling)
	}
	if set.MaxInputTokens != 1000000 {
		t.Errorf("MaxInputTokens = %d, want 1000000", set.MaxInputTokens)
	}
	if set.MaxOutputTokens != 128000 {
		t.Errorf("MaxOutputTokens = %d, want 128000", set.MaxOutputTokens)
	}
	if len(set.ThinkingTypes) != 2 {
		t.Errorf("ThinkingTypes len = %d, want 2", len(set.ThinkingTypes))
	}
	if len(set.EffortLevels) != 5 {
		t.Errorf("EffortLevels len = %d, want 5", len(set.EffortLevels))
	}
}

func TestCapabilitySetFromEntry_ToolsAliasSupportsFunctionCalling(t *testing.T) {
	set := CapabilitySetFromEntry(live.Entry{Features: []string{"tools", "reasoning"}})
	if set.FunctionCalling != CapabilitySupported {
		t.Fatalf("FunctionCalling = %q, want supported", set.FunctionCalling)
	}
}

func TestCapabilitySetFromLegacy_EmptyFeatures(t *testing.T) {
	t.Parallel()
	entry := ModelCatalogEntry{ID: "test-model"}
	set := capabilitySetFromLegacy(entry)
	if set.ServerTools != nil {
		t.Errorf("expected nil ServerTools, got %v", set.ServerTools)
	}
	if set.ExplicitThinkingBudget != "" {
		t.Errorf("expected empty ExplicitThinkingBudget, got %q", set.ExplicitThinkingBudget)
	}
}
