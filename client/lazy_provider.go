package client

import "context"

// LazyProvider adapts EyrieClient to the Provider interface without eagerly
// resolving credentials or constructing a concrete provider.
type LazyProvider struct {
	client   *EyrieClient
	provider string
}

// NewLazyProvider creates a provider wrapper that resolves the concrete
// provider only when chat or ping operations are invoked.
func NewLazyProvider(cfg *EyrieConfig) *LazyProvider {
	c := Client(cfg)
	provider := c.defaultProvider
	if cfg != nil && cfg.Provider != "" {
		provider = cfg.Provider
	}
	return &LazyProvider{
		client:   c,
		provider: provider,
	}
}

func (p *LazyProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	if opts.Provider == "" {
		opts.Provider = p.provider
	}
	return p.client.Chat(ctx, messages, opts)
}

func (p *LazyProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	if opts.Provider == "" {
		opts.Provider = p.provider
	}
	return p.client.StreamChat(ctx, messages, opts)
}

func (p *LazyProvider) Ping(ctx context.Context) error {
	return p.client.Ping(ctx, p.provider)
}

func (p *LazyProvider) Name() string {
	return p.provider
}

func (p *LazyProvider) SetAPIKey(provider, apiKey string) {
	p.client.SetAPIKey(provider, apiKey)
}
