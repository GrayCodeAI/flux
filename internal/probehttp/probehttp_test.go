package probehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     int
		wantSubstr string
	}{
		{"unauthorized", http.StatusUnauthorized, "invalid API key"},
		{"forbidden", http.StatusForbidden, "invalid API key"},
		{"rate limited", http.StatusTooManyRequests, "rate limited"},
		{"server 500", http.StatusInternalServerError, "provider unavailable"},
		{"bad gateway 502", http.StatusBadGateway, "provider unavailable"},
		{"client error 400", http.StatusBadRequest, "HTTP 400"},
		{"client error 404", http.StatusNotFound, "HTTP 404"},
		{"ok 200 still errs as HTTP 200", http.StatusOK, "HTTP 200"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ProbeError(tt.status)
			if err == nil {
				t.Fatalf("ProbeError(%d) returned nil; expected an error", tt.status)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ProbeError(%d) = %q; want substring %q", tt.status, err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestDoGet_RespondsAndBoundsBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "ok" {
			t.Errorf("missing X-Test header; got %q", r.Header.Get("X-Test"))
		}
		// Write 2 MiB; we expect DoGet to truncate to 1 MiB.
		_, _ = w.Write(make([]byte, 2<<20))
	}))
	defer srv.Close()

	status, body, err := DoGet(context.Background(), srv.URL+"/foo", map[string]string{"X-Test": "ok"})
	if err != nil {
		t.Fatalf("DoGet: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d; want 200", status)
	}
	if len(body) > (1<<20)+1024 {
		t.Errorf("body len = %d; expected <= 1 MiB + a few bytes for safety", len(body))
	}
}

func TestDoGet_RespectsContextDeadline(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, _, err := DoGet(ctx, srv.URL, nil)
	if err == nil {
		t.Fatalf("DoGet: expected error from context deadline, got nil")
	}
}

func TestJoinURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base, path, want string
	}{
		{"http://a", "b", "http://a/b"},
		{"http://a/", "b", "http://a/b"},
		{"http://a", "/b", "http://a/b"},
		{"http://a/", "/b", "http://a/b"},
		{"http://a/", "", "http://a"},
		{"http://a", "", "http://a"},
	}
	for _, tt := range tests {
		got := JoinURL(tt.base, tt.path)
		if got != tt.want {
			t.Errorf("JoinURL(%q, %q) = %q; want %q", tt.base, tt.path, got, tt.want)
		}
	}
}

func TestUserAgent(t *testing.T) {
	t.Parallel()
	if got := UserAgent(); !strings.HasPrefix(got, "eyrie-") {
		t.Errorf("UserAgent() = %q; want it to start with \"eyrie-\"", got)
	}
}

func TestDefaultClient_HasTimeout(t *testing.T) {
	t.Parallel()
	if DefaultClient.Timeout <= 0 {
		t.Errorf("DefaultClient.Timeout = %v; expected > 0 so the client bounds requests", DefaultClient.Timeout)
	}
}
