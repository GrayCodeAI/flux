package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog/registry"
)

// ConfigureProviderOpts is the complete engine input for provider setup.
type ConfigureProviderOpts struct {
	ProviderID string
	Secret     string
}

// ConfigureProviderResult is the normalized provider setup result for hosts.
type ConfigureProviderResult struct {
	ProviderID   string
	DeploymentID string
	Summary      string
	Models       []ModelEntry
}

// ConfigureProvider prepares provider environment, saves and probes its
// credential, refreshes its catalog, and returns normalized model rows.
func ConfigureProvider(ctx context.Context, opts ConfigureProviderOpts) (*ConfigureProviderResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	providerID := SetupGatewayID(opts.ProviderID)
	spec, ok := registry.SpecByProviderID(providerID)
	if !ok {
		return nil, fmt.Errorf("runtime: unknown setup provider %q", opts.ProviderID)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	ApplyGatewayEnv(ctx, providerID)
	var inference CredentialInference
	var err error
	if spec.RequiresKey {
		inference, err = InferenceForProvider(providerID)
	} else {
		inference, err = LocalCredentialInference(providerID)
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Secret) == "" {
		return nil, fmt.Errorf("runtime: credential value required for %s", providerID)
	}
	if err := SaveCredential(ctx, inference, opts.Secret); err != nil {
		return nil, err
	}
	applyResult, err := ApplyCredentialsForProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	models, err := ListModels(ctx, ListModelsOpts{ProviderID: providerID, Source: ListSourceAuto})
	if err != nil {
		return nil, err
	}
	return &ConfigureProviderResult{
		ProviderID: providerID, DeploymentID: inference.DeploymentID,
		Summary: FormatApplyCredentialsSummary(applyResult), Models: models,
	}, nil
}
