package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderName(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "hello"

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "mock/recorder"
	if got := rec.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestRecorderRecordMode(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "recorded response"

	dir := t.TempDir()
	path := filepath.Join(dir, "record.json")

	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	resp, err := rec.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "recorded response" {
		t.Errorf("Content = %q, want %q", resp.Content, "recorded response")
	}

	// Save the cassette
	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify the cassette file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cassette file not found: %v", err)
	}

	// Load and verify it has one interaction
	c, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette() error: %v", err)
	}
	if len(c.Interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(c.Interactions))
	}
	if c.Interactions[0].Response.Content != "recorded response" {
		t.Errorf("recorded content = %q, want %q", c.Interactions[0].Response.Content, "recorded response")
	}
	if c.Interactions[0].Request.Model != "test-model" {
		t.Errorf("recorded model = %q, want %q", c.Interactions[0].Request.Model, "test-model")
	}

	// Verify mock was actually called
	if mock.CallCount() != 1 {
		t.Errorf("mock CallCount = %d, want 1", mock.CallCount())
	}
}

func TestRecorderReplayMode(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "original response"

	dir := t.TempDir()
	path := filepath.Join(dir, "replay.json")

	// First, record a cassette
	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hello"}}
	opts := ChatOptions{Model: "test-model"}

	_, err = rec.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() record error: %v", err)
	}
	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Now replay it with a fresh mock (should NOT be called)
	mock2 := NewMockProvider(MockModeFixed)
	mock2.Response = "should not see this"

	rec2, err := NewRecorderProvider(mock2, path, RecordModeReplay)
	if err != nil {
		t.Fatalf("unexpected error creating replay recorder: %v", err)
	}

	resp, err := rec2.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() replay error: %v", err)
	}
	if resp.Content != "original response" {
		t.Errorf("replay Content = %q, want %q", resp.Content, "original response")
	}

	// Verify mock2 was never called (replay does not call inner)
	if mock2.CallCount() != 0 {
		t.Errorf("mock2 CallCount = %d, want 0 (replay should not call inner)", mock2.CallCount())
	}
}

func TestRecorderAutoMode_RecordsWhenNoFile(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "auto recorded"

	dir := t.TempDir()
	path := filepath.Join(dir, "auto.json")

	// File does not exist, so auto mode should record
	rec, err := NewRecorderProvider(mock, path, RecordModeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.mode != RecordModeRecord {
		t.Fatalf("expected mode=record, got %s", rec.mode)
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "test"}}
	opts := ChatOptions{Model: "m"}

	resp, err := rec.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}
	if resp.Content != "auto recorded" {
		t.Errorf("Content = %q, want %q", resp.Content, "auto recorded")
	}
	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify mock was called
	if mock.CallCount() != 1 {
		t.Errorf("mock CallCount = %d, want 1", mock.CallCount())
	}
}

func TestRecorderAutoMode_ReplaysWhenFileExists(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "first"

	dir := t.TempDir()
	path := filepath.Join(dir, "auto_replay.json")

	// Create a cassette first by recording
	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "hi"}}
	opts := ChatOptions{Model: "m"}

	_, err = rec.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() record error: %v", err)
	}
	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Now create a new recorder in auto mode — file exists, so it should replay
	mock2 := NewMockProvider(MockModeFixed)
	mock2.Response = "should not see"

	rec2, err := NewRecorderProvider(mock2, path, RecordModeAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec2.mode != RecordModeReplay {
		t.Fatalf("expected mode=replay when file exists, got %s", rec2.mode)
	}

	resp, err := rec2.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() replay error: %v", err)
	}
	if resp.Content != "first" {
		t.Errorf("Content = %q, want %q", resp.Content, "first")
	}
	if mock2.CallCount() != 0 {
		t.Errorf("mock2 CallCount = %d, want 0", mock2.CallCount())
	}
}

func TestRecorderHashBasedLookup(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)

	dir := t.TempDir()
	path := filepath.Join(dir, "hash.json")

	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	opts := ChatOptions{Model: "m"}

	// Record two different interactions
	mock.Response = "response A"
	_, err = rec.Chat(ctx, []GraycodeRouterMessage{{Role: "user", Content: "question A"}}, opts)
	if err != nil {
		t.Fatalf("Chat() A error: %v", err)
	}

	mock.Response = "response B"
	_, err = rec.Chat(ctx, []GraycodeRouterMessage{{Role: "user", Content: "question B"}}, opts)
	if err != nil {
		t.Fatalf("Chat() B error: %v", err)
	}

	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Replay and look up by hash (not position) — request B first
	mock2 := NewMockProvider(MockModeFixed)
	rec2, err := NewRecorderProvider(mock2, path, RecordModeReplay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Request B first (out of order compared to recording)
	resp, err := rec2.Chat(ctx, []GraycodeRouterMessage{{Role: "user", Content: "question B"}}, opts)
	if err != nil {
		t.Fatalf("Chat() B replay error: %v", err)
	}
	if resp.Content != "response B" {
		t.Errorf("Content = %q, want %q", resp.Content, "response B")
	}

	// Request A second
	resp, err = rec2.Chat(ctx, []GraycodeRouterMessage{{Role: "user", Content: "question A"}}, opts)
	if err != nil {
		t.Fatalf("Chat() A replay error: %v", err)
	}
	if resp.Content != "response A" {
		t.Errorf("Content = %q, want %q", resp.Content, "response A")
	}
}

func TestRecorderStreamRecord(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "streamed content"

	dir := t.TempDir()
	path := filepath.Join(dir, "stream.json")

	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "stream me"}}
	opts := ChatOptions{Model: "m"}

	result, err := rec.StreamChat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("StreamChat() error: %v", err)
	}

	// Drain the stream
	var content string
	for evt := range result.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}

	if content == "" {
		t.Error("expected content from stream, got empty")
	}

	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify cassette has the interaction
	c, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette() error: %v", err)
	}
	if len(c.Interactions) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(c.Interactions))
	}
	if c.Interactions[0].Response.Content == "" {
		t.Error("recorded stream content is empty")
	}
}

func TestRecorderStreamReplay(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "replay this stream"

	dir := t.TempDir()
	path := filepath.Join(dir, "stream_replay.json")

	// Record
	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "stream"}}
	opts := ChatOptions{Model: "m"}

	result, err := rec.StreamChat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("StreamChat() record error: %v", err)
	}
	for range result.Events {
	}
	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Replay stream
	mock2 := NewMockProvider(MockModeFixed)
	mock2.Response = "should not see"

	rec2, err := NewRecorderProvider(mock2, path, RecordModeReplay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result2, err := rec2.StreamChat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("StreamChat() replay error: %v", err)
	}

	var content string
	var gotDone bool
	for evt := range result2.Events {
		switch evt.Type {
		case "content":
			content += evt.Content
		case "done":
			gotDone = true
		}
	}

	if content == "" {
		t.Error("expected content from replayed stream, got empty")
	}
	if !gotDone {
		t.Error("expected done event from replayed stream")
	}
	if mock2.CallCount() != 0 {
		t.Errorf("mock2 CallCount = %d, want 0", mock2.CallCount())
	}
}

func TestRecorderReplayModeFailsIfNoFile(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	_, err := NewRecorderProvider(mock, "/nonexistent/path/cassette.json", RecordModeReplay)
	if err == nil {
		t.Fatal("expected error for replay mode with non-existent file")
	}
}

func TestRecorderRedactor(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider(MockModeFixed)
	mock.Response = "secret: my-api-key-12345"

	dir := t.TempDir()
	path := filepath.Join(dir, "redact.json")

	rec, err := NewRecorderProvider(mock, path, RecordModeRecord)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec.SetRedactor(func(s string) string {
		return "secret: [REDACTED]"
	})

	ctx := context.Background()
	msgs := []GraycodeRouterMessage{{Role: "user", Content: "show secret"}}
	opts := ChatOptions{Model: "m"}

	_, err = rec.Chat(ctx, msgs, opts)
	if err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if err := rec.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	c, err := LoadCassette(path)
	if err != nil {
		t.Fatalf("LoadCassette() error: %v", err)
	}
	if c.Interactions[0].Response.Content != "secret: [REDACTED]" {
		t.Errorf("recorded content = %q, want redacted", c.Interactions[0].Response.Content)
	}
}
