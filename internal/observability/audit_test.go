package graycoderouter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNoopSink(t *testing.T) {
	t.Parallel()
	var sink AuditSink = NoopSink{}
	if err := sink.Record(AuditEvent{Model: "m"}); err != nil {
		t.Errorf("NoopSink.Record should never error, got %v", err)
	}
}

func TestHashContentDeterminism(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    string
		b    string
		want bool // whether hashes should be equal
	}{
		{name: "same input same hash", a: "hello world", b: "hello world", want: true},
		{name: "different input different hash", a: "hello", b: "world", want: false},
		{name: "empty strings equal", a: "", b: "", want: true},
		{name: "case sensitive", a: "Hello", b: "hello", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ha, hb := HashContent(tt.a), HashContent(tt.b)
			if len(ha) != 64 {
				t.Errorf("expected 64 hex chars, got %d", len(ha))
			}
			if (ha == hb) != tt.want {
				t.Errorf("HashContent(%q)==HashContent(%q) was %v, want %v", tt.a, tt.b, ha == hb, tt.want)
			}
		})
	}

	// Known sha256 of "abc".
	const wantABC = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashContent("abc"); got != wantABC {
		t.Errorf("HashContent(\"abc\") = %q, want %q", got, wantABC)
	}
}

func TestJSONLFileSinkWriteAndReadBack(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	sink, err := NewJSONLFileSink(path)
	if err != nil {
		t.Fatalf("NewJSONLFileSink: %v", err)
	}

	events := []AuditEvent{
		{
			Timestamp:    time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
			Model:        "claude-sonnet-4",
			Provider:     "anthropic",
			SessionID:    "sess-1",
			VirtualKeyID: "vk-1",
			PromptHash:   HashContent("what is the capital of france?"),
			ResponseHash: HashContent("Paris."),
			InputTokens:  12,
			OutputTokens: 3,
			CostUSD:      0.000123,
			LatencyMS:    420,
			Status:       "ok",
		},
		{
			Timestamp: time.Date(2026, 6, 6, 12, 1, 0, 0, time.UTC),
			Model:     "gpt-4",
			Provider:  "openai",
			Status:    "error",
		},
	}

	for _, e := range events {
		if err := sink.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back: one JSON object per line.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var got []AuditEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal line %q: %v", sc.Text(), err)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(got) != len(events) {
		t.Fatalf("expected %d events, read %d", len(events), len(got))
	}
	if got[0].Model != "claude-sonnet-4" || got[0].PromptHash != events[0].PromptHash {
		t.Errorf("first event mismatch: %+v", got[0])
	}
	if !got[0].Timestamp.Equal(events[0].Timestamp) {
		t.Errorf("timestamp mismatch: got %v want %v", got[0].Timestamp, events[0].Timestamp)
	}
	if got[1].Status != "error" || got[1].Provider != "openai" {
		t.Errorf("second event mismatch: %+v", got[1])
	}

	// Ensure raw prompt text is never written to the file.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	if string(raw) == "" {
		t.Fatal("audit file is empty")
	}
	for _, secret := range []string{"capital of france", "Paris."} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("raw content %q leaked into audit log", secret)
		}
	}
}

func TestJSONLFileSinkAppends(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "audit.jsonl")

	// First sink writes one event.
	s1, err := NewJSONLFileSink(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	if err := s1.Record(AuditEvent{Model: "a"}); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	_ = s1.Close()

	// Second sink on same path should append, not truncate.
	s2, err := NewJSONLFileSink(path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	if err := s2.Record(AuditEvent{Model: "b"}); err != nil {
		t.Fatalf("record 2: %v", err)
	}
	_ = s2.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	lines := 0
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		lines++
	}
	if lines != 2 {
		t.Errorf("expected 2 appended lines, got %d", lines)
	}
}

// compile-time assertions.
var (
	_ AuditSink = NoopSink{}
	_ AuditSink = (*JSONLFileSink)(nil)
)
