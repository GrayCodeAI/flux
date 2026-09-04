package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBatchSubmitSendsCorrectFormat(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]interface{}
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "batch_123"})
	}))
	defer srv.Close()

	bc := NewBatchClient("test-key", srv.URL)
	requests := []BatchRequest{
		{
			CustomID: "req-1",
			Messages: []GraycodeRouterMessage{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
			Options: ChatOptions{Model: "claude-sonnet-4-6", MaxTokens: 1024},
		},
	}

	batchID, err := bc.Submit(context.Background(), requests)
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if batchID != "batch_123" {
		t.Errorf("batchID = %q, want %q", batchID, "batch_123")
	}

	// Verify headers
	if receivedHeaders.Get("X-Api-Key") != "test-key" {
		t.Errorf("X-Api-Key = %q, want %q", receivedHeaders.Get("X-Api-Key"), "test-key")
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("Anthropic-Version = %q, want 2023-06-01", receivedHeaders.Get("Anthropic-Version"))
	}

	// Verify body structure
	reqs, ok := receivedBody["requests"].([]interface{})
	if !ok || len(reqs) != 1 {
		t.Fatalf("expected 1 request in body, got %v", receivedBody["requests"])
	}
	item := reqs[0].(map[string]interface{})
	if item["custom_id"] != "req-1" {
		t.Errorf("custom_id = %v, want req-1", item["custom_id"])
	}
	params := item["params"].(map[string]interface{})
	if params["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v, want claude-sonnet-4-6", params["model"])
	}
	if params["system"] != "You are helpful." {
		t.Errorf("system = %v, want 'You are helpful.'", params["system"])
	}
}

func TestBatchPollReturnsBatchStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Poll method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BatchResult{
			ID:     "batch_456",
			Status: "ended",
		})
	}))
	defer srv.Close()

	bc := NewBatchClient("test-key", srv.URL)
	result, err := bc.Poll(context.Background(), "batch_456")
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if result.ID != "batch_456" {
		t.Errorf("result.ID = %q, want batch_456", result.ID)
	}
	if result.Status != "ended" {
		t.Errorf("result.Status = %q, want ended", result.Status)
	}
}

func TestBatchSubmitHandlesErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error": "invalid request"}`))
	}))
	defer srv.Close()

	bc := NewBatchClient("test-key", srv.URL)
	requests := []BatchRequest{
		{
			CustomID: "req-1",
			Messages: []GraycodeRouterMessage{{Role: "user", Content: "Hello"}},
			Options:  ChatOptions{Model: "claude-sonnet-4-6"},
		},
	}

	_, err := bc.Submit(context.Background(), requests)
	if err == nil {
		t.Fatal("Submit should return error on non-200 status")
	}
	if got := err.Error(); !containsStr(got, "422") {
		t.Errorf("error = %q, expected to contain '422'", got)
	}
}

func TestBatchSubmitEmptyRequests(t *testing.T) {
	t.Parallel()
	bc := NewBatchClient("test-key", "http://localhost")
	_, err := bc.Submit(context.Background(), nil)
	if err == nil {
		t.Fatal("Submit with empty requests should error")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
