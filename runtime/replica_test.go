package runtime

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/flux/catalog"
	"github.com/GrayCodeAI/flux/config"
	"github.com/GrayCodeAI/flux/router/controlplane"
)

func TestNewReplicaFromStateUsesOnlyExplicitLocalDeployment(t *testing.T) {
	if _, err := NewReplicaFromState(nil); err == nil {
		t.Fatal("nil local configuration accepted")
	}
	cfg := &config.ProviderConfig{Deployments: map[string]config.DeploymentConfig{
		"anthropic-direct": {APIKey: "local-test-key"},
	}}
	replica, err := NewReplicaFromState(cfg)
	if err != nil {
		t.Fatal(err)
	}
	manifest := controlplane.Manifest{
		Revision:    1,
		Catalog:     catalog.SeedCatalog(),
		Deployments: []controlplane.Deployment{{ID: "anthropic-direct"}},
	}
	// Changing caller state after construction cannot remove or change the
	// replica's local credential source.
	delete(cfg.Deployments, "anthropic-direct")
	if err := replica.Apply(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if replica.Revision() != 1 {
		t.Fatalf("revision = %d", replica.Revision())
	}
	manifest.Revision = 2
	manifest.Deployments = []controlplane.Deployment{{ID: "anthropic-vertex"}}
	if err := replica.Apply(context.Background(), manifest); err == nil {
		t.Fatal("deployment without local credentials was accepted")
	}
	if replica.Revision() != 1 {
		t.Fatal("rejected deployment changed active revision")
	}
}
