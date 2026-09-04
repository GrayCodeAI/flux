package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProviderErrorDetail holds the structured fields graycode-router can extract from a
// provider's error body. Providers vary (OpenAI nests under "error", some put
// a top-level "code"); the parser is lenient and fills what it can.
type ProviderErrorDetail struct {
	Message string // human-readable message from the body
	Type    string // provider error type, e.g. "invalid_request_error"
	Code    string // provider error code, e.g. "invalid_api_key", "model_not_found"
	Raw     string // raw body (truncated) when nothing structured was found
}

// ParseProviderError reads and classifies an error response body. It
// never returns a zero detail: on a read failure the detail is filled
// with a placeholder message and the read error is returned alongside
// so callers can attach it to the structured *GraycodeRouterError.
func ParseProviderError(body io.ReadCloser) (ProviderErrorDetail, error) {
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, 8192))
	if err != nil {
		return ProviderErrorDetail{Message: "failed to read error body"}, err
	}

	// Most OpenAI-compatible and Anthropic errors nest the detail under "error".
	var nested struct {
		Error struct {
			Message string          `json:"message"`
			Type    string          `json:"type"`
			Code    json.RawMessage `json:"code"` // string or number across providers
		} `json:"error"`
		// Some providers (and Anthropic top-level) also expose these.
		Message string `json:"message"`
		Type    string `json:"type"`
	}
	d := ProviderErrorDetail{Raw: string(data)}
	if json.Unmarshal(data, &nested) == nil {
		switch {
		case nested.Error.Message != "":
			d.Message = nested.Error.Message
			d.Type = nested.Error.Type
		case nested.Message != "":
			d.Message = nested.Message
			d.Type = nested.Type
		}
		d.Code = rawToString(nested.Error.Code)
	}
	return d, nil
}

// rawToString renders a JSON code field that may be a quoted string or a bare
// number into a plain string ("" if absent).
func rawToString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return strings.Trim(s, `"`)
}

// classifyProviderError returns a short, actionable hint for the given status
// code and parsed body, or "" when nothing specific applies. The hint leads the
// error message so an operator sees the likely cause before the raw detail.
//
// It is intentionally small and keyed on portable signals (HTTP status plus the
// common cross-provider code/type/message strings) rather than a per-provider
// table, to avoid becoming a maintenance sink across graycode-router's many providers.
func classifyProviderError(statusCode int, d ProviderErrorDetail) string {
	code := strings.ToLower(d.Code)
	typ := strings.ToLower(d.Type)
	msg := strings.ToLower(d.Message)

	has := func(subs ...string) bool {
		for _, s := range subs {
			if (code != "" && strings.Contains(code, s)) ||
				(typ != "" && strings.Contains(typ, s)) ||
				(msg != "" && strings.Contains(msg, s)) {
				return true
			}
		}
		return false
	}

	switch {
	case has("invalid_api_key", "invalid api key", "incorrect api key"):
		return "invalid API key — check the credential for this provider"
	case has("account_not_verified", "not verified", "verify your account"):
		return "account not verified — complete provider account verification"
	case has("model_not_found", "model_not_exist", "does not exist", "unknown model", "no such model"):
		return "model not found — check the model id against the provider catalog"
	case has("insufficient_quota", "insufficient_credits", "billing", "exceeded your current quota"):
		return "billing/quota problem — check the provider account's balance and limits"
	case has("content_filter", "content_policy", "responsible_ai"):
		return "request blocked by the provider's content filter"
	case has("context_length", "maximum context", "too many tokens", "reduce the length"):
		return "input exceeds the model's context window"
	}

	// Fall back to status-code-level hints when the body was unstructured.
	switch statusCode {
	case http.StatusUnauthorized:
		return "unauthorized — the API key is missing, invalid, or expired"
	case http.StatusForbidden:
		return "forbidden — the API key lacks permission for this resource"
	case http.StatusTooManyRequests:
		return "rate limited — slow down or check the provider's quota"
	case http.StatusNotFound:
		return "not found — check the endpoint/base URL and model id"
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "provider-side error — usually transient, retry shortly"
	}
	return ""
}

// FormatAPIError builds the *GraycodeRouterError used across every provider
// request path (chat, stream, embeddings). It always includes the
// provider name, the operation (e.g. "chat", "stream"), the HTTP
// status, the upstream correlation id (for support tickets), a
// classified actionable hint when one applies, and the raw detail.
//
// Returning *GraycodeRouterError (rather than a plain fmt.Errorf) lets
// downstream code use errors.As to dispatch on the structured type
// — retry middleware checks IsRetriable(), fallback chains check
// IsRetriable()/IsAuthError(), observability code can pull the
// provider/status/request-id without re-parsing the message.
//
// inner is the read error from ParseProviderError (when the body
// could not be read). It is attached via GraycodeRouterError.Err so
// errors.Is(err, io.EOF) and similar checks succeed; pass nil when
// the body was read cleanly.
func FormatAPIError(provider, op string, statusCode int, requestID string, d ProviderErrorDetail, inner error) error {
	detail := d.Message
	if detail == "" {
		detail = d.Raw
	}
	if d.Type != "" && d.Message != "" {
		detail = fmt.Sprintf("%s: %s", d.Type, d.Message)
	}

	hint := classifyProviderError(statusCode, d)
	msg := detail
	if hint != "" {
		msg = fmt.Sprintf("%s — %s", hint, detail)
	}

	return &GraycodeRouterError{
		Provider:   provider,
		Op:         op,
		StatusCode: statusCode,
		RequestID:  requestID,
		Message:    msg,
		Err:        inner,
	}
}
