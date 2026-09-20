// Package controlplane distributes versioned routing manifests while each
// Flux instance resolves credentials and executes provider calls locally.
package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GrayCodeAI/flux/catalog"
	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/router"
)

// Deployment describes a route target without credentials. Resolver supplies
// the local provider implementation and secrets for the deployment ID.
type Deployment struct {
	ID            string            `json:"id"`
	ModelMappings map[string]string `json:"model_mappings,omitempty"`
}

// Manifest is the shared, credential-free routing state. Revisions increase
// across publishers; peers reject conflicting contents at the same revision.
type Manifest struct {
	Revision    uint64               `json:"revision"`
	Catalog     catalog.Catalog      `json:"catalog"`
	Routing     router.RoutingPolicy `json:"routing"`
	Deployments []Deployment         `json:"deployments"`
}

// Source returns a complete manifest from a shared store or peer.
type Source interface {
	Latest(context.Context) (Manifest, error)
}

// Resolver constructs an instance-local provider for a named deployment.
// The shared manifest never carries API keys or access tokens.
type Resolver func(context.Context, string) (core.Provider, error)

// Replica holds the last valid routing manifest. Each replica serves requests
// independently; source outages leave the last good route active.
type Replica struct {
	resolver Resolver
	updateMu sync.Mutex
	current  atomic.Pointer[replicaState]
}

type replicaState struct {
	manifest Manifest
	digest   [sha256.Size]byte
	router   *router.DeploymentRouter
}

var _ core.Provider = (*Replica)(nil)

func NewReplica(resolver Resolver) (*Replica, error) {
	if resolver == nil {
		return nil, fmt.Errorf("controlplane: deployment resolver is required")
	}
	return &Replica{resolver: resolver}, nil
}

// Apply validates and atomically publishes a manifest on this instance. It
// rejects stale revisions and same-revision conflicts. A failed update leaves
// the current route unchanged.
func (r *Replica) Apply(ctx context.Context, manifest Manifest) error {
	if r == nil || r.resolver == nil {
		return fmt.Errorf("controlplane: replica is not initialized")
	}
	if manifest.Revision == 0 {
		return fmt.Errorf("controlplane: revision must be positive")
	}
	// Own the input before validation: callers may mutate their maps or slices
	// immediately after Apply returns, while active requests still use them.
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("controlplane: encode manifest: %w", err)
	}
	if len(encoded) > maxManifestBytes {
		return fmt.Errorf("controlplane: manifest exceeds %d bytes", maxManifestBytes)
	}
	var owned Manifest
	if err := json.Unmarshal(encoded, &owned); err != nil {
		return fmt.Errorf("controlplane: copy manifest: %w", err)
	}
	if err := validateManifestRoutes(owned); err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	if current := r.current.Load(); current != nil {
		if manifest.Revision < current.manifest.Revision {
			return fmt.Errorf("controlplane: stale revision %d; active revision is %d", manifest.Revision, current.manifest.Revision)
		}
		if manifest.Revision == current.manifest.Revision {
			if digest == current.digest {
				return nil
			}
			return fmt.Errorf("controlplane: conflicting manifests at revision %d", manifest.Revision)
		}
	}
	// Compilation normalizes catalog data in place. Keep the published
	// manifest byte-for-byte equivalent to the revision digest and signature.
	var compileCopy Manifest
	if err := json.Unmarshal(encoded, &compileCopy); err != nil {
		return fmt.Errorf("controlplane: copy catalog for compilation: %w", err)
	}
	compiled, err := catalog.CompileCatalog(&compileCopy.Catalog)
	if err != nil {
		return fmt.Errorf("controlplane: invalid catalog: %w", err)
	}
	deployments := make(map[string]router.DeploymentAdapter, len(owned.Deployments))
	for _, deployment := range owned.Deployments {
		provider, err := r.resolver(ctx, deployment.ID)
		if err != nil {
			return fmt.Errorf("controlplane: resolve deployment %q: %w", deployment.ID, err)
		}
		if provider == nil {
			return fmt.Errorf("controlplane: deployment %q resolved to nil provider", deployment.ID)
		}
		deployments[deployment.ID] = router.DeploymentAdapter{
			DeploymentID:  deployment.ID,
			Provider:      provider,
			ModelMappings: deployment.ModelMappings,
		}
	}
	active, err := router.NewDeploymentRouter(router.DeploymentRouterOptions{
		Catalog: compiled, Deployments: deployments, Routing: owned.Routing,
	})
	if err != nil {
		return err
	}
	r.current.Store(&replicaState{manifest: owned, digest: digest, router: active})
	return nil
}

func validateManifestRoutes(manifest Manifest) error {
	configured := make(map[string]struct{}, len(manifest.Deployments))
	for _, deployment := range manifest.Deployments {
		if deployment.ID == "" {
			return fmt.Errorf("controlplane: deployment ID is required")
		}
		if _, exists := configured[deployment.ID]; exists {
			return fmt.Errorf("controlplane: duplicate deployment %q", deployment.ID)
		}
		configured[deployment.ID] = struct{}{}
		for canonical, native := range deployment.ModelMappings {
			if canonical == "" || native == "" {
				return fmt.Errorf("controlplane: deployment %q has empty model mapping", deployment.ID)
			}
		}
	}
	check := func(scope string, stages []router.RoutingStage) error {
		for i, stage := range stages {
			if stage.Retries < 0 || stage.Retries > 32 {
				return fmt.Errorf("controlplane: %s stage %d has invalid retry count", scope, i)
			}
			if len(stage.Deployments) == 0 {
				return fmt.Errorf("controlplane: %s stage %d has no deployments", scope, i)
			}
			seen := make(map[string]struct{}, len(stage.Deployments))
			for _, choice := range stage.Deployments {
				if _, ok := configured[choice.DeploymentID]; !ok {
					return fmt.Errorf("controlplane: %s stage %d references unconfigured deployment %q", scope, i, choice.DeploymentID)
				}
				if choice.Weight <= 0 || choice.Weight > 1_000_000 {
					return fmt.Errorf("controlplane: %s stage %d has weight outside 1..1000000", scope, i)
				}
				if _, duplicate := seen[choice.DeploymentID]; duplicate {
					return fmt.Errorf("controlplane: %s stage %d repeats deployment %q", scope, i, choice.DeploymentID)
				}
				seen[choice.DeploymentID] = struct{}{}
			}
		}
		return nil
	}
	if manifest.Routing.Default != nil {
		if err := check("default", manifest.Routing.Default); err != nil {
			return err
		}
	}
	for provider, stages := range manifest.Routing.Providers {
		if provider == "" {
			return fmt.Errorf("controlplane: empty provider route key")
		}
		if err := check("provider "+provider, stages); err != nil {
			return err
		}
	}
	for model, stages := range manifest.Routing.Models {
		if model == "" {
			return fmt.Errorf("controlplane: empty model route key")
		}
		if err := check("model "+model, stages); err != nil {
			return err
		}
	}
	return nil
}

// Refresh applies the newest manifest returned by source. On source failure,
// requests continue using the last valid manifest.
func (r *Replica) Refresh(ctx context.Context, source Source) error {
	if source == nil {
		return fmt.Errorf("controlplane: source is required")
	}
	manifest, err := source.Latest(ctx)
	if err != nil {
		return err
	}
	return r.Apply(ctx, manifest)
}

// Run refreshes immediately, then periodically until ctx is canceled. Bad or
// unavailable sources are reported without stopping the last-good data plane.
// The caller owns this loop and its lifetime; Run does not spawn a goroutine.
func (r *Replica) Run(ctx context.Context, source Source, interval time.Duration, report func(error)) error {
	if r == nil || r.resolver == nil || source == nil {
		return fmt.Errorf("controlplane: initialized replica and source are required")
	}
	if interval <= 0 {
		return fmt.Errorf("controlplane: refresh interval must be positive")
	}
	refresh := func() {
		if err := r.Refresh(ctx, source); err != nil && report != nil && ctx.Err() == nil {
			report(err)
		}
	}
	refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			refresh()
		}
	}
}

func (r *Replica) Revision() uint64 {
	if r == nil {
		return 0
	}
	if current := r.current.Load(); current != nil {
		return current.manifest.Revision
	}
	return 0
}

func (r *Replica) Name() string { return "deployment-replica" }

func (r *Replica) Ping(ctx context.Context) error {
	active, err := r.active()
	if err != nil {
		return err
	}
	return active.Ping(ctx)
}

func (r *Replica) Chat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.FluxResponse, error) {
	active, err := r.active()
	if err != nil {
		return nil, err
	}
	return active.Chat(ctx, messages, opts)
}

func (r *Replica) StreamChat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	active, err := r.active()
	if err != nil {
		return nil, err
	}
	return active.StreamChat(ctx, messages, opts)
}

func (r *Replica) active() (*router.DeploymentRouter, error) {
	if r != nil {
		if current := r.current.Load(); current != nil {
			return current.router, nil
		}
	}
	return nil, fmt.Errorf("controlplane: no active routing manifest")
}

// Handler serves the current manifest to peer replicas. Authentication and
// transport security belong to the host's HTTP server or reverse proxy.
func (r *Replica) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r == nil {
			http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		current := r.current.Load()
		if current == nil {
			http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(current.manifest)
	})
}

func manifestDigest(manifest Manifest) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("controlplane: encode manifest: %w", err)
	}
	return sha256.Sum256(encoded), nil
}
