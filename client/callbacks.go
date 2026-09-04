package client

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ProviderCallback defines hooks that are invoked at various points during
// provider request lifecycle. All methods are optional — implement only the
// ones you need. Implementations MUST be safe for concurrent use.
type ProviderCallback interface {
	// OnRequest is called before each Chat or StreamChat request.
	// The messages and opts parameters must not be modified.
	OnRequest(ctx context.Context, provider string, model string, messages []GraycodeRouterMessage, opts ChatOptions)

	// OnResponse is called after a successful Chat request.
	OnResponse(ctx context.Context, provider string, model string, response *GraycodeRouterResponse, duration time.Duration)

	// OnError is called after a Chat or StreamChat request fails.
	OnError(ctx context.Context, provider string, model string, err error, duration time.Duration)

	// OnStreamEvent is called for each event emitted during streaming.
	OnStreamEvent(ctx context.Context, provider string, model string, event GraycodeRouterStreamEvent)
}

// CallbackProvider wraps any Provider and invokes registered ProviderCallback
// hooks at the appropriate points in the request lifecycle.
//
// Callbacks are executed in separate goroutines so they never block the main
// request path. A panic in a callback is recovered and logged; it does not
// crash the caller.
//
// CallbackProvider is safe for concurrent use.
type CallbackProvider struct {
	inner     Provider
	mu        sync.RWMutex
	callbacks []ProviderCallback
	logger    *slog.Logger
}

// Compile-time check that CallbackProvider implements Provider.
var _ Provider = (*CallbackProvider)(nil)

// NewCallbackProvider wraps the given provider with callback support.
// The inner provider must not be nil; an error is returned otherwise.
func NewCallbackProvider(inner Provider) (*CallbackProvider, error) {
	if inner == nil {
		return nil, errors.New("graycode-router: NewCallbackProvider inner provider must not be nil")
	}
	return &CallbackProvider{
		inner:  inner,
		logger: slog.Default(),
	}, nil
}

// SetLogger sets the logger used for panic-recovery messages.
func (cp *CallbackProvider) SetLogger(l *slog.Logger) {
	if l == nil {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.logger = l
}

// AddCallback registers a callback. It is safe to call from any goroutine.
func (cp *CallbackProvider) AddCallback(cb ProviderCallback) {
	if cb == nil {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.callbacks = append(cp.callbacks, cb)
}

// RemoveCallback removes a previously registered callback by identity (pointer
// comparison). Returns true if the callback was found and removed.
func (cp *CallbackProvider) RemoveCallback(cb ProviderCallback) bool {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	for i, existing := range cp.callbacks {
		if existing == cb {
			cp.callbacks = append(cp.callbacks[:i], cp.callbacks[i+1:]...)
			return true
		}
	}
	return false
}

// Callbacks returns a snapshot of the currently registered callbacks.
func (cp *CallbackProvider) Callbacks() []ProviderCallback {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	out := make([]ProviderCallback, len(cp.callbacks))
	copy(out, cp.callbacks)
	return out
}

// Name returns the inner provider name suffixed with "/callbacks".
func (cp *CallbackProvider) Name() string {
	return cp.inner.Name() + "/callbacks"
}

// Inner returns the wrapped provider.
func (cp *CallbackProvider) Inner() Provider {
	return cp.inner
}

// Ping delegates to the inner provider.
func (cp *CallbackProvider) Ping(ctx context.Context) error {
	return cp.inner.Ping(ctx)
}

// Chat sends a non-streaming chat request. OnRequest is called before the
// request; OnResponse or OnError is called after.
func (cp *CallbackProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	model := opts.Model
	provider := cp.inner.Name()

	cp.fireOnRequest(ctx, provider, model, messages, opts)

	start := time.Now()
	resp, err := cp.inner.Chat(ctx, messages, opts)
	duration := time.Since(start)

	if err != nil {
		cp.fireOnError(ctx, provider, model, err, duration)
		return nil, err
	}

	cp.fireOnResponse(ctx, provider, model, resp, duration)
	return resp, nil
}

// StreamChat sends a streaming chat request. OnRequest is called before the
// request. OnError is called if the initial request fails. OnStreamEvent is
// called for each event in the resulting stream.
func (cp *CallbackProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	model := opts.Model
	provider := cp.inner.Name()

	cp.fireOnRequest(ctx, provider, model, messages, opts)

	start := time.Now()
	result, err := cp.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		cp.fireOnError(ctx, provider, model, err, time.Since(start))
		return nil, err
	}

	// Wrap the events channel to invoke OnStreamEvent for each event.
	cbs := cp.snapshotCallbacks()
	origEvents := result.Events
	wrappedEvents := make(chan GraycodeRouterStreamEvent, cap(origEvents))

	go func() {
		defer close(wrappedEvents)
		for evt := range origEvents {
			// Fire stream event callbacks.
			for _, cb := range cbs {
				cp.safeCall("OnStreamEvent", func() {
					cb.OnStreamEvent(ctx, provider, model, evt)
				})
			}
			select {
			case wrappedEvents <- evt:
			case <-ctx.Done():
				result.Close()
				return
			}
		}
	}()

	return NewStreamResultWithRequestID(wrappedEvents, result.RequestID, result.Close), nil
}

// --- internal helpers ---

// snapshotCallbacks returns a copy of the current callback slice.
func (cp *CallbackProvider) snapshotCallbacks() []ProviderCallback {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	if len(cp.callbacks) == 0 {
		return nil
	}
	out := make([]ProviderCallback, len(cp.callbacks))
	copy(out, cp.callbacks)
	return out
}

// fireOnRequest invokes OnRequest on all callbacks in goroutines.
func (cp *CallbackProvider) fireOnRequest(ctx context.Context, provider, model string, messages []GraycodeRouterMessage, opts ChatOptions) {
	for _, cb := range cp.snapshotCallbacks() {
		cp.safeCall("OnRequest", func() {
			cb.OnRequest(ctx, provider, model, messages, opts)
		})
	}
}

// fireOnResponse invokes OnResponse on all callbacks in goroutines.
func (cp *CallbackProvider) fireOnResponse(ctx context.Context, provider, model string, response *GraycodeRouterResponse, duration time.Duration) {
	for _, cb := range cp.snapshotCallbacks() {
		cp.safeCall("OnResponse", func() {
			cb.OnResponse(ctx, provider, model, response, duration)
		})
	}
}

// fireOnError invokes OnError on all callbacks in goroutines.
func (cp *CallbackProvider) fireOnError(ctx context.Context, provider, model string, err error, duration time.Duration) {
	for _, cb := range cp.snapshotCallbacks() {
		cp.safeCall("OnError", func() {
			cb.OnError(ctx, provider, model, err, duration)
		})
	}
}

// safeCall runs fn in a new goroutine with panic recovery.
func (cp *CallbackProvider) safeCall(method string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				cp.mu.RLock()
				logger := cp.logger
				cp.mu.RUnlock()
				if logger != nil {
					logger.Error(
						"graycode-router: callback panic recovered",
						"method", method,
						"provider", cp.inner.Name(),
						"panic", r,
					)
				}
			}
		}()
		fn()
	}()
}
