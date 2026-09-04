package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-router/credentials"
)

func TestDiscoveryCredentialsFromState_IsolatedFromProcessGlobals(t *testing.T) {
	ambientDir := t.TempDir()
	t.Setenv("GRAYCODE_ROUTER_CONFIG_DIR", ambientDir)
	t.Setenv("HAWK_CONFIG_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "sk-process-openai-1234567890")
	t.Setenv("GROQ_BASE_URL", "https://process-env.invalid/v1")
	t.Setenv(EnvXiaomiTokenPlanRegion, "cn")

	// Make both process-global persistence mechanisms contain values that must
	// not leak into a host-injected discovery snapshot.
	ambientState := &ProviderConfig{
		Version:           "1",
		OpenAIAPIKey:      "sk-path-openai-1234567890",
		GroqBaseURL:       "https://process-path.invalid/v1",
		CanopyWaveBaseURL: "https://process-canopy.invalid/v1",
	}
	data, err := json.Marshal(ambientState)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately seed a historical plaintext fixture; production writes reject it.
	if err := os.WriteFile(filepath.Join(ambientDir, "provider.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &credentials.MapStore{}
	credentials.SetDefaultStore(store)
	t.Cleanup(func() { credentials.SetDefaultStore(nil) })
	if err := store.Set(context.Background(), credentials.AccountForEnv("CANOPYWAVE_API_KEY"), "sk-store-canopy-1234567890"); err != nil {
		t.Fatal(err)
	}

	injected := map[string]string{
		"OPENROUTER_API_KEY":  "sk-injected-openrouter-1234567890",
		"PLACEHOLDER_API_KEY": "your-api-key",
	}
	cfg := &ProviderConfig{
		OpenAIAPIKey:              "sk-state-openai-must-not-leak",
		CanopyWaveAPIKey:          "sk-state-canopy-must-not-leak",
		CanopyWaveBaseURL:         "https://state-canopy.example/v1",
		XiaomiMimoTokenPlanRegion: "sgp",
		XiaomiMimoTokenPlanAPIKey: "sk-state-xiaomi-must-not-leak",
		Deployments: map[string]DeploymentConfig{
			"openai-azure": {
				APIKey:        "sk-state-azure-must-not-leak",
				Endpoint:      "https://state-azure.example",
				APIVersion:    "2026-01-01",
				ModelMappings: map[string]string{"gpt": "deployment-gpt"},
			},
			"anthropic-bedrock": {
				AccessKeyID:     "state-access-key-must-not-leak",
				SecretAccessKey: "state-secret-key-must-not-leak",
				SessionToken:    "state-session-token-must-not-leak",
				Region:          "ap-south-1",
			},
			"anthropic-vertex": {
				Token:     "state-vertex-token-must-not-leak",
				ProjectID: "state-project",
				Region:    "asia-south1",
			},
			"openai-direct": {
				APIKey:  "sk-state-deployment-must-not-leak",
				BaseURL: "https://state-openai.example/v1",
			},
		},
	}

	got := DiscoveryCredentialsFromState(injected, cfg).APIKeys

	want := map[string]string{
		"OPENROUTER_API_KEY":       "sk-injected-openrouter-1234567890",
		"CANOPYWAVE_BASE_URL":      "https://state-canopy.example/v1",
		EnvXiaomiTokenPlanRegion:   "sgp",
		EnvXiaomiTokenPlanBaseURL:  "https://token-plan-sgp.xiaomimimo.com/v1",
		"AZURE_OPENAI_ENDPOINT":    "https://state-azure.example",
		"AZURE_OPENAI_API_VERSION": "2026-01-01",
		"AZURE_OPENAI_DEPLOYMENT":  "deployment-gpt",
		"AWS_REGION":               "ap-south-1",
		"VERTEX_PROJECT_ID":        "state-project",
		"VERTEX_REGION":            "asia-south1",
		"OPENAI_BASE_URL":          "https://state-openai.example/v1",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}

	for _, key := range []string{
		"OPENAI_API_KEY",
		"CANOPYWAVE_API_KEY",
		EnvXiaomiTokenPlanAPIKey,
		"AZURE_OPENAI_API_KEY",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"VERTEX_ACCESS_TOKEN",
		"PLACEHOLDER_API_KEY",
		"GROQ_BASE_URL",
	} {
		if value := got[key]; value != "" {
			t.Errorf("process-global or persisted secret %s leaked as %q", key, value)
		}
	}

	// The returned snapshot must not alias the host's injected credential map.
	got["OPENROUTER_API_KEY"] = "changed"
	if injected["OPENROUTER_API_KEY"] != "sk-injected-openrouter-1234567890" {
		t.Fatal("DiscoveryCredentialsFromState mutated the injected credential map")
	}
}
