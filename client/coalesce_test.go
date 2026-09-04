package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCoalesceKeyString(t *testing.T) {
	t.Parallel()
	key1 := CoalesceKey{
		Provider:  "anthropic",
		Model:     "claude-3-5-haiku",
		MaxTokens: 1024,
		Messages: []GraycodeRouterMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key2 := CoalesceKey{
		Provider:  "anthropic",
		Model:     "claude-3-5-haiku",
		MaxTokens: 1024,
		Messages: []GraycodeRouterMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key3 := CoalesceKey{
		Provider:  "openai",
		Model:     "gpt-4",
		MaxTokens: 512,
		Messages: []GraycodeRouterMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// Same inputs should produce same key
	keyStr1 := key1.String()
	keyStr2 := key2.String()
	if keyStr1 != keyStr2 {
		t.Errorf("Same keys should produce same string: %q != %q", keyStr1, keyStr2)
	}

	// Different inputs should produce different keys
	keyStr3 := key3.String()
	if keyStr1 == keyStr3 {
		t.Errorf("Different keys should produce different strings: %q == %q", keyStr1, keyStr3)
	}
}

func TestCoalesceDeduplicatesIdenticalRequests(t *testing.T) {
	t.Parallel()
	callCount := 0
	var mu sync.Mutex
	response := "coalesced response"

	fn := func() (*GraycodeRouterResponse, error) {
		mu.Lock()
		callCount++
		count := callCount
		mu.Unlock()

		// Simulate slow API call
		time.Sleep(100 * time.Millisecond)

		if count == 1 {
			return &GraycodeRouterResponse{
				Content:      response,
				FinishReason: "end_turn",
				Usage: &GraycodeRouterUsage{
					PromptTokens:     10,
					CompletionTokens: 15,
					TotalTokens:      25,
				},
			}, nil
		}
		return nil, errors.New("should not be called more than once")
	}

	coalescer := NewCoalescer(100 * time.Millisecond)

	key := CoalesceKey{
		Provider:  "anthropic",
		Model:     "claude-3-5-haiku",
		MaxTokens: 1024,
		Messages: []GraycodeRouterMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// Launch multiple concurrent identical requests
	numWorkers := 5
	var wg sync.WaitGroup
	results := make([]*GraycodeRouterResponse, numWorkers)
	errs := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = coalescer.Coalesce(context.Background(), key, fn)
		}(i)
	}

	wg.Wait()

	// Verify that fn was only called once despite multiple concurrent requests
	if callCount != 1 {
		t.Errorf("Expected fn to be called 1 time, got %d", callCount)
	}

	// Verify all workers received the same result
	for i := 0; i < numWorkers; i++ {
		if errs[i] != nil {
			t.Errorf("Worker %d got error: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Errorf("Worker %d got nil result", i)
			continue
		}
		if results[i].Content != response {
			t.Errorf("Worker %d got wrong content: %q, want %q", i, results[i].Content, response)
		}
	}
}

func TestCoalesceWaiterGetsError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("API rate limit exceeded")
	fn := func() (*GraycodeRouterResponse, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, expectedErr
	}

	coalescer := NewCoalescer(100 * time.Millisecond)

	key := CoalesceKey{
		Provider: "openai",
		Model:    "gpt-4",
		Messages: []GraycodeRouterMessage{{Role: "user", Content: "test"}},
	}

	var wg sync.WaitGroup
	numWorkers := 3
	results := make([]*GraycodeRouterResponse, numWorkers)
	errs := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = coalescer.Coalesce(context.Background(), key, fn)
		}(i)
	}

	wg.Wait()

	// All workers should get the error
	for i := 0; i < numWorkers; i++ {
		if errs[i] != expectedErr {
			t.Errorf("Worker %d expected error %v, got: %v", i, expectedErr, errs[i])
		}
		if results[i] != nil {
			t.Errorf("Worker %d expected nil result", i)
		}
	}
}

func TestCoalesceRespectsContextCancellation(t *testing.T) {
	t.Parallel()
	fn := func() (*GraycodeRouterResponse, error) {
		time.Sleep(500 * time.Millisecond)
		return &GraycodeRouterResponse{Content: "slow response"}, nil
	}

	coalescer := NewCoalescer(100 * time.Millisecond)
	key := CoalesceKey{
		Provider: "anthropic",
		Model:    "claude",
		Messages: []GraycodeRouterMessage{{Role: "user", Content: "test"}},
	}

	// First goroutine has long context, starts the request
	errCh := make(chan error, 1)
	go func() {
		_, err := coalescer.Coalesce(context.Background(), key, fn)
		errCh <- err
	}()

	// Small delay to ensure first goroutine creates the inflight entry
	time.Sleep(10 * time.Millisecond)

	// Second goroutine uses a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	result, err := coalescer.Coalesce(ctx, key, fn)

	if err != context.Canceled {
		t.Errorf("Second goroutine expected context canceled, got: %v", err)
	}
	if result != nil {
		t.Errorf("Second goroutine expected nil result on cancellation, got: %v", result)
	}
}

func TestCoalesceDifferentKeysNotDeduplicated(t *testing.T) {
	t.Parallel()
	callCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	fn := func(prefix string) func() (*GraycodeRouterResponse, error) {
		return func() (*GraycodeRouterResponse, error) {
			mu.Lock()
			callCount++
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			return &GraycodeRouterResponse{
				Content: prefix,
			}, nil
		}
	}

	coalescer := NewCoalescer(100 * time.Millisecond)

	keys := []CoalesceKey{
		{Provider: "anthropic", Model: "claude-3-5-haiku", Messages: []GraycodeRouterMessage{{Role: "user", Content: "A"}}},
		{Provider: "anthropic", Model: "claude-3-5-haiku", Messages: []GraycodeRouterMessage{{Role: "user", Content: "B"}}},
		{Provider: "openai", Model: "gpt-4", Messages: []GraycodeRouterMessage{{Role: "user", Content: "C"}}},
	}
	responses := []string{"response A", "response B", "response C"}

	// Each key should trigger a separate fn call
	for i, key := range keys {
		wg.Add(1)
		go func(idx int, k CoalesceKey) {
			defer wg.Done()
			resp, err := coalescer.Coalesce(context.Background(), k, fn(responses[idx]))
			if err != nil {
				t.Errorf("Key %d got error: %v", idx, err)
			}
			if resp.Content != responses[idx] {
				t.Errorf("Key %d expected %q, got %q", idx, responses[idx], resp.Content)
			}
		}(i, key)
	}

	wg.Wait()

	// Should have called fn 3 times (once per unique key)
	if callCount != 3 {
		t.Errorf("Expected 3 fn calls, got %d", callCount)
	}
}

func TestCoalesceStats(t *testing.T) {
	t.Parallel()
	coalescer := NewCoalescer(100 * time.Millisecond)

	// Should start empty
	stats := coalescer.Stats()
	if stats.InflightRequests != 0 || stats.TotalWaiters != 0 {
		t.Errorf("Expected empty stats, got: %+v", stats)
	}

	fn := func() (*GraycodeRouterResponse, error) {
		time.Sleep(200 * time.Millisecond)
		return &GraycodeRouterResponse{Content: "test"}, nil
	}

	key := CoalesceKey{
		Provider: "anthropic",
		Model:    "claude",
		Messages: []GraycodeRouterMessage{{Role: "user", Content: "test"}},
	}

	var wg sync.WaitGroup

	// Start 3 concurrent requests for same key
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			coalescer.Coalesce(context.Background(), key, fn)
		}()
	}

	// Small delay, should see one inflight request with all three waiters
	waitForStats(t, coalescer, func(s InflightStats) bool {
		return s.InflightRequests == 1 && s.TotalWaiters == 3
	})

	wg.Wait()

	// After completion, should still have one cached entry
	waitForStats(t, coalescer, func(s InflightStats) bool {
		return s.InflightRequests == 1
	})

	// After TTL expires, entry should be cleaned up
	waitForStats(t, coalescer, func(s InflightStats) bool {
		return s.InflightRequests == 0
	})
}

// waitForStats polls the coalescer until pred is satisfied or the deadline
// elapses. Avoids wall-clock timing flakiness under the race detector and
// heavily loaded CI runners.
func waitForStats(t *testing.T, c *Coalescer, pred func(InflightStats) bool) InflightStats {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s := c.Stats()
		if pred(s) {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for coalescing stats, last=%+v", s)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCoalesceIntegrationWithGraycodeRouterClient(t *testing.T) {
	t.Parallel()
	// Test that coalescing integrates properly with GraycodeRouterClient.Chat()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "coalesced integration response"
	mock.Delay = 200 * time.Millisecond

	c := Client(&GraycodeRouterConfig{Provider: "mock"}, WithCoalescing(200*time.Millisecond))

	// Manually set the mock provider
	c.mu.Lock()
	c.providers["mock"] = mock
	c.defaultProvider = "mock"
	c.mu.Unlock()

	messages := []GraycodeRouterMessage{{Role: "user", Content: "Integration test"}}
	opts := ChatOptions{Model: "mock-model", Temperature: floatPtr(0.7), MaxTokens: 100}

	var wg sync.WaitGroup
	numWorkers := 4
	results := make([]*GraycodeRouterResponse, numWorkers)
	errs := make([]error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Chat(context.Background(), messages, opts)
		}(i)
	}

	wg.Wait()

	// Mock should only receive 1 call due to coalescing
	if mock.CallCount() != 1 {
		t.Errorf("Expected mock to be called 1 time, got %d", mock.CallCount())
	}

	// All goroutines should get the same response
	for i := 0; i < numWorkers; i++ {
		if errs[i] != nil {
			t.Errorf("Worker %d got error: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Errorf("Worker %d got nil result", i)
			continue
		}
		if results[i].Content != "coalesced integration response" {
			t.Errorf("Worker %d got unexpected content: %q", i, results[i].Content)
		}
	}
}

func TestCoalesceDisabledByDefault(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "test"
	c := Client(&GraycodeRouterConfig{Provider: "mock"}) // No coalescing enabled

	// Manually set the mock provider
	c.mu.Lock()
	c.providers["mock"] = mock
	c.defaultProvider = "mock"
	c.mu.Unlock()

	messages := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	opts := ChatOptions{Model: "mock-model"}

	// Make multiple calls - each should hit the mock
	for i := 0; i < 3; i++ {
		_, err := c.Chat(context.Background(), messages, opts)
		if err != nil {
			t.Fatalf("Call %d failed: %v", i, err)
		}
	}

	// Each call should go to the mock individually
	if mock.CallCount() != 3 {
		t.Errorf("Expected 3 calls to mock (no coalescing), got %d", mock.CallCount())
	}
}
