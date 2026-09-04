package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/GrayCodeAI/graycode-router/types"
)

// GraycodeRouterError is a structured error that preserves provider context,
// HTTP metadata, and request identification for debugging.
type GraycodeRouterError struct {
	Provider   string
	Op         string // operation that failed (e.g. "chat", "stream", "ping")
	StatusCode int
	RequestID  string
	Message    string
	Err        error
}

func (e *GraycodeRouterError) Error() string {
	s := fmt.Sprintf("graycode-router: %s %s failed", e.Provider, e.Op)
	if e.StatusCode > 0 {
		s += fmt.Sprintf(" (HTTP %d)", e.StatusCode)
	}
	if e.RequestID != "" {
		s += fmt.Sprintf(" [request_id=%s]", e.RequestID)
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *GraycodeRouterError) Unwrap() error { return e.Err }

// IsRetriable returns true if the error is likely transient and retrying may help.
func (e *GraycodeRouterError) IsRetriable() bool {
	switch e.StatusCode {
	case 408, 429, 500, 502, 503, 504, 529:
		return true
	}
	return false
}

// IsAuthError returns true if the error indicates an authentication/authorization problem.
func (e *GraycodeRouterError) IsAuthError() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

// IsRateLimited returns true if the error indicates rate limiting.
func (e *GraycodeRouterError) IsRateLimited() bool {
	return e.StatusCode == 429
}

// nonRetriableStatusCodes are HTTP status codes that indicate a client-side
// error — falling back won't help because the request itself is bad.
var nonRetriableStatusCodes = map[int]bool{
	400: true, // bad request
	401: true, // unauthorized
	403: true, // forbidden
	404: true, // not found (wrong model name, etc.)
	422: true, // unprocessable entity
}

// IsRetriableError determines whether a fallback to the next provider should
// be attempted. It delegates to types.IsTransient for known error patterns
// but diverges on unknown errors: where IsTransient is conservative (returns
// false for unrecognized errors), IsRetriableError is optimistic (returns true).
//
// Rationale: in a fallback chain, trying the next provider is cheap and may
// succeed even if the current provider failed with an unexpected error type.
// In contrast, retry middleware (which uses IsTransient) should be conservative
// to avoid wasting requests on errors that won't resolve with a retry.
//
// *GraycodeRouterError (returned by FormatAPIError and friends) is preferred over
// the string-based heuristic: it carries the structured status code so the
// classification is exact rather than regex-parsed.
func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Structured path: trust *GraycodeRouterError's IsRetriable.
	var graycodeRouterErr *GraycodeRouterError
	if errors.As(err, &graycodeRouterErr) {
		return graycodeRouterErr.IsRetriable()
	}
	// Heuristic path for legacy errors that predate the GraycodeRouterError migration.
	if types.IsTransient(err) {
		return true
	}
	// If the error contains a known non-retriable HTTP status code, give up
	// immediately — the request itself is bad, not the provider.
	if code, ok := types.ExtractHTTPStatus(err); ok {
		if nonRetriableStatusCodes[code] {
			return false
		}
	}
	// Unknown error types: treat as retriable so we at least try the next provider.
	return true
}
