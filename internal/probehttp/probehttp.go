// Package probehttp contains shared helpers for the graycode-router credential-probe
// and catalog-probe call sites. It centralises the HTTP-client configuration
// and the HTTP-status-to-error mapping that probe code reaches for on every
// request. Keeping it in one place means timeout policy and error wording
// stay aligned across credential probes and live catalog probes.
package probehttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultRequestTimeout caps the time a single probe HTTP request can take.
// The probe context already carries a deadline, but the http.Client.Timeout
// is a second line of defence: it bounds the time spent in TLS, redirects,
// and the like even if the caller's context deadline is missing.
const DefaultRequestTimeout = 15 * time.Second

// DefaultClient is the shared *http.Client used by probe code in the graycode-router
// repo. Callers should reuse it instead of http.DefaultClient so the
// per-request timeout policy stays consistent.
var DefaultClient = &http.Client{Timeout: DefaultRequestTimeout}

// ProbeError builds a credential-probe error message for a non-2xx response.
// The wording is part of the public surface that hawk surfaces to users when
// /config probe fails, so the strings here are stable.
//
// status is the HTTP status code returned by the provider. The function
// collapses 401/403 into a single "invalid key" message, distinguishes
// 429 (rate limited) from a hard 5xx (provider unavailable), and falls
// back to a generic HTTP-status message for everything else.
func ProbeError(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("credential probe failed: invalid API key (HTTP %d)", status)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("credential probe failed: rate limited — try again shortly")
	case status >= 500:
		return fmt.Errorf("credential probe failed: provider unavailable (HTTP %d)", status)
	default:
		return fmt.Errorf("credential probe failed: HTTP %d", status)
	}
}

// DoGet issues a GET against url with the given headers, returns the status
// code and body. The body is bounded to 1 MiB so a malicious or buggy
// provider cannot exhaust memory. The body is read and closed on the caller's
// behalf; callers only need to inspect (status, body, err).
//
// The request inherits the supplied context and the package-level
// DefaultClient, so a missing context deadline is still capped by the
// client Timeout.
func DoGet(ctx context.Context, url string, headers map[string]string) (int, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	const maxBody = 1 << 20 // 1 MiB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// UserAgent returns the standard graycode-router User-Agent string for probe traffic.
func UserAgent() string { return "graycode-router-probe/1.0" }

// JoinURL trims a trailing slash from base and joins it with the supplied
// path. It's a tiny helper kept here so the various probe call sites stop
// re-implementing the trim/concat dance.
func JoinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	path = strings.TrimLeft(path, "/")
	if path == "" {
		return base
	}
	return base + "/" + path
}
