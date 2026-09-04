//nolint:bodyclose,noctx
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client"
)

// Node, alias, prompt-from, rate-limit, error-simulation, content-type,
// concurrency, and tree endpoint integration tests. Split out of
// integration_test.go for clarity.
// --- Node Management Integration Tests ---

func TestNodes_CreatePromptAndGetNode(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a conversation via prompt
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"test conversation","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	nodeID := result["node_id"].(string)
	if nodeID == "" {
		t.Fatal("expected node_id")
	}

	// Retrieve the node
	resp, err = http.Get(ts.URL + "/nodes/" + nodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNodes_ListAfterMultiplePrompts(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create multiple conversations
	for i := 0; i < 3; i++ {
		resp, err := http.Post(ts.URL+"/prompt", "application/json",
			jsonBody(fmt.Sprintf(`{"message":"conv-%d","model":"test"}`, i)))
		if err != nil {
			t.Fatal(err)
		}
		drainBody(t, resp)
	}

	// List nodes
	resp, err := http.Get(ts.URL + "/nodes")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var nodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if len(nodes) != 3 {
		t.Errorf("expected 3 root nodes, got %d", len(nodes))
	}
}

func TestNodes_DeleteNode(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"to delete","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	// Delete it
	req, _ := http.NewRequestWithContext(context.Background(), "DELETE", ts.URL+"/nodes/"+nodeID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify it's gone
	resp, err = http.Get(ts.URL + "/nodes/" + nodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestNodes_GetNonExistent_Returns404(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nodes/nonexistent-id-12345")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent node, got %d", resp.StatusCode)
	}
}

// --- Alias Integration Tests ---

func TestAlias_CreateAndGet(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"alias test","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	// Create alias
	req, _ := http.NewRequestWithContext(context.Background(), "PUT",
		ts.URL+"/nodes/"+nodeID+"/aliases/my-alias", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	drainBody(t, resp)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for alias creation, got %d", resp.StatusCode)
	}

	// Get node by alias
	resp, err = http.Get(ts.URL + "/nodes/my-alias")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for alias lookup, got %d", resp.StatusCode)
	}
}

func TestAlias_Delete(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"alias delete test","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	// Create alias
	req, _ := http.NewRequestWithContext(context.Background(), "PUT",
		ts.URL+"/nodes/"+nodeID+"/aliases/temp-alias", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	drainBody(t, resp)

	// Delete alias
	req, _ = http.NewRequestWithContext(context.Background(), "DELETE",
		ts.URL+"/aliases/temp-alias", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for alias deletion, got %d", resp.StatusCode)
	}
}

// --- PromptFrom Endpoint Tests ---

func TestPromptFrom_ContinueConversation(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Start a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"start","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	// Continue from the assistant node
	resp, err = http.Post(ts.URL+"/nodes/"+nodeID+"/prompt", "application/json",
		jsonBody(`{"message":"continue","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for PromptFrom, got %d", resp.StatusCode)
	}

	var contResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&contResult); err != nil {
		t.Fatal(err)
	}

	if contResult["node_id"] == nil || contResult["node_id"] == "" {
		t.Error("expected node_id in continuation response")
	}
}

func TestPromptFrom_StreamingContinuation(t *testing.T) {
	ts := testServerWithProvider(t, &streamingProvider{})
	defer ts.Close()

	// Start a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"start","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	nodeID := result["node_id"].(string)

	// Continue with streaming
	resp, err = http.Post(ts.URL+"/nodes/"+nodeID+"/prompt", "application/json",
		jsonBody(`{"message":"continue","model":"test","stream":true}`))
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

	// Verify we get SSE events
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var eventCount int
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			eventCount++
		}
	}

	if eventCount < 2 {
		t.Errorf("expected at least 2 SSE events, got %d", eventCount)
	}
}

// --- Rate Limit / 429 Simulation Tests ---

// rateLimitProvider simulates a 429-like behavior by returning an error
// that would typically come from a rate-limited provider.
type rateLimitProvider struct {
	callCount int
	limit     int
}

func (r *rateLimitProvider) Name() string                 { return "ratelimit" }
func (r *rateLimitProvider) Ping(_ context.Context) error { return nil }
func (r *rateLimitProvider) Chat(_ context.Context, _ []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	r.callCount++
	if r.callCount > r.limit {
		return nil, fmt.Errorf("429 Too Many Requests: rate limit exceeded")
	}
	return &client.GraycodeRouterResponse{Content: "ok", FinishReason: "end_turn", Usage: &client.GraycodeRouterUsage{CompletionTokens: 1}}, nil
}

func (r *rateLimitProvider) StreamChat(_ context.Context, _ []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	r.callCount++
	if r.callCount > r.limit {
		return nil, fmt.Errorf("429 Too Many Requests: rate limit exceeded")
	}
	ch := make(chan client.GraycodeRouterStreamEvent, 2)
	ch <- client.GraycodeRouterStreamEvent{Type: "content", Content: "ok"}
	ch <- client.GraycodeRouterStreamEvent{Type: "done", StopReason: "end_turn", Usage: &client.GraycodeRouterUsage{CompletionTokens: 1}}
	close(ch)
	return &client.StreamResult{Events: ch}, nil
}

func TestPrompt_RateLimitSimulation(t *testing.T) {
	provider := &rateLimitProvider{limit: 1}
	ts := testServerWithProvider(t, provider)
	defer ts.Close()

	// First request succeeds
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"first","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	drainBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", resp.StatusCode)
	}

	// Second request hits rate limit
	resp, err = http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"second","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	// The server returns 500 since the provider error propagates
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("second request: expected 500, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	errMsg := result["error"].(string)
	if !strings.Contains(errMsg, "rate limit") {
		t.Errorf("expected rate limit error, got %v", errMsg)
	}
}

// --- Provider Error 500 Simulation ---

func TestPrompt_InternalServerError(t *testing.T) {
	provider := &errorProvider{err: fmt.Errorf("internal server error: model overloaded")}
	ts := testServerWithProvider(t, provider)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"hello","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	result := parseJSON(t, resp.Body)
	if !strings.Contains(result["error"].(string), "model overloaded") {
		t.Errorf("expected error to contain 'model overloaded', got %v", result["error"])
	}
}

// --- Content-Type Validation ---

func TestPrompt_WrongContentType(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Go's JSON decoder does not enforce Content-Type; the server accepts
	// valid JSON regardless of Content-Type header. Verify it still works.
	resp, err := http.Post(ts.URL+"/prompt", "text/plain", jsonBody(`{"message":"hello","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (server parses JSON regardless of Content-Type), got %d", resp.StatusCode)
	}
}

// --- Concurrent Request Tests ---

func TestPrompt_ConcurrentRequests(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	const numRequests = 10
	errs := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			resp, err := http.Post(ts.URL+"/prompt", "application/json",
				jsonBody(fmt.Sprintf(`{"message":"concurrent-%d","model":"test"}`, idx)))
			if err != nil {
				errs <- fmt.Errorf("request %d: %v", idx, err)
				return
			}
			drainBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				errs <- fmt.Errorf("request %d: expected 200, got %d", idx, resp.StatusCode)
				return
			}
			errs <- nil
		}(i)
	}

	for i := 0; i < numRequests; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

// --- Tree Endpoint Tests ---

func TestTree_ReturnsSubtree(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a conversation
	resp, err := http.Post(ts.URL+"/prompt", "application/json",
		jsonBody(`{"message":"tree root","model":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	assistantNodeID := result["node_id"].(string)

	// Get the root user node from /nodes
	resp, err = http.Get(ts.URL + "/nodes")
	if err != nil {
		t.Fatal(err)
	}
	var nodes []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&nodes)
	_ = resp.Body.Close()

	if len(nodes) == 0 {
		t.Fatal("expected at least 1 root node")
	}
	rootNodeID := nodes[0]["id"].(string)

	// Verify user node != assistant node
	if rootNodeID == assistantNodeID {
		t.Error("root node should differ from assistant node")
	}

	// Get tree from root
	resp, err = http.Get(ts.URL + "/nodes/" + rootNodeID + "/tree")
	if err != nil {
		t.Fatal(err)
	}
	defer drainBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var treeNodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&treeNodes); err != nil {
		t.Fatal(err)
	}

	// Should contain the user node and the assistant node
	if len(treeNodes) < 2 {
		t.Errorf("expected at least 2 nodes in tree, got %d", len(treeNodes))
	}
}
