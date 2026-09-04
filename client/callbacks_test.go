package client

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- test helpers ---

// syncedBuffer is a thread-safe bytes.Buffer for use in tests where
// a logger writes from a goroutine while the test goroutine reads.
type syncedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *syncedBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *syncedBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// recordingCallback is a test callback that records all invocations.
type recordingCallback struct {
	mu             sync.Mutex
	requests       []requestRecord
	responses      []responseRecord
	errors         []errorRecord
	streamEvents   []streamEventRecord
	panicOnRequest bool
}

type requestRecord struct {
	provider string
	model    string
	messages []GraycodeRouterMessage
	opts     ChatOptions
}

type responseRecord struct {
	provider string
	model    string
	response *GraycodeRouterResponse
	duration time.Duration
}

type errorRecord struct {
	provider string
	model    string
	err      error
	duration time.Duration
}

type streamEventRecord struct {
	provider string
	model    string
	event    GraycodeRouterStreamEvent
}

func (r *recordingCallback) OnRequest(_ context.Context, provider, model string, messages []GraycodeRouterMessage, opts ChatOptions) {
	if r.panicOnRequest {
		panic("intentional test panic")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, requestRecord{provider, model, messages, opts})
}

func (r *recordingCallback) OnResponse(_ context.Context, provider, model string, response *GraycodeRouterResponse, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append(r.responses, responseRecord{provider, model, response, duration})
}

func (r *recordingCallback) OnError(_ context.Context, provider, model string, err error, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, errorRecord{provider, model, err, duration})
}

func (r *recordingCallback) OnStreamEvent(_ context.Context, provider, model string, event GraycodeRouterStreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streamEvents = append(r.streamEvents, streamEventRecord{provider, model, event})
}

func (r *recordingCallback) requestCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *recordingCallback) responseCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.responses)
}

func (r *recordingCallback) errorCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.errors)
}

func (r *recordingCallback) streamEventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.streamEvents)
}

func (r *recordingCallback) hasEventType(eventType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, se := range r.streamEvents {
		if se.event.Type == eventType {
			return true
		}
	}
	return false
}

// waitUntil polls cond until it returns true or timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("waitUntil: condition not met within timeout")
}

// --- tests ---

func TestCallbackProviderImplementsProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*CallbackProvider)(nil)
}

func TestCallbackProviderName(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)
	if cp.Name() != "mock/callbacks" {
		t.Errorf("Name() = %q, want %q", cp.Name(), "mock/callbacks")
	}
}

func TestCallbackProviderPing(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)
	if err := cp.Ping(context.Background()); err != nil {
		t.Errorf("Ping() = %v, want nil", err)
	}
}

func TestCallbackProviderInner(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)
	if cp.Inner() != mock {
		t.Error("Inner() should return the wrapped provider")
	}
}

func TestCallbackBasicInvocation(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	cb := &recordingCallback{}
	cp.AddCallback(cb)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	resp, err := cp.Chat(context.Background(), msgs, opts)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == nil {
		t.Fatal("Chat returned nil response")
	}

	// Wait for async callbacks to complete.
	waitUntil(t, 2*time.Second, func() bool {
		return cb.requestCount() == 1 && cb.responseCount() == 1
	})

	// Verify OnRequest.
	if len(cb.requests) != 1 {
		t.Fatalf("OnRequest called %d times, want 1", len(cb.requests))
	}
	if cb.requests[0].provider != "mock" {
		t.Errorf("OnRequest provider = %q, want %q", cb.requests[0].provider, "mock")
	}
	if cb.requests[0].model != "test-model" {
		t.Errorf("OnRequest model = %q, want %q", cb.requests[0].model, "test-model")
	}
	if len(cb.requests[0].messages) != 1 || cb.requests[0].messages[0].Content != "hello" {
		t.Errorf("OnRequest messages not passed correctly")
	}

	// Verify OnResponse.
	if len(cb.responses) != 1 {
		t.Fatalf("OnResponse called %d times, want 1", len(cb.responses))
	}
	if cb.responses[0].provider != "mock" {
		t.Errorf("OnResponse provider = %q, want %q", cb.responses[0].provider, "mock")
	}
	if cb.responses[0].duration <= 0 {
		t.Error("OnResponse duration should be positive")
	}

	// OnError should not have been called.
	if cb.errorCount() != 0 {
		t.Errorf("OnError called %d times, want 0", cb.errorCount())
	}
}

func TestCallbackErrorInvocation(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeError)
	cp := mustCallbackProvider(t, mock)

	cb := &recordingCallback{}
	cp.AddCallback(cb)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	_, err := cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})
	if err == nil {
		t.Fatal("expected error from Chat")
	}

	// Wait for async callbacks.
	waitUntil(t, 2*time.Second, func() bool {
		return cb.requestCount() == 1 && cb.errorCount() == 1
	})

	// OnRequest should have been called.
	if cb.requestCount() != 1 {
		t.Errorf("OnRequest called %d times, want 1", cb.requestCount())
	}

	// OnError should have been called with the error.
	if len(cb.errors) != 1 {
		t.Fatalf("OnError called %d times, want 1", len(cb.errors))
	}
	if cb.errors[0].err == nil || cb.errors[0].err.Error() != "graycode-router: mock error" {
		t.Errorf("OnError err = %v, want %q", cb.errors[0].err, "graycode-router: mock error")
	}

	// OnResponse should NOT have been called.
	if cb.responseCount() != 0 {
		t.Errorf("OnResponse called %d times, want 0", cb.responseCount())
	}
}

func TestCallbackMultipleCallbacks(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	cb1 := &recordingCallback{}
	cb2 := &recordingCallback{}
	cp.AddCallback(cb1)
	cp.AddCallback(cb2)

	if len(cp.Callbacks()) != 2 {
		t.Fatalf("Callbacks() returned %d, want 2", len(cp.Callbacks()))
	}

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	_, err := cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return cb1.requestCount() == 1 && cb2.requestCount() == 1 &&
			cb1.responseCount() == 1 && cb2.responseCount() == 1
	})

	if cb1.responseCount() != 1 {
		t.Error("cb1 OnResponse not called")
	}
	if cb2.responseCount() != 1 {
		t.Error("cb2 OnResponse not called")
	}
}

func TestCallbackRemoveCallback(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	cb1 := &recordingCallback{}
	cb2 := &recordingCallback{}
	cp.AddCallback(cb1)
	cp.AddCallback(cb2)

	if !cp.RemoveCallback(cb1) {
		t.Fatal("RemoveCallback should return true")
	}
	if len(cp.Callbacks()) != 1 {
		t.Fatalf("Callbacks() = %d, want 1", len(cp.Callbacks()))
	}

	// Removing again should return false.
	if cp.RemoveCallback(cb1) {
		t.Fatal("RemoveCallback should return false for already-removed callback")
	}

	// Only cb2 should fire.
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})

	waitUntil(t, 2*time.Second, func() bool {
		return cb2.responseCount() == 1
	})

	if cb1.requestCount() != 0 {
		t.Error("removed callback should not be called")
	}
}

func TestCallbackPanicRecovery(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)

	// Use a thread-safe buffer since the slog handler writes from a goroutine.
	var logBuf syncedBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError}))

	cp := mustCallbackProvider(t, mock)
	cp.SetLogger(logger)

	panicCb := &recordingCallback{panicOnRequest: true}
	safeCb := &recordingCallback{}
	cp.AddCallback(panicCb)
	cp.AddCallback(safeCb)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	resp, err := cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})
	if err != nil {
		t.Fatalf("Chat should succeed despite callback panic: %v", err)
	}
	if resp == nil {
		t.Fatal("Chat returned nil response")
	}

	// The safe callback should still have fired.
	waitUntil(t, 2*time.Second, func() bool {
		return safeCb.responseCount() == 1
	})

	// Wait for the panic recovery log to be written (goroutine-based).
	waitUntil(t, 2*time.Second, func() bool {
		return logBuf.String() != ""
	})

	// Verify the panic was logged.
	logOutput := logBuf.String()
	if logOutput == "" {
		t.Error("expected panic recovery to be logged")
	}
}

func TestCallbackPanicRecoveryDefaultLogger(t *testing.T) {
	t.Parallel()
	// Verify that a nil logger set via SetLogger is a no-op (uses the default).
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)
	cp.SetLogger(nil) // should be a no-op

	// Should not panic.
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	_, err := cp.Chat(context.Background(), msgs, ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

func TestCallbackThreadSafety(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	var (
		totalRequests  atomic.Int64
		totalResponses atomic.Int64
		totalErrors    atomic.Int64
	)

	// A callback that just counts.
	countingCb := &countingCallback{
		requests:  &totalRequests,
		responses: &totalResponses,
		errors:    &totalErrors,
	}
	cp.AddCallback(countingCb)

	const numGoroutines = 20
	const callsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
				cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})
			}
		}()
	}

	wg.Wait()

	// Wait for all async callbacks.
	expected := int64(numGoroutines * callsPerGoroutine)
	waitUntil(t, 5*time.Second, func() bool {
		return totalRequests.Load() >= expected && totalResponses.Load() >= expected
	})

	if totalRequests.Load() != expected {
		t.Errorf("totalRequests = %d, want %d", totalRequests.Load(), expected)
	}
	if totalResponses.Load() != expected {
		t.Errorf("totalResponses = %d, want %d", totalResponses.Load(), expected)
	}
	if totalErrors.Load() != 0 {
		t.Errorf("totalErrors = %d, want 0", totalErrors.Load())
	}
}

func TestCallbackConcurrentRegistration(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	cbs := make([]*recordingCallback, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		cbs[i] = &recordingCallback{}
	}

	// Add callbacks concurrently.
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cp.AddCallback(cbs[idx])
		}(i)
	}
	wg.Wait()

	if len(cp.Callbacks()) != numGoroutines {
		t.Fatalf("Callbacks() = %d, want %d", len(cp.Callbacks()), numGoroutines)
	}

	// Remove callbacks concurrently.
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cp.RemoveCallback(cbs[idx])
		}(i)
	}
	wg.Wait()

	if len(cp.Callbacks()) != 0 {
		t.Errorf("Callbacks() = %d, want 0", len(cp.Callbacks()))
	}
}

func TestCallbackStreamChat(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	cb := &recordingCallback{}
	cp.AddCallback(cb)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "Hello world"}}
	sr, err := cp.StreamChat(context.Background(), msgs, ChatOptions{Model: "stream-model"})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer sr.Close()

	// Drain the stream.
	for evt := range sr.Events {
		if evt.Type == "done" {
			break
		}
	}

	// Wait for async callbacks.
	// We expect at least: 1 OnRequest + several OnStreamEvent (content + done).
	// Since callbacks run in separate goroutines, wait for all expected events.
	waitUntil(t, 2*time.Second, func() bool {
		return cb.requestCount() >= 1 && cb.hasEventType("done") && cb.streamEventCount() >= 2
	})

	if cb.requestCount() != 1 {
		t.Errorf("OnRequest called %d times, want 1", cb.requestCount())
	}

	// We should have received stream events (at minimum content and done).
	if cb.streamEventCount() < 2 {
		t.Errorf("OnStreamEvent called %d times, want >= 2", cb.streamEventCount())
	}

	// Verify a "done" event was received (order is not guaranteed since
	// callbacks run in separate goroutines).
	if !cb.hasEventType("done") {
		t.Error("expected a 'done' stream event but none received")
	}
}

func TestCallbackStreamChatError(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeError)
	cp := mustCallbackProvider(t, mock)

	cb := &recordingCallback{}
	cp.AddCallback(cb)

	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	_, err := cp.StreamChat(context.Background(), msgs, ChatOptions{Model: "m"})
	if err == nil {
		t.Fatal("expected error from StreamChat")
	}

	waitUntil(t, 2*time.Second, func() bool {
		return cb.requestCount() == 1 && cb.errorCount() == 1
	})

	if cb.requestCount() != 1 {
		t.Errorf("OnRequest called %d times, want 1", cb.requestCount())
	}
	if cb.errorCount() != 1 {
		t.Errorf("OnError called %d times, want 1", cb.errorCount())
	}
	if cb.streamEventCount() != 0 {
		t.Errorf("OnStreamEvent called %d times, want 0 (stream never started)", cb.streamEventCount())
	}
}

func TestCallbackNilCallbackIgnored(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	cp.AddCallback(nil) // should not panic
	if len(cp.Callbacks()) != 0 {
		t.Errorf("nil callback should not be added")
	}
}

func TestCallbackEmptyCallbacksNoop(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeEcho)
	cp := mustCallbackProvider(t, mock)

	// No callbacks registered — should still work fine.
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	resp, err := cp.Chat(context.Background(), msgs, ChatOptions{Model: "m"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp == nil {
		t.Fatal("Chat returned nil response")
	}
}

func TestCallbackErrorInNewCallbackProvider(t *testing.T) {
	t.Parallel()
	if _, err := NewCallbackProvider(nil); err == nil {
		t.Error("NewCallbackProvider(nil) should return an error")
	}
}

// --- helper: countingCallback for thread-safety test ---

type countingCallback struct {
	requests  *atomic.Int64
	responses *atomic.Int64
	errors    *atomic.Int64
}

func (c *countingCallback) OnRequest(_ context.Context, _, _ string, _ []GraycodeRouterMessage, _ ChatOptions) {
	c.requests.Add(1)
}

func (c *countingCallback) OnResponse(_ context.Context, _, _ string, _ *GraycodeRouterResponse, _ time.Duration) {
	c.responses.Add(1)
}

func (c *countingCallback) OnError(_ context.Context, _, _ string, _ error, _ time.Duration) {
	c.errors.Add(1)
}

func (c *countingCallback) OnStreamEvent(_ context.Context, _, _ string, _ GraycodeRouterStreamEvent) {
}

// Ensure we're using the fmt package for potential debug printing.
var _ = fmt.Sprintf

// mustCallbackProvider constructs a CallbackProvider, failing the test on error.
func mustCallbackProvider(tb testing.TB, inner Provider) *CallbackProvider {
	tb.Helper()
	cp, err := NewCallbackProvider(inner)
	if err != nil {
		tb.Fatalf("NewCallbackProvider: %v", err)
	}
	return cp
}
