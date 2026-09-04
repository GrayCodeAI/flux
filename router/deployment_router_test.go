package router

import (
	"context"
	"fmt"
	"testing"

	"github.com/GrayCodeAI/graycode-router/catalog"
	"github.com/GrayCodeAI/graycode-router/client"
)

type deploymentMockProvider struct {
	name       string
	err        error
	streamErr  error
	lastModel  string
	lastTools  []client.GraycodeRouterTool
	streamDone bool
	callCount  int
}

func (m *deploymentMockProvider) Chat(_ context.Context, _ []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	m.lastModel = opts.Model
	m.lastTools = opts.Tools
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return &client.GraycodeRouterResponse{Content: "from " + m.name}, nil
}

func (m *deploymentMockProvider) StreamChat(_ context.Context, _ []client.GraycodeRouterMessage, opts client.ChatOptions) (*client.StreamResult, error) {
	m.lastModel = opts.Model
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan client.GraycodeRouterStreamEvent, 2)
	if m.streamErr != nil {
		ch <- client.GraycodeRouterStreamEvent{Type: "error", Error: m.streamErr.Error()}
	} else {
		ch <- client.GraycodeRouterStreamEvent{Type: "content", Content: "from " + m.name}
		ch <- client.GraycodeRouterStreamEvent{Type: "done"}
		m.streamDone = true
	}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func (m *deploymentMockProvider) Ping(_ context.Context) error { return m.err }
func (m *deploymentMockProvider) Name() string                 { return m.name }

func testCompiledCatalog(t *testing.T) *catalog.CompiledCatalog {
	t.Helper()
	compiled, err := catalog.CompileTestCatalog()
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	return compiled
}

func TestDeploymentRouterRewritesCanonicalModelToNativeModel(t *testing.T) {
	t.Parallel()
	p := &deploymentMockProvider{name: "anthropic"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: p},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{
			"anthropic": {{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "from anthropic" {
		t.Fatalf("unexpected response %q", resp.Content)
	}
	if p.lastModel != "claude-sonnet-4-6" {
		t.Fatalf("model sent to provider = %q, want native model", p.lastModel)
	}
}

func TestDeploymentRouterFallsBackAcrossStages(t *testing.T) {
	t.Parallel()
	primary := &deploymentMockProvider{name: "direct", err: fmt.Errorf("HTTP 503 unavailable")}
	vertex := &deploymentMockProvider{name: "vertex"}
	bedrock := &deploymentMockProvider{name: "bedrock"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct":  {Provider: primary},
			"anthropic-vertex":  {Provider: vertex},
			"anthropic-bedrock": {Provider: bedrock},
		},
		Routing: RoutingPolicy{
			Providers: map[string][]RoutingStage{
				"anthropic": {
					{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}}},
					{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-vertex", Weight: 50}, {DeploymentID: "anthropic-bedrock", Weight: 50}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "from vertex" && resp.Content != "from bedrock" {
		t.Fatalf("expected fallback deployment, got %q", resp.Content)
	}
}

func TestShouldTryNextDeploymentCredits(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("requires more credits, or fewer max_tokens; can only afford 5705")
	if !ShouldTryNextDeployment(err) {
		t.Fatal("expected credit error to allow next deployment")
	}
	if ShouldTryNextDeployment(fmt.Errorf("HTTP 401 unauthorized")) {
		t.Fatal("auth errors should not try next deployment")
	}
}

func TestDeploymentRouterFallsBackOnInsufficientCredits(t *testing.T) {
	t.Parallel()
	c := catalog.SeedCatalog()
	c.Providers["moonshotai"] = catalog.Provider{ID: "moonshotai", Name: "Moonshot AI"}
	c.Models["moonshotai/kimi-k2.6"] = catalog.Model{
		ID:         "moonshotai/kimi-k2.6",
		ProviderID: "moonshotai",
		Name:       "Kimi K2.6",
	}
	c.Offerings = append(
		c.Offerings,
		catalog.ModelOffering{
			ID: "openrouter:moonshotai/kimi-k2.6", CanonicalModelID: "moonshotai/kimi-k2.6",
			DeploymentID: "openrouter", NativeModelID: "moonshotai/kimi-k2.6",
			Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
		},
		catalog.ModelOffering{
			ID: "canopywave:moonshotai/kimi-k2.6", CanonicalModelID: "moonshotai/kimi-k2.6",
			DeploymentID: "canopywave", NativeModelID: "moonshotai/kimi-k2.6",
			Pricing: catalog.Pricing{Status: catalog.PricingUnknown},
		},
	)
	compiled, err := catalog.CompileCatalog(&c)
	if err != nil {
		t.Fatal(err)
	}
	openrouter := &deploymentMockProvider{
		name: "openrouter",
		err:  fmt.Errorf("requires more credits, or fewer max_tokens; can only afford 5705"),
	}
	canopywave := &deploymentMockProvider{name: "canopywave"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: compiled,
		Deployments: map[string]DeploymentAdapter{
			"openrouter": {Provider: openrouter},
			"canopywave": {Provider: canopywave},
		},
		Routing: RoutingPolicy{
			Default: []RoutingStage{{
				Deployments: []DeploymentChoice{{DeploymentID: "openrouter", Weight: 100}},
				Retries:     1,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "moonshotai/kimi-k2.6"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "from canopywave" {
		t.Fatalf("expected canopywave fallback, got %q", resp.Content)
	}
	if canopywave.lastModel != "moonshotai/kimi-k2.6" {
		t.Fatalf("canopywave model = %q", canopywave.lastModel)
	}
}

func TestDeploymentRouterNonTransientDoesNotFallback(t *testing.T) {
	t.Parallel()
	primary := &deploymentMockProvider{name: "direct", err: fmt.Errorf("HTTP 401 unauthorized")}
	fallback := &deploymentMockProvider{name: "vertex"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: primary},
			"anthropic-vertex": {Provider: fallback},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{"anthropic": {
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}}},
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-vertex", Weight: 100}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if fallback.lastModel != "" {
		t.Fatal("fallback provider should not be called for non-transient errors")
	}
}

func TestDeploymentRouterMaterializesAzureModelMapping(t *testing.T) {
	t.Parallel()
	azure := &deploymentMockProvider{name: "azure"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"openai-azure": {
				Provider: azure,
				ModelMappings: map[string]string{
					"openai/gpt-4o": "gpt-4o-prod",
				},
			},
		},
		Routing: RoutingPolicy{Models: map[string][]RoutingStage{
			"openai/gpt-4o": {{Deployments: []DeploymentChoice{{DeploymentID: "openai-azure", Weight: 100}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "openai/gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	if azure.lastModel != "gpt-4o-prod" {
		t.Fatalf("azure native model = %q, want mapping", azure.lastModel)
	}
}

func TestDeploymentRouterModelMappingOverridesCatalogOffering(t *testing.T) {
	t.Parallel()
	bedrock := &deploymentMockProvider{name: "bedrock"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-bedrock": {
				Provider: bedrock,
				ModelMappings: map[string]string{
					"anthropic/claude-sonnet-4-6": "anthropic.claude-sonnet-4-6-bedrock",
				},
			},
		},
		Routing: RoutingPolicy{Models: map[string][]RoutingStage{
			"anthropic/claude-sonnet-4-6": {{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-bedrock", Weight: 100}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	if bedrock.lastModel != "anthropic.claude-sonnet-4-6-bedrock" {
		t.Fatalf("bedrock native model = %q, want mapping override", bedrock.lastModel)
	}
}

func TestDeploymentRouterStreamFallbackBeforeOutput(t *testing.T) {
	t.Parallel()
	primary := &deploymentMockProvider{name: "direct", streamErr: fmt.Errorf("HTTP 503")}
	fallback := &deploymentMockProvider{name: "vertex"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: primary},
			"anthropic-vertex": {Provider: fallback},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{"anthropic": {
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-direct", Weight: 100}}},
			{Deployments: []DeploymentChoice{{DeploymentID: "anthropic-vertex", Weight: 100}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := r.StreamChat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	var content string
	for event := range stream.Events {
		if event.Content != "" {
			content += event.Content
		}
		if event.Type == "error" {
			t.Fatalf("unexpected stream error: %s", event.Error)
		}
	}
	if content != "from vertex" {
		t.Fatalf("content = %q, want fallback output", content)
	}
}

func TestDeploymentRouterNativeMimoUsesConfiguredXiaomiDeployment(t *testing.T) {
	t.Parallel()
	mimo := &deploymentMockProvider{name: "xiaomi"}
	compiled := &catalog.CompiledCatalog{
		Catalog: &catalog.Catalog{
			Aliases: map[string]string{"mimo-v2.5-pro": "opencodego/mimo-v2.5-pro"},
		},
		ModelsByID: map[string]catalog.Model{
			"opencodego/mimo-v2.5-pro": {ID: "opencodego/mimo-v2.5-pro", ProviderID: "opencodego"},
			"xiaomi_mimo_token_plan/mimo-v2.5-pro": {
				ID: "xiaomi_mimo_token_plan/mimo-v2.5-pro", ProviderID: "xiaomi_mimo_token_plan",
			},
		},
		DeploymentsByID: map[string]catalog.Deployment{
			"xiaomi_mimo_token_plan-direct": {ID: "xiaomi_mimo_token_plan-direct", ProviderID: "xiaomi_mimo_token_plan"},
		},
		OfferingsByDeployment: map[string][]catalog.ModelOffering{
			"xiaomi_mimo_token_plan-direct": {{
				CanonicalModelID: "xiaomi_mimo_token_plan/mimo-v2.5-pro",
				DeploymentID:     "xiaomi_mimo_token_plan-direct",
				NativeModelID:    "mimo-v2.5-pro",
			}},
		},
		OfferingsByCanonicalModel: map[string][]catalog.ModelOffering{
			"xiaomi_mimo_token_plan/mimo-v2.5-pro": {{
				CanonicalModelID: "xiaomi_mimo_token_plan/mimo-v2.5-pro",
				DeploymentID:     "xiaomi_mimo_token_plan-direct",
				NativeModelID:    "mimo-v2.5-pro",
			}},
		},
	}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: compiled,
		Deployments: map[string]DeploymentAdapter{
			"xiaomi_mimo_token_plan-direct": {DeploymentID: "xiaomi_mimo_token_plan-direct", Provider: mimo},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{
			"xiaomi_mimo_token_plan": {{Deployments: []DeploymentChoice{
				{DeploymentID: "xiaomi_mimo_token_plan-direct", Weight: 100},
			}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Chat(context.Background(), []client.GraycodeRouterMessage{{Role: "user", Content: "hi"}}, client.ChatOptions{Model: "mimo-v2.5-pro"})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if mimo.lastModel != "mimo-v2.5-pro" {
		t.Fatalf("native model = %q", mimo.lastModel)
	}
}

// TestDeploymentRouterRetriesPreferDifferentEndpoint verifies the fix for
// "deployment retry can re-select the same dead deployment": when a stage
// has multiple deployments and the first choice fails transiently, the next
// attempt should prefer a different deployment (and the healthy one is
// reached) instead of retrying the same dead endpoint up to stage.Retries.
func TestDeploymentRouterRetriesPreferDifferentEndpoint(t *testing.T) {
	t.Parallel()
	dead := &deploymentMockProvider{name: "direct", err: fmt.Errorf("HTTP 503 unavailable")}
	healthy := &deploymentMockProvider{name: "vertex"}
	r, err := NewDeploymentRouter(DeploymentRouterOptions{
		Catalog: testCompiledCatalog(t),
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: dead},
			"anthropic-vertex": {Provider: healthy},
		},
		Routing: RoutingPolicy{Providers: map[string][]RoutingStage{"anthropic": {{
			Deployments: []DeploymentChoice{
				{DeploymentID: "anthropic-direct", Weight: 100},
				{DeploymentID: "anthropic-vertex", Weight: 1},
			},
			Retries: 3,
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := r.Chat(context.Background(),
		[]client.GraycodeRouterMessage{{Role: "user", Content: "hi"}},
		client.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "from vertex" {
		t.Fatalf("expected vertex (healthy) deployment after direct (dead) failed, got %q", resp.Content)
	}
	// The dead provider is tried at most once (to discover the failure); the
	// retry must prefer the healthy endpoint instead of re-selecting the same
	// dead one up to stage.Retries times.
	if dead.callCount > 1 {
		t.Fatalf("dead deployment retried %d times; want at most 1", dead.callCount)
	}
	if healthy.callCount != 1 {
		t.Fatalf("healthy deployment called %d times; want 1", healthy.callCount)
	}
}
