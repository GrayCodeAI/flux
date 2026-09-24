package observability

import (
	"context"
	"errors"
	"fmt"

	"github.com/GrayCodeAI/flux/provider/core"
)

// UsageLimitProvider wraps any Provider and enforces token/cost budgets
// via a UsageTracker. It calls CanProceed() before each Chat/StreamChat
// request and Record() after successful responses.
//
// If the budget is exhausted, calls return a non-nil error immediately
// without contacting the upstream provider.
//
// UsageLimitProvider is safe for concurrent use (the underlying
// UsageTracker is internally synchronised).
type UsageLimitProvider struct {
	inner   Provider
	tracker *UsageTracker
}

// Compile-time check that UsageLimitProvider implements Provider.
var _ Provider = (*UsageLimitProvider)(nil)

// NewUsageLimitProvider wraps inner with budget enforcement via tracker.
// Both arguments must be non-nil; an error is returned otherwise.
func NewUsageLimitProvider(inner Provider, tracker *UsageTracker) (*UsageLimitProvider, error) {
	if inner == nil {
		return nil, errors.New("flux: NewUsageLimitProvider inner provider must not be nil")
	}
	if tracker == nil {
		return nil, errors.New("flux: NewUsageLimitProvider tracker must not be nil")
	}
	return &UsageLimitProvider{inner: inner, tracker: tracker}, nil
}

// Name returns the inner provider name suffixed with "/usage-limit".
func (u *UsageLimitProvider) Name() string {
	return u.inner.Name() + "/usage-limit"
}

// Tracker returns the underlying UsageTracker for inspection or configuration.
func (u *UsageLimitProvider) Tracker() *UsageTracker {
	return u.tracker
}

// Ping delegates directly to the inner provider (budget is not checked).
func (u *UsageLimitProvider) Ping(ctx context.Context) error {
	return u.inner.Ping(ctx)
}

// Chat sends a non-streaming chat request. The call is gated by the
// usage tracker's CanProceed() and the response tokens are recorded on
// success.
func (u *UsageLimitProvider) Chat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*FluxResponse, error) {
	if ok, reason := u.tracker.CanProceed(); !ok {
		return nil, fmt.Errorf("flux: usage limit exceeded: %s", reason)
	}

	resp, err := u.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	u.recordUsage(resp.Usage, opts)
	return resp, nil
}

// StreamChat sends a streaming chat request. The budget check happens
// before the stream starts. Usage is recorded once the stream delivers
// a "usage" event (typically the final chunk).
func (u *UsageLimitProvider) StreamChat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*StreamResult, error) {
	if ok, reason := u.tracker.CanProceed(); !ok {
		return nil, fmt.Errorf("flux: usage limit exceeded: %s", reason)
	}

	result, err := u.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	var previousUsage *core.FluxUsage
	return core.TransformStreamResult(ctx, result, func(_ context.Context, evt FluxStreamEvent) (FluxStreamEvent, error) {
		if evt.Type == "continuation" {
			previousUsage = nil
			return evt, nil
		}
		if (evt.Type == "usage" || evt.Type == "done") && evt.Usage != nil {
			delta := core.UsageDelta(previousUsage, evt.Usage)
			previousUsage = core.MergeUsage(previousUsage, evt.Usage)
			if delta != nil {
				total := delta.TotalTokens
				if total == 0 {
					total = delta.PromptTokens + delta.CompletionTokens
				}
				u.tracker.Record(total, 0, opts.Provider, opts.Model)
			}
		}
		return evt, nil
	}), nil
}

// recordUsage extracts token count from an FluxResponse and records it.
func (u *UsageLimitProvider) recordUsage(usage *FluxUsage, opts ChatOptions) {
	if usage == nil {
		return
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	u.tracker.Record(total, 0, opts.Provider, opts.Model)
}
