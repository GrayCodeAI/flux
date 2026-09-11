// Package client provides request coalescing for identical concurrent LLM requests.
// When multiple goroutines send identical requests simultaneously (same provider,
// model, messages, temperature, max_tokens), the Coalescer deduplicates them into
// a single API call and broadcasts the result to all waiters.
package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CoalesceKey uniquely identifies an LLM request for deduplication.
// It hashes provider, model, messages, temperature, and max_tokens.
type CoalesceKey struct {
	Provider    string         `json:"provider"`
	Model       string         `json:"model"`
	Messages    []EyrieMessage `json:"messages"`
	Temperature *float64       `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
}

// String returns a stable hash of the coalesce key for map lookup.
func (k CoalesceKey) String() string {
	structured := struct {
		Provider    string         `json:"provider"`
		Model       string         `json:"model"`
		Messages    []EyrieMessage `json:"messages"`
		Temperature *float64       `json:"temperature,omitempty"`
		MaxTokens   int            `json:"max_tokens,omitempty"`
	}{
		Provider:    k.Provider,
		Model:       k.Model,
		Messages:    k.Messages,
		Temperature: k.Temperature,
		MaxTokens:   k.MaxTokens,
	}

	data, err := json.Marshal(structured)
	if err != nil {
		return k.Provider + ":" + k.Model + ":" + fmt.Sprintf("%d", len(k.Messages))
	}

	hash := sha256.Sum256(data)
	return string(hash[:])
}

// InflightRequest represents a pending request that multiple waiters can join.
// The first goroutine to create this request is responsible for executing it
// and broadcasting the result to all waiting goroutines.
type InflightRequest struct {
	// done is closed once the result is ready
	done chan struct{}
	// result holds the response (set before done is closed)
	result *EyrieResponse
	// err holds the error (set before done is closed)
	err error
	// ctx is the context for this request
	ctx context.Context
	// cancel cancels the context
	cancel context.CancelFunc
	// mu protects waiters
	mu sync.Mutex
	// waiters tracks how many goroutines are waiting for this request
	waiters int
}

const maxCoalesceWaiters = 100

// Coalescer deduplicates identical concurrent LLM requests.
// It maintains a map of inflight requests indexed by CoalesceKey.
// When a request arrives that matches an existing inflight request,
// the caller waits on the existing request's done channel instead
// of making a new API call.
//
// The Coalescer automatically cleans up completed requests after a TTL.
// Coalescer is safe for concurrent use.
type Coalescer struct {
	mu       sync.Mutex
	inflight map[string]*InflightRequest
	ttl      time.Duration
}

// NewCoalescer creates a new Coalescer with the specified TTL for completed requests.
// The ttl controls how long completed requests remain in the map for potential reuse.
func NewCoalescer(ttl time.Duration) *Coalescer {
	return &Coalescer{
		inflight: make(map[string]*InflightRequest),
		ttl:      ttl,
	}
}

// Coalesce deduplicates concurrent identical requests.
//
// If an inflight request exists for the given key, the caller waits on its done
// channel and returns the same result. Otherwise, a new InflightRequest is created,
// added to the inflight map, and fn is called to execute the actual request.
//
// The executing goroutine is responsible for:
//  1. Calling fn() to get the result
//  2. Storing the result in the InflightRequest
//  3. Closing the done channel to wake all waiters
//
// All waiting goroutines receive the same response.
func (c *Coalescer) Coalesce(ctx context.Context, key CoalesceKey, fn func() (*EyrieResponse, error)) (*EyrieResponse, error) {
	keyStr := key.String()

	c.mu.Lock()

	// Check if there's already an inflight request for this key
	if existing, ok := c.inflight[keyStr]; ok {
		existing.mu.Lock()
		if existing.waiters >= maxCoalesceWaiters {
			existing.mu.Unlock()
			c.mu.Unlock()
			return nil, fmt.Errorf("eyrie: coalesce limit reached (%d waiters); retry independently", maxCoalesceWaiters)
		}
		existing.waiters++
		existing.mu.Unlock()
		c.mu.Unlock()

		// Wait for the result from the existing request
		return existing.wait(ctx)
	}

	// No inflight request found - create a new one
	reqCtx, cancel := context.WithCancel(ctx)
	inflight := &InflightRequest{
		done:    make(chan struct{}),
		ctx:     reqCtx,
		cancel:  cancel,
		waiters: 1, // The creator counts as a waiter
	}
	c.inflight[keyStr] = inflight
	c.mu.Unlock()

	// Execute the request (this goroutine is responsible for it).
	// Use a closure so the defer can broadcast to waiters even on panic.
	var result *EyrieResponse
	var fnErr error
	func() {
		defer func() {
			inflight.err = fnErr
			close(inflight.done)
		}()
		result, fnErr = fn()
		inflight.result = result
	}()

	// Schedule cleanup after TTL. Use a cancellable timer so the goroutine
	// exits promptly if the enclosing context is cancelled (e.g. caller
	// shutdown), instead of lingering for the full TTL.
	go func() {
		timer := time.NewTimer(c.ttl)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		c.mu.Lock()
		delete(c.inflight, keyStr)
		c.mu.Unlock()
	}()

	// Return the result directly (we already have it)
	if fnErr != nil {
		return nil, fnErr
	}
	return result, nil
}

// wait blocks until the inflight request completes or the provided context is cancelled.
func (r *InflightRequest) wait(ctx context.Context) (*EyrieResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.ctx.Done():
		return nil, r.ctx.Err()
	case <-r.done:
		if r.err != nil {
			return nil, r.err
		}
		return r.result, nil
	}
}

// Stats returns the number of inflight requests (for monitoring/testing).
func (c *Coalescer) Stats() InflightStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	totalWaiters := 0
	for _, inflight := range c.inflight {
		inflight.mu.Lock()
		totalWaiters += inflight.waiters
		inflight.mu.Unlock()
	}

	return InflightStats{
		InflightRequests: len(c.inflight),
		TotalWaiters:     totalWaiters,
	}
}

// InflightStats contains statistics about inflight coalesced requests.
type InflightStats struct {
	// InflightRequests is the number of unique requests being executed
	InflightRequests int
	// TotalWaiters is the total number of goroutines waiting across all requests
	TotalWaiters int
}
