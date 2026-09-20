package provider

import (
	"context"
	"fmt"

	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/provider/resilience"
)

// Chat sends a chat request to the specified (or default) provider.
func (c *FluxClient) Chat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.FluxResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("flux: messages must not be empty")
	}
	provider := opts.Provider
	if provider == "" {
		provider = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return nil, err
	}
	if opts.Model == "" {
		opts.Model = ResolveDefaultModel(provider)
	}

	// Use coalescing if enabled
	if c.coalescer != nil {
		key := resilience.CoalesceKey{
			Provider:    provider,
			Model:       opts.Model,
			Messages:    messages,
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
		}
		return c.coalescer.Coalesce(ctx, key, func() (*core.FluxResponse, error) {
			return p.Chat(ctx, messages, opts)
		})
	}

	return p.Chat(ctx, messages, opts)
}

// StreamChat sends a streaming chat request.
func (c *FluxClient) StreamChat(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("flux: messages must not be empty")
	}
	provider := opts.Provider
	if provider == "" {
		provider = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return nil, err
	}
	if opts.Model == "" {
		opts.Model = ResolveDefaultModel(provider)
	}
	return p.StreamChat(ctx, messages, opts)
}

// StreamChatContinue is like StreamChat but automatically continues if the response
// hits max_tokens with text-only content. Continuations are transparent to the caller.
func (c *FluxClient) StreamChatContinue(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions, cfg core.ContinuationConfig) (*core.StreamResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("flux: messages must not be empty")
	}
	provider := opts.Provider
	if provider == "" {
		provider = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return nil, err
	}
	if opts.Model == "" {
		opts.Model = ResolveDefaultModel(provider)
	}
	return resilience.StreamChatWithContinuation(ctx, p, messages, opts, cfg)
}
