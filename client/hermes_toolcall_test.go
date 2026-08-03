package client

import "testing"

func TestParseHermesToolCalls(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		text      string
		wantClean string
		wantCalls []ToolCall
	}{
		{
			name:      "no tool calls",
			text:      "just a plain answer",
			wantClean: "just a plain answer",
		},
		{
			name:      "single call object arguments",
			text:      `here you go <tool_call>{"name": "get_weather", "arguments": {"city": "Paris"}}</tool_call>`,
			wantClean: "here you go",
			wantCalls: []ToolCall{{Name: "get_weather", Arguments: map[string]interface{}{"city": "Paris"}}},
		},
		{
			name:      "arguments as json-encoded string",
			text:      `<tool_call>{"name": "search", "arguments": "{\"q\": \"go\"}"}</tool_call>`,
			wantClean: "",
			wantCalls: []ToolCall{{Name: "search", Arguments: map[string]interface{}{"q": "go"}}},
		},
		{
			name:      "parallel calls",
			text:      `<tool_call>{"name": "a", "arguments": {}}</tool_call><tool_call>{"name": "b", "arguments": {"x": 1.0}}</tool_call>`,
			wantClean: "",
			wantCalls: []ToolCall{
				{Name: "a", Arguments: map[string]interface{}{}},
				{Name: "b", Arguments: map[string]interface{}{"x": 1.0}},
			},
		},
		{
			name:      "prose around call is preserved",
			text:      `Let me check. <tool_call>{"name": "ls", "arguments": {}}</tool_call> Done.`,
			wantClean: "Let me check.  Done.",
			wantCalls: []ToolCall{{Name: "ls", Arguments: map[string]interface{}{}}},
		},
		{
			name:      "unterminated tag left verbatim",
			text:      `<tool_call>{"name": "x"`,
			wantClean: `<tool_call>{"name": "x"`,
		},
		{
			name:      "malformed body left in place, no call",
			text:      `<tool_call>not json</tool_call>`,
			wantClean: `<tool_call>not json</tool_call>`,
		},
		{
			name:      "missing name is not a call",
			text:      `<tool_call>{"arguments": {"a": 1}}</tool_call>`,
			wantClean: `<tool_call>{"arguments": {"a": 1}}</tool_call>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, calls := ParseInlineToolCalls(tt.text)
			if clean != tt.wantClean {
				t.Errorf("clean text = %q, want %q", clean, tt.wantClean)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("got %d calls, want %d: %+v", len(calls), len(tt.wantCalls), calls)
			}
			for i, want := range tt.wantCalls {
				if calls[i].Name != want.Name {
					t.Errorf("call %d name = %q, want %q", i, calls[i].Name, want.Name)
				}
				if len(calls[i].Arguments) != len(want.Arguments) {
					t.Errorf("call %d args = %+v, want %+v", i, calls[i].Arguments, want.Arguments)
				}
				for k, v := range want.Arguments {
					if got := calls[i].Arguments[k]; got != v {
						t.Errorf("call %d arg %q = %v, want %v", i, k, got, v)
					}
				}
			}
		})
	}
}

// Ensure the Moonshot/kimi format still routes correctly and is not shadowed by
// the new Hermes fallback.
func TestParseInlineToolCalls_MoonshotStillWorks(t *testing.T) {
	t.Parallel()
	text := `answer <|tool_calls_section_begin|><|tool_call_begin|>functions.do_thing:0<|tool_call_argument_begin|>{"a":1}<|tool_call_end|><|tool_calls_section_end|>`
	clean, calls := ParseInlineToolCalls(text)
	if clean != "answer" {
		t.Errorf("clean = %q, want %q", clean, "answer")
	}
	if len(calls) != 1 || calls[0].Name != "do_thing" {
		t.Fatalf("got %+v, want one call named do_thing", calls)
	}
}

// --- Tier 3: bare brace-match JSON fallback tests ---

func TestParseBraceMatch_RawJSONNoFences(t *testing.T) {
	t.Parallel()
	text := `{"name":"search","arguments":{"q":"golang"}}`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "search" {
		t.Errorf("call name = %q, want search", calls[0].Name)
	}
	q, _ := calls[0].Arguments["q"].(string)
	if q != "golang" {
		t.Errorf("args[q] = %q, want golang", q)
	}
	if clean != "" {
		t.Errorf("clean text = %q, want empty (JSON was the whole text)", clean)
	}
}

func TestParseBraceMatch_JSONWithTextPrefix(t *testing.T) {
	t.Parallel()
	text := `Sure, let me call that. {"name":"list_files","arguments":{"dir":"/tmp"}}`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "list_files" {
		t.Errorf("call name = %q, want list_files", calls[0].Name)
	}
	if clean != "Sure, let me call that." {
		t.Errorf("clean text = %q, want 'Sure, let me call that.'", clean)
	}
}

func TestParseBraceMatch_MalformedJSONReturnedVerbatim(t *testing.T) {
	t.Parallel()
	text := `{"name": "x", "arguments": INVALID}`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for malformed JSON, got %d", len(calls))
	}
	if clean != text {
		t.Errorf("clean = %q, want original text verbatim", clean)
	}
}

func TestParseBraceMatch_MissingNameReturnedVerbatim(t *testing.T) {
	t.Parallel()
	text := `{"arguments":{"a":1}}`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls when name is absent, got %d", len(calls))
	}
	if clean != text {
		t.Errorf("clean = %q, want original text verbatim", clean)
	}
}

func TestParseBraceMatch_NotShadowingHermesTags(t *testing.T) {
	t.Parallel()
	// When Hermes tags are present, the brace-match tier must NOT run.
	text := `<tool_call>{"name":"ping","arguments":{}}</tool_call>`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 1 || calls[0].Name != "ping" {
		t.Fatalf("expected Hermes call named ping, got %+v", calls)
	}
	_ = clean
}

func TestParseBraceMatch_NoJSONInText(t *testing.T) {
	t.Parallel()
	text := "just plain text with no JSON at all"
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
	if clean != text {
		t.Errorf("clean = %q, want original text", clean)
	}
}

func TestParseBraceMatch_JSONExampleInProseNotExecuted(t *testing.T) {
	t.Parallel()
	// A tool-call-shaped JSON snippet embedded mid-sentence as an example
	// must NOT be executed as a tool call (false-positive guard).
	text := `The user asked me to look up data. {"name":"search","arguments":{"q":"x"}} is the format I would use.`
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for JSON example in prose, got %d: %+v", len(calls), calls)
	}
	if clean != text {
		t.Errorf("clean = %q, want original text verbatim", clean)
	}
}

func TestParseBraceMatch_FencedJSONExampleNotExecuted(t *testing.T) {
	t.Parallel()
	// A JSON example inside a code fence must not be executed.
	text := "Call tools like this:\n```json\n{\"name\":\"Bash\",\"arguments\":{\"command\":\"ls\"}}\n```"
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for fenced JSON example, got %d: %+v", len(calls), calls)
	}
	if clean != text {
		t.Errorf("clean = %q, want original text verbatim", clean)
	}
}

func TestParseBraceMatch_TrailingWhitespaceStillParsed(t *testing.T) {
	t.Parallel()
	// A bare JSON call with trailing whitespace/newlines is a real call.
	text := "{\"name\":\"search\",\"arguments\":{\"q\":\"golang\"}}\n\n"
	clean, calls := ParseInlineToolCalls(text)
	if len(calls) != 1 || calls[0].Name != "search" {
		t.Fatalf("expected 1 call named search, got %+v", calls)
	}
	if clean != "" {
		t.Errorf("clean = %q, want empty", clean)
	}
}
