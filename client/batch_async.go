package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Async batch reconciliation, completing the BatchClient surface: a
// wait-until-terminal loop with exponential backoff + jitter + Retry-After
// honoring, per-request result retrieval from the JSONL results endpoint,
// and a reconcile primitive that tolerates eventual consistency — an
// individual request's result row can lag the batch reaching a terminal
// state, so callers must keep polling for the row itself instead of
// trusting the batch status alone.

// PollOptions configures wait loops.
type PollOptions struct {
	// InitialInterval is the first sleep between polls (default 2s).
	InitialInterval time.Duration
	// MaxInterval caps the exponential growth of the sleep (default 30s).
	MaxInterval time.Duration
	// Timeout bounds total wall-clock waiting (default 10m).
	Timeout time.Duration
	// JitterFraction randomizes each sleep by ± this fraction (default 0.2).
	JitterFraction float64
}

func (o PollOptions) withDefaults() PollOptions {
	if o.InitialInterval <= 0 {
		o.InitialInterval = 2 * time.Second
	}
	if o.MaxInterval <= 0 {
		o.MaxInterval = 30 * time.Second
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Minute
	}
	if o.JitterFraction <= 0 || o.JitterFraction >= 1 {
		o.JitterFraction = 0.2
	}
	return o
}

var terminalBatchStates = map[string]bool{
	"ended":     true,
	"completed": true,
	"failed":    true,
	"expired":   true,
	"canceled":  true,
	"cancelled": true,
}

func isTerminalBatchState(s string) bool { return terminalBatchStates[strings.ToLower(s)] }

// backoffDelay computes attempt-th exponential delay with jitter, capped by
// MaxInterval; a Retry-After seconds header on resp overrides it when larger.
func backoffDelay(attempt int, retryAfter string, o PollOptions) time.Duration {
	d := o.InitialInterval << uint(min(attempt, 16))
	if d > o.MaxInterval || d <= 0 {
		d = o.MaxInterval
	}
	j := 1 - o.JitterFraction + rand.Float64()*2*o.JitterFraction
	d = time.Duration(float64(d) * j)
	if ra, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && ra > 0 {
		rd := time.Duration(ra) * time.Second
		if rd > d {
			return rd
		}
	}
	return d
}

// WaitUntilDone polls the batch until it reaches a terminal state or the
// timeout elapses. Non-terminal responses keep polling; 429/5xx responses
// are retried with Retry-After-aware backoff; other non-200s fail fast.
func (bc *BatchClient) WaitUntilDone(ctx context.Context, batchID string, opts PollOptions) (*BatchResult, error) {
	opts = opts.withDefaults()
	deadline := time.Now().Add(opts.Timeout)
	attempt := 0
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, bc.baseURL+"/v1/messages/batches/"+batchID, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", bc.apiKey)
		req.Header.Set("Anthropic-Version", "2023-06-01")
		req.Header.Set("Anthropic-Beta", "message-batches-2024-09-24")
		resp, err := bc.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("eyrie: batch wait: %w", err)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			var res BatchResult
			derr := json.NewDecoder(resp.Body).Decode(&res)
			_ = resp.Body.Close()
			if derr != nil {
				return nil, derr
			}
			if isTerminalBatchState(res.Status) {
				return &res, nil
			}
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			// transient: retried below with Retry-After-aware backoff
		default:
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("eyrie: batch wait error %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("eyrie: batch %s not terminal within %s", batchID, opts.Timeout)
		}
		delay := backoffDelay(attempt, resp.Header.Get("Retry-After"), opts)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		attempt++
	}
}

// BatchRequestResult is one row of the batch results JSONL: the raw provider
// payload is preserved byte-exact so hosts decode per-provider shapes.
type BatchRequestResult struct {
	CustomID string          `json:"custom_id"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    json.RawMessage `json:"error,omitempty"`
}

// ErrResultNotVisible reports that a request's result row was absent even
// though polling continued past batch completion — callers may retry.
var ErrResultNotVisible = errors.New("eyrie: batch result row not yet visible")

// RequestResults fetches and parses the newline-delimited results document.
func (bc *BatchClient) RequestResults(ctx context.Context, batchID string) ([]BatchRequestResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bc.baseURL+"/v1/messages/batches/"+batchID+"/results", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", bc.apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "message-batches-2024-09-24")
	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("eyrie: batch results: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("batch: close results body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("eyrie: batch results error %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	var out []BatchRequestResult
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var r BatchRequestResult
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("eyrie: parse results line: %w", err)
		}
		out = append(out, r)
	}
	return out, scanner.Err()
}

// PollRequestResult reconciles one request: waits until the batch is terminal
// AND the custom_id's row appears, tolerating eventual consistency where the
// results endpoint lags batch completion. After the batch completes, up to
// opts.InitialInterval-scaled extra polls are made before surfacing
// ErrResultNotVisible.
func (bc *BatchClient) PollRequestResult(ctx context.Context, batchID, customID string, opts PollOptions) (*BatchRequestResult, error) {
	opts = opts.withDefaults()
	if _, err := bc.WaitUntilDone(ctx, batchID, opts); err != nil {
		return nil, err
	}
	const maxExtra = 5
	for extra := 0; ; extra++ {
		rows, err := bc.RequestResults(ctx, batchID)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			if rows[i].CustomID == customID {
				return &rows[i], nil
			}
		}
		if extra >= maxExtra {
			return nil, fmt.Errorf("%w after batch completion: %s", ErrResultNotVisible, customID)
		}
		delay := backoffDelay(extra, "", opts)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
}
