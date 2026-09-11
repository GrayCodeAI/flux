package client

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cassette stores recorded LLM interactions for deterministic replay.
type Cassette struct {
	Name         string        `json:"name"`
	RecordedAt   time.Time     `json:"recorded_at"`
	Provider     string        `json:"provider"`
	Interactions []Interaction `json:"interactions"`
}

// Interaction pairs a recorded request with its response.
type Interaction struct {
	Request  RecordedRequest  `json:"request"`
	Response RecordedResponse `json:"response"`
}

// RecordedRequest captures the essential fields of a chat request.
type RecordedRequest struct {
	Messages []EyrieMessage `json:"messages"`
	Model    string         `json:"model"`
	System   string         `json:"system,omitempty"`
	Hash     string         `json:"hash"`
}

// RecordedResponse captures the response from a provider.
type RecordedResponse struct {
	Content      string      `json:"content,omitempty"`
	ToolCalls    []ToolCall  `json:"tool_calls,omitempty"`
	Usage        *EyrieUsage `json:"usage,omitempty"`
	FinishReason string      `json:"finish_reason,omitempty"`
	Error        string      `json:"error,omitempty"`
}

// LoadCassette reads a cassette from a JSON file at path.
func LoadCassette(path string) (*Cassette, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied local cassette file, not untrusted input
	if err != nil {
		return nil, fmt.Errorf("cassette: failed to read %s: %w", path, err)
	}
	var c Cassette
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("cassette: failed to parse %s: %w", path, err)
	}
	return &c, nil
}

// SaveCassette writes a cassette to a JSON file atomically (temp file + rename).
func SaveCassette(c *Cassette, path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cassette: failed to marshal: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cassette: failed to create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "cassette-*.tmp")
	if err != nil {
		return fmt.Errorf("cassette: failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("cassette: failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cassette: failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cassette: failed to rename temp file to %s: %w", path, err)
	}

	return nil
}

// requestHash computes a SHA256 hash of the canonical request fields.
// Only model, system prompt, and message roles/content are included in the hash.
// Images, temperature, and other varying options are excluded for stability.
func requestHash(messages []EyrieMessage, opts ChatOptions) string {
	type hashMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type hashPayload struct {
		Model    string        `json:"model"`
		System   string        `json:"system,omitempty"`
		Messages []hashMessage `json:"messages"`
	}

	payload := hashPayload{
		Model:    opts.Model,
		System:   opts.System,
		Messages: make([]hashMessage, len(messages)),
	}
	for i, m := range messages {
		payload.Messages[i] = hashMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
