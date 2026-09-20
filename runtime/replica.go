package runtime

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/flux/config"
	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/router/controlplane"
	"github.com/GrayCodeAI/flux/setup"
)

// NewReplicaFromState creates an independent chat data plane. Shared manifests
// contain deployment IDs and routes, never secrets. This instance resolves
// each ID from an explicit, local provider configuration captured at creation.
// To rotate credentials, create a new replica with new local state and switch
// the host's provider reference after it has accepted a manifest.
func NewReplicaFromState(cfg *config.ProviderConfig) (*controlplane.Replica, error) {
	if cfg == nil || len(cfg.Deployments) == 0 {
		return nil, fmt.Errorf("runtime: explicit local deployments are required for replica")
	}
	owned := *cfg
	owned.Deployments = make(map[string]config.DeploymentConfig, len(cfg.Deployments))
	for id, deployment := range cfg.Deployments {
		deployment.ModelMappings = setup.CloneStringMap(deployment.ModelMappings)
		owned.Deployments[id] = deployment
	}
	return controlplane.NewReplica(func(_ context.Context, id string) (core.Provider, error) {
		deployment, ok := owned.Deployments[id]
		if !ok {
			return nil, fmt.Errorf("runtime: deployment %q has no local configuration", id)
		}
		provider, ready := setup.ProviderForDeploymentFromState(id, deployment, &owned)
		if !ready {
			return nil, fmt.Errorf("runtime: deployment %q has incomplete local credentials", id)
		}
		return provider, nil
	})
}
