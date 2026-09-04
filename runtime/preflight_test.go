package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

// failingStore is a mock credential store that always returns errors.
type failingStore struct{}

func (f *failingStore) Set(ctx context.Context, account, secret string) error {
	_ = ctx
	_ = account
	_ = secret
	return errors.New("mock failing store: Set not implemented")
}

func (f *failingStore) Get(ctx context.Context, account string) (string, error) {
	_ = ctx
	_ = account
	return "", errors.New("mock failing store: Get not implemented")
}

func (f *failingStore) Delete(ctx context.Context, account string) error {
	_ = ctx
	_ = account
	return errors.New("mock failing store: Delete not implemented")
}

// --- PreflightStatus constants ---

func TestPreflightStatusConstants(t *testing.T) {
	tests := []struct {
		name string
		got  PreflightStatus
		want string
	}{
		{"ok", PreflightOK, "ok"},
		{"warn", PreflightWarn, "warn"},
		{"fail", PreflightFail, "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, tt.got)
			}
		})
	}
}

// --- Preflight basic scenarios ---

func setupPreflightEnv(t *testing.T, providerJSON string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", filepath.Join(dir, "missing.json"))
	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte(providerJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
}

func TestPreflight_ReportsMissingModel(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	found := false
	for _, c := range r.Checks {
		if c.Name == "model" && c.Status == PreflightFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected model fail check: %+v", r.Checks)
	}
	if r.Ready {
		t.Fatal("expected not ready")
	}
}

func TestPreflight_NilContext(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	// Should not panic with nil context (Preflight uses context.Background internally)
	r := Preflight(context.Background())
	if len(r.Checks) == 0 {
		t.Fatal("expected at least one check")
	}
}

func TestPreflight_ReportsCredentialStatus(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	found := false
	for _, c := range r.Checks {
		if c.Name == "credentials_store" {
			found = true
			if c.Status != PreflightOK && c.Status != PreflightWarn {
				t.Fatalf("unexpected credentials_store status: %q", c.Status)
			}
		}
	}
	if !found {
		t.Fatal("expected credentials_store check in preflight report")
	}
}

func TestPreflight_ModelCheckFail_WhenNoModel(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	var modelCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "model" {
			modelCheck = &r.Checks[i]
		}
	}
	if modelCheck == nil {
		t.Fatal("expected 'model' check in report")
	}
	if modelCheck.Status != PreflightFail {
		t.Fatalf("expected model check status=fail, got %q", modelCheck.Status)
	}
	if !strings.Contains(modelCheck.Detail, "no model selected") {
		t.Fatalf("expected 'no model selected' in detail, got %q", modelCheck.Detail)
	}
}

func TestPreflight_CredentialsCheck_WhenNoneConfigured(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	var credsCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "credentials" {
			credsCheck = &r.Checks[i]
		}
	}
	if credsCheck == nil {
		t.Fatal("expected 'credentials' check in report")
	}
	if credsCheck.Status != PreflightFail {
		t.Fatalf("expected credentials check status=fail, got %q", credsCheck.Status)
	}
	if !strings.Contains(credsCheck.Detail, "no provider credentials") {
		t.Fatalf("expected 'no provider credentials' in detail, got %q", credsCheck.Detail)
	}
}

func TestPreflight_CatalogCheck_WhenMissing(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	var catalogCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "catalog" {
			catalogCheck = &r.Checks[i]
		}
	}
	if catalogCheck == nil {
		t.Fatal("expected 'catalog' check in report")
	}
	if catalogCheck.Status != PreflightWarn {
		t.Fatalf("expected catalog check status=warn, got %q", catalogCheck.Status)
	}
}

func TestPreflight_KeychainCheck(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	var keychainCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "keychain" {
			keychainCheck = &r.Checks[i]
		}
	}
	if keychainCheck == nil {
		t.Fatal("expected 'keychain' check in report")
	}
	// In test env, keychain check status is either ok or warn (never fail)
	if keychainCheck.Status == PreflightFail {
		t.Fatalf("expected keychain check status != fail, got %q", keychainCheck.Status)
	}
}

func TestPreflight_NotReady_WhenCredentialsFail(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	// Use failing store to ensure credentials check fails
	credentials.SetDefaultStore(&failingStore{})

	r := Preflight(context.Background())
	// If credentials check fails, Ready should be false.
	for _, c := range r.Checks {
		if c.Name == "credentials" && c.Status == PreflightFail {
			if r.Ready {
				t.Fatal("expected not ready when credentials fail")
			}
			return
		}
	}
	t.Fatal("expected credentials check to fail with failing store")
}

func TestPreflight_WithValidModel(t *testing.T) {
	setupPreflightEnv(t, `{"active_model":"claude-opus-4-6","active_provider":"anthropic"}`)

	r := Preflight(context.Background())
	var modelCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "model" {
			modelCheck = &r.Checks[i]
		}
	}
	if modelCheck == nil {
		t.Fatal("expected 'model' check in report")
	}
	if modelCheck.Status != PreflightOK {
		t.Fatalf("expected model check status=ok, got %q", modelCheck.Status)
	}
	if modelCheck.Detail != "claude-opus-4-6" {
		t.Fatalf("expected detail 'claude-opus-4-6', got %q", modelCheck.Detail)
	}
}

// --- PreflightReport / PreflightCheck struct ---

func TestPreflightReport_Fields(t *testing.T) {
	report := PreflightReport{
		Ready: true,
		Checks: []PreflightCheck{
			{Name: "test", Status: PreflightOK, Detail: "all good"},
		},
	}
	if !report.Ready {
		t.Fatal("expected Ready=true")
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}
	if report.Checks[0].Name != "test" {
		t.Fatalf("expected check name 'test', got %q", report.Checks[0].Name)
	}
}

func TestPreflightReport_JSONRoundTrip(t *testing.T) {
	report := PreflightReport{
		Ready: false,
		Checks: []PreflightCheck{
			{Name: "catalog", Status: PreflightWarn, Detail: "stale"},
			{Name: "credentials", Status: PreflightFail, Detail: "none configured"},
		},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded PreflightReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.Ready != report.Ready {
		t.Fatalf("Ready mismatch: got %v, want %v", decoded.Ready, report.Ready)
	}
	if len(decoded.Checks) != len(report.Checks) {
		t.Fatalf("Checks length mismatch: got %d, want %d", len(decoded.Checks), len(report.Checks))
	}
	for i, c := range decoded.Checks {
		if c.Name != report.Checks[i].Name {
			t.Fatalf("Check[%d].Name = %q, want %q", i, c.Name, report.Checks[i].Name)
		}
		if c.Status != report.Checks[i].Status {
			t.Fatalf("Check[%d].Status = %q, want %q", i, c.Status, report.Checks[i].Status)
		}
	}
}

// --- FormatPreflightReport ---

func TestFormatPreflightReport(t *testing.T) {
	tests := []struct {
		name     string
		report   PreflightReport
		contains []string
		excludes []string
	}{
		{
			name: "not ready",
			report: PreflightReport{
				Ready:  false,
				Checks: []PreflightCheck{{Name: "model", Status: PreflightFail, Detail: "none"}},
			},
			contains: []string{"setup incomplete", "model", "none"},
		},
		{
			name: "ready",
			report: PreflightReport{
				Ready: true,
				Checks: []PreflightCheck{
					{Name: "model", Status: PreflightOK, Detail: "claude-opus-4-6"},
					{Name: "credentials", Status: PreflightOK, Detail: "configured"},
				},
			},
			contains: []string{"ready to chat", "claude-opus-4-6", "configured"},
		},
		{
			name: "warn icon",
			report: PreflightReport{
				Ready:  true,
				Checks: []PreflightCheck{{Name: "catalog", Status: PreflightWarn, Detail: "stale"}},
			},
			contains: []string{"!", "catalog", "stale"},
		},
		{
			name: "fail icon",
			report: PreflightReport{
				Ready:  false,
				Checks: []PreflightCheck{{Name: "creds", Status: PreflightFail, Detail: "missing"}},
			},
			contains: []string{"creds", "missing"},
		},
		{
			name: "empty checks",
			report: PreflightReport{
				Ready:  true,
				Checks: nil,
			},
			contains: []string{"ready to chat"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := FormatPreflightReport(tt.report)
			for _, s := range tt.contains {
				if !strings.Contains(out, s) {
					t.Errorf("output missing %q:\n%s", s, out)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(out, s) {
					t.Errorf("output should not contain %q:\n%s", s, out)
				}
			}
		})
	}
}

// --- Preflight with catalog cache present ---

func TestPreflight_WithCatalogCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HAWK_CONFIG_DIR", dir)

	// Write a valid catalog cache
	cachePath := filepath.Join(dir, "model_catalog.json")
	c := catalog.SeedCatalog()
	if err := catalog.WriteCatalogCache(cachePath, &c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRAYCODE_ROUTER_MODEL_CATALOG_PATH", cachePath)

	if err := os.WriteFile(filepath.Join(dir, "provider.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	r := Preflight(context.Background())
	var catalogCheck *PreflightCheck
	for i, c := range r.Checks {
		if c.Name == "catalog" {
			catalogCheck = &r.Checks[i]
		}
	}
	if catalogCheck == nil {
		t.Fatal("expected 'catalog' check in report")
	}
	if catalogCheck.Status != PreflightOK {
		t.Fatalf("expected catalog check status=ok, got %q (detail: %s)", catalogCheck.Status, catalogCheck.Detail)
	}
	if !strings.Contains(catalogCheck.Detail, "models cached") {
		t.Fatalf("expected 'models cached' in detail, got %q", catalogCheck.Detail)
	}
}

// --- Preflight always checks known names ---

func TestPreflight_CheckNames(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	names := map[string]bool{}
	for _, c := range r.Checks {
		names[c.Name] = true
	}
	for _, want := range []string{"catalog", "credentials_store", "keychain", "credentials", "model"} {
		if !names[want] {
			t.Fatalf("expected check %q in report, checks: %+v", want, r.Checks)
		}
	}
}

// --- PreflightReport with multiple failures ---

func TestPreflight_MultipleFailures(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	// Use failing store to ensure credential failures
	credentials.SetDefaultStore(&failingStore{})

	r := Preflight(context.Background())
	failCount := 0
	for _, c := range r.Checks {
		if c.Status == PreflightFail {
			failCount++
		}
	}
	if failCount == 0 {
		t.Fatal("expected at least one failure with failing store")
	}
	if r.Ready {
		t.Fatal("expected not ready with failures")
	}
}

// --- PreflightCheck detail always non-empty ---

func TestPreflightCheck_DetailAlwaysSet(t *testing.T) {
	setupPreflightEnv(t, "{}\n")

	r := Preflight(context.Background())
	for _, c := range r.Checks {
		if c.Detail == "" {
			t.Fatalf("expected non-empty detail for check %q", c.Name)
		}
	}
}
