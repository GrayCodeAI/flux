package core

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GrayCodeAI/graycode-router/types"
)

func TestRetryDefaultRetryConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 500*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 500ms", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if len(cfg.RetryOn) != 5 {
		t.Fatalf("RetryOn has %d codes, want 5", len(cfg.RetryOn))
	}
	expected := []int{429, 500, 502, 503, 529}
	for i, code := range expected {
		if cfg.RetryOn[i] != code {
			t.Errorf("RetryOn[%d] = %d, want %d", i, cfg.RetryOn[i], code)
		}
	}
}

func TestRetryShouldRetryTrue(t *testing.T) {
	t.Parallel()
	cfg := DefaultRetryConfig()
	codes := []int{429, 500, 502, 503, 529}
	for _, code := range codes {
		if !cfg.ShouldRetry(code) {
			t.Errorf("ShouldRetry(%d) = false, want true", code)
		}
	}
}

func TestRetryShouldRetryFalse(t *testing.T) {
	t.Parallel()
	cfg := DefaultRetryConfig()
	codes := []int{400, 401, 403, 404}
	for _, code := range codes {
		if cfg.ShouldRetry(code) {
			t.Errorf("ShouldRetry(%d) = true, want false", code)
		}
	}
}

func TestRetryDelayIncreasesExponentially(t *testing.T) {
	t.Parallel()
	cfg := NewRetryConfig(0, 100*time.Millisecond, 10*time.Second)

	// Run multiple samples to confirm the trend despite jitter
	samples := 100
	var avg0, avg1, avg2 time.Duration
	for i := 0; i < samples; i++ {
		avg0 += cfg.backoffDelay(0, nil)
		avg1 += cfg.backoffDelay(1, nil)
		avg2 += cfg.backoffDelay(2, nil)
	}
	avg0 /= time.Duration(samples)
	avg1 /= time.Duration(samples)
	avg2 /= time.Duration(samples)

	// With full jitter, average should be half the exponential cap:
	// attempt 0: base*2^0 = 100ms, avg jitter ~50ms
	// attempt 1: base*2^1 = 200ms, avg jitter ~100ms
	// attempt 2: base*2^2 = 400ms, avg jitter ~200ms
	if avg1 <= avg0 {
		t.Errorf("average delay for attempt 1 (%v) should exceed attempt 0 (%v)", avg1, avg0)
	}
	if avg2 <= avg1 {
		t.Errorf("average delay for attempt 2 (%v) should exceed attempt 1 (%v)", avg2, avg1)
	}
}

func TestRetryParseRetryAfterHeader(t *testing.T) {
	t.Parallel()
	cfg := NewRetryConfig(0, 100*time.Millisecond, 60*time.Second)

	resp := &http.Response{
		Header: http.Header{},
	}
	resp.Header.Set("Retry-After", "5")

	delay := cfg.backoffDelay(0, resp)
	if delay != 5*time.Second {
		t.Errorf("backoffDelay with Retry-After=5 returned %v, want 5s", delay)
	}
}

func TestRetryParseRetryDelayFromMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		msg  string
		want time.Duration
	}{
		{"retry after 2 seconds", 2 * time.Second},
		{"try again in 500 ms", 500 * time.Millisecond},
		{"retry in 1.5 seconds", 1500 * time.Millisecond},
		{"no delay hint here", 0},
	}
	for _, tc := range tests {
		got := parseRetryDelay(tc.msg)
		if got != tc.want {
			t.Errorf("parseRetryDelay(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestNewRetryConfig(t *testing.T) {
	t.Parallel()
	rc := NewRetryConfig(5, 200*time.Millisecond, 10*time.Second, 429, 500)
	if rc.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", rc.MaxRetries)
	}
	if rc.BaseDelay != 200*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 200ms", rc.BaseDelay)
	}
	if rc.MaxDelay != 10*time.Second {
		t.Errorf("MaxDelay = %v, want 10s", rc.MaxDelay)
	}
	if len(rc.RetryOn) != 2 {
		t.Fatalf("RetryOn has %d codes, want 2", len(rc.RetryOn))
	}
	if rc.RetryOn[0] != 429 || rc.RetryOn[1] != 500 {
		t.Errorf("RetryOn = %v, want [429 500]", rc.RetryOn)
	}
}

func TestDoWithRetrySuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	rc := NewRetryConfig(3, 10*time.Millisecond, 50*time.Millisecond, 429, 500)
	logger := slog.Default()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)

	resp, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err != nil {
		t.Fatalf("DoWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestDoWithRetryRetriesOn500ThenSucceeds(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	rc := NewRetryConfig(3, 10*time.Millisecond, 50*time.Millisecond, 500)
	logger := slog.Default()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)

	resp, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err != nil {
		t.Fatalf("DoWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestDoWithRetryExhaustsRetries(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	rc := NewRetryConfig(2, 5*time.Millisecond, 20*time.Millisecond, 429)
	logger := slog.Default()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)

	_, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err == nil {
		t.Fatal("DoWithRetry should fail after exhausting retries")
	}
	// Initial attempt + 2 retries = 3 total
	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestDoWithRetryNoRetryOn400(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	rc := NewRetryConfig(3, 10*time.Millisecond, 50*time.Millisecond, 429, 500)
	logger := slog.Default()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)

	resp, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err != nil {
		t.Fatalf("DoWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", resp.StatusCode)
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1 (no retry for 400)", n)
	}
}

func TestDoWithRetryContextCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	rc := NewRetryConfig(10, 1*time.Second, 10*time.Second, 429)
	logger := slog.Default()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)

	done := make(chan error, 1)
	go func() {
		_, err := DoWithRetry(ctx, http.DefaultClient, req, rc, logger)
		done <- err
	}()

	// Cancel after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err == nil {
		t.Fatal("DoWithRetry should fail when context is cancelled")
	}
}

func TestDoWithRetryRespectsRetryAfter(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rc := NewRetryConfig(2, 10*time.Millisecond, 5*time.Second, 429)
	logger := slog.Default()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)

	resp, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err != nil {
		t.Fatalf("DoWithRetry: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestDoWithRetryBodyReplay(t *testing.T) {
	t.Parallel()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [256]byte
		n, _ := r.Body.Read(buf[:])
		bodies = append(bodies, string(buf[:n]))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rc := NewRetryConfig(0, 10*time.Millisecond, 50*time.Millisecond)
	logger := slog.Default()

	body := `{"test":"data"}`
	req, _ := http.NewRequestWithContext(context.Background(), "POST", srv.URL, nil)
	req.Body = http.NoBody
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}

	resp, err := DoWithRetry(context.Background(), http.DefaultClient, req, rc, logger)
	if err != nil {
		t.Fatalf("DoWithRetry: %v", err)
	}
	defer resp.Body.Close()
}

func TestCryptoRandDurationZero(t *testing.T) {
	t.Parallel()
	if v := types.CryptoRandDuration(0); v != 0 {
		t.Errorf("CryptoRandDuration(0) = %v, want 0", v)
	}
}

func TestCryptoRandDurationNegative(t *testing.T) {
	t.Parallel()
	if v := types.CryptoRandDuration(-1); v != 0 {
		t.Errorf("CryptoRandDuration(-1) = %v, want 0", v)
	}
}

func TestCryptoRandDurationRange(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		v := types.CryptoRandDuration(10 * time.Second)
		if v < 0 || v >= 10*time.Second {
			t.Errorf("CryptoRandDuration(10s) = %v, want [0, 10s)", v)
		}
	}
}

func TestBackoffDelayRetryAfterDate(t *testing.T) {
	t.Parallel()
	cfg := NewRetryConfig(0, 100*time.Millisecond, 60*time.Second)
	resp := &http.Response{Header: http.Header{}}
	// Set Retry-After to a date 2 seconds in the future
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	resp.Header.Set("Retry-After", future)

	delay := cfg.backoffDelay(0, resp)
	if delay < 1*time.Second || delay > 3*time.Second {
		t.Errorf("backoffDelay with Retry-After date = %v, want ~2s", delay)
	}
}

func TestBackoffDelayMaxCap(t *testing.T) {
	t.Parallel()
	cfg := NewRetryConfig(0, 1*time.Second, 500*time.Millisecond)
	// Even with large attempt, delay should cap at MaxDelay
	delay := cfg.backoffDelay(20, nil)
	if delay > cfg.MaxDelay {
		t.Errorf("backoffDelay(20) = %v, should be capped at %v", delay, cfg.MaxDelay)
	}
}
