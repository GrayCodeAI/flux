package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrBudgetExceeded is returned when a virtual key has exhausted its budget.
var ErrBudgetExceeded = errors.New("graycode-router: virtual key budget exceeded")

// ErrUnknownVirtualKey is returned when a request references a virtual key that
// the store does not recognize.
var ErrUnknownVirtualKey = errors.New("graycode-router: unknown virtual key")

// virtualKeyCtxKey is the context key under which a virtual key id is carried.
type virtualKeyCtxKey struct{}

// WithVirtualKey returns a context carrying the given virtual key id. The
// BudgetProvider reads this (falling back to ChatOptions.VirtualKeyID) to
// attribute and enforce spend.
func WithVirtualKey(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, virtualKeyCtxKey{}, id)
}

// VirtualKeyFromContext extracts a virtual key id from the context, if present.
func VirtualKeyFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(virtualKeyCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// BudgetStore is the persistence contract the BudgetProvider depends on. Its
// methods use only primitive types so both the in-memory store here and the
// SQLite store in the storage package satisfy it without an import cycle.
type BudgetStore interface {
	// CheckBudget returns ErrBudgetExceeded if charging estCostUSD to the key
	// would exceed its budget, ErrUnknownVirtualKey if the key is unknown, or
	// nil if the request may proceed.
	CheckBudget(ctx context.Context, virtualKey string, estCostUSD float64) error
	// RecordUsage records actual spend against a virtual key after a call.
	RecordUsage(ctx context.Context, virtualKey string, costUSD float64, tokensIn, tokensOut int) error
}

// BudgetProvider wraps a Provider and enforces per-virtual-key budgets. Before
// each call it estimates cost and rejects the request if the key is over
// budget; after a successful call it records the actual spend.
//
// Requests without a virtual key (none in context or options) pass through
// unmetered, preserving existing behavior.
type BudgetProvider struct {
	inner     Provider
	store     BudgetStore
	estimator *CostEstimator
}

// Compile-time check that BudgetProvider implements Provider.
var _ Provider = (*BudgetProvider)(nil)

// NewBudgetProvider wraps inner with budget enforcement backed by store.
func NewBudgetProvider(inner Provider, store BudgetStore) *BudgetProvider {
	return &BudgetProvider{inner: inner, store: store, estimator: NewCostEstimator()}
}

// Name returns the inner provider's name.
func (bp *BudgetProvider) Name() string { return bp.inner.Name() }

// Ping delegates to the inner provider.
func (bp *BudgetProvider) Ping(ctx context.Context) error { return bp.inner.Ping(ctx) }

// Chat enforces the budget for the request's virtual key, calls the inner
// provider, then records actual spend.
func (bp *BudgetProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	vk := resolveVirtualKey(ctx, opts)
	if vk == "" || bp.store == nil {
		return bp.inner.Chat(ctx, messages, opts)
	}

	est := bp.estimator.Estimate(messages, opts.Model, opts.MaxTokens)
	if err := bp.store.CheckBudget(ctx, vk, est.TotalCostUSD); err != nil {
		return nil, err
	}

	resp, err := bp.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	bp.recordUsage(ctx, vk, opts.Model, resp)
	return resp, nil
}

// StreamChat enforces budget up-front, then streams. Actual streamed usage is
// recorded from the final usage event if present.
func (bp *BudgetProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	vk := resolveVirtualKey(ctx, opts)
	if vk == "" || bp.store == nil {
		return bp.inner.StreamChat(ctx, messages, opts)
	}

	est := bp.estimator.Estimate(messages, opts.Model, opts.MaxTokens)
	if err := bp.store.CheckBudget(ctx, vk, est.TotalCostUSD); err != nil {
		return nil, err
	}

	result, err := bp.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	// Wrap the events channel to record actual spend from the final usage
	// event. Without this, streamed calls under a virtual key never debit the
	// budget (unlike the non-streaming Chat path), so streaming-heavy clients
	// would underreport spend. Mirrors UsageLimitProvider.StreamChat.
	wrappedCh := make(chan GraycodeRouterStreamEvent, cap(result.Events))
	go func() {
		defer close(wrappedCh)
		for evt := range result.Events {
			if evt.Type == "usage" && evt.Usage != nil {
				cost := ActualCostUSD(opts.Model, evt.Usage)
				_ = bp.store.RecordUsage(ctx, vk, cost, evt.Usage.PromptTokens, evt.Usage.CompletionTokens)
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

func (bp *BudgetProvider) recordUsage(ctx context.Context, vk, model string, resp *GraycodeRouterResponse) {
	if resp == nil || resp.Usage == nil {
		return
	}
	cost := ActualCostUSD(model, resp.Usage)
	_ = bp.store.RecordUsage(ctx, vk, cost, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}

// resolveVirtualKey prefers the explicit option, then the context value.
func resolveVirtualKey(ctx context.Context, opts ChatOptions) string {
	if opts.VirtualKeyID != "" {
		return opts.VirtualKeyID
	}
	return VirtualKeyFromContext(ctx)
}

// ActualCostUSD computes the realized USD cost of a completed call from token
// usage using the same per-model pricing as the cost estimator.
func ActualCostUSD(model string, usage *GraycodeRouterUsage) float64 {
	if usage == nil {
		return 0
	}
	inPrice := pricePerToken(model, true)
	outPrice := pricePerToken(model, false)
	regularIn := usage.PromptTokens - usage.CacheReadTokens
	if regularIn < 0 {
		regularIn = 0
	}
	// Cached input tokens are billed at ~10% per existing streaming logic.
	return float64(regularIn)*inPrice +
		float64(usage.CacheReadTokens)*inPrice*0.1 +
		float64(usage.CompletionTokens)*outPrice
}

// ---------- in-memory store ----------

type memBudget struct {
	limitUSD  float64
	usedUSD   float64
	tokensIn  int
	tokensOut int
}

// MemoryBudgetStore is an in-memory BudgetStore suitable for tests and
// single-process use. It is safe for concurrent use.
type MemoryBudgetStore struct {
	mu      sync.Mutex
	budgets map[string]*memBudget
}

// Compile-time check.
var _ BudgetStore = (*MemoryBudgetStore)(nil)

// NewMemoryBudgetStore creates an empty in-memory budget store.
func NewMemoryBudgetStore() *MemoryBudgetStore {
	return &MemoryBudgetStore{budgets: make(map[string]*memBudget)}
}

// SetBudget creates or updates a virtual key with the given USD limit. A
// non-positive limit means unlimited.
func (s *MemoryBudgetStore) SetBudget(virtualKey string, limitUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[virtualKey]
	if !ok {
		b = &memBudget{}
		s.budgets[virtualKey] = b
	}
	b.limitUSD = limitUSD
}

// CheckBudget implements BudgetStore.
func (s *MemoryBudgetStore) CheckBudget(_ context.Context, virtualKey string, estCostUSD float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[virtualKey]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownVirtualKey, virtualKey)
	}
	if b.limitUSD <= 0 {
		return nil // unlimited
	}
	if b.usedUSD+estCostUSD > b.limitUSD {
		return fmt.Errorf("%w: %q (used $%.4f + est $%.4f > limit $%.4f)",
			ErrBudgetExceeded, virtualKey, b.usedUSD, estCostUSD, b.limitUSD)
	}
	return nil
}

// RecordUsage implements BudgetStore.
func (s *MemoryBudgetStore) RecordUsage(_ context.Context, virtualKey string, costUSD float64, tokensIn, tokensOut int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[virtualKey]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownVirtualKey, virtualKey)
	}
	b.usedUSD += costUSD
	b.tokensIn += tokensIn
	b.tokensOut += tokensOut
	return nil
}

// Usage returns the recorded spend for a virtual key.
func (s *MemoryBudgetStore) Usage(virtualKey string) (usedUSD float64, tokensIn, tokensOut int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, exists := s.budgets[virtualKey]
	if !exists {
		return 0, 0, 0, false
	}
	return b.usedUSD, b.tokensIn, b.tokensOut, true
}
