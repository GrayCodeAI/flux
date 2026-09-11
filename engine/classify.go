package engine

import (
	"context"
	"errors"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

func classify(operation string, route Route, err error) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return err
	}
	code := ErrorInternal
	retryable := false
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = ErrorCancelled
	default:
		var providerErr *client.EyrieError
		if errors.As(err, &providerErr) {
			retryable = providerErr.IsRetriable()
			switch {
			case providerErr.IsAuthError():
				code = ErrorAuthentication
			case providerErr.IsRateLimited():
				code = ErrorRateLimited
			case retryable:
				code = ErrorProviderUnavailable
			default:
				code = ErrorInvalidRequest
			}
		} else {
			message := strings.ToLower(err.Error())
			switch {
			case strings.Contains(message, "context") && (strings.Contains(message, "long") || strings.Contains(message, "token")):
				code = ErrorContextExceeded
			case strings.Contains(message, "credential") || strings.Contains(message, "api key"):
				code = ErrorCredentialMissing
			case strings.Contains(message, "rate limit") || strings.Contains(message, "429"):
				code = ErrorRateLimited
				retryable = true
			case strings.Contains(message, "provider") || strings.Contains(message, "transport"):
				code = ErrorProviderUnavailable
			}
		}
	}
	return &Error{
		Code: code, Operation: operation, Provider: route.Provider, Model: route.Model,
		Message: err.Error(), Retryable: retryable, Cause: err,
	}
}
