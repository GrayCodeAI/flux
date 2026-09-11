package catalog_test

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/catalog"
)

func TestGatewayForModel_OpenRouterPrefix(t *testing.T) {
	t.Parallel()
	if got := catalog.GatewayForModel(nil, "openrouter/auto"); got != "openrouter" {
		t.Fatalf("gateway = %q", got)
	}
}

func TestIsSetupGateway_OwnerSlugFalse(t *testing.T) {
	t.Parallel()
	if catalog.IsSetupGateway("moonshotai") {
		t.Fatal("moonshotai is an owner, not a gateway")
	}
	if !catalog.IsSetupGateway("openrouter") {
		t.Fatal("openrouter should be a gateway")
	}
}
