package core

import (
	"context"
	"strings"
	"testing"
)

func TestDetectResponseHealth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sig  ResponseSignals
		want ResponseHealth
	}{
		{"content ok", ResponseSignals{ContentLen: 10, StreamEnded: true}, ResponseOK},
		{"tool call ok", ResponseSignals{ToolCalls: 1, StreamEnded: true}, ResponseOK},
		{"reasoning only", ResponseSignals{SawReasoning: true, StreamEnded: true, FinishReason: "stop"}, ResponseErrorOnlyReasoning},
		{"empty", ResponseSignals{StreamEnded: true, FinishReason: "stop"}, ResponseEmpty},
		{"stream error dominates", ResponseSignals{StreamErr: true, ContentLen: 5}, ResponseMalformedStream},
		{"no terminal event", ResponseSignals{StreamEnded: false, FinishReason: ""}, ResponseMalformedStream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectResponseHealth(tc.sig); got != tc.want {
				t.Errorf("DetectResponseHealth = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResponseHealth_DiagnosticAndErr(t *testing.T) {
	t.Parallel()
	if ResponseOK.Diagnostic() != "" {
		t.Error("OK should have no diagnostic")
	}
	if ResponseOK.Err() != nil {
		t.Error("OK should have no error")
	}
	if !strings.Contains(ResponseErrorOnlyReasoning.Diagnostic(), "reasoning") {
		t.Error("reasoning-only diagnostic should mention reasoning")
	}
	if ResponseErrorOnlyReasoning.Err() == nil {
		t.Error("non-OK should produce an error")
	}
}

func TestHealthFromResponse(t *testing.T) {
	t.Parallel()
	if got := healthFromResponse(&GraycodeRouterResponse{Content: "hello"}, false); got != ResponseOK {
		t.Errorf("got %q, want ok", got)
	}
	if got := healthFromResponse(&GraycodeRouterResponse{}, true); got != ResponseErrorOnlyReasoning {
		t.Errorf("got %q, want error_only_reasoning", got)
	}
	if got := healthFromResponse(&GraycodeRouterResponse{}, false); got != ResponseEmpty {
		t.Errorf("got %q, want empty", got)
	}
	if got := healthFromResponse(nil, false); got != ResponseEmpty {
		t.Errorf("nil resp got %q, want empty", got)
	}
}

// Integration: a stream that emits only reasoning_content then finishes must
// produce a diagnostic error event before the terminal done. The diagnostic
// must carry the Warning marker (non-fatal) so downstream consumers keep
// streaming and still observe the terminal done event.
func TestStreamEmitsErrorOnlyReasoningDiagnostic(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"reasoning_content":"thinking hard..."},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ch := ProcessOpenAIStream(context.Background(), events, testLogger())

	var sawThinking, sawDiagnostic, sawContent, sawDone bool
	diagnosticIdx, doneIdx := -1, -1
	var i int
	for evt := range ch {
		switch evt.Type {
		case "thinking":
			sawThinking = true
		case "content":
			sawContent = true
		case "error":
			if strings.Contains(evt.Error, "reasoning") {
				sawDiagnostic = true
				diagnosticIdx = i
				if evt.Warning == "" {
					t.Error("diagnostic error event must set the Warning marker")
				}
			}
		case "done":
			sawDone = true
			doneIdx = i
		}
		i++
	}
	if !sawThinking {
		t.Error("expected a thinking event from reasoning_content")
	}
	if sawContent {
		t.Error("did not expect any content event")
	}
	if !sawDiagnostic {
		t.Error("expected an error-only-reasoning diagnostic event")
	}
	if !sawDone {
		t.Fatal("expected a terminal done event after the diagnostic")
	}
	if diagnosticIdx < 0 || doneIdx < diagnosticIdx {
		t.Fatalf("diagnostic (%d) must precede done (%d)", diagnosticIdx, doneIdx)
	}
}

// A normal stream with content must NOT Emit a health diagnostic.
func TestStreamNoDiagnosticOnHealthyResponse(t *testing.T) {
	t.Parallel()
	events := make(chan SSEEvent, 10)
	events <- SSEEvent{Data: `{"choices":[{"delta":{"content":"answer"},"finish_reason":null}]}`}
	events <- SSEEvent{Data: `{"choices":[{"delta":{},"finish_reason":"stop"}]}`}
	close(events)

	ch := ProcessOpenAIStream(context.Background(), events, testLogger())
	for evt := range ch {
		if evt.Type == "error" {
			t.Errorf("healthy stream should not Emit error diagnostic, got %q", evt.Error)
		}
	}
}
