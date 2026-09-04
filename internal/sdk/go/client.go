package graycoderouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL string, apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type PromptRequest struct {
	Message      string    `json:"message"`
	Model        string    `json:"model,omitempty"`
	SystemPrompt string    `json:"system_prompt,omitempty"`
	MaxTokens    int       `json:"max_tokens,omitempty"`
	Stream       bool      `json:"stream,omitempty"`
	Tools        []ToolDef `json:"tools,omitempty"`
}

type AliasResult struct {
	Alias  string `json:"alias"`
	NodeID string `json:"node_id"`
}

type PromptResponse struct {
	Content string `json:"content"`
	NodeID  string `json:"node_id"`
}

type StreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Node struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id,omitempty"`
	RootID    string `json:"root_id,omitempty"`
	Sequence  int    `json:"sequence"`
	NodeType  string `json:"node_type"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
}

type APIError struct {
	StatusCode int
	Path       string
	Method     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("graycode-router: %s %s: %d %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func (c *Client) Prompt(ctx context.Context, req PromptRequest) (*PromptResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("use StreamPrompt for streaming requests")
	}
	var resp PromptResponse
	err := c.post(ctx, "/prompt", req, &resp)
	return &resp, err
}

func (c *Client) StreamPrompt(ctx context.Context, req PromptRequest) (<-chan StreamEvent, error) {
	req.Stream = true
	events := make(chan StreamEvent, 64)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/prompt", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Path: "/prompt", Method: "POST", Body: string(body)}
	}
	go func() {
		defer resp.Body.Close()
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var evt struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				continue
			}
			events <- StreamEvent{Type: evt.Type, Data: evt.Data}
			if evt.Type == "done" || evt.Type == "error" {
				return
			}
		}
	}()
	return events, nil
}

func (c *Client) PromptFrom(ctx context.Context, nodeID string, req PromptRequest) (*PromptResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("use StreamPromptFrom for streaming requests")
	}
	var resp PromptResponse
	err := c.post(ctx, "/nodes/"+nodeID+"/prompt", req, &resp)
	return &resp, err
}

func (c *Client) StreamPromptFrom(ctx context.Context, nodeID string, req PromptRequest) (<-chan StreamEvent, error) {
	req.Stream = true
	events := make(chan StreamEvent, 64)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	path := "/nodes/" + nodeID + "/prompt"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Path: path, Method: "POST", Body: string(body)}
	}
	go func() {
		defer resp.Body.Close()
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var evt struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal([]byte(data), &evt); err != nil {
				continue
			}
			events <- StreamEvent{Type: evt.Type, Data: evt.Data}
			if evt.Type == "done" || evt.Type == "error" {
				return
			}
		}
	}()
	return events, nil
}

func (c *Client) ListConversations(ctx context.Context) ([]Node, error) {
	var nodes []Node
	err := c.get(ctx, "/nodes", &nodes)
	return nodes, err
}

func (c *Client) GetNode(ctx context.Context, id string) (*Node, error) {
	var node Node
	err := c.get(ctx, "/nodes/"+id, &node)
	return &node, err
}

func (c *Client) GetTree(ctx context.Context, id string) ([]Node, error) {
	var nodes []Node
	err := c.get(ctx, "/nodes/"+id+"/tree", &nodes)
	return nodes, err
}

func (c *Client) DeleteNode(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/nodes/"+id, nil, nil)
}

func (c *Client) CreateAlias(ctx context.Context, nodeID, alias string) (*AliasResult, error) {
	var result AliasResult
	err := c.put(ctx, "/nodes/"+nodeID+"/aliases/"+alias, nil, &result)
	return &result, err
}

func (c *Client) DeleteAlias(ctx context.Context, alias string) error {
	return c.do(ctx, "DELETE", "/aliases/"+alias, nil, nil)
}

func (c *Client) Health(ctx context.Context) error {
	return c.get(ctx, "/health", nil)
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	return c.do(ctx, "GET", path, nil, out)
}

func (c *Client) post(ctx context.Context, path string, body, out interface{}) error {
	return c.do(ctx, "POST", path, body, out)
}

func (c *Client) put(ctx context.Context, path string, body, out interface{}) error {
	return c.do(ctx, "PUT", path, body, out)
}

func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{StatusCode: resp.StatusCode, Path: path, Method: method, Body: string(b)}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
