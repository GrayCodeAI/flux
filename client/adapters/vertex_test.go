package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestNewVertexClient(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("my-project", "us-central1", "ya29.token")
	if c == nil {
		t.Fatal("NewVertexClient returned nil")
	}
	if c.projectID != "my-project" {
		t.Errorf("projectID = %q", c.projectID)
	}
	if c.region != "us-central1" {
		t.Errorf("region = %q", c.region)
	}
	if c.token != "ya29.token" {
		t.Errorf("token = %q", c.token)
	}
}

func TestVertexClient_Name(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("p", "us-central1", "tok")
	if c.Name() != "anthropic-vertex" {
		t.Errorf("Name() = %q, want anthropic-vertex", c.Name())
	}
}

func TestVertexClient_BaseURL(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("my-project", "us-central1", "tok")
	expected := "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/anthropic/models"
	if c.BaseURL() != expected {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), expected)
	}
}

func TestVertexClient_RegionAndProject(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "europe-west4", "tok")
	if c.Region() != "europe-west4" {
		t.Errorf("Region = %q", c.Region())
	}
	if c.ProjectID() != "proj" {
		t.Errorf("ProjectID = %q", c.ProjectID())
	}
}

func TestVertexClient_Chat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "msg_vertex_1", "type": "message", "role": "assistant",
			"content":     []map[string]string{{"type": "text", "text": "Hello Vertex!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 5, "output_tokens": 10},
		}), nil
	})

	c := NewVertexClient("proj", "us-central1", "tok")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}

	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello Vertex!" {
		t.Errorf("content = %q, want Hello Vertex!", resp.Content)
	}
}

func TestVertexClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "tok")
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestVertexClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "permission denied"},
		}), nil
	})

	c := NewVertexClient("proj", "us-central1", "bad-tok")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}

	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error for forbidden")
	}
}

func TestVertexClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello Vertex stream!\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	c := NewVertexClient("proj", "us-central1", "tok")
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}

	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "claude-sonnet-4-20250514", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()

	var content string
	for event := range result.Events {
		if event.Type == "error" {
			t.Fatalf("unexpected error: %s", event.Error)
		}
		if event.Type == "content" {
			content += event.Content
		}
	}
	if content != "Hello Vertex stream!" {
		t.Errorf("content = %q, want Hello Vertex stream!", content)
	}
}

func TestVertexClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "tok")
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestVertexClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"models": []map[string]any{{"name": "claude-sonnet-4-20250514"}},
		}), nil
	})

	c := NewVertexClient("proj", "us-central1", "tok")
	c.httpClient = &http.Client{Transport: transport}

	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestVertexClient_Ping_AuthError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"message": "invalid credentials"},
		}), nil
	})

	c := NewVertexClient("proj", "us-central1", "bad-tok")
	c.httpClient = &http.Client{Transport: transport}

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestVertexClient_SetHTTPClientAndRetry(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "tok")
	c2 := NewVertexClient("proj", "us-central1", "tok")

	c.SetHTTPClient(c2.httpClient)
	if c.httpClient != c2.httpClient {
		t.Error("SetHTTPClient did not replace client")
	}
	if c.HTTPClient() != c2.httpClient {
		t.Error("HTTPClient getter mismatch")
	}

	rc := core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 7}}
	c.SetRetry(rc)
	if c.retry.MaxRetries != 7 {
		t.Errorf("expected MaxRetries=7, got %d", c.retry.MaxRetries)
	}
}

func TestVertexClient_buildBody(t *testing.T) {
	t.Parallel()
	c := NewVertexClient("proj", "us-central1", "tok")

	body, err := c.BuildBody([]core.EyrieMessage{
		{Role: "user", Content: "hello"},
	}, core.ChatOptions{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 256,
		System:    "Be helpful",
	}, false)
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}

	var parsed map[string]interface{}
	if err := jsonDecodeRequest(&http.Request{Body: io.NopCloser(strings.NewReader(string(body)))}, &parsed); err != nil {
		t.Fatalf("parse body: %v", err)
	}

	if parsed["anthropic_version"] != "vertex-2023-10-16" {
		t.Errorf("expected anthropic_version, got %v", parsed["anthropic_version"])
	}
	if parsed["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("model = %v", parsed["model"])
	}
	if parsed["max_tokens"] != float64(256) {
		t.Errorf("max_tokens = %v", parsed["max_tokens"])
	}
}
