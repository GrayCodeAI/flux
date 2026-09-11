package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCassette(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-cassette.json")

	c := &Cassette{
		Name:       "test",
		RecordedAt: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
		Provider:   "openai",
		Interactions: []Interaction{
			{
				Request: RecordedRequest{
					Messages: []EyrieMessage{
						{Role: "user", Content: "Hello"},
					},
					Model:  "gpt-4o",
					System: "You are helpful.",
				},
				Response: RecordedResponse{
					Content:      "Hi there!",
					FinishReason: "end_turn",
					Usage:        &EyrieUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
				},
			},
		},
	}

	if err := SaveCassette(c, path); err != nil {
		t.Fatalf("SaveCassette: %v", err)
	}

	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette: %v", err)
	}

	if loaded.Name != "test" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test")
	}
	if loaded.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", loaded.Provider, "openai")
	}
	if len(loaded.Interactions) != 1 {
		t.Fatalf("Interactions length = %d, want 1", len(loaded.Interactions))
	}

	msg := loaded.Interactions[0].Request.Messages[0]
	if msg.Role != "user" || msg.Content != "Hello" {
		t.Errorf("Message = %+v, want role=user content=Hello", msg)
	}
	if loaded.Interactions[0].Request.Model != "gpt-4o" {
		t.Errorf("Model = %q, want %q", loaded.Interactions[0].Request.Model, "gpt-4o")
	}
	if loaded.Interactions[0].Response.Content != "Hi there!" {
		t.Errorf("Response.Content = %q, want %q", loaded.Interactions[0].Response.Content, "Hi there!")
	}
	if loaded.Interactions[0].Response.Usage == nil {
		t.Fatal("Response.Usage is nil")
	}
	if loaded.Interactions[0].Response.Usage.TotalTokens != 8 {
		t.Errorf("Usage.TotalTokens = %d, want 8", loaded.Interactions[0].Response.Usage.TotalTokens)
	}
}

func TestSaveCassetteCreatesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "deep", "cassette.json")

	c := &Cassette{Name: "nested"}
	if err := SaveCassette(c, path); err != nil {
		t.Fatalf("SaveCassette: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestLoadCassetteMissingFile(t *testing.T) {
	t.Parallel()
	_, err := LoadCassette("/nonexistent/path/cassette.json")
	if err == nil {
		t.Fatal("LoadCassette should fail for missing file")
	}
}

func TestLoadCassetteInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadCassette(path)
	if err == nil {
		t.Fatal("LoadCassette should fail for invalid JSON")
	}
}

func TestCassetteMultipleInteractions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.json")

	c := &Cassette{
		Name:     "multi",
		Provider: "anthropic",
		Interactions: []Interaction{
			{
				Request:  RecordedRequest{Messages: []EyrieMessage{{Role: "user", Content: "Q1"}}, Model: "claude-3"},
				Response: RecordedResponse{Content: "A1"},
			},
			{
				Request:  RecordedRequest{Messages: []EyrieMessage{{Role: "user", Content: "Q2"}}, Model: "claude-3"},
				Response: RecordedResponse{Content: "A2"},
			},
			{
				Request:  RecordedRequest{Messages: []EyrieMessage{{Role: "user", Content: "Q3"}}, Model: "claude-3"},
				Response: RecordedResponse{Content: "A3"},
			},
		},
	}

	if err := SaveCassette(c, path); err != nil {
		t.Fatalf("SaveCassette: %v", err)
	}

	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette: %v", err)
	}

	if len(loaded.Interactions) != 3 {
		t.Fatalf("Interactions length = %d, want 3", len(loaded.Interactions))
	}
	for i, want := range []string{"A1", "A2", "A3"} {
		if loaded.Interactions[i].Response.Content != want {
			t.Errorf("Interactions[%d].Response.Content = %q, want %q", i, loaded.Interactions[i].Response.Content, want)
		}
	}
}

func TestCassetteWithToolCalls(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")

	c := &Cassette{
		Name:     "tool-calls",
		Provider: "openai",
		Interactions: []Interaction{
			{
				Request: RecordedRequest{
					Messages: []EyrieMessage{{Role: "user", Content: "Use a tool"}},
					Model:    "gpt-4o",
				},
				Response: RecordedResponse{
					ToolCalls: []ToolCall{
						{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"query": "test"}},
					},
					FinishReason: "tool_use",
				},
			},
		},
	}

	if err := SaveCassette(c, path); err != nil {
		t.Fatalf("SaveCassette: %v", err)
	}

	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette: %v", err)
	}

	tc := loaded.Interactions[0].Response.ToolCalls
	if len(tc) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(tc))
	}
	if tc[0].Name != "search" {
		t.Errorf("ToolCalls[0].Name = %q, want %q", tc[0].Name, "search")
	}
	if tc[0].Arguments["query"] != "test" {
		t.Errorf("ToolCalls[0].Arguments[query] = %v, want %q", tc[0].Arguments["query"], "test")
	}
}

func TestCassetteWithError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "error.json")

	c := &Cassette{
		Name:     "error-case",
		Provider: "openai",
		Interactions: []Interaction{
			{
				Request: RecordedRequest{
					Messages: []EyrieMessage{{Role: "user", Content: "fail"}},
					Model:    "gpt-4o",
				},
				Response: RecordedResponse{
					Error: "rate limit exceeded",
				},
			},
		},
	}

	if err := SaveCassette(c, path); err != nil {
		t.Fatalf("SaveCassette: %v", err)
	}

	loaded, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette: %v", err)
	}
	if loaded.Interactions[0].Response.Error != "rate limit exceeded" {
		t.Errorf("Error = %q, want %q", loaded.Interactions[0].Response.Error, "rate limit exceeded")
	}
}

func TestRequestHashDeterministic(t *testing.T) {
	t.Parallel()
	messages := []EyrieMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	opts := ChatOptions{Model: "gpt-4o", System: "test"}

	h1 := requestHash(messages, opts)
	h2 := requestHash(messages, opts)
	if h1 != h2 {
		t.Errorf("requestHash not deterministic: %q != %q", h1, h2)
	}
}

func TestRequestHashDifferentInputs(t *testing.T) {
	t.Parallel()
	opts := ChatOptions{Model: "gpt-4o"}

	h1 := requestHash([]EyrieMessage{{Role: "user", Content: "Hello"}}, opts)
	h2 := requestHash([]EyrieMessage{{Role: "user", Content: "Different"}}, opts)
	if h1 == h2 {
		t.Error("requestHash should differ for different messages")
	}
}

func TestRequestHashDifferentModel(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "Hello"}}

	h1 := requestHash(msgs, ChatOptions{Model: "gpt-4o"})
	h2 := requestHash(msgs, ChatOptions{Model: "claude-3"})
	if h1 == h2 {
		t.Error("requestHash should differ for different models")
	}
}

func TestRequestHashDifferentSystem(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "Hello"}}

	h1 := requestHash(msgs, ChatOptions{Model: "gpt-4o", System: "You are helpful"})
	h2 := requestHash(msgs, ChatOptions{Model: "gpt-4o", System: "You are a pirate"})
	if h1 == h2 {
		t.Error("requestHash should differ for different system prompts")
	}
}

func TestRequestHashExcludesTemperature(t *testing.T) {
	t.Parallel()
	msgs := []EyrieMessage{{Role: "user", Content: "Hello"}}
	t1 := 0.5
	t2 := 0.9

	h1 := requestHash(msgs, ChatOptions{Model: "gpt-4o", Temperature: &t1})
	h2 := requestHash(msgs, ChatOptions{Model: "gpt-4o", Temperature: &t2})
	if h1 != h2 {
		t.Error("requestHash should not include temperature")
	}
}
