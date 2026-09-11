//nolint:errcheck
package client

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/credentials"
)

func TestGetOrCreateProvider_VertexUsesAnthropicVertexClient(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("VERTEX_PROJECT_ID"), "my-project"); err != nil {
		t.Fatalf("set VERTEX_PROJECT_ID: %v", err)
	}
	if err := store.Set(ctx, credentials.AccountForEnv("VERTEX_REGION"), "us-east1"); err != nil {
		t.Fatalf("set VERTEX_REGION: %v", err)
	}

	c := Client(&EyrieConfig{Provider: "vertex", APIKey: "test-bearer-token"})
	p, err := c.getOrCreateProvider("vertex")
	if err != nil {
		t.Fatalf("getOrCreateProvider: %v", err)
	}
	vc, ok := p.(*VertexClient)
	if !ok {
		t.Fatalf("provider type = %T, want *VertexClient (regression: registry was creating a GeminiClient for ProviderTypeVertex)", p)
	}
	if vc.ProjectID() != "my-project" {
		t.Errorf("projectID = %q, want %q", vc.ProjectID(), "my-project")
	}
	if vc.Region() != "us-east1" {
		t.Errorf("region = %q, want %q", vc.Region(), "us-east1")
	}
	if got := vc.BaseURL(); got != "https://us-east1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east1/publishers/anthropic/models" {
		t.Errorf("baseURL() = %q, want Anthropic-on-Vertex URL", got)
	}
}

func TestGetOrCreateProvider_VertexRegionDefaultsToUsCentral1(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	ctx := context.Background()
	if err := store.Set(ctx, credentials.AccountForEnv("VERTEX_PROJECT_ID"), "my-project"); err != nil {
		t.Fatalf("set VERTEX_PROJECT_ID: %v", err)
	}

	c := Client(&EyrieConfig{Provider: "vertex", APIKey: "test-token"})
	p, err := c.getOrCreateProvider("vertex")
	if err != nil {
		t.Fatalf("getOrCreateProvider: %v", err)
	}
	vc, ok := p.(*VertexClient)
	if !ok {
		t.Fatalf("provider type = %T, want *VertexClient", p)
	}
	if vc.Region() != "us-central1" {
		t.Errorf("region = %q, want default %q", vc.Region(), "us-central1")
	}
}

func TestGetOrCreateProvider_VertexRequiresProjectID(t *testing.T) {
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	c := Client(&EyrieConfig{Provider: "vertex", APIKey: "test-token"})
	_, err := c.getOrCreateProvider("vertex")
	if err == nil {
		t.Fatal("expected error when VERTEX_PROJECT_ID is missing, got nil")
	}
	if got := err.Error(); got != "eyrie: vertex requires VERTEX_PROJECT_ID" {
		t.Errorf("error = %q, want %q", got, "eyrie: vertex requires VERTEX_PROJECT_ID")
	}
}

// TestDynamicProvider_DefaultDeny: when EYRIE_ALLOW_DYNAMIC_PROVIDERS is
// unset (the default), an unknown provider name is NOT auto-registered
// from OPENAI_API_BASE. The caller receives ErrUnknownProvider. This is
// the safe-by-default behavior that prevents a poisoned OPENAI_API_BASE
// from silently exfiltrating the user's OPENAI_API_KEY.
func TestDynamicProvider_DefaultDeny(t *testing.T) {
	_ = os.Unsetenv(dynamicProviderEnvVar)
	t.Setenv("OPENAI_API_BASE", "http://attacker.example/v1")

	c := Client(&EyrieConfig{Provider: "openai", APIKey: "test-key"})
	_, err := c.getOrCreateProvider("ghost-default-deny")
	if err == nil {
		t.Fatal("expected ErrUnknownProvider when opt-in is not set, got nil")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("err = %q, want 'unknown provider'", err.Error())
	}
	if c.GetProviderInfo("ghost-default-deny") != nil {
		t.Error("ghost-default-deny must not be registered when opt-in is not set")
	}
}

// TestDynamicProvider_OptIn_Registers: with EYRIE_ALLOW_DYNAMIC_PROVIDERS=1
// and OPENAI_API_BASE set, the unknown provider IS auto-registered as an
// OpenAI-compatible client pointed at the base URL. The subsequent lookup
// sees the registered provider.
func TestDynamicProvider_OptIn_Registers(t *testing.T) {
	t.Setenv(dynamicProviderEnvVar, "1")
	t.Setenv("OPENAI_API_BASE", "http://localhost:9999/v1")

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "test-key"); err != nil {
		t.Fatalf("set OPENAI_API_KEY: %v", err)
	}

	c := Client(&EyrieConfig{Provider: "openai", APIKey: "test-key"})
	p, err := c.getOrCreateProvider("ghost-optin-registers")
	if err != nil {
		t.Fatalf("getOrCreateProvider: %v", err)
	}
	oc, ok := p.(*OpenAIClient)
	if !ok {
		t.Fatalf("provider type = %T, want *OpenAIClient", p)
	}
	if oc.BaseURL() != "http://localhost:9999/v1" {
		t.Errorf("baseURL = %q, want %q", oc.BaseURL(), "http://localhost:9999/v1")
	}
}

// TestDynamicProvider_OptInRequiresBaseURL: the opt-in flag alone is not
// enough — OPENAI_API_BASE (or OPENAI_BASE_URL) must also be set. If the
// opt-in is on but the base URL is empty, the unknown-provider error is
// returned (this is the same path as default-deny).
func TestDynamicProvider_OptInRequiresBaseURL(t *testing.T) {
	t.Setenv(dynamicProviderEnvVar, "1")
	_ = os.Unsetenv("OPENAI_API_BASE")
	_ = os.Unsetenv("OPENAI_BASE_URL")

	c := Client(&EyrieConfig{Provider: "openai", APIKey: "test-key"})
	_, err := c.getOrCreateProvider("ghost-optin-no-base")
	if err == nil {
		t.Fatal("expected ErrUnknownProvider when base URL is missing, got nil")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("err = %q, want 'unknown provider'", err.Error())
	}
}

// TestDynamicProvider_LogsWarning: when auto-registration fires, a WARN
// line is emitted to the default logger. The test captures the slog
// default and asserts the message.
func TestDynamicProvider_LogsWarning(t *testing.T) {
	t.Setenv(dynamicProviderEnvVar, "1")
	t.Setenv("OPENAI_API_BASE", "http://localhost:9999/v1")

	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(orig) })

	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("OPENAI_API_KEY"), "test-key"); err != nil {
		t.Fatalf("set OPENAI_API_KEY: %v", err)
	}

	c := Client(&EyrieConfig{Provider: "openai", APIKey: "test-key"})
	_, _ = c.getOrCreateProvider("ghost-logs-warning")

	output := buf.String()
	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN level, got: %q", output)
	}
	if !strings.Contains(output, "auto-registering OpenAI-compatible provider") {
		t.Errorf("expected message 'auto-registering OpenAI-compatible provider', got: %q", output)
	}
	if !strings.Contains(output, "ghost-logs-warning") {
		t.Errorf("expected log to include provider name, got: %q", output)
	}
	if !strings.Contains(output, dynamicProviderEnvVar) {
		t.Errorf("expected log to include opt-in env var, got: %q", output)
	}
}

// TestDynamicProvider_OptInValues: the opt-in env var accepts "1", "true",
// and "yes" (case-insensitive, whitespace-trimmed). Other values are
// treated as deny.
func TestDynamicProvider_OptInValues(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"yes", true},
		{"YES", true},
		{" 1 ", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"", false},
		{"enable", false},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(dynamicProviderEnvVar, tc.value)
			if got := dynamicProviderEnabled(); got != tc.want {
				t.Errorf("dynamicProviderEnabled(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
