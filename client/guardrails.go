package client

import (
	"context"
	"log/slog"
)

// ---------------------------------------------------------------------------
// GuardrailProvider: standalone Provider wrapper
// ---------------------------------------------------------------------------

// GuardrailProvider wraps any Provider and runs guardrail checks on LLM
// responses before returning them to the caller. When a guardrail with
// Action=Block matches, the response is replaced with an error. When
// Action=Redact, matched content is scrubbed. When Action=Warn, the
// violation is logged but the response passes through unchanged.
//
// GuardrailProvider is safe for concurrent use.
type GuardrailProvider struct {
	inner      Provider
	guardrails *Guardrails
}

// Compile-time check that GuardrailProvider implements Provider.
var _ Provider = (*GuardrailProvider)(nil)

// NewGuardrailProvider wraps the given provider with output guardrails.
// The inner provider must not be nil. The guardrails parameter may be nil
// (in which case the wrapper is a no-op).
func NewGuardrailProvider(inner Provider, g *Guardrails) *GuardrailProvider {
	if inner == nil {
		slog.Error("NewGuardrailProvider inner provider must not be nil; returning nil")
		return nil
	}
	return &GuardrailProvider{
		inner:      inner,
		guardrails: g,
	}
}

// Name returns the inner provider name suffixed with "/guardrails".
func (gp *GuardrailProvider) Name() string {
	return gp.inner.Name() + "/guardrails"
}

// Ping delegates to the inner provider.
func (gp *GuardrailProvider) Ping(ctx context.Context) error {
	return gp.inner.Ping(ctx)
}

// Inner returns the wrapped provider.
func (gp *GuardrailProvider) Inner() Provider {
	return gp.inner
}

// Chat sends a chat request and validates the response against guardrails.
func (gp *GuardrailProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	resp, err := gp.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	if err := applyGuardrails(ctx, resp, gp.guardrails); err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamChat sends a streaming request and validates content events.
// Blocked violations cause the stream to be cancelled and an error event emitted.
// Redactions are applied to individual content chunks.
func (gp *GuardrailProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	result, err := gp.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	if gp.guardrails == nil {
		return result, nil
	}

	origEvents := result.Events
	wrappedEvents := make(chan GraycodeRouterStreamEvent, cap(origEvents))

	go func() {
		defer close(wrappedEvents)
		for evt := range origEvents {
			if evt.Type == "content" && gp.guardrails != nil {
				violations, checkErr := gp.guardrails.Check(ctx, evt.Content)
				if checkErr != nil {
					select {
					case wrappedEvents <- GraycodeRouterStreamEvent{
						Type:  "error",
						Error: checkErr.Error(),
					}:
					case <-ctx.Done():
					}
					result.Close()
					return
				}
				if len(violations) > 0 {
					evt.Content = ApplyRedactions(evt.Content, violations)
				}
			}
			select {
			case wrappedEvents <- evt:
			case <-ctx.Done():
				result.Close()
				return
			}
		}
	}()

	return &StreamResult{
		Events:    wrappedEvents,
		RequestID: result.RequestID,
	}, nil
}
