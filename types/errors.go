package types

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// --- API Error types ---

type APIError struct {
	Status  int                    `json:"status"`
	Headers map[string]string      `json:"headers"`
	Err     map[string]interface{} `json:"error"`
	Message string                 `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

func NewAPIError(status int, headers map[string]string, err map[string]interface{}, message string) *APIError {
	return &APIError{Status: status, Headers: headers, Err: err, Message: message}
}

type APIConnectionError struct{ APIError }

func NewAPIConnectionError(message string) *APIConnectionError {
	return &APIConnectionError{APIError{Message: message}}
}

type APIConnectionTimeoutError struct{ APIError }

func NewAPIConnectionTimeoutError(message string) *APIConnectionTimeoutError {
	return &APIConnectionTimeoutError{APIError{Message: message}}
}

type APIUserAbortError struct{ APIError }

func NewAPIUserAbortError(message string) *APIUserAbortError {
	return &APIUserAbortError{APIError{Message: message}}
}

type NotFoundError struct{ APIError }

func NewNotFoundError(message string) *NotFoundError {
	return &NotFoundError{APIError{Status: 404, Message: message}}
}

type AuthenticationError struct{ APIError }

func NewAuthenticationError(message string) *AuthenticationError {
	return &AuthenticationError{APIError{Status: 401, Message: message}}
}

// TransientError wraps an HTTP status code and message to indicate a retriable error.
// Use errors.As to check for transient errors programmatically.
type TransientError struct {
	StatusCode int
	Message    string
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("transient error (HTTP %d): %s", e.StatusCode, e.Message)
}

var httpStatusRe = regexp.MustCompile(`(?i)(?:HTTP[/:]?\s*|status[:\s]+|code[:\s]+)(\d{3})`)

// IsTransient reports whether the error is likely temporary and worth retrying.
// This is the canonical retry classifier used by the retry middleware.
//
// Conservative by default: unknown errors (those that match neither retriable
// nor non-retriable patterns) are treated as NOT retriable. This avoids
// unnecessary retries for unexpected error types (e.g., malformed responses,
// serialization failures). Callers like FallbackProvider that want optimistic
// fallback on unknown errors implement their own wrapper — see provider.isRetriableError.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Check for typed TransientError
	var te *TransientError
	if errors.As(err, &te) {
		return true
	}
	msg := strings.ToLower(err.Error())

	// Extract HTTP status code if present and check explicit lists.
	if matches := httpStatusRe.FindStringSubmatch(err.Error()); len(matches) >= 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			if nonRetriableStatusCodes[code] {
				return false
			}
			if retriableStatusCodes[code] {
				return true
			}
		}
	}

	for _, pattern := range []string{
		"timeout", "timed out", "deadline exceeded",
		"connection refused", "connection reset", "eof", "broken pipe",
		"temporarily", "overloaded", "try again",
		"unavailable", "server error", "bad gateway", "service unavailable",
		"rate limit", "rate_limit",
	} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	if _, ok := err.(*APIConnectionTimeoutError); ok {
		return true
	}
	return false
}

// ExtractHTTPStatus attempts to extract an HTTP status code from err's message.
// Uses the same regex as IsTransient to find 3-digit codes in error text.
// Returns the status code and true if found, or 0 and false otherwise.
func ExtractHTTPStatus(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	matches := httpStatusRe.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0, false
	}
	code, convErr := strconv.Atoi(matches[1])
	if convErr != nil {
		return 0, false
	}
	return code, true
}

// retriableStatusCodes are HTTP status codes that warrant a retry.
var retriableStatusCodes = map[int]bool{408: true, 429: true, 500: true, 502: true, 503: true, 504: true, 529: true}

// nonRetriableStatusCodes are HTTP status codes that should never be retried.
var nonRetriableStatusCodes = map[int]bool{400: true, 401: true, 403: true, 404: true, 422: true}

// ClassifyError creates a structured error from a status code and message.
// It returns the most specific error type for the given error condition.
// For retriable status codes (408, 429, 500, 502, 503, 504, 529), returns a TransientError.
func ClassifyError(statusCode int, message string) error {
	if retriableStatusCodes[statusCode] {
		return &TransientError{StatusCode: statusCode, Message: message}
	}
	return &APIError{
		Status:  statusCode,
		Message: message,
	}
}
