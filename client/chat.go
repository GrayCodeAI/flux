package client

import (
	"context"
	"fmt"
)

// Chat sends a chat request to the specified (or default) provider.
func (c *GraycodeRouterClient) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("graycode-router: messages must not be empty")
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
		key := CoalesceKey{
			Provider:    provider,
			Model:       opts.Model,
			Messages:    messages,
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
		}
		return c.coalescer.Coalesce(ctx, key, func() (*GraycodeRouterResponse, error) {
			return p.Chat(ctx, messages, opts)
		})
	}

	return p.Chat(ctx, messages, opts)
}

// StreamChat sends a streaming chat request.
func (c *GraycodeRouterClient) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("graycode-router: messages must not be empty")
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
func (c *GraycodeRouterClient) StreamChatContinue(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions, cfg ContinuationConfig) (*StreamResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("graycode-router: messages must not be empty")
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
	return StreamChatWithContinuation(ctx, p, messages, opts, cfg)
}
