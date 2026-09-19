package router

import (
	"context"
	"testing"

	"github.com/GrayCodeAI/flux/catalog"
	"github.com/GrayCodeAI/flux/provider/core"
)

type liveReplyProvider struct {
	reply   string
	started chan struct{}
	release chan struct{}
}

func (p *liveReplyProvider) Name() string               { return p.reply }
func (p *liveReplyProvider) Ping(context.Context) error { return nil }
func (p *liveReplyProvider) Chat(ctx context.Context, _ []core.FluxMessage, _ core.ChatOptions) (*core.FluxResponse, error) {
	if p.started != nil {
		close(p.started)
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &core.FluxResponse{Content: p.reply}, nil
}
func (p *liveReplyProvider) StreamChat(context.Context, []core.FluxMessage, core.ChatOptions) (*core.StreamResult, error) {
	return nil, nil
}

func liveOptions(compiled *catalog.CompiledCatalog, p core.Provider) DeploymentRouterOptions {
	return DeploymentRouterOptions{
		Catalog: compiled,
		Deployments: map[string]DeploymentAdapter{
			"anthropic-direct": {Provider: p},
		},
	}
}

func TestLiveDeploymentRouterAtomicReplace(t *testing.T) {
	compiled := testCompiledCatalog(t)
	old := &liveReplyProvider{reply: "old", started: make(chan struct{}), release: make(chan struct{})}
	live, err := NewLiveDeploymentRouter(liveOptions(compiled, old))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	messages := []core.FluxMessage{{Role: "user", Content: "hello"}}
	opts := core.ChatOptions{Model: "anthropic/claude-sonnet-4-6"}
	oldResult := make(chan *core.FluxResponse, 1)
	go func() {
		resp, _ := live.Chat(ctx, messages, opts)
		oldResult <- resp
	}()
	<-old.started
	newProvider := &liveReplyProvider{reply: "new"}
	if err := live.Replace(2, liveOptions(compiled, newProvider)); err != nil {
		t.Fatal(err)
	}
	resp, err := live.Chat(ctx, messages, opts)
	if err != nil || resp.Content != "new" {
		t.Fatalf("new request = %v, %v; want new deployment", resp, err)
	}
	close(old.release)
	if resp := <-oldResult; resp == nil || resp.Content != "old" {
		t.Fatalf("in-flight request = %v; want old deployment", resp)
	}
	if live.Revision() != 2 {
		t.Fatalf("revision = %d, want 2", live.Revision())
	}
	if err := live.Replace(2, liveOptions(compiled, old)); err == nil {
		t.Fatal("equal revision must be rejected")
	}
	if err := live.Replace(3, DeploymentRouterOptions{}); err == nil {
		t.Fatal("invalid configuration must be rejected")
	}
	if live.Revision() != 2 {
		t.Fatal("rejected update changed active revision")
	}
}
