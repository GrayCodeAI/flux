package client

import (
	"context"
	"errors"
	"fmt"
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
		return nil, errors.New("eyrie: NewUsageLimitProvider inner provider must not be nil")
	}
	if tracker == nil {
		return nil, errors.New("eyrie: NewUsageLimitProvider tracker must not be nil")
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
func (u *UsageLimitProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	if ok, reason := u.tracker.CanProceed(); !ok {
		return nil, fmt.Errorf("eyrie: usage limit exceeded: %s", reason)
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
func (u *UsageLimitProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	if ok, reason := u.tracker.CanProceed(); !ok {
		return nil, fmt.Errorf("eyrie: usage limit exceeded: %s", reason)
	}

	result, err := u.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	// Wrap the events channel to intercept usage events.
	wrappedCh := make(chan EyrieStreamEvent, cap(result.Events))
	go func() {
		defer close(wrappedCh)
		for evt := range result.Events {
			if evt.Type == "usage" && evt.Usage != nil {
				total := evt.Usage.TotalTokens
				if total == 0 {
					total = evt.Usage.PromptTokens + evt.Usage.CompletionTokens
				}
				u.tracker.Record(total, 0, opts.Provider, opts.Model)
			}
			select {
			case wrappedCh <- evt:
			case <-ctx.Done():
				result.Close()
				return
			}
		}
	}()

	return NewStreamResult(wrappedCh, result.Close), nil
}

// recordUsage extracts token count from an EyrieResponse and records it.
func (u *UsageLimitProvider) recordUsage(usage *EyrieUsage, opts ChatOptions) {
	if usage == nil {
		return
	}
	total := usage.TotalTokens
	if total == 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	u.tracker.Record(total, 0, opts.Provider, opts.Model)
}
