//nolint:bodyclose,noctx
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/storage"
)

// Node, alias, prompt-from, rate-limit, error-simulation, content-type,
// concurrency, and tree integration tests live in integration_nodes_test.go.

// --- Configurable mock providers ---

// errorProvider returns an error from Chat/StreamChat.
type errorProvider struct {
	err error
}

func (e *errorProvider) Name() string                 { return "error-provider" }
func (e *errorProvider) Ping(_ context.Context) error { return nil }
func (e *errorProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return nil, e.err
}

func (e *errorProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	return nil, e.err
}

// streamingProvider returns multiple content chunks.
type streamingProvider struct{}

func (s *streamingProvider) Name() string                 { return "streaming-provider" }
func (s *streamingProvider) Ping(_ context.Context) error { return nil }
func (s *streamingProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{Content: "hello world", FinishReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 2}}, nil
}

func (s *streamingProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 5)
	go func() {
		chunks := []string{"hello", " ", "world"}
		for _, c := range chunks {
			ch <- client.EyrieStreamEvent{Type: "content", Content: c}
		}
		ch <- client.EyrieStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 3}}
		close(ch)
	}()
	return &client.StreamResult{Events: ch}, nil
}

// errorStreamProvider streams a content chunk then emits an error.
type errorStreamProvider struct{}

func (e *errorStreamProvider) Name() string                 { return "error-stream" }
func (e *errorStreamProvider) Ping(_ context.Context) error { return nil }
func (e *errorStreamProvider) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return nil, fmt.Errorf("provider error")
}

func (e *errorStreamProvider) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 3)
	go func() {
		ch <- client.EyrieStreamEvent{Type: "content", Content: "partial"}
		ch <- client.EyrieStreamEvent{Type: "error", Error: "rate limit exceeded"}
		close(ch)
	}()
	return &client.StreamResult{Events: ch}, nil
}

// --- Helper functions ---

func testServerWithProvider(t *testing.T, prov client.Provider) *httptest.Server {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := NewServer(Config{Store: store, Provider: prov})
	return httptest.NewServer(srv)
}

func testServerWithAPIKey(t *testing.T, prov client.Provider, apiKey string) *httptest.Server {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := NewServer(Config{Store: store, Provider: prov, APIKey: apiKey})
	return httptest.NewServer(srv)
}

func jsonBody(msg string) io.Reader {
	return strings.NewReader(msg)
}

func parseJSON(t *testing.T, body io.Reader) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

// --- Health Endpoint Tests ---

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := parseJSON(t, resp.Body)
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
}

func TestHealthEndpoint_ReturnsJSON(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %s", ct)
	}
}

func TestHealthEndpoint_NoAuthRequired(t *testing.T) {
	// Health endpoint should work without authentication even when API key is set.
	ts := testServerWithAPIKey(t, &mockProv{}, "secret")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health should not require auth, got %d", resp.StatusCode)
	}
}

// --- Chat Endpoint Tests (non-streaming) ---

func TestPrompt_ReturnsContentAndNodeID(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", jsonBody(`{"message":"hello","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	_ = resp.Body.Close()

	if result["content"] != "hi" {
		t.Errorf("expected content 'hi', got %v", result["content"])
	}
	if result["node_id"] == nil || result["node_id"] == "" {
		t.Error("expected node_id to be set")
	}
}

func TestPrompt_MissingMessage(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", jsonBody(`{"model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	if result["error"] != "message is required" {
		t.Errorf("expected 'message is required' error, got %v", result["error"])
	}
}

func TestPrompt_InvalidJSON(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", jsonBody(`{invalid`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestPrompt_EmptyBody(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", jsonBody(""))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestPrompt_WrongMethod(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on POST endpoint, got %d", resp.StatusCode)
	}
}

func TestPrompt_MultipleRequests(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	for i := 0; i < 3; i++ {
		resp, err := http.Post(ts.URL+"/prompt", "application/json",
			jsonBody(fmt.Sprintf(`{"message":"msg-%d","model":"test"}`, i)))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
		drainBody(t, resp)
	}
}

// --- Streaming SSE Tests ---

func TestPrompt_StreamingSSE(t *testing.T) {
	ts := testServerWithProvider(t, &streamingProvider{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"hello","model":"test","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}

	cc := resp.Header.Get("Cache-Control")
	if cc != "no-cache" {
		t.Errorf("expected no-cache, got %s", cc)
	}

	// Read SSE events
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}

	if len(events) < 3 {
		t.Errorf("expected at least 3 SSE events (2 content + 1 done), got %d", len(events))
	}

	// Verify we got the content chunks and done event
	var foundContent strings.Builder
	var foundDone bool
	for _, evt := range events {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(evt), &parsed); err != nil {
			t.Errorf("failed to parse SSE event: %v (data: %s)", err, evt)
			continue
		}
		switch parsed["type"] {
		case "delta":
			foundContent.WriteString(parsed["content"].(string))
		case "done":
			foundDone = true
		}
	}

	if foundContent.String() != "hello world" {
		t.Errorf("expected content 'hello world', got '%s'", foundContent.String())
	}
	if !foundDone {
		t.Error("expected done event in stream")
	}
}

func TestPrompt_StreamingMultipleContentDeltas(t *testing.T) {
	ts := testServerWithProvider(t, &streamingProvider{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"chunk test","model":"test","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var deltaCount int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var parsed map[string]interface{}
		data := strings.TrimPrefix(line, "data: ")
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			continue
		}
		if parsed["type"] == "delta" {
			deltaCount++
		}
	}

	// streamingProvider sends 3 content chunks
	if deltaCount != 3 {
		t.Errorf("expected 3 delta events, got %d", deltaCount)
	}
}

// --- Error Handling Tests ---

func TestPrompt_ProviderError(t *testing.T) {
	ts := testServerWithProvider(t, &errorProvider{err: fmt.Errorf("provider unavailable")})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"hello","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 for provider error, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	if !strings.Contains(result["error"].(string), "provider unavailable") {
		t.Errorf("expected error to contain 'provider unavailable', got %v", result["error"])
	}
}

func TestPrompt_StreamingProviderError(t *testing.T) {
	ts := testServerWithProvider(t, &errorStreamProvider{})
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"hello","model":"test","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Streaming starts with 200, then error is sent as SSE event
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for streaming, got %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var foundError bool
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var parsed map[string]interface{}
		data := strings.TrimPrefix(line, "data: ")
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			continue
		}
		if parsed["type"] == "error" {
			foundError = true
			if !strings.Contains(parsed["error"].(string), "rate limit") {
				t.Errorf("expected rate limit error, got %v", parsed["error"])
			}
		}
	}

	if !foundError {
		t.Error("expected error event in SSE stream")
	}
}

func TestPrompt_MethodNotAllowed(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "GET", ts.URL+"/prompt", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

// --- Authentication Error Tests ---

func TestAuth_MissingKey_Returns401(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "my-secret-key")
	defer ts.Close()

	// No auth header
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	if result["error"] != "unauthorized" {
		t.Errorf("expected 'unauthorized' error, got %v", result["error"])
	}
}

func TestAuth_WrongKey_Returns401(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "correct-key")
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/prompt",
		jsonBody(`{"message":"hello","model":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong key, got %d", resp.StatusCode)
	}
}

func TestAuth_BearerPrefix(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "test-token")
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/prompt",
		jsonBody(`{"message":"hello","model":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d", resp.StatusCode)
	}
}

func TestAuth_XAPIKey(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "test-token")
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/prompt",
		jsonBody(`{"message":"hello","model":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with X-API-Key, got %d", resp.StatusCode)
	}
}

func TestAuth_ProtectedEndpoints_AllRequireAuth(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "secret")
	defer ts.Close()

	protectedEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/prompt", `{"message":"hi"}`},
		{"GET", "/nodes", ""},
		{"GET", "/nodes/test-id", ""},
		{"GET", "/nodes/test-id/tree", ""},
		{"DELETE", "/nodes/test-id", ""},
		{"GET", "/api/usage", ""},
		{"GET", "/api/costs", ""},
		{"GET", "/api/health/providers", ""},
	}

	for _, ep := range protectedEndpoints {
		var body io.Reader
		if ep.body != "" {
			body = jsonBody(ep.body)
		}
		req, _ := http.NewRequestWithContext(context.Background(), ep.method, ts.URL+ep.path, body)
		if ep.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", ep.method, ep.path, err)
		}
		drainBody(t, resp)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, resp.StatusCode)
		}
	}
}

func TestAuth_HealthEndpoint_NotProtected(t *testing.T) {
	ts := testServerWithAPIKey(t, &mockProv{}, "secret")
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health should not require auth, got %d", resp.StatusCode)
	}
}
