// Package httputil provides shared HTTP server primitives used across
// eyrie's API surfaces. Centralizing these eliminates drift in auth
// comparison, body decoding, JSON responses, security headers, and
// loopback validation that previously existed as duplicated copies
// in internal/api/server.go and other handlers.
package httputil

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// MaxRequestBodyBytes is the default maximum request body size (1 MiB).
const MaxRequestBodyBytes = 1 << 20

// ConstantTimeEqual compares two strings in constant time. The shorter
// value is padded with NUL bytes so comparison time does not leak token
// length. This is the preferred bearer-token comparison for API servers.
func ConstantTimeEqual(a, b string) bool {
	if len(a) < len(b) {
		a += strings.Repeat("\x00", len(b)-len(a))
	} else if len(b) < len(a) {
		b += strings.Repeat("\x00", len(a)-len(b))
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// DecodeJSONBody decodes a JSON request body into dst with a size limit
// and strict unknown-field rejection. Returns true on success. On failure
// it writes a 400 JSON error response and returns false.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return DecodeJSONBodyWithLimit(w, r, dst, MaxRequestBodyBytes)
}

// DecodeJSONBodyWithLimit is like DecodeJSONBody but with a custom body
// size limit.
func DecodeJSONBodyWithLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain a single JSON object"})
		return false
	}
	return true
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// SecurityHeaders wraps an http.Handler with standard security headers
// (X-Content-Type-Options, X-Frame-Options, Cache-Control).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// IsLoopbackHost reports whether host is a loopback address:
// 127.0.0.0/8, ::1, or "localhost". An empty string is treated as
// non-loopback (fail-safe).
func IsLoopbackHost(host string) bool {
	if host == "" || host == "localhost" {
		return host == "localhost"
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ValidateAuthConfig refuses to start a server with no API key on a
// non-loopback bind. Returns nil if the API key is set or the bind
// address is loopback.
func ValidateAuthConfig(addr, apiKey string) error {
	if apiKey != "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid bind address %q: %w", addr, err)
	}
	if !IsLoopbackHost(host) {
		return fmt.Errorf("API key is empty and bind address %q is not loopback; refusing to start. Set an API key or bind to 127.0.0.1", addr)
	}
	return nil
}

// ExtractBearerToken extracts a bearer/API-key token from request headers.
// It checks "Authorization: Bearer ..." first, then "X-API-Key".
// The Bearer scheme is matched case-insensitively per RFC 7235.
func ExtractBearerToken(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if len(token) > 7 && strings.EqualFold(token[:7], "Bearer ") {
		return token[7:]
	}
	if token == "" {
		token = r.Header.Get("X-API-Key")
	}
	return token
}
