package client

import (
	"errors"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

func TestMimoRetryableChatError_HTTPStatus(t *testing.T) {
	err401 := errors.New("eyrie: openai API error: credential probe failed: invalid API key (HTTP 401)")
	if !mimoRetryableChatError(err401) {
		t.Fatal("expected 401 retryable")
	}
	err400 := errors.New("eyrie: openai API error (HTTP 400)")
	if mimoRetryableChatError(err400) {
		t.Fatal("expected 400 not retryable")
	}
}

func TestMimoRetryableChatError_UsesXiaomiHelper(t *testing.T) {
	if !xiaomi.IsRetryableHTTPStatus(http.StatusServiceUnavailable) {
		t.Fatal("expected 503 retryable in xiaomi helper")
	}
	err := errors.New("provider unavailable (HTTP 503)")
	if !mimoRetryableChatError(err) {
		t.Fatal("expected 503 retryable")
	}
}

func TestGetOrCreateProvider_XiaomiTokenPlanUsesMimoBase(t *testing.T) {
	t.Setenv("HAWK_CONFIG_DIR", t.TempDir())
	if err := eyriecfg.SaveProviderConfig(&eyriecfg.ProviderConfig{
		XiaomiMimoTokenPlanRegion: "sgp",
	}, ""); err != nil {
		t.Fatalf("SaveProviderConfig: %v", err)
	}

	c := Client(&EyrieConfig{Provider: "xiaomi_mimo_token_plan", APIKey: "tp-test-key"})
	p, err := c.getOrCreateProvider("xiaomi_mimo_token_plan")
	if err != nil {
		t.Fatalf("getOrCreateProvider: %v", err)
	}
	mimo, ok := p.(*MiMoClient)
	if !ok {
		t.Fatalf("provider type = %T, want *MiMoClient", p)
	}
	if mimo.ProviderID() != "xiaomi_mimo_token_plan" {
		t.Fatalf("providerID = %q, want xiaomi_mimo_token_plan", mimo.ProviderID())
	}
}
