package router

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/GrayCodeAI/eyrie/client"
)

type DeploymentChoice struct {
	DeploymentID string `json:"deployment_id"`
	Weight       int    `json:"weight"`
}

type RoutingStage struct {
	Deployments []DeploymentChoice `json:"deployments"`
	Retries     int                `json:"retries,omitempty"`
}

type RoutingPolicy struct {
	Default   []RoutingStage            `json:"default,omitempty"`
	Providers map[string][]RoutingStage `json:"providers,omitempty"`
	Models    map[string][]RoutingStage `json:"models,omitempty"`
}

type DeploymentAdapter struct {
	DeploymentID  string
	Provider      client.Provider
	ModelMappings map[string]string
}

// CircuitBreakerConfig holds tunable circuit breaker parameters.
type CircuitBreakerConfig struct {
	Threshold int           `json:"threshold"` // failures before opening (default 5)
	Cooldown  time.Duration `json:"cooldown"`  // time before half-open (default 30s)
}

// DefaultCircuitBreakerConfig returns production defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold: 5,
		Cooldown:  30 * time.Second,
	}
}

type DeploymentRouterOptions struct {
	Catalog        *catalog.CompiledCatalog
	Deployments    map[string]DeploymentAdapter
	Routing        RoutingPolicy
	CircuitBreaker *CircuitBreakerConfig // optional, defaults applied if nil
}

type DeploymentRouter struct {
	catalog     *catalog.CompiledCatalog
	deployments map[string]DeploymentAdapter
	routing     RoutingPolicy
	cbConfig    CircuitBreakerConfig
	statsMu     sync.RWMutex
	stats       map[string]*atomic.Int64
	breakersMu  sync.RWMutex
	breakers    map[string]*CircuitBreaker
}

var _ client.Provider = (*DeploymentRouter)(nil)

func NewDeploymentRouter(opts DeploymentRouterOptions) (*DeploymentRouter, error) {
	if opts.Catalog == nil {
		return nil, fmt.Errorf("deployment router: catalog is required")
	}
	if len(opts.Deployments) == 0 {
		return nil, fmt.Errorf("deployment router: at least one deployment is required")
	}
	deployments := make(map[string]DeploymentAdapter, len(opts.Deployments))
	stats := make(map[string]*atomic.Int64, len(opts.Deployments))
	for id, adapter := range opts.Deployments {
		if adapter.DeploymentID == "" {
			adapter.DeploymentID = id
		}
		if adapter.DeploymentID != id {
			return nil, fmt.Errorf("deployment router: deployment key %q does not match adapter %q", id, adapter.DeploymentID)
		}
		if adapter.Provider == nil {
			return nil, fmt.Errorf("deployment router: deployment %q has nil provider", id)
		}
		if opts.Catalog.DeploymentsByID[id].ID == "" {
			return nil, fmt.Errorf("deployment router: deployment %q is not in catalog", id)
		}
		adapter.ModelMappings = cloneStrings(adapter.ModelMappings)
		deployments[id] = adapter
		stats[id] = &atomic.Int64{}
	}
	cbConfig := DefaultCircuitBreakerConfig()
	if opts.CircuitBreaker != nil {
		cbConfig = *opts.CircuitBreaker
	}
	router := &DeploymentRouter{
		catalog:     opts.Catalog,
		deployments: deployments,
		routing:     cloneRoutingPolicy(opts.Routing),
		cbConfig:    cbConfig,
		stats:       stats,
		breakers:    make(map[string]*CircuitBreaker, len(deployments)),
	}
	return router, nil
}

func (r *DeploymentRouter) Name() string {
	return "deployment-router"
}

func (r *DeploymentRouter) Ping(ctx context.Context) error {
	var lastErr error
	for _, id := range sortedDeploymentIDs(r.deployments) {
		if err := r.deployments[id].Provider.Ping(ctx); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("deployment router: all deployments failed ping: %w", lastErr)
	}
	return fmt.Errorf("deployment router: no deployments configured")
}

func (r *DeploymentRouter) Chat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.EyrieResponse, error) {
	target, err := r.resolveTarget(opts.Model)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for stageIndex, stage := range r.routeFor(target.canonicalModelID) {
		choices := r.eligibleChoices(target, stage, opts)
		if len(choices) == 0 {
			lastErr = fmt.Errorf("stage %d has no eligible deployments", stageIndex)
			continue
		}
		attempts := stage.Retries + 1
		if attempts < 1 {
			attempts = 1
		}
		// Track the deployment that just failed this stage so the next attempt
		// prefers a different deployment when one is available, instead of
		// re-selecting the same dead endpoint up to stage.Retries times.
		recentlyFailed := ""
		for attempt := 0; attempt < attempts; attempt++ {
			choice := selectDeploymentChoice(choices, recentlyFailed)
			resp, err := r.chatWithDeployment(ctx, messages, opts, target, choice.DeploymentID)
			if err == nil {
				r.recordSuccess(choice.DeploymentID)
				return resp, nil
			}
			lastErr = err
			r.recordFailure(choice.DeploymentID)
			if !IsTransient(err) {
				if ShouldTryNextDeployment(err) {
					break
				}
				return nil, err
			}
			recentlyFailed = choice.DeploymentID
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no route configured")
	}
	return nil, fmt.Errorf("deployment router: all deployments failed for %q: %w", target.canonicalModelID, lastErr)
}

func (r *DeploymentRouter) StreamChat(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions) (*client.StreamResult, error) {
	target, err := r.resolveTarget(opts.Model)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	out := make(chan client.EyrieStreamEvent, 64)
	go func() {
		defer close(out)
		var lastErr error
		for stageIndex, stage := range r.routeFor(target.canonicalModelID) {
			choices := r.eligibleChoices(target, stage, opts)
			if len(choices) == 0 {
				lastErr = fmt.Errorf("stage %d has no eligible deployments", stageIndex)
				continue
			}
			attempts := stage.Retries + 1
			if attempts < 1 {
				attempts = 1
			}
			// Prefer a different deployment than the one that just failed
			// this stage, instead of re-selecting the same dead endpoint
			// up to stage.Retries times.
			recentlyFailed := ""
			for attempt := 0; attempt < attempts; attempt++ {
				choice := selectDeploymentChoice(choices, recentlyFailed)
				fallback, err := r.streamWithDeployment(streamCtx, out, messages, opts, target, choice.DeploymentID)
				if err == nil {
					r.recordSuccess(choice.DeploymentID)
					return
				}
				lastErr = err
				r.recordFailure(choice.DeploymentID)
				if !fallback {
					select {
					case out <- client.EyrieStreamEvent{Type: "error", Error: err.Error()}:
					case <-streamCtx.Done():
					}
					return
				}
				if !IsTransient(err) {
					if ShouldTryNextDeployment(err) {
						break
					}
					select {
					case out <- client.EyrieStreamEvent{Type: "error", Error: err.Error()}:
					case <-streamCtx.Done():
					}
					return
				}
				recentlyFailed = choice.DeploymentID
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no route configured")
		}
		select {
		case out <- client.EyrieStreamEvent{Type: "error", Error: fmt.Sprintf("deployment router: all deployments failed for %q: %v", target.canonicalModelID, lastErr)}:
		case <-streamCtx.Done():
		}
	}()
	return client.NewStreamResult(out, cancel), nil
}

func (r *DeploymentRouter) Stats() map[string]int64 {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()
	out := make(map[string]int64, len(r.stats))
	for id, counter := range r.stats {
		out[id] = counter.Load()
	}
	return out
}

type deploymentTarget struct {
	canonicalModelID string
	nativeHint       string
}

func (r *DeploymentRouter) resolveTarget(requested string) (deploymentTarget, error) {
	if requested == "" {
		return deploymentTarget{}, fmt.Errorf("deployment router: model is required")
	}
	requested = strings.TrimSpace(requested)
	// Prefer offerings on configured deployments over global aliases (native mimo-v2.5-pro → xiaomi token plan).
	if target, ok := r.resolveViaConfiguredDeployments(requested); ok {
		return target, nil
	}
	if canonical, ok := r.catalog.CanonicalModelForAliasOrID(requested); ok {
		return deploymentTarget{canonicalModelID: canonical}, nil
	}
	var matches []string
	for _, offering := range r.catalog.Catalog.Offerings {
		if offering.NativeModelID == requested || offering.ID == requested {
			matches = append(matches, offering.CanonicalModelID)
		}
	}
	for _, adapter := range r.deployments {
		for canonical, native := range adapter.ModelMappings {
			if native == requested {
				matches = append(matches, canonical)
			}
		}
	}
	matches = uniqueStrings(matches)
	if len(matches) == 1 {
		return deploymentTarget{canonicalModelID: matches[0]}, nil
	}
	if len(matches) > 1 {
		sort.Strings(matches)
		return deploymentTarget{}, fmt.Errorf("deployment router: model %q is ambiguous: %s", requested, strings.Join(matches, ", "))
	}
	if strings.Contains(requested, "/") {
		return deploymentTarget{canonicalModelID: requested, nativeHint: requested}, nil
	}
	return deploymentTarget{}, fmt.Errorf("deployment router: model %q is not in catalog", requested)
}

func (r *DeploymentRouter) resolveViaConfiguredDeployments(requested string) (deploymentTarget, bool) {
	var matches []string
	for depID := range r.deployments {
		for _, offering := range r.catalog.OfferingsByDeployment[depID] {
			if offering.NativeModelID == requested || offering.CanonicalModelID == requested {
				matches = append(matches, offering.CanonicalModelID)
			}
		}
	}
	matches = uniqueStrings(matches)
	if len(matches) != 1 {
		return deploymentTarget{}, false
	}
	return deploymentTarget{canonicalModelID: matches[0]}, true
}

func (r *DeploymentRouter) routeFor(canonicalModelID string) []RoutingStage {
	var stages []RoutingStage
	if explicit, ok := r.routing.Models[canonicalModelID]; ok && len(explicit) > 0 {
		stages = cloneRoutingStages(explicit)
	} else {
		providerID := ownerProviderID(canonicalModelID)
		if model := r.catalog.ModelsByID[canonicalModelID]; model.ID != "" {
			providerID = model.ProviderID
		}
		if providerID != "" {
			if explicit, ok := r.routing.Providers[providerID]; ok && len(explicit) > 0 {
				stages = cloneRoutingStages(explicit)
			} else {
				for key, explicit := range r.routing.Providers {
					if catalog.CanonicalProviderID(key) == providerID && len(explicit) > 0 {
						stages = cloneRoutingStages(explicit)
						break
					}
				}
			}
		}
		if len(stages) == 0 {
			if r.routing.Default != nil {
				stages = cloneRoutingStages(r.routing.Default)
			} else {
				return r.automaticStages(canonicalModelID)
			}
		}
	}
	return appendAutomaticFallback(stages, r.automaticStages(canonicalModelID))
}

func (r *DeploymentRouter) automaticStages(canonicalModelID string) []RoutingStage {
	var choices []DeploymentChoice
	for _, id := range sortedDeploymentIDs(r.deployments) {
		if _, _, err := r.resolveOffering(deploymentTarget{canonicalModelID: canonicalModelID}, id); err == nil {
			choices = append(choices, DeploymentChoice{DeploymentID: id, Weight: 100})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return []RoutingStage{{Deployments: choices}}
}

func (r *DeploymentRouter) eligibleChoices(target deploymentTarget, stage RoutingStage, opts client.ChatOptions) []DeploymentChoice {
	var choices []DeploymentChoice
	var toolCapable []DeploymentChoice
	requiredTools := requestedServerTools(opts.Tools)
	for _, choice := range stage.Deployments {
		if choice.DeploymentID == "" || choice.Weight <= 0 {
			continue
		}
		// Skip deployments with open circuit breakers.
		if cb := r.getCircuitBreaker(choice.DeploymentID); !cb.Allow() {
			continue
		}
		offering, _, err := r.resolveOffering(target, choice.DeploymentID)
		if err != nil {
			continue
		}
		choices = append(choices, choice)
		if len(requiredTools) == 0 || offeringSupportsTools(offering, requiredTools) {
			toolCapable = append(toolCapable, choice)
		}
	}
	if len(requiredTools) > 0 && len(toolCapable) > 0 {
		return toolCapable
	}
	return choices
}

// getCircuitBreaker returns or lazily creates a circuit breaker for a deployment.
func (r *DeploymentRouter) getCircuitBreaker(deploymentID string) *CircuitBreaker {
	r.breakersMu.RLock()
	if cb, ok := r.breakers[deploymentID]; ok {
		r.breakersMu.RUnlock()
		return cb
	}
	r.breakersMu.RUnlock()

	r.breakersMu.Lock()
	defer r.breakersMu.Unlock()
	// Double-check after acquiring write lock.
	if cb, ok := r.breakers[deploymentID]; ok {
		return cb
	}
	cb := NewCircuitBreaker(r.cbConfig.Threshold, r.cbConfig.Cooldown)
	r.breakers[deploymentID] = cb
	return cb
}

func (r *DeploymentRouter) chatWithDeployment(ctx context.Context, messages []client.EyrieMessage, opts client.ChatOptions, target deploymentTarget, deploymentID string) (*client.EyrieResponse, error) {
	offering, adapter, err := r.resolveOffering(target, deploymentID)
	if err != nil {
		return nil, err
	}
	nativeOpts := optsForOffering(opts, offering)
	return adapter.Provider.Chat(ctx, messages, nativeOpts)
}

func (r *DeploymentRouter) streamWithDeployment(ctx context.Context, out chan<- client.EyrieStreamEvent, messages []client.EyrieMessage, opts client.ChatOptions, target deploymentTarget, deploymentID string) (fallback bool, err error) {
	offering, adapter, err := r.resolveOffering(target, deploymentID)
	if err != nil {
		return true, err
	}
	nativeOpts := optsForOffering(opts, offering)
	stream, err := adapter.Provider.StreamChat(ctx, messages, nativeOpts)
	if err != nil {
		return true, err
	}
	defer stream.Close()
	emitted := false
	var buffered []client.EyrieStreamEvent
	flush := func() {
		for _, event := range buffered {
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
		buffered = nil
	}
	for event := range stream.Events {
		if event.Type == "error" {
			if emitted {
				select {
				case out <- event:
				case <-ctx.Done():
				}
				return false, fmt.Errorf("%s", event.Error)
			}
			if event.Error == "" {
				return true, fmt.Errorf("deployment %q stream failed before output", deploymentID)
			}
			return true, fmt.Errorf("%s", event.Error)
		}
		if isOutputEvent(event) {
			emitted = true
			flush()
			select {
			case out <- event:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			continue
		}
		if emitted || event.Type == "done" {
			flush()
			select {
			case out <- event:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			return false, nil
		}
		buffered = append(buffered, event)
	}
	if emitted {
		return false, fmt.Errorf("deployment %q stream ended after output without done", deploymentID)
	}
	return true, fmt.Errorf("deployment %q stream ended before output", deploymentID)
}

func (r *DeploymentRouter) resolveOffering(target deploymentTarget, deploymentID string) (catalog.ModelOffering, DeploymentAdapter, error) {
	adapter, ok := r.deployments[deploymentID]
	if !ok {
		return catalog.ModelOffering{}, DeploymentAdapter{}, fmt.Errorf("deployment %q is not configured", deploymentID)
	}
	if offering, ok := r.catalog.OfferingForDeployment(target.canonicalModelID, deploymentID); ok {
		if nativeID := adapter.ModelMappings[target.canonicalModelID]; nativeID != "" {
			offering.NativeModelID = nativeID
			offering.ID = deploymentID + ":" + nativeID
		}
		return offering, adapter, nil
	}
	for _, tmpl := range r.catalog.TemplatesByCanonicalModel[target.canonicalModelID] {
		if tmpl.DeploymentID != deploymentID {
			continue
		}
		nativeID := adapter.ModelMappings[target.canonicalModelID]
		if nativeID == "" {
			return catalog.ModelOffering{}, DeploymentAdapter{}, fmt.Errorf("deployment %q requires model mapping for %q", deploymentID, target.canonicalModelID)
		}
		return materializeTemplate(tmpl, nativeID), adapter, nil
	}
	if target.nativeHint != "" {
		deployment := r.catalog.DeploymentsByID[deploymentID]
		if deployment.ID != "" && deployment.NativeModelIDSource == catalog.NativeModelIDDiscovered {
			return catalog.ModelOffering{
				ID:               deploymentID + ":" + target.nativeHint,
				CanonicalModelID: target.canonicalModelID,
				DeploymentID:     deploymentID,
				NativeModelID:    nativeModelHintForDeployment(target.nativeHint, deployment),
				Pricing:          catalog.Pricing{Status: catalog.PricingUnknown},
			}, adapter, nil
		}
	}
	return catalog.ModelOffering{}, DeploymentAdapter{}, fmt.Errorf("deployment %q cannot serve %q", deploymentID, target.canonicalModelID)
}

func materializeTemplate(tmpl catalog.ModelOfferingTemplate, nativeID string) catalog.ModelOffering {
	return catalog.ModelOffering{
		ID:               tmpl.DeploymentID + ":" + nativeID,
		CanonicalModelID: tmpl.CanonicalModelID,
		DeploymentID:     tmpl.DeploymentID,
		NativeModelID:    nativeID,
		Capabilities:     tmpl.Capabilities,
		Pricing:          tmpl.Pricing,
		Provenance:       tmpl.Provenance,
	}
}

func optsForOffering(opts client.ChatOptions, offering catalog.ModelOffering) client.ChatOptions {
	copied := opts
	copied.Model = offering.NativeModelID
	copied.Provider = offering.DeploymentID
	if len(copied.Tools) > 0 {
		copied.Tools = filterTools(copied.Tools, offering)
	}
	return copied
}

func filterTools(tools []client.EyrieTool, offering catalog.ModelOffering) []client.EyrieTool {
	if len(offering.Capabilities.ServerTools) == 0 {
		return tools
	}
	filtered := make([]client.EyrieTool, 0, len(tools))
	for _, tool := range tools {
		if offering.Capabilities.ServerTools[tool.Name] == catalog.CapabilityUnsupported ||
			offering.Capabilities.ServerTools[tool.Name] == catalog.CapabilityUnknown {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

func requestedServerTools(tools []client.EyrieTool) []string {
	seen := map[string]bool{}
	var out []string
	for _, tool := range tools {
		if tool.Name == "" || seen[tool.Name] {
			continue
		}
		seen[tool.Name] = true
		out = append(out, tool.Name)
	}
	sort.Strings(out)
	return out
}

func offeringSupportsTools(offering catalog.ModelOffering, tools []string) bool {
	for _, tool := range tools {
		if offering.Capabilities.ServerTools[tool] != catalog.CapabilitySupported {
			return false
		}
	}
	return true
}

// selectDeploymentChoice picks a deployment from choices using weighted random
// selection. The deployment whose ID equals exclude is skipped when more than
// one option is available, so a retry after a failure prefers a different
// endpoint instead of hammering the same dead one.
func selectDeploymentChoice(choices []DeploymentChoice, exclude string) DeploymentChoice {
	if len(choices) == 1 {
		return choices[0]
	}
	alternatives := choices
	if exclude != "" {
		filtered := make([]DeploymentChoice, 0, len(choices))
		for _, c := range choices {
			if c.DeploymentID != exclude {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			alternatives = filtered
		}
	}
	total := 0
	for _, choice := range alternatives {
		total += choice.Weight
	}
	if total <= 0 {
		return alternatives[0]
	}
	n := rand.IntN(total) // #nosec G404 -- non-cryptographic weighted load-balancing choice, not a security decision
	for _, choice := range alternatives {
		n -= choice.Weight
		if n < 0 {
			return choice
		}
	}
	return alternatives[len(alternatives)-1]
}

func isOutputEvent(event client.EyrieStreamEvent) bool {
	return event.Content != "" || event.Thinking != "" || event.ToolCall != nil || event.Type == "content" || event.Type == "thinking" || event.Type == "tool_call"
}

func ownerProviderID(canonicalModelID string) string {
	owner, _, _ := strings.Cut(canonicalModelID, "/")
	return catalog.CanonicalProviderID(owner)
}

func nativeModelHintForDeployment(model string, deployment catalog.Deployment) string {
	if deployment.ID == "openrouter" {
		if native, ok := strings.CutPrefix(model, "openrouter/"); ok {
			return native
		}
		return model
	}
	if native, ok := strings.CutPrefix(model, deployment.ProviderID+"/"); ok {
		return native
	}
	return model
}

func sortedDeploymentIDs(deployments map[string]DeploymentAdapter) []string {
	ids := make([]string, 0, len(deployments))
	for id := range deployments {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func appendAutomaticFallback(stages, automatic []RoutingStage) []RoutingStage {
	if len(automatic) == 0 {
		return stages
	}
	tried := map[string]bool{}
	for _, stage := range stages {
		for _, choice := range stage.Deployments {
			tried[choice.DeploymentID] = true
		}
	}
	var remaining []DeploymentChoice
	for _, stage := range automatic {
		for _, choice := range stage.Deployments {
			if choice.DeploymentID == "" || choice.Weight <= 0 || tried[choice.DeploymentID] {
				continue
			}
			remaining = append(remaining, choice)
			tried[choice.DeploymentID] = true
		}
	}
	if len(remaining) == 0 {
		return stages
	}
	return append(stages, RoutingStage{Deployments: remaining, Retries: 1})
}

func cloneRoutingPolicy(policy RoutingPolicy) RoutingPolicy {
	return RoutingPolicy{
		Default:   cloneRoutingStages(policy.Default),
		Providers: cloneRoutingStageMap(policy.Providers),
		Models:    cloneRoutingStageMap(policy.Models),
	}
}

func cloneRoutingStageMap(in map[string][]RoutingStage) map[string][]RoutingStage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]RoutingStage, len(in))
	for key, stages := range in {
		out[key] = cloneRoutingStages(stages)
	}
	return out
}

func cloneRoutingStages(stages []RoutingStage) []RoutingStage {
	if stages == nil {
		return nil
	}
	out := make([]RoutingStage, len(stages))
	for i, stage := range stages {
		out[i].Retries = stage.Retries
		out[i].Deployments = append([]DeploymentChoice(nil), stage.Deployments...)
	}
	return out
}

func cloneStrings(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (r *DeploymentRouter) recordSuccess(deploymentID string) {
	r.statsMu.RLock()
	counter := r.stats[deploymentID]
	r.statsMu.RUnlock()
	if counter != nil {
		counter.Add(1)
	}
	if cb := r.getCircuitBreaker(deploymentID); cb != nil {
		cb.Success()
	}
}

// recordFailure records a deployment failure on its circuit breaker.
func (r *DeploymentRouter) recordFailure(deploymentID string) {
	if cb := r.getCircuitBreaker(deploymentID); cb != nil {
		cb.Failure()
	}
}
