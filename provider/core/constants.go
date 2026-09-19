package core

import "time"

// Default retry configuration.
const (
	// DefaultMaxRetries is the default number of request retries on transient errors.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the default initial backoff between retries.
	DefaultBaseDelay = 500 * time.Millisecond
	// DefaultMaxDelay is the default maximum backoff cap.
	DefaultMaxDelay = 30 * time.Second
	// DefaultRetryStatusCodes are the HTTP status codes retried by default.
	DefaultRetryStatusCodes = "429,500,502,503,529"
)

// Default transport timeouts.
const (
	// DefaultRequestTimeout is the default overall per-request timeout.
	DefaultRequestTimeout = 120 * time.Second
	// DefaultHandshakeTimeout is the default TLS handshake timeout.
	DefaultHandshakeTimeout = 10 * time.Second
	// DefaultIdleConnTimeout is the default idle connection keep-alive window.
	DefaultIdleConnTimeout = 90 * time.Second
)

// Default cooldown windows.
const (
	// DefaultCooldownDuration is how long a provider stays "cooling down" after a 429/5xx.
	DefaultCooldownDuration = 5 * time.Second
)
