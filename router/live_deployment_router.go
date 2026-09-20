package router

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/GrayCodeAI/flux/provider/core"
)

// LiveDeploymentRouter routes each new call through one immutable, validated
// deployment configuration. Replace installs a complete new configuration in
// one step; calls already in progress finish on the configuration they loaded.
// Each instance owns its state and can be updated by any control-plane source.
type LiveDeploymentRouter struct {
	updateMu sync.Mutex
	current  atomic.Pointer[deploymentSnapshot]
}

type deploymentSnapshot struct {
	revision uint64
	router   *DeploymentRouter
}

var _ core.Provider = (*LiveDeploymentRouter)(nil)

// NewLiveDeploymentRouter creates a router at revision 1. The catalog passed
// in opts must be treated as immutable after construction; publish a newly
// compiled catalog with Replace when catalog contents change.
func NewLiveDeploymentRouter(opts DeploymentRouterOptions) (*LiveDeploymentRouter, error) {
	r, err := NewDeploymentRouter(opts)
	if err != nil {
		return nil, err
	}
	live := &LiveDeploymentRouter{}
	live.current.Store(&deploymentSnapshot{revision: 1, router: r})
	return live, nil
}

// Replace validates and atomically publishes a new routing configuration.
// Revisions must increase so delayed control-plane updates cannot roll back
// newer state. A rejected update leaves the active configuration untouched.
func (l *LiveDeploymentRouter) Replace(revision uint64, opts DeploymentRouterOptions) error {
	if l == nil || revision == 0 {
		return fmt.Errorf("live deployment router: positive revision and initialized router required")
	}
	r, err := NewDeploymentRouter(opts)
	if err != nil {
		return err
	}
	l.updateMu.Lock()
	defer l.updateMu.Unlock()
	current := l.current.Load()
	if current == nil || revision <= current.revision {
		return fmt.Errorf("live deployment router: revision %d is not newer than active revision", revision)
	}
	l.current.Store(&deploymentSnapshot{revision: revision, router: r})
	return nil
}

// Revision returns the active configuration revision.
func (l *LiveDeploymentRouter) Revision() uint64 {
	if l == nil {
		return 0
	}
	if current := l.current.Load(); current != nil {
		return current.revision
	}
	return 0
}

func (l *LiveDeploymentRouter) Name() string { return "live-deployment-router" }

func (l *LiveDeploymentRouter) Ping(ctx context.Context) error {
	r, err := l.active()
	if err != nil {
		return err
	}
	return r.Ping(ctx)
}

func (l *LiveDeploymentRouter) Chat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.FluxResponse, error) {
	r, err := l.active()
	if err != nil {
		return nil, err
	}
	return r.Chat(ctx, messages, opts)
}

func (l *LiveDeploymentRouter) StreamChat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	r, err := l.active()
	if err != nil {
		return nil, err
	}
	return r.StreamChat(ctx, messages, opts)
}

func (l *LiveDeploymentRouter) active() (*DeploymentRouter, error) {
	if l == nil {
		return nil, fmt.Errorf("live deployment router: not initialized")
	}
	current := l.current.Load()
	if current == nil {
		return nil, fmt.Errorf("live deployment router: not initialized")
	}
	return current.router, nil
}
