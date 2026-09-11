package setup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/config"
)

// ApplyCredentialsResult is the full eyrie response after API keys are applied:
// refreshed catalog, provider.json (deployments + routing), and paths.
type ApplyCredentialsResult struct {
	Catalog            *catalog.RefreshResult
	ProviderConfig     *config.ProviderConfig
	ProviderConfigPath string
	RoutingJSON        string
	Setup              *SetupUI
}

// ApplyCredentialsForProvider discovers models for one provider after key setup, then updates routing.
func ApplyCredentialsForProvider(ctx context.Context, providerID string, creds catalog.Credentials) (*ApplyCredentialsResult, error) {
	catResult, err := DiscoverProviderCatalog(ctx, providerID, creds)
	if err != nil {
		return nil, fmt.Errorf("catalog discover: %w", err)
	}
	env := creds.Env()
	if len(env) == 0 {
		env = config.DiscoveryCredentials(ctx).Env()
	}
	cfg := config.SyncProviderConfigFromCatalog(catResult.Compiled, env)
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return nil, err
	}
	if err := config.SaveProviderConfig(cfg, path); err != nil {
		return nil, fmt.Errorf("save provider config: %w", err)
	}
	routingJSON := ""
	if cfg.Routing != nil {
		raw, err := json.MarshalIndent(cfg.Routing, "", "  ")
		if err != nil {
			return nil, err
		}
		routingJSON = string(raw)
	}
	setupUI := BuildSetupUI(catResult.Compiled, providerID)
	return &ApplyCredentialsResult{
		Catalog:            catResult,
		ProviderConfig:     cfg,
		ProviderConfigPath: path,
		RoutingJSON:        routingJSON,
		Setup:              setupUI,
	}, nil
}

// ApplyCredentials discovers the model catalog from env API keys, then writes
// ~/.hawk/provider.json deployments and routing derived from the catalog.
func ApplyCredentials(ctx context.Context, creds catalog.Credentials) (*ApplyCredentialsResult, error) {
	catResult, err := DiscoverModelCatalog(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("catalog discover: %w", err)
	}
	env := creds.Env()
	if len(env) == 0 {
		env = config.DiscoveryCredentials(ctx).Env()
	}
	cfg := config.SyncProviderConfigFromCatalog(catResult.Compiled, env)
	path, err := config.GetProviderConfigPath()
	if err != nil {
		return nil, err
	}
	if err := config.SaveProviderConfig(cfg, path); err != nil {
		return nil, fmt.Errorf("save provider config: %w", err)
	}
	routingJSON := ""
	if cfg.Routing != nil {
		raw, err := json.MarshalIndent(cfg.Routing, "", "  ")
		if err != nil {
			return nil, err
		}
		routingJSON = string(raw)
	}
	setupUI := BuildSetupUI(catResult.Compiled, "")
	return &ApplyCredentialsResult{
		Catalog:            catResult,
		ProviderConfig:     cfg,
		ProviderConfigPath: path,
		RoutingJSON:        routingJSON,
		Setup:              setupUI,
	}, nil
}
