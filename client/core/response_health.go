package core

import (
	"fmt"
	"strings"
)

// ResponseHealth classifies the outcome of a model response so eyrie can turn a
// confusing "the agent did nothing" symptom into a precise, named diagnostic.
// It is most valuable for reasoning-capable models behind OpenAI-compatible
// providers, where a misconfiguration commonly yields thinking tokens but no
// usable answer.
type ResponseHealth string

const (
	// ResponseOK means the response carried usable content and/or tool calls.
	ResponseOK ResponseHealth = "ok"
	// ResponseErrorOnlyReasoning means the model emitted reasoning/thinking
	// tokens but produced zero content and zero tool calls — usually a sign the
	// provider is dropping the post-reasoning answer (wrong thinking-format
	// config, truncated stream, or a reasoning-only model used as a chat model).
	ResponseErrorOnlyReasoning ResponseHealth = "error_only_reasoning"
	// ResponseEmpty means there was no reasoning, no content, and no tool calls.
	ResponseEmpty ResponseHealth = "empty_response"
	// ResponseMalformedStream means the stream ended abnormally (a stream-level
	// error, or no terminal "done"/finish_reason was observed).
	ResponseMalformedStream ResponseHealth = "malformed_stream"
)

// ResponseSignals are the minimal observations needed to classify health. They
// are cheap to gather from both the streaming path (counts of thinking/content/
// tool-call events) and the non-streaming path (response field lengths).
type ResponseSignals struct {
	SawReasoning bool // any thinking/reasoning tokens were produced
	ContentLen   int  // length of usable assistant content
	ToolCalls    int  // number of tool calls produced
	FinishReason string
	StreamErr    bool // a stream-level error event was seen
	StreamEnded  bool // a terminal done/finish event was observed
}

// DetectResponseHealth classifies a completed response. Order matters: a
// stream-level error dominates, then the reasoning-only case, then plain empty.
func DetectResponseHealth(sig ResponseSignals) ResponseHealth {
	switch {
	case sig.StreamErr:
		return ResponseMalformedStream
	case sig.ContentLen > 0 || sig.ToolCalls > 0:
		return ResponseOK
	case sig.SawReasoning:
		// Reasoning but nothing usable — the headline failure mode.
		return ResponseErrorOnlyReasoning
	case !sig.StreamEnded && sig.FinishReason == "":
		// No content, no reasoning, and no clean termination.
		return ResponseMalformedStream
	default:
		return ResponseEmpty
	}
}

// Diagnostic returns a human-readable, actionable description for a non-OK
// health value, or "" when the response was OK.
func (h ResponseHealth) Diagnostic() string {
	switch h {
	case ResponseErrorOnlyReasoning:
		return "model produced reasoning tokens but no answer or tool call — " +
			"check the provider's thinking-format/reasoning configuration, or whether a " +
			"reasoning-only model is being used for chat"
	case ResponseEmpty:
		return "model returned an empty response (no content, reasoning, or tool calls)"
	case ResponseMalformedStream:
		return "the response stream ended abnormally (stream error or missing terminal event)"
	default:
		return ""
	}
}

// Err returns a non-nil error for a non-OK health value, suitable for surfacing
// to callers/operators. OK returns nil.
func (h ResponseHealth) Err() error {
	if d := h.Diagnostic(); d != "" {
		return fmt.Errorf("eyrie: %s: %s", h, d)
	}
	return nil
}

// healthFromResponse classifies a non-streaming EyrieResponse. Because the
// non-streaming response shape does not carry a reasoning field today, the
// caller passes whether reasoning was observed (e.g. from a thinking field on
// the raw provider payload); when unknown, pass false.
// ResponseHasContent reports whether a non-streaming response carries usable text.
func ResponseHasContent(resp *EyrieResponse) bool {
	return resp != nil && strings.TrimSpace(resp.Content) != ""
}

func healthFromResponse(resp *EyrieResponse, sawReasoning bool) ResponseHealth {
	if resp == nil {
		return ResponseEmpty
	}
	return DetectResponseHealth(ResponseSignals{
		SawReasoning: sawReasoning,
		ContentLen:   len(resp.Content),
		ToolCalls:    len(resp.ToolCalls),
		FinishReason: resp.FinishReason,
		StreamEnded:  true, // a returned non-streaming response is, by definition, complete
	})
}
