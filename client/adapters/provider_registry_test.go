package adapters

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestResolveEnvSecret(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"openai_api_key": "sk-test-key",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := ResolveEnvSecret("OPENAI_API_KEY")
	if got != "sk-test-key" {
		t.Errorf("ResolveEnvSecret = %q, want sk-test-key", got)
	}
}

func TestResolveEnvSecret_NotFound(t *testing.T) {
	store := &credentials.MapStore{Data: map[string]string{}}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := ResolveEnvSecret("NONEXISTENT_KEY")
	if got != "" {
		t.Errorf("ResolveEnvSecret = %q, want empty", got)
	}
}

func TestResolveProviderModelEnvOverride(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"openai_model": "gpt-4o",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := ResolveProviderModelEnvOverride("openai")
	if got != "gpt-4o" {
		t.Errorf("ResolveProviderModelEnvOverride = %q, want gpt-4o", got)
	}
}

func TestResolveProviderModelEnvOverride_Empty(t *testing.T) {
	store := &credentials.MapStore{Data: map[string]string{}}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := ResolveProviderModelEnvOverride("openai")
	if got != "" {
		t.Errorf("ResolveProviderModelEnvOverride = %q, want empty", got)
	}
}

func TestDetectProvider(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"openai_api_key": "sk-test-key",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "openai" {
		t.Errorf("DetectProvider = %q, want openai", got)
	}
}

func TestDetectProvider_NoProvider(t *testing.T) {
	store := &credentials.MapStore{Data: map[string]string{}}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "anthropic" {
		t.Errorf("DetectProvider = %q, want anthropic (default)", got)
	}
}

func TestDetectProvider_AllProfileCredentialProviders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		env      string
	}{
		{"concentrate", "CONCENTRATE_API_KEY"},
		{"agnes", "AGNES_API_KEY"},
		{"longcat", "LONGCAT_API_KEY"},
		{"fireworks", "FIREWORKS_API_KEY"},
		{"stepfun", "STEP_API_KEY"},
		{"opengateway", "OPENGATEWAY_API_KEY"},
		{"clinepass", "CLINE_API_KEY"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			store := &credentials.MapStore{}
			credentials.SetDefaultStore(store)
			t.Cleanup(func() { credentials.SetDefaultStore(nil) })
			if err := store.Set(context.Background(), credentials.AccountForEnv(tc.env), "test-key"); err != nil {
				t.Fatal(err)
			}
			if got := DetectProvider(); got != tc.provider {
				t.Fatalf("DetectProvider() = %q, want %q", got, tc.provider)
			}
		})
	}
}

func TestDetectProvider_PriorityOrder(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"anthropic_api_key": "sk-ant-test",
			"openai_api_key":    "sk-test-key",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "anthropic" {
		t.Errorf("DetectProvider = %q, want anthropic (priority order)", got)
	}
}

func TestDetectProvider_Ollama(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"ollama_base_url": "http://localhost:11434",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "ollama" {
		t.Errorf("DetectProvider = %q, want ollama", got)
	}
}

func TestDetectProvider_Azure(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"azure_openai_api_key":  "azure-key",
			"azure_openai_endpoint": "https://test.openai.azure.com",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "azure" {
		t.Errorf("DetectProvider = %q, want azure", got)
	}
}

func TestDetectProvider_Bedrock(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"aws_access_key_id":     "AKIA-TEST",
			"aws_secret_access_key": "secret",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "bedrock" {
		t.Errorf("DetectProvider = %q, want bedrock", got)
	}
}

func TestDetectProvider_Vertex(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"vertex_project_id":   "test-project",
			"vertex_access_token": "token",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "vertex" {
		t.Errorf("DetectProvider = %q, want vertex", got)
	}
}

func TestDetectProvider_Grok(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"xai_api_key": "xai-test",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "grok" {
		t.Errorf("DetectProvider = %q, want grok", got)
	}
}

func TestDetectProvider_Gemini(t *testing.T) {
	store := &credentials.MapStore{
		Data: map[string]string{
			"gemini_api_key": "gemini-key",
		},
	}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })

	got := DetectProvider()
	if got != "gemini" {
		t.Errorf("DetectProvider = %q, want gemini", got)
	}
}
