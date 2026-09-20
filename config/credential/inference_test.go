package credential

import (
	"testing"
)

func TestInferenceForProvider_Anthropic(t *testing.T) {
	t.Parallel()
	inf, err := InferenceForProvider("anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if inf.ProviderID != "anthropic" || inf.DeploymentID != "anthropic-direct" || inf.EnvVar != "ANTHROPIC_API_KEY" {
		t.Fatalf("unexpected inference: %+v", inf)
	}
}

func TestInferenceForProvider_Ollama(t *testing.T) {
	t.Parallel()
	inf, err := InferenceForProvider("ollama")
	if err != nil {
		t.Fatal(err)
	}
	if inf.EnvVar != "OLLAMA_BASE_URL" || inf.DeploymentID != "ollama-local" {
		t.Fatalf("unexpected ollama inference: %+v", inf)
	}
}

func TestInferenceForProvider_Unknown(t *testing.T) {
	t.Parallel()
	if _, err := InferenceForProvider("not-a-provider"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
