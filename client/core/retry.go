package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/GrayCodeAI/graycode-router/types"
)

// RetryConfig controls retry behavior for HTTP clients.
// It embeds types.RetryConfig for the core fields and adds RetryOn for
// HTTP-status-code–driven retry decisions.
type RetryConfig struct {
	types.RetryConfig
	RetryOn []int // HTTP status codes to retry on
}

// NewRetryConfig constructs a RetryConfig from core fields and optional
// HTTP status codes to retry on.
func NewRetryConfig(maxRetries int, baseDelay, maxDelay time.Duration, retryOn ...int) RetryConfig {
	return RetryConfig{
		RetryConfig: types.RetryConfig{MaxRetries: maxRetries, BaseDelay: baseDelay, MaxDelay: maxDelay},
		RetryOn:     retryOn,
	}
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return NewRetryConfig(
		DefaultMaxRetries, DefaultBaseDelay, DefaultMaxDelay,
		429, 500, 502, 503, 529,
	)
}

// ShouldRetry checks if a status code is retryable.
func (rc RetryConfig) ShouldRetry(statusCode int) bool {
	for _, code := range rc.RetryOn {
		if code == statusCode {
			return true
		}
	}
	return false
}

// backoffDelay calculates delay with exponential backoff + jitter.
// Respects Retry-After headers from HTTP responses when present.
func (rc RetryConfig) backoffDelay(attempt int, resp *http.Response) time.Duration {
	// Respect Retry-After header if present
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				d := time.Duration(secs) * time.Second
				if d > rc.MaxDelay {
					d = rc.MaxDelay
				}
				return d
			}
			if t, err := http.ParseTime(ra); err == nil {
				d := time.Until(t)
				if d > rc.MaxDelay {
					d = rc.MaxDelay
				}
				if d > 0 {
					return d
				}
			}
		}
	}

	// Use shared exponential backoff with full jitter
	return types.BackoffDelay(attempt, rc.BaseDelay, rc.MaxDelay)
}

var retryDelayRe = regexp.MustCompile(`(?i)(?:retry|try again)\s+(?:in|after)\s+(\d+(?:\.\d+)?)\s*(ms|milliseconds?|s|seconds?)`)

// parseRetryDelay extracts a delay hint from an error message.
func parseRetryDelay(errMsg string) time.Duration {
	m := retryDelayRe.FindStringSubmatch(errMsg)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	switch {
	case len(m[2]) > 0 && m[2][0] == 'm':
		return time.Duration(val * float64(time.Millisecond))
	default:
		return time.Duration(val * float64(time.Second))
	}
}

// DoWithRetry executes an HTTP request with retry logic.
//
// Note: DoWithRetry operates at the transport layer, before
// formatAPIError constructs *GraycodeRouterError. Structured-error awareness
// lives in the fallback chain (fallback.go:240-244) where
// *GraycodeRouterError.IsRetriable() / IsAuthError() drive provider
// rotation. DoWithRetry only needs the raw transport status code
// and the underlying network error to decide whether to retry the
// same request.
func DoWithRetry(ctx context.Context, httpClient *http.Client, req *http.Request, rc RetryConfig, logger *slog.Logger) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt <= rc.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := rc.backoffDelay(attempt-1, lastResp)
			if lastErr != nil {
				if parsed := parseRetryDelay(lastErr.Error()); parsed > delay {
					delay = parsed
				}
			}
			logger.Debug(
				"retrying request",
				"attempt", attempt, "max", rc.MaxRetries,
				"delay", delay, "url", req.URL.String(),
			)
			// Use time.NewTimer + Stop instead of time.After to avoid leaking
			// the timer in the runtime when ctx is cancelled before the delay
			// elapses. time.After allocates a timer that lives until it fires,
			// even if the caller has already moved on.
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		// Clone request body for retry (body may have been consumed).
		// If a Body is set but GetBody is missing, retries would silently
		// send a drained body and likely fail with a confusing provider
		// error; surface that immediately instead.
		retryReq := req.Clone(ctx)
		if req.Body != nil && req.GetBody == nil {
			return nil, fmt.Errorf("retry requires GetBody set on *http.Request (setBody=%T)", req.Body)
		}
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("failed to clone request body: %w", err)
			}
			retryReq.Body = body
		}

		resp, err := httpClient.Do(retryReq)
		if err != nil {
			lastErr = err
			logger.Warn("request failed", "attempt", attempt, "error", err)
			continue
		}

		if !rc.ShouldRetry(resp.StatusCode) {
			return resp, nil
		}

		lastResp = resp
		lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, req.URL.String())
		logger.Warn("retryable status", "attempt", attempt, "status", resp.StatusCode)
		_ = resp.Body.Close()
	}

	return nil, fmt.Errorf("graycode-router: %s: max retries (%d) exceeded for %s: %w", req.URL.Host, rc.MaxRetries, req.URL.Path, lastErr)
}
