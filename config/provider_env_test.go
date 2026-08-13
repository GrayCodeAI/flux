//nolint:errcheck
package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func testModelCatalog() catalog.ModelCatalog {
	return catalog.ModelCatalog{Providers: map[string][]catalog.ModelCatalogEntry{}}
}

func TestLoadProviderConfig_ValidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")

	cfg := ProviderConfig{
		Version:         "1",
		ActiveProvider:  "anthropic",
		AnthropicAPIKey: "sk-ant-test-key-1234567890",
		AnthropicModel:  "claude-sonnet-4-6",
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(path, data, 0o644)

	loaded := LoadProviderConfig(path)
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
	if loaded.ActiveProvider != "anthropic" {
		t.Errorf("expected active_provider 'anthropic', got %q", loaded.ActiveProvider)
	}
	if loaded.AnthropicAPIKey != "sk-ant-test-key-1234567890" {
		t.Errorf("expected API key 'sk-ant-test-key-1234567890', got %q", loaded.AnthropicAPIKey)
	}
	if loaded.AnthropicModel != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", loaded.AnthropicModel)
	}
}

func TestLoadProviderConfig_MissingFile(t *testing.T) {
	t.Parallel()
	loaded := LoadProviderConfig("/nonexistent/path/provider.json")
	if loaded != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoadProviderConfig_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")

	os.WriteFile(path, []byte("not valid json {{{"), 0o644)

	loaded := LoadProviderConfig(path)
	if loaded != nil {
		t.Error("expected nil config for invalid JSON")
	}
}

func TestLoadProviderConfigWithError_MissingFile(t *testing.T) {
	t.Parallel()
	cfg, err := LoadProviderConfigWithError("/nonexistent/path/provider.json")
	if cfg != nil {
		t.Error("expected nil config")
	}
	if err != nil {
		t.Error("expected nil error for missing file")
	}
}

func TestLoadProviderConfigWithError_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	os.WriteFile(path, []byte("{bad json"), 0o644)

	cfg, err := LoadProviderConfigWithError(path)
	if cfg != nil {
		t.Error("expected nil config")
	}
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("expected 'corrupt' in error, got %q", err.Error())
	}
}

func TestLoadProviderConfigWithErrorRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := LoadProviderConfigWithError(path); err == nil || cfg != nil {
		t.Fatalf("trailing provider JSON accepted: cfg=%#v err=%v", cfg, err)
	}
}

func TestLoadProviderConfigWithError_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "provider.json")
	cfg := ProviderConfig{Version: "99"}
	data, _ := json.Marshal(cfg)
	os.WriteFile(path, data, 0o644)

	loaded, err := LoadProviderConfigWithError(path)
	if loaded != nil {
		t.Error("expected nil config for unsupported version")
	}
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got %q", err.Error())
	}
}

func TestLoadProviderConfigWithErrorAcceptsLegacyVersionAlias(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(`{"version":"1","active_provider":"openai"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProviderConfigWithError(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Version != "1" || cfg.ActiveProvider != "openai" {
		t.Fatalf("legacy version was not normalized: %#v", cfg)
	}

	canonicalPath := filepath.Join(t.TempDir(), "provider.json")
	if err := SaveProviderConfig(cfg, canonicalPath); err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(canonical, []byte(`"version"`)) || !bytes.Contains(canonical, []byte(`"_version"`)) {
		t.Fatalf("provider config did not use canonical version field: %s", canonical)
	}
}

func TestLoadProviderConfigWithErrorRejectsConflictingVersionAliases(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := os.WriteFile(path, []byte(`{"_version":"1","version":"2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := LoadProviderConfigWithError(path); err == nil || cfg != nil {
		t.Fatalf("conflicting provider versions accepted: cfg=%#v err=%v", cfg, err)
	}
}

func TestGetProviderConfigDir(t *testing.T) {
	// Test with env var set
	dir := t.TempDir()
	os.Setenv("HAWK_CONFIG_DIR", dir)
	defer os.Unsetenv("HAWK_CONFIG_DIR")

	got, err := GetProviderConfigDir()
	if err != nil {
		t.Fatalf("GetProviderConfigDir() error = %v", err)
	}
	if got != dir {
		t.Errorf("expected %q, got %q", dir, got)
	}

	// Test without env var (uses OS config dir when available)
	os.Unsetenv("HAWK_CONFIG_DIR")
	got, _ = GetProviderConfigDir()
	if !strings.HasSuffix(got, filepath.Join("eyrie")) {
		t.Errorf("expected path ending in eyrie, got %q", got)
	}
}

func TestGetProviderConfigDirPrefersEyrieNamespace(t *testing.T) {
	t.Setenv("EYRIE_CONFIG_DIR", "/tmp/eyrie-config")
	t.Setenv("HAWK_CONFIG_DIR", "/tmp/legacy-hawk-config")
	got, err := GetProviderConfigDir()
	if err != nil {
		t.Fatalf("GetProviderConfigDir() error = %v", err)
	}
	if got != "/tmp/eyrie-config" {
		t.Fatalf("GetProviderConfigDir() = %q, want EYRIE_CONFIG_DIR", got)
	}
}

func TestGetProviderConfigPath(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HAWK_CONFIG_DIR", dir)
	defer os.Unsetenv("HAWK_CONFIG_DIR")

	got, err := GetProviderConfigPath()
	if err != nil {
		t.Fatalf("GetProviderConfigPath() error = %v", err)
	}
	expected := filepath.Join(dir, "provider.json")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestApplyProviderEnv_Anthropic(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		AnthropicAPIKey: "sk-ant-test-1234567890",
		AnthropicModel:  "claude-sonnet-4-6",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderAnthropic, cfg, "claude-opus-4-6", true, &cat)

	if env["ANTHROPIC_API_KEY"] != "sk-ant-test-1234567890" {
		t.Errorf("expected ANTHROPIC_API_KEY 'sk-ant-test-1234567890', got %q", env["ANTHROPIC_API_KEY"])
	}
	if env["ANTHROPIC_MODEL"] != "claude-opus-4-6" {
		t.Errorf("expected ANTHROPIC_MODEL 'claude-opus-4-6', got %q", env["ANTHROPIC_MODEL"])
	}
}

func TestApplyProviderEnv_OpenAI(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		OpenAIAPIKey: "sk-openai-test-1234567890",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderOpenAI, cfg, "gpt-4o", true, &cat)

	if env["OPENAI_API_KEY"] != "sk-openai-test-1234567890" {
		t.Errorf("expected OPENAI_API_KEY, got %q", env["OPENAI_API_KEY"])
	}
	if env["OPENAI_MODEL"] != "gpt-4o" {
		t.Errorf("expected OPENAI_MODEL 'gpt-4o', got %q", env["OPENAI_MODEL"])
	}
	if env["OPENAI_BASE_URL"] != DefaultOpenAIBaseURL {
		t.Errorf("expected OPENAI_BASE_URL %q, got %q", DefaultOpenAIBaseURL, env["OPENAI_BASE_URL"])
	}
}

func TestApplyProviderEnv_Gemini(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		GeminiAPIKey: "gemini-key-1234567890",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderGemini, cfg, "gemini-2.0-flash", true, &cat)

	if env["GEMINI_API_KEY"] != "gemini-key-1234567890" {
		t.Errorf("expected GEMINI_API_KEY, got %q", env["GEMINI_API_KEY"])
	}
	if env["GEMINI_MODEL"] != "gemini-2.0-flash" {
		t.Errorf("expected GEMINI_MODEL 'gemini-2.0-flash', got %q", env["GEMINI_MODEL"])
	}
	// Gemini uses OpenAI-compatible mapping
	if env["OPENAI_API_KEY"] != "gemini-key-1234567890" {
		t.Errorf("expected OPENAI_API_KEY set for gemini compat, got %q", env["OPENAI_API_KEY"])
	}
}

func TestApplyProviderEnv_DeepSeek(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		DeepSeekAPIKey: "deepseek-key-1234567890",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderDeepSeek, cfg, "deepseek-v4-flash", true, &cat)

	if env["DEEPSEEK_API_KEY"] != "deepseek-key-1234567890" {
		t.Errorf("expected DEEPSEEK_API_KEY, got %q", env["DEEPSEEK_API_KEY"])
	}
	if env["DEEPSEEK_MODEL"] != "deepseek-v4-flash" {
		t.Errorf("expected DEEPSEEK_MODEL 'deepseek-v4-flash', got %q", env["DEEPSEEK_MODEL"])
	}
	if env["OPENAI_API_KEY"] != "deepseek-key-1234567890" {
		t.Errorf("expected OPENAI_API_KEY set for deepseek compat, got %q", env["OPENAI_API_KEY"])
	}
}

func TestApplyProviderEnv_Fireworks(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		FireworksAPIKey:  "fireworks-key-1234567890",
		FireworksBaseURL: "https://fireworks.example/v1",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderFireworks, cfg, "accounts/fireworks/models/deepseek-v4-flash", true, &cat)

	if env["FIREWORKS_API_KEY"] != "fireworks-key-1234567890" {
		t.Errorf("expected FIREWORKS_API_KEY, got %q", env["FIREWORKS_API_KEY"])
	}
	if env["FIREWORKS_MODEL"] != "accounts/fireworks/models/deepseek-v4-flash" {
		t.Errorf("expected FIREWORKS_MODEL, got %q", env["FIREWORKS_MODEL"])
	}
	if env["FIREWORKS_BASE_URL"] != "https://fireworks.example/v1" {
		t.Errorf("expected FIREWORKS_BASE_URL, got %q", env["FIREWORKS_BASE_URL"])
	}
	if env["OPENAI_API_KEY"] != "fireworks-key-1234567890" || env["OPENAI_BASE_URL"] != "https://fireworks.example/v1" {
		t.Errorf("expected OpenAI compatibility env, got key=%q base=%q", env["OPENAI_API_KEY"], env["OPENAI_BASE_URL"])
	}
}

func TestApplyProviderEnv_Ollama(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		OllamaBaseURL: "http://localhost:11434",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderOllama, cfg, "llama3.1:8b", true, &cat)

	if env["OPENAI_MODEL"] != "llama3.1:8b" {
		t.Errorf("expected OPENAI_MODEL 'llama3.1:8b', got %q", env["OPENAI_MODEL"])
	}
	if env["OPENAI_BASE_URL"] != "http://localhost:11434/v1" {
		t.Errorf("expected OPENAI_BASE_URL 'http://localhost:11434/v1', got %q", env["OPENAI_BASE_URL"])
	}
}

func TestApplyProviderEnv_DefaultModel(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		AnthropicAPIKey: "sk-ant-test-1234567890",
	}
	cat := testModelCatalog()

	// Empty activeModel with empty catalog → model is empty (fully dynamic)
	env := ApplyProviderEnv(ProviderAnthropic, cfg, "", true, &cat)

	if env["ANTHROPIC_MODEL"] != "" {
		t.Errorf("expected ANTHROPIC_MODEL to be empty with empty catalog, got %q", env["ANTHROPIC_MODEL"])
	}
}

func TestApplyProviderEnv_OverwriteFalse(t *testing.T) {
	// Set an env var that should not be overwritten
	os.Setenv("ANTHROPIC_API_KEY", "existing-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	cfg := &ProviderConfig{
		AnthropicAPIKey: "new-key-1234567890",
	}
	cat := testModelCatalog()

	env := ApplyProviderEnv(ProviderAnthropic, cfg, "claude-sonnet-4-6", false, &cat)

	// With overwrite=false and env already set, the key should NOT be in the returned map
	if _, ok := env["ANTHROPIC_API_KEY"]; ok {
		t.Error("expected ANTHROPIC_API_KEY to not be set when overwrite=false and env exists")
	}
}

func TestSaveProviderConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "provider.json")

	cfg := &ProviderConfig{
		Version:        "1",
		ActiveProvider: "openai",
		OpenAIModel:    "gpt-4o",
	}

	err := SaveProviderConfig(cfg, path)
	if err != nil {
		t.Fatalf("SaveProviderConfig failed: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	// Verify it's valid JSON
	var loaded ProviderConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("saved config is invalid JSON: %v", err)
	}
	if loaded.ActiveProvider != "openai" {
		t.Errorf("expected active_provider 'openai', got %q", loaded.ActiveProvider)
	}
	// Verify file ends with newline
	if data[len(data)-1] != '\n' {
		t.Error("expected trailing newline")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("provider config permissions = %o, want 600", got)
	}
}

func TestSaveProviderConfigRefusesPlaintextCredentials(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := SaveProviderConfig(&ProviderConfig{OpenAIAPIKey: "sk-openai-1234567890"}, path); err == nil {
		t.Fatal("SaveProviderConfig accepted a plaintext credential")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("secret-bearing provider config reached disk: %v", err)
	}
}

func TestSaveProviderConfigDoesNotReplaceExistingStateOnRefusal(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "provider.json")
	if err := SaveProviderConfig(&ProviderConfig{ActiveProvider: "anthropic"}, path); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveProviderConfig(&ProviderConfig{OpenAIAPIKey: "sk-openai-1234567890"}, path); err == nil {
		t.Fatal("secret-bearing replacement was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatalf("refused write changed existing provider state: err=%v", err)
	}
}

func TestSaveProviderConfig_CreatesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "dir", "provider.json")

	cfg := &ProviderConfig{ActiveProvider: "gemini"}
	err := SaveProviderConfig(cfg, path)
	if err != nil {
		t.Fatalf("SaveProviderConfig failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist at nested path")
	}
}

func TestGetProviderModel(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		AnthropicModel: "claude-sonnet-4-6",
		OpenAIModel:    "gpt-4o",
		GrokModel:      "",
		XAIModel:       "grok-2",
	}

	if m := GetProviderModel(cfg, ProviderAnthropic); m != "claude-sonnet-4-6" {
		t.Errorf("expected 'claude-sonnet-4-6', got %q", m)
	}
	if m := GetProviderModel(cfg, ProviderOpenAI); m != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got %q", m)
	}
	// Grok should fall through empty GrokModel to XAIModel
	if m := GetProviderModel(cfg, ProviderGrok); m != "grok-2" {
		t.Errorf("expected 'grok-2' from XAIModel fallback, got %q", m)
	}
	if m := GetProviderModel(cfg, "nonexistent"); m != "" {
		t.Errorf("expected empty for unknown provider, got %q", m)
	}
}

func TestGetProviderAPIKey(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		AnthropicAPIKey: "sk-ant-key",
		GrokAPIKey:      "",
		XAIAPIKey:       "xai-key-fallback",
	}

	if k := GetProviderAPIKey(cfg, ProviderAnthropic); k != "sk-ant-key" {
		t.Errorf("expected 'sk-ant-key', got %q", k)
	}
	// Grok should fall through to XAI key
	if k := GetProviderAPIKey(cfg, ProviderGrok); k != "xai-key-fallback" {
		t.Errorf("expected 'xai-key-fallback', got %q", k)
	}
	if k := GetProviderAPIKey(cfg, ProviderOllama); k != "" {
		t.Errorf("expected empty for ollama (no API keys), got %q", k)
	}
}

func TestIsProviderConfigured(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		AnthropicAPIKey: "sk-ant-key",
		OllamaBaseURL:   "http://localhost:11434",
	}

	if !IsProviderConfigured(cfg, ProviderAnthropic) {
		t.Error("expected anthropic to be configured")
	}
	if !IsProviderConfigured(cfg, ProviderOllama) {
		t.Error("expected ollama to be configured")
	}
	if IsProviderConfigured(cfg, ProviderOpenAI) {
		t.Error("expected openai to NOT be configured")
	}
}

func TestDefaultProviderFromConfig(t *testing.T) {
	t.Parallel()
	cfg := &ProviderConfig{
		ActiveProvider:  "openai",
		OpenAIAPIKey:    "sk-openai-key",
		AnthropicAPIKey: "sk-ant-key",
	}

	provider := DefaultProviderFromConfig(cfg)
	if provider != "openai" {
		t.Errorf("expected 'openai' (explicit active), got %q", provider)
	}

	// If active provider is not configured, fall through detection order
	cfg2 := &ProviderConfig{
		ActiveProvider:  "gemini", // not configured
		AnthropicAPIKey: "sk-ant-key",
	}
	provider = DefaultProviderFromConfig(cfg2)
	if provider != "anthropic" {
		t.Errorf("expected 'anthropic' (first configured in detection order), got %q", provider)
	}

	// Nil config
	if DefaultProviderFromConfig(nil) != "" {
		t.Error("expected empty for nil config")
	}
}

func TestSetEnvValue(t *testing.T) {
	key := "TEST_SET_ENV_VALUE_KEY_12345"
	defer os.Unsetenv(key)

	// Empty value should not set
	SetEnvValue(key, "", true)
	if os.Getenv(key) != "" {
		t.Error("expected empty env for empty value")
	}

	// Non-empty value should set
	SetEnvValue(key, "hello", true)
	if os.Getenv(key) != "hello" {
		t.Errorf("expected 'hello', got %q", os.Getenv(key))
	}

	// overwrite=false should not replace existing
	SetEnvValue(key, "world", false)
	if os.Getenv(key) != "hello" {
		t.Errorf("expected 'hello' (not overwritten), got %q", os.Getenv(key))
	}

	// overwrite=true should replace
	SetEnvValue(key, "world", true)
	if os.Getenv(key) != "world" {
		t.Errorf("expected 'world', got %q", os.Getenv(key))
	}
}
