package engine

import (
	"context"

	"github.com/GrayCodeAI/graycode-router/runtime"
)

// SupportsNativeCompaction reports whether the selection and configured
// credential store support native compaction.
func (e *Engine) SupportsNativeCompaction(ctx context.Context, provider, model string) bool {
	return runtime.SupportsNativeCompactionWithStore(nonNilContext(ctx), provider, model, e.secretStore)
}

// CompactNative performs provider-native compaction and returns a normalized
// summary. Conversation mutation remains the host's responsibility.
func (e *Engine) CompactNative(ctx context.Context, req NativeCompactionRequest) (string, error) {
	result, err := runtime.CompactNativeConversationWithStore(nonNilContext(ctx), runtime.NativeCompactionOpts{
		Provider: req.Provider, Model: req.Model, Messages: toClientMessages(req.Messages),
		ContextWindow: req.ContextWindow, ThresholdPct: req.ThresholdPct, MaxOutputTokens: req.MaxOutputTokens,
	}, e.secretStore)
	if err != nil {
		return "", classify("native_compaction", Route{Provider: req.Provider, Model: req.Model}, err)
	}
	return result.Summary, nil
}
