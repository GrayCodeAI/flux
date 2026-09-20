package core

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultTransportReturnsNonNil(t *testing.T) {
	t.Parallel()
	tr := defaultTransport()
	if tr == nil {
		t.Fatal("defaultTransport() returned nil")
	}
}

func TestDefaultTransportIsSingleton(t *testing.T) {
	t.Parallel()
	a := defaultTransport()
	b := defaultTransport()
	if a != b {
		t.Fatal("defaultTransport() returned different instances; expected singleton")
	}
}

func TestDefaultTransportIdleConnSettings(t *testing.T) {
	t.Parallel()
	tr := defaultTransport()
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 20 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 20", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
}

func TestDefaultTransportTLSSettings(t *testing.T) {
	t.Parallel()
	tr := defaultTransport()
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
	if tr.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("ExpectContinueTimeout = %v, want 1s", tr.ExpectContinueTimeout)
	}
}

func TestNewPooledHTTPClientTimeout(t *testing.T) {
	t.Parallel()
	c := NewPooledHTTPClient(30 * time.Second)
	if c.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.Timeout)
	}
}

func TestNewPooledHTTPClientUsesSharedTransport(t *testing.T) {
	t.Parallel()
	a := NewPooledHTTPClient(1 * time.Second)
	b := NewPooledHTTPClient(2 * time.Second)

	ta, ok := a.Transport.(*http.Transport)
	if !ok {
		t.Fatal("client A transport is not *http.Transport")
	}
	tb, ok := b.Transport.(*http.Transport)
	if !ok {
		t.Fatal("client B transport is not *http.Transport")
	}
	if ta != tb {
		t.Fatal("two pooled clients do not share the same transport")
	}
}

func TestNewPooledHTTPClientSharesDefaultTransport(t *testing.T) {
	t.Parallel()
	c := NewPooledHTTPClient(5 * time.Second)
	tr := defaultTransport()
	if c.Transport != tr {
		t.Fatal("NewPooledHTTPClient does not use defaultTransport()")
	}
}
