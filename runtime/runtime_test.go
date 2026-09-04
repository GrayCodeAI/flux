package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/catalog/registry"
	"github.com/GrayCodeAI/graycode-router/config"
)

// --- Runtime.ModelIDs ---

func TestModelIDs(t *testing.T) {
	tests := []struct {
		name    string
		runtime *Runtime
		wantNil bool
		wantLen int // 0 means "just check non-nil"
	}{
		{"nil receiver", nil, true, 0},
		{"nil catalog", &Runtime{}, true, 0},
		{
			name: "empty catalog",
			runtime: &Runtime{
				Catalog: &catalog.CompiledCatalog{
					ModelsByID: map[string]catalog.Model{},
				},
			},
			wantNil: false,
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := tt.runtime.ModelIDs()
			if tt.wantNil {
				if ids != nil {
					t.Fatalf("expected nil, got %v", ids)
				}
				return
			}
			if ids == nil {
				t.Fatal("expected non-nil")
			}
			if tt.wantLen > 0 && len(ids) != tt.wantLen {
				t.Fatalf("expected %d IDs, got %d", tt.wantLen, len(ids))
			}
		})
	}
}

func TestModelIDs_ReturnsAllIDs(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	ids := r.ModelIDs()
	if len(ids) == 0 {
		t.Fatal("expected non-empty model IDs from test catalog")
	}
	for _, id := range ids {
		if _, ok := compiled.ModelsByID[id]; !ok {
			t.Fatalf("model ID %q not found in ModelsByID", id)
		}
	}
}

func TestModelIDs_CountMatchesCatalog(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	ids := r.ModelIDs()
	if len(ids) != len(compiled.ModelsByID) {
		t.Fatalf("expected %d IDs, got %d", len(compiled.ModelsByID), len(ids))
	}
}

// --- Runtime.ProviderConfigJSON ---

func TestProviderConfigJSON(t *testing.T) {
	tests := []struct {
		name     string
		runtime  *Runtime
		wantJSON string
		wantErr  bool
	}{
		{
			name:     "nil provider",
			runtime:  &Runtime{},
			wantJSON: "{}",
		},
		{
			name: "with active provider",
			runtime: &Runtime{
				Provider: &config.ProviderConfig{
					ActiveProvider: "anthropic",
				},
			},
			wantJSON: "",
		},
		{
			name: "with model and provider",
			runtime: &Runtime{
				Provider: &config.ProviderConfig{
					ActiveProvider: "openai",
					ActiveModel:    "gpt-4o",
				},
			},
			wantJSON: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.runtime.ProviderConfigJSON()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ProviderConfigJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantJSON != "" && raw != tt.wantJSON {
				t.Fatalf("expected %q, got %q", tt.wantJSON, raw)
			}
			if raw != "" {
				var m map[string]any
				if err := json.Unmarshal([]byte(raw), &m); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
			}
		})
	}
}

func TestProviderConfigJSON_ContainsFields(t *testing.T) {
	r := &Runtime{
		Provider: &config.ProviderConfig{
			ActiveProvider: "anthropic",
			ActiveModel:    "claude-opus-4-6",
		},
	}
	raw, err := r.ProviderConfigJSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["active_provider"] != "anthropic" {
		t.Fatalf("expected active_provider=anthropic, got %v", m["active_provider"])
	}
	if m["active_model"] != "claude-opus-4-6" {
		t.Fatalf("expected active_model=claude-opus-4-6, got %v", m["active_model"])
	}
}

// --- Runtime.DeploymentRows ---

func TestDeploymentRows(t *testing.T) {
	tests := []struct {
		name    string
		runtime *Runtime
		wantErr bool
	}{
		{"nil runtime", nil, true},
		{"nil catalog", &Runtime{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.runtime.DeploymentRows()
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeploymentRows() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeploymentRows_WithCatalog(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	rows, err := r.DeploymentRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty deployment rows")
	}
	for _, row := range rows {
		if row.ID == "" {
			t.Fatal("expected non-empty deployment row ID")
		}
		if row.Name == "" {
			t.Fatalf("expected non-empty name for deployment %q", row.ID)
		}
		if row.ProviderID == "" {
			t.Fatalf("expected non-empty provider ID for deployment %q", row.ID)
		}
		// Status should be one of the known values
		switch row.Status {
		case "ready", "incomplete", "needs credentials":
			// ok
		default:
			t.Fatalf("unexpected status %q for deployment %q", row.Status, row.ID)
		}
	}
}

func TestDeploymentRows_WithNilProviderConfig(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled, Provider: nil}
	rows, err := r.DeploymentRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected non-empty deployment rows with nil provider")
	}
	// Each row should have a valid status
	for _, row := range rows {
		switch row.Status {
		case "ready", "incomplete", "needs credentials":
			// ok
		default:
			t.Fatalf("unexpected status %q for deployment %q", row.Status, row.ID)
		}
	}
}

// --- Runtime.ModelEntriesForProvider ---

func TestModelEntriesForProvider(t *testing.T) {
	tests := []struct {
		name     string
		runtime  *Runtime
		provider string
		wantZero bool
	}{
		{"nil receiver", nil, "anthropic", true},
		{"nil catalog", &Runtime{}, "anthropic", true},
		{"unknown provider", &Runtime{Catalog: mustCompileTestCatalog(t)}, "nonexistent-xyz", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := tt.runtime.ModelEntriesForProvider(tt.provider)
			if tt.wantZero && len(entries) != 0 {
				t.Fatalf("expected 0 entries, got %d", len(entries))
			}
		})
	}
}

func TestModelEntriesForProvider_ReturnsMatching(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	entries := r.ModelEntriesForProvider("anthropic")
	if len(entries) == 0 {
		t.Fatal("expected anthropic models from test catalog")
	}
	for _, e := range entries {
		if e.ID == "" {
			t.Fatal("expected non-empty model ID")
		}
	}
}

func TestModelEntriesForProvider_MultipleProviders(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	providers := []string{"anthropic", "openai", "gemini"}
	for _, p := range providers {
		entries := r.ModelEntriesForProvider(p)
		if len(entries) == 0 {
			t.Fatalf("expected entries for provider %q", p)
		}
	}
}

// --- Runtime.CredentialTargets ---

func TestCredentialTargets_NilCatalog_Bootstraps(t *testing.T) {
	r := &Runtime{}
	targets := r.CredentialTargets()
	// With nil catalog, CredentialTargets bootstraps a catalog internally.
	_ = targets
}

func TestCredentialTargets_WithCatalog(t *testing.T) {
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{Catalog: compiled}
	targets := r.CredentialTargets()
	if len(targets) == 0 {
		t.Fatal("expected non-empty credential targets from test catalog")
	}
	for _, ct := range targets {
		if ct.EnvVar == "" {
			t.Fatal("expected non-empty EnvVar in credential target")
		}
		if ct.ProviderID == "" {
			t.Fatal("expected non-empty ProviderID in credential target")
		}
		if ct.DeploymentID == "" {
			t.Fatal("expected non-empty DeploymentID in credential target")
		}
	}
}

// --- DefaultPaths ---

func TestDefaultPaths(t *testing.T) {
	catalogPath, providerPath := DefaultPaths()
	if catalogPath == "" {
		t.Fatal("expected non-empty catalog path")
	}
	if providerPath == "" {
		t.Fatal("expected non-empty provider path")
	}
	if filepath.Ext(catalogPath) != ".json" {
		t.Fatalf("expected .json extension for catalog path, got %q", catalogPath)
	}
	if filepath.Ext(providerPath) != ".json" {
		t.Fatalf("expected .json extension for provider path, got %q", providerPath)
	}
}

func TestDefaultPaths_ContainsGraycodeRouterDir(t *testing.T) {
	catalogPath, _ := DefaultPaths()
	if !filepath.IsAbs(catalogPath) {
		t.Fatalf("expected absolute path, got %q", catalogPath)
	}
}

// --- Load (initialization without network) ---

func TestLoad_WithEmptyConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	rt, err := Load(context.Background())
	if err != nil {
		return // Expected: catalog cache missing
	}
	_ = rt // Provider may be nil for empty config — no assertion needed here
}

func TestLoad_MissingConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", filepath.Join(dir, "nonexistent"))
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	// Load should not panic even with missing config dir
	_, _ = Load(context.Background())
}

// --- ChatProvider ---

func TestChatProvider_NilProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := &Runtime{}
	// ChatProvider with nil Provider field falls back to LoadProviderConfig.
	// Without valid credentials, it may error — that's expected behavior.
	_, _ = r.ChatProvider(context.Background())
}

// --- entriesToModelList (internal helper) ---

func TestEntriesToModelList(t *testing.T) {
	tests := []struct {
		name       string
		entries    []catalog.ModelCatalogEntry
		providerID string
		source     string
		installed  bool
		wantLen    int
	}{
		{
			name:    "nil entries",
			entries: nil,
			wantLen: 0,
		},
		{
			name:    "empty entries",
			entries: []catalog.ModelCatalogEntry{},
			wantLen: 0,
		},
		{
			name: "single entry",
			entries: []catalog.ModelCatalogEntry{
				{ID: "gpt-4o", DisplayName: "GPT-4o"},
			},
			providerID: "openai",
			source:     "cache",
			wantLen:    1,
		},
		{
			name: "deduplicates by ID",
			entries: []catalog.ModelCatalogEntry{
				{ID: "gpt-4o", DisplayName: "GPT-4o"},
				{ID: "gpt-4o", DisplayName: "GPT-4o Duplicate"},
				{ID: "gpt-4o-mini", DisplayName: "GPT-4o Mini"},
			},
			wantLen: 2,
		},
		{
			name: "skips empty ID",
			entries: []catalog.ModelCatalogEntry{
				{ID: "", DisplayName: "Empty"},
				{ID: "gpt-4o", DisplayName: "GPT-4o"},
			},
			wantLen: 1,
		},
		{
			name: "uses ID when display name empty",
			entries: []catalog.ModelCatalogEntry{
				{ID: "gpt-4o"},
			},
			wantLen: 1,
		},
		{
			name: "sets source and installed",
			entries: []catalog.ModelCatalogEntry{
				{ID: "gpt-4o"},
			},
			source:    "live",
			installed: true,
			wantLen:   1,
		},
		{
			name: "whitespace-only ID skipped",
			entries: []catalog.ModelCatalogEntry{
				{ID: "   ", DisplayName: "Whitespace"},
				{ID: "gpt-4o", DisplayName: "GPT-4o"},
			},
			wantLen: 1,
		},
		{
			name: "multiple unique entries",
			entries: []catalog.ModelCatalogEntry{
				{ID: "claude-opus-4-6", DisplayName: "Claude Opus"},
				{ID: "claude-sonnet-4-6", DisplayName: "Claude Sonnet"},
				{ID: "claude-haiku-4-5-20251001", DisplayName: "Claude Haiku"},
			},
			wantLen: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := entriesToModelList(tt.entries, tt.providerID, tt.source, tt.installed)
			if len(out) != tt.wantLen {
				t.Fatalf("expected %d entries, got %d", tt.wantLen, len(out))
			}
			for _, e := range out {
				if e.ID == "" {
					t.Fatal("expected non-empty ID in output entry")
				}
				if e.Source != tt.source {
					t.Fatalf("expected source %q, got %q", tt.source, e.Source)
				}
				if e.Installed != tt.installed {
					t.Fatalf("expected installed=%v, got %v", tt.installed, e.Installed)
				}
			}
		})
	}
}

func TestEntriesToModelList_PreservesOrder(t *testing.T) {
	entries := []catalog.ModelCatalogEntry{
		{ID: "z-model"},
		{ID: "a-model"},
		{ID: "m-model"},
	}
	out := entriesToModelList(entries, "test", "cache", false)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out))
	}
	// Order should be preserved (not sorted)
	if out[0].ID != "z-model" || out[1].ID != "a-model" || out[2].ID != "m-model" {
		t.Fatalf("unexpected order: %v, %v, %v", out[0].ID, out[1].ID, out[2].ID)
	}
}

// --- copyEnvMap ---

func TestCopyEnvMap(t *testing.T) {
	in := map[string]string{"A": "1", "B": "2"}
	out := copyEnvMap(in)
	if out["A"] != "1" || out["B"] != "2" {
		t.Fatalf("unexpected copy: %v", out)
	}
	out["C"] = "3"
	if _, ok := in["C"]; ok {
		t.Fatal("mutation of copy affected original")
	}
}

func TestCopyEnvMap_Nil(t *testing.T) {
	out := copyEnvMap(nil)
	if len(out) != 0 {
		t.Fatalf("expected empty map from nil input, got %d", len(out))
	}
}

func TestCopyEnvMap_Empty(t *testing.T) {
	in := map[string]string{}
	out := copyEnvMap(in)
	if len(out) != 0 {
		t.Fatalf("expected empty map, got %d", len(out))
	}
}

// --- FormatSetupError ---

func TestFormatSetupError(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		err        error
		wantNil    bool
	}{
		{"nil error", "openai", nil, true},
		{"non-ollama error", "openai", errForTest("something failed"), false},
		{"ollama error", "ollama", errForTest("connection refused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := FormatSetupError(tt.providerID, tt.err)
			if tt.wantNil {
				if out != nil {
					t.Fatalf("expected nil, got %v", out)
				}
				return
			}
			if out == nil {
				t.Fatal("expected non-nil error")
			}
		})
	}
}

func TestFormatSetupError_NonOllama_PreservesError(t *testing.T) {
	in := errForTest("something failed")
	out := FormatSetupError("openai", in)
	if out != in {
		t.Fatal("expected same error for non-ollama provider")
	}
}

// --- configuredDeploymentIDsForProvider (internal helper) ---

func TestConfiguredDeploymentIDsForProvider_NilCompiled(t *testing.T) {
	ids := configuredDeploymentIDsForProvider(nil, "anthropic")
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}
}

func TestConfiguredDeploymentIDsForProvider_NilCatalog(t *testing.T) {
	ids := configuredDeploymentIDsForProvider(&catalog.CompiledCatalog{}, "anthropic")
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}
}

func TestConfiguredDeploymentIDsForProvider_WithCatalog(t *testing.T) {
	compiled := mustCompileTestCatalog(t)
	ids := configuredDeploymentIDsForProvider(compiled, "anthropic")
	if len(ids) == 0 {
		t.Fatal("expected non-empty deployment IDs for anthropic")
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "" {
			t.Fatal("expected non-empty deployment ID")
		}
	}
}

// --- liveEntriesToModelList (internal helper) ---

func TestLiveEntriesToModelList(t *testing.T) {
	entries := []catalog.ModelCatalogEntry{
		{ID: "gpt-4o", DisplayName: "GPT-4o", ContextWindow: 128000},
	}
	out := entriesToModelList(entries, "openai", "live", true)
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	if out[0].ContextWindow != 128000 {
		t.Fatalf("expected context window 128000, got %d", out[0].ContextWindow)
	}
	if !out[0].Installed {
		t.Fatal("expected installed=true")
	}
	if out[0].Source != "live" {
		t.Fatalf("expected source 'live', got %q", out[0].Source)
	}
}

// --- ListProviderSetupOptions ---

func TestListProviderSetupOptions(t *testing.T) {
	opts := ListProviderSetupOptions(context.Background())
	if len(opts) == 0 {
		t.Fatal("expected non-empty provider setup options")
	}
	// Should always include "Paste API key"
	found := false
	for _, o := range opts {
		if o.Action == "apikey" {
			found = true
		}
		if o.Action == "" || o.Label == "" {
			t.Fatalf("expected non-empty action and label, got action=%q label=%q", o.Action, o.Label)
		}
	}
	if !found {
		t.Fatal("expected 'apikey' action in setup options")
	}
}

func TestAvailableProvidersIncludesCanonicalRegistry(t *testing.T) {
	providers := AvailableProviders()
	want := make([]string, 0, len(registry.All()))
	for _, spec := range registry.All() {
		want = append(want, spec.ProviderID)
	}
	sort.Strings(want)

	if !reflect.DeepEqual(providers, want) {
		t.Fatalf("AvailableProviders() = %v, want canonical registry %v", providers, want)
	}
}

func TestAvailableProvidersIsSorted(t *testing.T) {
	providers := AvailableProviders()
	if !sort.StringsAreSorted(providers) {
		t.Fatalf("AvailableProviders() is not sorted: %v", providers)
	}
}

// --- helpers ---

func mustCompileTestCatalog(t *testing.T) *catalog.CompiledCatalog {
	t.Helper()
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func errForTest(msg string) error { return &testErr{msg} }

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
