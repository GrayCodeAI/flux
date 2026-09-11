// Audit log sink for LLM calls.
//
// AuditEvent captures the privacy-preserving metadata of a single provider call:
// model/provider, session and key identifiers, sha256 hashes of the prompt and
// response (never the raw text), token counts, cost, latency, and status.
//
// Sinks are pluggable via the AuditSink interface. The default NoopSink discards
// events; JSONLFileSink appends one JSON object per line to a configurable file.
// Everything here is stdlib-only.
package eyrie

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// AuditEvent is a single audit record. It deliberately stores only hashes of
// prompt/response content so that raw text is never persisted to the audit log.
type AuditEvent struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	SessionID    string    `json:"session_id,omitempty"`
	VirtualKeyID string    `json:"virtual_key_id,omitempty"`
	PromptHash   string    `json:"prompt_hash,omitempty"`
	ResponseHash string    `json:"response_hash,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	LatencyMS    int64     `json:"latency_ms"`
	Status       string    `json:"status"`
}

// AuditSink records audit events. Implementations must be safe for concurrent use.
type AuditSink interface {
	Record(event AuditEvent) error
}

// --- NoopSink (default) ---

// NoopSink is an AuditSink that discards all events. It is the safe default when
// auditing is not configured.
type NoopSink struct{}

// Record implements AuditSink and does nothing.
func (NoopSink) Record(AuditEvent) error { return nil }

// --- JSONLFileSink ---

// JSONLFileSink appends audit events as newline-delimited JSON to a file. Writes
// are serialized with a mutex so concurrent callers do not interleave lines.
type JSONLFileSink struct {
	mu sync.Mutex
	f  *os.File
}

// NewJSONLFileSink opens (creating if needed) the file at path for append-only
// writes and returns a sink that writes to it. The caller owns the sink and
// should call Close when done.
func NewJSONLFileSink(path string) (*JSONLFileSink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is an operator-supplied audit log destination, not derived from untrusted request input
	if err != nil {
		return nil, err
	}
	return &JSONLFileSink{f: f}, nil
}

// Record implements AuditSink, appending the event as one JSON line.
func (s *JSONLFileSink) Record(event AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.f.Write(data)
	return err
}

// Close closes the underlying file.
func (s *JSONLFileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}

// --- Hashing helper ---

// HashContent returns the lowercase hex-encoded sha256 of s. Use it to derive
// PromptHash / ResponseHash without persisting raw prompt or response text.
func HashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
