package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/config"
)

// --- SetActiveModel ---

func TestSetActiveModel(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		wantErr bool
	}{
		{"empty id", "", true},
		{"whitespace only", "   ", true},
		{"tab only", "\t", true},
		{"valid model", "claude-opus-4-6", false},
		{"valid gpt model", "gpt-4o", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HAWK_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := SetActiveModel(context.Background(), tt.modelID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetActiveModel(%q) error = %v, wantErr %v", tt.modelID, err, tt.wantErr)
			}
		})
	}
}

func TestSetActiveModel_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetActiveModel(context.Background(), "claude-opus-4-6"); err != nil {
		t.Fatalf("SetActiveModel: %v", err)
	}

	got := ActiveModel(context.Background())
	if got != "claude-opus-4-6" {
		t.Fatalf("expected 'claude-opus-4-6', got %q", got)
	}
}

func TestSetActiveModel_OverwritesPrevious(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetActiveModel(context.Background(), "gpt-4o"); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveModel(context.Background(), "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}

	got := ActiveModel(context.Background())
	if got != "claude-sonnet-4-6" {
		t.Fatalf("expected 'claude-sonnet-4-6', got %q", got)
	}
}

// --- SetActiveProvider ---

func TestSetActiveProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"valid anthropic", "anthropic", false},
		{"valid openai", "openai", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HAWK_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := SetActiveProvider(context.Background(), tt.provider)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetActiveProvider(%q) error = %v, wantErr %v", tt.provider, err, tt.wantErr)
			}
		})
	}
}

func TestSetActiveProvider_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetActiveProvider(context.Background(), "anthropic"); err != nil {
		t.Fatalf("SetActiveProvider: %v", err)
	}

	got := ActiveProvider(context.Background())
	if got != "anthropic" {
		t.Fatalf("expected 'anthropic', got %q", got)
	}
}

// --- ActiveModel / ActiveProvider ---

func TestActiveModel_NoConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	got := ActiveModel(context.Background())
	if got != "" {
		t.Fatalf("expected empty active model with no config, got %q", got)
	}
}

func TestActiveProvider_NoConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	got := ActiveProvider(context.Background())
	if got != "" {
		t.Fatalf("expected empty active provider with no config, got %q", got)
	}
}

func TestActiveModel_AfterSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte(`{"active_model":"gpt-4o","active_provider":"openai"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := ActiveModel(context.Background())
	if got != "gpt-4o" {
		t.Fatalf("expected 'gpt-4o', got %q", got)
	}
}

func TestActiveProvider_AfterSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte(`{"active_provider":"openai"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := ActiveProvider(context.Background())
	if got != "openai" {
		t.Fatalf("expected 'openai', got %q", got)
	}
}

// --- ClearActiveSelection ---

func TestClearActiveSelection_ClearsValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set values first
	if err := SetActiveProvider(context.Background(), "anthropic"); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveModel(context.Background(), "claude-opus-4-6"); err != nil {
		t.Fatal(err)
	}

	// Clear
	if err := ClearActiveSelection(context.Background()); err != nil {
		t.Fatalf("ClearActiveSelection: %v", err)
	}

	// Verify cleared
	cfg := config.LoadProviderConfig("")
	if cfg != nil && cfg.ActiveProvider != "" {
		t.Fatalf("expected empty active provider after clear, got %q", cfg.ActiveProvider)
	}
	if cfg != nil && cfg.ActiveModel != "" {
		t.Fatalf("expected empty active model after clear, got %q", cfg.ActiveModel)
	}
}

func TestClearActiveSelection_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	err := ClearActiveSelection(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when no config file, got %v", err)
	}
}

func TestClearActiveSelection_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Clear once
	if err := ClearActiveSelection(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Clear again — should not error
	if err := ClearActiveSelection(context.Background()); err != nil {
		t.Fatalf("expected nil on second clear, got %v", err)
	}
}

// --- inferProviderForModel (internal helper) ---

func TestInferProviderForModel_WithPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	// With no catalog loaded, inferProviderForModel falls back to prefix parsing.
	got := inferProviderForModel(context.Background(), "anthropic/claude-opus-4-6")
	if got != "anthropic" {
		t.Fatalf("expected 'anthropic', got %q", got)
	}
}

func TestInferProviderForModel_OpenAIPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	got := inferProviderForModel(context.Background(), "openai/gpt-4o")
	if got != "openai" {
		t.Fatalf("expected 'openai', got %q", got)
	}
}

func TestInferProviderForModel_NoPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	// Without a known prefix, should return empty when catalog is unavailable.
	got := inferProviderForModel(context.Background(), "gpt-4o")
	if got != "" {
		t.Fatalf("expected empty for model without prefix and no catalog, got %q", got)
	}
}

func TestInferProviderForModel_EmptyModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))

	got := inferProviderForModel(context.Background(), "")
	if got != "" {
		t.Fatalf("expected empty for empty model, got %q", got)
	}
}

// --- SetActiveModel preserves provider ---

func TestSetActiveModel_PreservesProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte(`{"active_provider":"openai"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SetActiveModel(context.Background(), "gpt-4o"); err != nil {
		t.Fatal(err)
	}

	cfg := config.LoadProviderConfig("")
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.ActiveProvider != "openai" {
		t.Fatalf("expected provider preserved as 'openai', got %q", cfg.ActiveProvider)
	}
	if cfg.ActiveModel != "gpt-4o" {
		t.Fatalf("expected model 'gpt-4o', got %q", cfg.ActiveModel)
	}
}
