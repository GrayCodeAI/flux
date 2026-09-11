//nolint:bodyclose,noctx
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/storage"
)

type mockProv struct{}

func (m *mockProv) Name() string                 { return "mock" }
func (m *mockProv) Ping(_ context.Context) error { return nil }
func (m *mockProv) Chat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.EyrieResponse, error) {
	return &client.EyrieResponse{Content: "hi", FinishReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 2}}, nil
}

func (m *mockProv) StreamChat(_ context.Context, _ []client.EyrieMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	ch := make(chan client.EyrieStreamEvent, 2)
	ch <- client.EyrieStreamEvent{Type: "content", Content: "hi"}
	ch <- client.EyrieStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.EyrieUsage{CompletionTokens: 2}}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := NewServer(Config{Store: store, Provider: &mockProv{}})
	return httptest.NewServer(srv)
}

func drainBody(t *testing.T, resp *http.Response) {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
}

func TestHealth(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPromptAndList(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()

	body := `{"message":"hello","model":"test"}`
	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("prompt: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if result["content"] != "hi" {
		t.Errorf("expected 'hi', got %v", result["content"])
	}
	nodeID, _ := result["node_id"].(string)
	if nodeID == "" {
		t.Error("expected node_id")
	}

	resp, err = http.Get(ts.URL + "/nodes")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var nodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Errorf("expected 1 root, got %d", len(nodes))
	}
}

func TestGetNodeAndTree(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()

	body := `{"message":"test","model":"m"}`
	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	resp, err = http.Get(ts.URL + "/nodes/" + nodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("get node: %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/nodes/" + nodeID + "/tree")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("get tree: %d", resp.StatusCode)
	}
}

func TestDeleteNode(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()

	body := `{"message":"del","model":"m"}`
	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	req, err := http.NewRequestWithContext(context.Background(), "DELETE", ts.URL+"/nodes/"+nodeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
}

func TestAuthRequired(t *testing.T) {
	t.Parallel()
	store, _ := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	t.Cleanup(func() { _ = store.Close() })
	srv := NewServer(Config{Store: store, Provider: &mockProv{}, APIKey: "secret"})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 without key, got %d", resp.StatusCode)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/prompt", strings.NewReader(`{"message":"hi","model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with key, got %d", resp.StatusCode)
	}
}

func TestAuthAcceptsXAPIKey(t *testing.T) {
	t.Parallel()
	store, _ := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	t.Cleanup(func() { _ = store.Close() })
	srv := NewServer(Config{Store: store, Provider: &mockProv{}, APIKey: "secret"})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/prompt", strings.NewReader(`{"message":"hi","model":"m"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with X-API-Key, got %d", resp.StatusCode)
	}
}

func TestPromptRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()

	body := `{"message":"` + strings.Repeat("x", maxRequestBodyBytes+1) + `"}`
	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", resp.StatusCode)
	}
}

func TestPromptRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json", strings.NewReader(`{"message":"hi","unexpected":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", resp.StatusCode)
	}
}
