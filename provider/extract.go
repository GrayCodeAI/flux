package provider

import (
	"context"

	"github.com/GrayCodeAI/flux/provider/extraction"
)

// ExtractRelationships delegates relationship extraction to the extraction
// feature while retaining FluxClient's public convenience method.
func (c *FluxClient) ExtractRelationships(ctx context.Context, text string, opts extraction.ExtractOptions) ([]extraction.Relationship, error) {
	return extraction.ExtractRelationships(ctx, c, text, opts)
}

var _ extraction.StructuredChatter = (*FluxClient)(nil)
