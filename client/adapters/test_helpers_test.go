package adapters

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, payload any) *http.Response {
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func jsonDecodeRequest(req *http.Request, value any) error {
	if req.Body == nil {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}

func float64Ptr(v float64) *float64 { return &v }
