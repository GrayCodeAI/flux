package router

import (
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/types"
)

// RetryConfig controls retry behavior at the router level.
// It embeds types.RetryConfig for the core fields and adds an OnRetry
// callback for observability.
type RetryConfig struct {
	types.RetryConfig
	OnRetry func(RetryEvent)
}

type RetryEvent struct {
	Err        error
	Attempt    int
	MaxRetries int
	Delay      time.Duration
}

// NewRetryConfig constructs a router RetryConfig.
func NewRetryConfig(maxRetries int, baseDelay, maxDelay time.Duration) RetryConfig {
	return RetryConfig{
		RetryConfig: types.RetryConfig{MaxRetries: maxRetries, BaseDelay: baseDelay, MaxDelay: maxDelay},
	}
}

func DefaultRetryConfig() RetryConfig {
	return NewRetryConfig(3, 1*time.Second, 30*time.Second)
}

func IsTransient(err error) bool {
	return types.IsTransient(err)
}

// ShouldTryNextDeployment reports billing/credit errors where another deployment may succeed.
func ShouldTryNextDeployment(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"requires more credits", "can only afford", "insufficient credits",
		"insufficient balance", "payment required", "out of credits", "402",
		"insufficient_user_quota", "insufficient_quota",
		"pre-deduct", "pre-deduction", "预扣费",
	} {
		if strings.Contains(low, pattern) {
			return true
		}
	}
	return false
}

// BackoffDelay calculates exponential backoff delay with jitter using the shared implementation.
func BackoffDelay(attempt int, cfg RetryConfig) time.Duration {
	return types.BackoffDelay(attempt, cfg.BaseDelay, cfg.MaxDelay)
}

// newTimer is a variable so tests can inject a fake timer. Uses
// time.NewTimer (not time.After) to avoid leaking the timer in the
// runtime when ctx is cancelled before the delay elapses.
var newTimer = time.NewTimer
