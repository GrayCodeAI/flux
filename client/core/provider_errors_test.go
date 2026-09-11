package core

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func body(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestParseProviderError(t *testing.T) {
	t.Parallel()
	d, readErr := ParseProviderError(body(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`))
	if readErr != nil {
		t.Errorf("readErr = %v, want nil", readErr)
	}
	if d.Message != "Incorrect API key provided" {
		t.Errorf("Message = %q", d.Message)
	}
	if d.Type != "invalid_request_error" {
		t.Errorf("Type = %q", d.Type)
	}
	if d.Code != "invalid_api_key" {
		t.Errorf("Code = %q", d.Code)
	}
}

func TestParseProviderError_NumericCode(t *testing.T) {
	t.Parallel()
	// Some providers send a bare numeric code.
	d, readErr := ParseProviderError(body(`{"error":{"message":"nope","code":404}}`))
	if readErr != nil {
		t.Errorf("readErr = %v, want nil", readErr)
	}
	if d.Code != "404" {
		t.Errorf("Code = %q, want 404", d.Code)
	}
}

func TestParseProviderError_Unstructured(t *testing.T) {
	t.Parallel()
	d, readErr := ParseProviderError(body(`upstream proxy: 502 bad gateway`))
	if readErr != nil {
		t.Errorf("readErr = %v, want nil", readErr)
	}
	if d.Message != "" {
		t.Errorf("Message should be empty for unstructured body, got %q", d.Message)
	}
	if !strings.Contains(d.Raw, "bad gateway") {
		t.Errorf("Raw = %q", d.Raw)
	}
}

func TestClassifyProviderError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		detail ProviderErrorDetail
		want   string // substring expected in the hint
	}{
		{"invalid key by code", 400, ProviderErrorDetail{Code: "invalid_api_key"}, "invalid API key"},
		{"model not found by code", 404, ProviderErrorDetail{Code: "model_not_found"}, "model not found"},
		{"quota by message", 429, ProviderErrorDetail{Message: "You exceeded your current quota"}, "billing/quota"},
		{"content filter", 400, ProviderErrorDetail{Type: "content_filter"}, "content filter"},
		{"context length", 400, ProviderErrorDetail{Message: "maximum context length is 8192"}, "context window"},
		{"bare 401", http.StatusUnauthorized, ProviderErrorDetail{}, "unauthorized"},
		{"bare 429", http.StatusTooManyRequests, ProviderErrorDetail{}, "rate limited"},
		{"bare 503", http.StatusServiceUnavailable, ProviderErrorDetail{}, "transient"},
		{"no hint", 400, ProviderErrorDetail{Message: "weird"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyProviderError(tc.status, tc.detail)
			if tc.want == "" {
				if got != "" {
					t.Errorf("expected no hint, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("hint = %q, want substring %q", got, tc.want)
			}
		})
	}
}

func TestFormatAPIError(t *testing.T) {
	t.Parallel()
	err := FormatAPIError("openai", "chat", 401, "req_123",
		ProviderErrorDetail{Message: "bad key", Type: "auth_error", Code: "invalid_api_key"}, nil)
	s := err.Error()
	for _, want := range []string{"openai", "chat", "HTTP 401", "request_id=req_123", "invalid API key", "bad key"} {
		if !strings.Contains(s, want) {
			t.Errorf("error %q missing %q", s, want)
		}
	}
}

func TestFormatAPIError_OmitsRequestIDWhenEmpty(t *testing.T) {
	t.Parallel()
	err := FormatAPIError("vertex", "chat", 400, "", ProviderErrorDetail{Raw: "totally opaque"}, nil)
	s := err.Error()
	if !strings.Contains(s, "totally opaque") {
		t.Errorf("error %q should include raw detail", s)
	}
	if strings.Contains(s, "request_id=") {
		t.Errorf("error %q should NOT include request_id when caller passes empty", s)
	}
}

// TestFormatAPIError_ReturnsEyrieError: the returned error is a
// concrete *EyrieError, dispatchable via errors.As. This is the
// core contract that lets retry/fallback middleware use the
// structured IsRetriable()/IsAuthError()/IsRateLimited() helpers
// instead of regex-parsing the message.
func TestFormatAPIError_ReturnsEyrieError(t *testing.T) {
	t.Parallel()
	err := FormatAPIError("openai", "chat", 429, "req_429",
		ProviderErrorDetail{Message: "rate limited"}, nil)
	var eyrieErr *EyrieError
	if !errors.As(err, &eyrieErr) {
		t.Fatalf("FormatAPIError must return *EyrieError, got %T (%v)", err, err)
	}
	if eyrieErr.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", eyrieErr.Provider)
	}
	if eyrieErr.Op != "chat" {
		t.Errorf("Op = %q, want chat", eyrieErr.Op)
	}
	if eyrieErr.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", eyrieErr.StatusCode)
	}
	if eyrieErr.RequestID != "req_429" {
		t.Errorf("RequestID = %q, want req_429", eyrieErr.RequestID)
	}
	if !eyrieErr.IsRateLimited() {
		t.Errorf("IsRateLimited = false, want true for 429")
	}
	if !eyrieErr.IsRetriable() {
		t.Errorf("IsRetriable = false, want true for 429")
	}
	if eyrieErr.IsAuthError() {
		t.Errorf("IsAuthError = true, want false for 429")
	}
}

// TestFormatAPIError_AuthError: 401/403 are flagged as auth errors
// (not retriable on the same provider).
func TestFormatAPIError_AuthError(t *testing.T) {
	t.Parallel()
	for _, status := range []int{401, 403} {
		err := FormatAPIError("openai", "chat", status, "req",
			ProviderErrorDetail{Message: "unauthorized", Code: "invalid_api_key"}, nil)
		var eyrieErr *EyrieError
		if !errors.As(err, &eyrieErr) {
			t.Fatalf("status %d: not *EyrieError: %T", status, err)
		}
		if !eyrieErr.IsAuthError() {
			t.Errorf("status %d: IsAuthError = false, want true", status)
		}
		if eyrieErr.IsRetriable() {
			t.Errorf("status %d: IsRetriable = true, want false (auth errors don't retry)", status)
		}
	}
}

// TestFormatAPIError_RetriableCodes: 5xx and 429 are retriable.
func TestFormatAPIError_RetriableCodes(t *testing.T) {
	t.Parallel()
	for _, status := range []int{408, 429, 500, 502, 503, 504, 529} {
		err := FormatAPIError("openai", "chat", status, "req",
			ProviderErrorDetail{Message: "try again"}, nil)
		var eyrieErr *EyrieError
		if !errors.As(err, &eyrieErr) {
			t.Fatalf("status %d: not *EyrieError: %T", status, err)
		}
		if !eyrieErr.IsRetriable() {
			t.Errorf("status %d: IsRetriable = false, want true", status)
		}
	}
}

// TestFormatAPIError_InnerErrorUnwrap: a non-nil inner error passed
// in (e.g. a body read error from ParseProviderError) is wired into
// EyrieError.Err, so errors.Is / errors.Unwrap traverse it. This
// fixes the contract gap where Unwrap() always returned nil even
// when the provider body failed to read.
func TestFormatAPIError_InnerErrorUnwrap(t *testing.T) {
	t.Parallel()
	inner := io.ErrUnexpectedEOF
	err := FormatAPIError("openai", "chat", 500, "req_inner",
		ProviderErrorDetail{Message: "bad gateway"}, inner)

	var eyrieErr *EyrieError
	if !errors.As(err, &eyrieErr) {
		t.Fatalf("FormatAPIError must return *EyrieError, got %T (%v)", err, err)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("errors.Is(err, io.ErrUnexpectedEOF) = false, want true (Err field must be wired)")
	}
	if !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("error %q should include inner error message", err.Error())
	}
}
