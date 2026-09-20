package provider

import (
	"context"

	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/provider/media"
)

// ChatWithStructuredOutput validates structured JSON responses while retaining
// FluxClient's provider resolution and default-model behavior. The schema,
// validation, prompting, and retry implementation lives in provider/media.
func (c *FluxClient) ChatWithStructuredOutput(ctx context.Context, messages []core.FluxMessage, opts core.ChatOptions, validation core.SchemaValidation) (*core.FluxResponse, error) {
	providerName := opts.Provider
	if providerName == "" {
		providerName = c.defaultProvider
	}
	p, err := c.getOrCreateProvider(providerName)
	if err != nil {
		return nil, err
	}
	if opts.Model == "" {
		opts.Model = ResolveDefaultModel(providerName)
	}
	return media.ChatWithStructuredOutput(ctx, p, providerName, messages, opts, validation)
}
