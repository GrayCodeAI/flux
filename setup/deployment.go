// Package setup wires catalog-backed deployment routing for hawk and eyrie CLIs.
package setup

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/catalog/registry"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/catalog/zai"
	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/router"
)

// oidcBedrockCreds and oidcVertexToken are injectable seams over the
// credentials OIDC helpers so the opt-in branch can be tested without real
// network or a live GitHub Actions runner. They default to the real helpers.
var (
	oidcBedrockCreds = credentials.BedrockCredentialsFromOIDC
	oidcVertexToken  = credentials.VertexTokenFromOIDC
)

// oidcEnabled reports whether the OIDC opt-in branch should be considered for
// the given deployment. It is gated by EYRIE_OIDC=1 (or true/yes/on) OR by the
// deployment specifying a roleARN / WIF audience. It is OFF by default.
func oidcEnabled(deployment config.DeploymentConfig) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EYRIE_OIDC"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return strings.TrimSpace(deployment.RoleARN) != "" || strings.TrimSpace(deployment.WIFAudience) != ""
}

func storeSecret(envKeys ...string) string {
	ctx := context.Background()
	for _, k := range envKeys {
		if v := credentials.LookupSecret(ctx, k); v != "" {
			return v
		}
	}
	return ""
}

// UseDeploymentRouting mirrors eyrie CLI behavior: env override, then provider.json shape.
func UseDeploymentRouting(cfg *config.ProviderConfig) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EYRIE_DEPLOYMENT_ROUTING"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return DeploymentRoutingFromState(cfg)
}

// DeploymentRoutingFromState reports routing state without consulting process
// environment. Host-facing Engine code must use this pure form.
func DeploymentRoutingFromState(cfg *config.ProviderConfig) bool {
	return cfg != nil && (cfg.ConfigVersion >= 2 || len(cfg.Deployments) > 0 || cfg.Routing != nil)
}

// DeploymentProvider builds a catalog-aware router over configured deployments.
func DeploymentProvider(ctx context.Context, cfg *config.ProviderConfig) (client.Provider, error) {
	compiled, err := catalog.LoadCatalog(ctx, catalog.LoadCatalogOptions{
		CachePath:     catalog.DefaultCachePath(),
		RefreshRemote: strings.EqualFold(os.Getenv("EYRIE_MODEL_CATALOG_REFRESH"), "true"),
	})
	if err != nil {
		return nil, err
	}
	return DeploymentProviderFromCatalog(cfg, compiled)
}

// DeploymentProviderFromCatalog is the ambient compatibility constructor. It
// may consult the default store, process environment, and flat-config detection.
// Host integrations must use DeploymentProviderFromState instead.
func DeploymentProviderFromCatalog(cfg *config.ProviderConfig, compiled *catalog.CompiledCatalog) (client.Provider, error) {
	return deploymentProviderFromCatalog(cfg, compiled, true)
}

// DeploymentProviderFromState builds a router exclusively from the supplied
// provider state. It never reads the default credential store, process
// environment, process-default provider path, or flat-config detection.
// Host-facing Engine code must use this strict constructor.
func DeploymentProviderFromState(cfg *config.ProviderConfig, compiled *catalog.CompiledCatalog) (client.Provider, error) {
	return deploymentProviderFromCatalog(cfg, compiled, false)
}

func deploymentProviderFromCatalog(cfg *config.ProviderConfig, compiled *catalog.CompiledCatalog, allowAmbient bool) (client.Provider, error) {
	if compiled == nil {
		return nil, fmt.Errorf("deployment provider: catalog is nil")
	}
	deployments := configuredDeploymentAdapters(cfg, allowAmbient)
	if len(deployments) == 0 {
		return nil, fmt.Errorf("no deployment credentials configured")
	}
	var routing *config.RoutingPolicy
	if cfg != nil {
		routing = cfg.Routing
	}
	return router.NewDeploymentRouter(router.DeploymentRouterOptions{
		Catalog:     compiled,
		Deployments: deployments,
		Routing:     RouterRoutingPolicy(routing),
	})
}

// ConfiguredDeploymentAdapters maps deployment IDs to live provider clients.
func ConfiguredDeploymentAdapters(cfg *config.ProviderConfig) map[string]router.DeploymentAdapter {
	return configuredDeploymentAdapters(cfg, true)
}

func configuredDeploymentAdapters(cfg *config.ProviderConfig, allowAmbient bool) map[string]router.DeploymentAdapter {
	out := map[string]router.DeploymentAdapter{}
	configured := explicitDeployments(cfg)
	if allowAmbient {
		configured = ConfiguredDeployments(cfg)
	}
	for id, deployment := range configured {
		provider, ok := providerForDeployment(id, deployment, cfg, allowAmbient)
		if !ok {
			continue
		}
		out[id] = router.DeploymentAdapter{
			DeploymentID:  id,
			Provider:      provider,
			ModelMappings: CloneStringMap(deployment.ModelMappings),
		}
	}
	return out
}

func explicitDeployments(cfg *config.ProviderConfig) map[string]config.DeploymentConfig {
	out := map[string]config.DeploymentConfig{}
	if cfg != nil {
		for id, deployment := range cfg.Deployments {
			out[id] = deployment
		}
	}
	return out
}

// ConfiguredDeployments merges explicit deployments with flat provider.json config.
func ConfiguredDeployments(cfg *config.ProviderConfig) map[string]config.DeploymentConfig {
	out := map[string]config.DeploymentConfig{}
	if cfg != nil {
		for id, deployment := range cfg.Deployments {
			out[id] = deployment
		}
	}
	if len(out) > 0 {
		return out
	}
	provider := client.DetectProvider()
	if cfg != nil {
		if configured := config.DefaultProviderFromConfig(cfg); configured != "" {
			provider = configured
		}
	}
	if id := DefaultDeploymentForProvider(provider); id != "" {
		out[id] = DeploymentConfigFromProviderState(cfg, provider)
	}
	return out
}

// ProviderForDeployment constructs one adapter with ambient compatibility
// fallbacks. Host integrations must use ProviderForDeploymentFromState.
func ProviderForDeployment(id string, deployment config.DeploymentConfig) (client.Provider, bool) {
	return providerForDeployment(id, deployment, nil, true)
}

// ProviderForDeploymentFromState constructs exactly one adapter without any
// process-global credential, environment, OIDC, or provider-config fallback.
func ProviderForDeploymentFromState(id string, deployment config.DeploymentConfig, cfg *config.ProviderConfig) (client.Provider, bool) {
	return providerForDeployment(id, deployment, cfg, false)
}

func providerForDeployment(id string, deployment config.DeploymentConfig, cfg *config.ProviderConfig, allowAmbient bool) (client.Provider, bool) {
	lookup := func(...string) string { return "" }
	getenv := func(string) string { return "" }
	if allowAmbient {
		lookup = storeSecret
		getenv = os.Getenv
		if cfg == nil {
			cfg = config.LoadProviderConfig("")
		}
	}
	switch id {
	case "anthropic-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("ANTHROPIC_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewAnthropicClient(apiKey, FirstNonEmpty(deployment.BaseURL, getenv("ANTHROPIC_BASE_URL"))), true
	case "anthropic-vertex":
		projectID := FirstNonEmpty(deployment.ProjectID, getenv("VERTEX_PROJECT_ID"))
		region := FirstNonEmpty(deployment.Region, getenv("VERTEX_REGION"))
		// Opt-in OIDC keyless auth: only when enabled AND running in GitHub
		// Actions. On ErrNoOIDC or any failure we fall through to stored token.
		if allowAmbient && projectID != "" && region != "" && oidcEnabled(deployment) && credentials.DetectGitHubActions() {
			audience := FirstNonEmpty(deployment.WIFAudience, getenv("VERTEX_WIF_AUDIENCE"))
			sa := FirstNonEmpty(deployment.ServiceAccountEmail, getenv("VERTEX_SERVICE_ACCOUNT_EMAIL"))
			if oidcTok, err := oidcVertexToken(context.Background(), audience, sa); err == nil && oidcTok != "" {
				return client.NewVertexClient(projectID, region, oidcTok), true
			}
		}
		token := FirstNonEmpty(deployment.Token, deployment.APIKey, lookup("VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN"))
		if projectID == "" || region == "" || token == "" {
			return nil, false
		}
		return client.NewVertexClient(projectID, region, token), true
	case "anthropic-bedrock":
		region := FirstNonEmpty(deployment.Region, getenv("AWS_REGION"), getenv("AWS_DEFAULT_REGION"))
		// Opt-in OIDC keyless auth: only when enabled AND running in GitHub
		// Actions. On ErrNoOIDC or any failure we fall through to stored creds.
		if allowAmbient && oidcEnabled(deployment) && credentials.DetectGitHubActions() {
			roleARN := FirstNonEmpty(deployment.RoleARN, getenv("AWS_ROLE_ARN"))
			if creds, err := oidcBedrockCreds(context.Background(), roleARN, region); err == nil &&
				creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
				oidcRegion := FirstNonEmpty(creds.Region, region)
				if oidcRegion != "" {
					return client.NewBedrockClient(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, oidcRegion), true
				}
			}
		}
		accessKeyID := FirstNonEmpty(deployment.AccessKeyID, deployment.APIKey, lookup("AWS_ACCESS_KEY_ID"))
		secretAccessKey := FirstNonEmpty(deployment.SecretAccessKey, deployment.Token, lookup("AWS_SECRET_ACCESS_KEY"))
		sessionToken := FirstNonEmpty(deployment.SessionToken, lookup("AWS_SESSION_TOKEN"))
		if region == "" || accessKeyID == "" || secretAccessKey == "" {
			return nil, false
		}
		return client.NewBedrockClient(accessKeyID, secretAccessKey, sessionToken, region), true
	case "openai-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("OPENAI_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewOpenAIClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultOpenAIBaseURL), &client.OpenAICompat), true
	case "openai-azure":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("AZURE_OPENAI_API_KEY"))
		endpoint := FirstNonEmpty(deployment.Endpoint, getenv("AZURE_OPENAI_ENDPOINT"))
		apiVersion := FirstNonEmpty(deployment.APIVersion, getenv("AZURE_OPENAI_API_VERSION"))
		if apiKey == "" || endpoint == "" {
			return nil, false
		}
		return client.NewAzureClient(apiKey, endpoint, apiVersion), true
	case "grok-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("XAI_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewGrokClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultGrokOpenAIBaseURL), &client.GrokCompat), true
	case "gemini-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("GEMINI_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewGeminiOpenAIClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultGeminiOpenAIBaseURL), &client.GeminiCompat), true
	case "gemini-vertex":
		projectID := FirstNonEmpty(deployment.ProjectID, getenv("VERTEX_PROJECT_ID"))
		region := FirstNonEmpty(deployment.Region, getenv("VERTEX_REGION"))
		token := FirstNonEmpty(deployment.Token, deployment.APIKey, lookup("VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN"))
		if projectID == "" || region == "" || token == "" {
			return nil, false
		}
		return client.NewGeminiClient(token, config.VertexGeminiBaseURL(projectID, region)), true
	case "openrouter":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("OPENROUTER_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewOpenRouterClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultOpenRouterOpenAIBaseURL), &client.OpenRouterCompat), true
	case "canopywave":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("CANOPYWAVE_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewCanopyWaveClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultCanopyWaveOpenAIBaseURL), &client.CanopyWaveCompat), true
	case "opengateway-payg":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("OPENGATEWAY_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewOpenGatewayClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultOpenGatewayOpenAIBaseURL), &client.OpenGatewayCompat), true
	case "deepseek-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("DEEPSEEK_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		openBase := FirstNonEmpty(deployment.BaseURL, "https://api.deepseek.com")
		return client.NewDeepSeekClient(apiKey, openBase, &client.DeepSeekCompat), true
	case "poolside":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("POOLSIDE_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewPoolsideClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultPoolsideOpenAIBaseURL)), true
	case "groq-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("GROQ_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewGroqClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultGroqOpenAIBaseURL), &client.GroqCompat), true
	case "clinepass":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("CLINE_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewClinePassClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultClinePassOpenAIBaseURL), &client.ClinePassCompat), true
	case "zai_payg-direct":
		return newZAIDeploymentClient(deployment, "zai_payg", "ZAI_API_KEY", lookup, cfg)
	case "zai_coding-direct":
		return newZAIDeploymentClient(deployment, "zai_coding", "ZAI_CODING_API_KEY", lookup, cfg)
	case "ollama-local":
		baseURL := config.NormalizeOllamaOpenAIBaseURL(FirstNonEmpty(deployment.BaseURL, getenv("OLLAMA_BASE_URL"), config.OllamaDefaultBaseURL))
		return client.NewOllamaClient(FirstNonEmpty(deployment.APIKey, lookup("OLLAMA_API_KEY")), baseURL, &client.OllamaCompat), true
	case "opencodego":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("OPENCODEGO_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewOpenCodeGoClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultOpenCodeGoBaseURL)), true
	case "kimi-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("MOONSHOT_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewKimiClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultKimiOpenAIBaseURL), &client.KimiCompat), true
	case "agnes-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("AGNES_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewAgnesClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultAgnesOpenAIBaseURL), &client.AgnesCompat), true
	case "longcat-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("LONGCAT_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewLongCatClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultLongCatOpenAIBaseURL), config.DefaultLongCatAnthropicBaseURL, &client.LongCatCompat), true
	case "stepfun-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("STEP_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewStepFunClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultStepFunOpenAIBaseURL), &client.StepFunCompat), true
	case "xiaomi_mimo_payg-direct":
		return newMiMoDeploymentClient(deployment, config.ProviderXiaomiMimoPayg, "XIAOMI_MIMO_PAYG_API_KEY", lookup, cfg)
	case "xiaomi_mimo_token_plan-direct":
		return newMiMoDeploymentClient(deployment, config.ProviderXiaomiMimoTokenPlan, "XIAOMI_MIMO_TOKEN_PLAN_API_KEY", lookup, cfg)
	case "minimax_token_plan-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("MINIMAX_TOKEN_PLAN_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewMiniMaxClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultMiniMaxOpenAIBaseURL), &client.MiniMaxCompat), true
	case "minimax_payg-direct":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("MINIMAX_PAYG_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		return client.NewMiniMaxClient(apiKey, FirstNonEmpty(deployment.BaseURL, config.DefaultMiniMaxOpenAIBaseURL), &client.MiniMaxCompat), true
	case "concentrate-payg":
		apiKey := FirstNonEmpty(deployment.APIKey, lookup("CONCENTRATE_API_KEY"))
		if apiKey == "" {
			return nil, false
		}
		baseURL := FirstNonEmpty(deployment.BaseURL, config.DefaultConcentrateOpenAIBaseURL)
		return client.NewConcentrateResponsesClient(apiKey, baseURL), true
	default:
		return nil, false
	}
}

func newMiMoDeploymentClient(deployment config.DeploymentConfig, providerID, envKey string, lookup func(...string) string, cfg *config.ProviderConfig) (client.Provider, bool) {
	apiKey := FirstNonEmpty(deployment.APIKey, lookup(envKey))
	if apiKey == "" {
		return nil, false
	}
	openBase, err := config.ResolveXiaomiOpenAIBase(providerID, cfg)
	if err != nil || openBase == "" {
		openBase = FirstNonEmpty(deployment.BaseURL, config.DefaultXiaomiOpenAIBaseURL)
	}
	// Token Plan hosts are region-specific; do not let stale deployment.BaseURL override provider.json routing.
	if billing, ok := xiaomi.BillingForProvider(providerID); !ok || billing != xiaomi.BillingTokenPlan {
		if override := FirstNonEmpty(deployment.BaseURL); override != "" {
			openBase = override
		}
	}
	return client.NewMiMoClient(apiKey, openBase, &client.XiaomiCompat, providerID), true
}

// newZAIDeploymentClient constructs a dual-protocol (OpenAI + Anthropic) Z.AI client
// for either the general or Coding Plan gateway, resolving the correct bases
// for the plan + region (international or china) per official docs.
func newZAIDeploymentClient(deployment config.DeploymentConfig, providerID, envKey string, lookup func(...string) string, cfg *config.ProviderConfig) (client.Provider, bool) {
	apiKey := FirstNonEmpty(deployment.APIKey, lookup(envKey))
	if apiKey == "" {
		return nil, false
	}

	plan, _ := zai.PlanForProvider(providerID)

	openBase, err := resolveZAIOpenAIBaseForDeployment(plan, providerID, cfg, deployment.BaseURL)
	if err != nil || openBase == "" {
		if plan == zai.PlanCoding {
			openBase = FirstNonEmpty(deployment.BaseURL, config.DefaultZAICodingOpenAIBaseURL, zai.CodingInternationalOpenAIBase)
		} else {
			openBase = FirstNonEmpty(deployment.BaseURL, config.DefaultZAIOpenAIBaseURL, zai.GeneralInternationalOpenAIBase)
		}
	}

	anthropicBase := resolveZAIAnthropicBaseForDeployment(plan, cfg)

	return client.NewZAIClient(apiKey, openBase, anthropicBase, &client.ZAICompat, providerID), true
}

func resolveZAIOpenAIBaseForDeployment(plan zai.Plan, providerID string, cfg *config.ProviderConfig, override string) (string, error) {
	regionStr := ""
	if cfg != nil {
		if providerID == "zai_coding" {
			regionStr = cfg.ZAICodingRegion
		} else {
			regionStr = cfg.ZAIRegion
		}
	}
	region, _ := zai.NormalizeRegion(regionStr)
	return zai.ResolveOpenAIBase(plan, region, override)
}

func resolveZAIAnthropicBaseForDeployment(plan zai.Plan, cfg *config.ProviderConfig) string {
	// region from general or coding, prefer coding if set
	regionStr := ""
	if cfg != nil {
		regionStr = cfg.ZAICodingRegion
		if regionStr == "" {
			regionStr = cfg.ZAIRegion
		}
	}
	region, _ := zai.NormalizeRegion(regionStr)
	return zai.ResolveAnthropicBase(region)
}

// DefaultDeploymentForProvider resolves the default deployment for a logical
// provider name from the provider registry. New providers need no setup-side
// mapping.
func DefaultDeploymentForProvider(provider string) string {
	if spec, ok := registry.SpecByProviderID(provider); ok {
		return spec.DeploymentID
	}
	return ""
}

// DeploymentConfigFromProviderState reads API keys and base URLs from flat
// provider.json fields via the provider registry.
func DeploymentConfigFromProviderState(cfg *config.ProviderConfig, provider string) config.DeploymentConfig {
	return config.DeploymentConfigFromProviderState(cfg, provider)
}

// RouterRoutingPolicy converts config routing JSON into router policy.
func RouterRoutingPolicy(policy *config.RoutingPolicy) router.RoutingPolicy {
	if policy == nil {
		return router.RoutingPolicy{}
	}
	return router.RoutingPolicy{
		Default:   RouterRoutingStages(policy.Default),
		Providers: RouterRoutingStageMap(policy.Providers),
		Models:    RouterRoutingStageMap(policy.Models),
	}
}

// RouterRoutingStageMap converts per-key routing stages.
func RouterRoutingStageMap(in map[string][]config.RoutingStage) map[string][]router.RoutingStage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]router.RoutingStage, len(in))
	for key, stages := range in {
		out[key] = RouterRoutingStages(stages)
	}
	return out
}

// RouterRoutingStages converts config stages to router stages.
func RouterRoutingStages(stages []config.RoutingStage) []router.RoutingStage {
	out := make([]router.RoutingStage, len(stages))
	for i, stage := range stages {
		out[i].Retries = stage.Retries
		out[i].Deployments = make([]router.DeploymentChoice, len(stage.Deployments))
		for j, choice := range stage.Deployments {
			out[i].Deployments[j] = router.DeploymentChoice{DeploymentID: choice.DeploymentID, Weight: choice.Weight}
		}
	}
	return out
}

// FirstNonEmpty returns the first non-empty trimmed string.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// CloneStringMap returns a shallow copy of m.
func CloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
