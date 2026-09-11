//nolint:errcheck
package client

import (
	"encoding/json"
	"testing"
)

// --- OpenAI reasoning_effort wiring ---

func TestBuildRequestBase_ReasoningEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		compat *OpenAICompatConfig
		effort string
		want   string // expected reasoning_effort on the request ("" = unset)
	}{
		{
			name:   "supported and set",
			compat: &OpenAICompatConfig{SupportsReasoningEffort: true, MaxTokensField: "max_completion_tokens"},
			effort: "high",
			want:   "high",
		},
		{
			name:   "supported but empty",
			compat: &OpenAICompatConfig{SupportsReasoningEffort: true},
			effort: "",
			want:   "",
		},
		{
			name:   "not supported but set",
			compat: &OpenAICompatConfig{SupportsReasoningEffort: false},
			effort: "high",
			want:   "",
		},
		{
			name:   "nil compat",
			compat: nil,
			effort: "high",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ChatOptions{Model: "gpt-4o", ReasoningEffort: tt.effort}
			req := buildRequestBase(basicMessages(), opts, false, tt.compat)
			if req.ReasoningEffort != tt.want {
				t.Errorf("ReasoningEffort = %q, want %q", req.ReasoningEffort, tt.want)
			}

			// Confirm JSON serialization honors omitempty.
			b, _ := json.Marshal(req)
			var m map[string]interface{}
			_ = json.Unmarshal(b, &m)
			_, present := m["reasoning_effort"]
			if tt.want == "" && present {
				t.Errorf("reasoning_effort should be omitted, got %v", m["reasoning_effort"])
			}
			if tt.want != "" && m["reasoning_effort"] != tt.want {
				t.Errorf("serialized reasoning_effort = %v, want %q", m["reasoning_effort"], tt.want)
			}
		})
	}
}

// --- GLM/Z.ai thinking toggle wiring ---

func boolPtr(b bool) *bool { return &b }

func TestBuildRequestBase_GLMThinking(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		compat   *OpenAICompatConfig
		enabled  *bool
		wantType string // expected thinking.type ("" = thinking omitted)
		wantOmit bool
	}{
		{
			name:     "zai enabled",
			compat:   &OpenAICompatConfig{ThinkingFormat: "zai"},
			enabled:  boolPtr(true),
			wantType: "enabled",
		},
		{
			name:     "zai disabled",
			compat:   &OpenAICompatConfig{ThinkingFormat: "zai"},
			enabled:  boolPtr(false),
			wantType: "disabled",
		},
		{
			name:     "zai nil toggle omits thinking",
			compat:   &OpenAICompatConfig{ThinkingFormat: "zai"},
			enabled:  nil,
			wantOmit: true,
		},
		{
			name:     "non-zai compat ignores toggle",
			compat:   &OpenAICompatConfig{ThinkingFormat: "openai"},
			enabled:  boolPtr(true),
			wantOmit: true,
		},
		{
			name:     "nil compat ignores toggle",
			compat:   nil,
			enabled:  boolPtr(true),
			wantOmit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ChatOptions{Model: "glm-4.6", GLMThinkingEnabled: tt.enabled}
			req := buildRequestBase(basicMessages(), opts, false, tt.compat)

			b, _ := json.Marshal(req)
			var m map[string]interface{}
			_ = json.Unmarshal(b, &m)
			th, present := m["thinking"]
			if tt.wantOmit {
				if present {
					t.Errorf("thinking should be omitted, got %v", th)
				}
				return
			}
			obj, ok := th.(map[string]interface{})
			if !ok {
				t.Fatalf("expected thinking object, got %T (%v)", th, th)
			}
			if obj["type"] != tt.wantType {
				t.Errorf("thinking.type = %v, want %q", obj["type"], tt.wantType)
			}
		})
	}
}

// --- Anthropic thinking budget wiring ---

func TestThinkingForBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		budget    int
		wantNil   bool
		wantBudge int
	}{
		{name: "positive budget", budget: 2048, wantNil: false, wantBudge: 2048},
		{name: "zero budget", budget: 0, wantNil: true},
		{name: "negative budget", budget: -1, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := thinkingForBudget(tt.budget)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil thinking, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil thinking, got nil")
			}
			if got.Type != "enabled" {
				t.Errorf("Type = %q, want %q", got.Type, "enabled")
			}
			if got.BudgetTokens != tt.wantBudge {
				t.Errorf("BudgetTokens = %d, want %d", got.BudgetTokens, tt.wantBudge)
			}
		})
	}
}

// TestBuildRequestBase_DeepSeekForwardsReasoningContent verifies that the
// EyrieMessage.Thinking field (which carries reasoning_content captured from a
// prior DeepSeek response) IS forwarded back into assistant messages for the
// DeepSeek provider. DeepSeek requires the assistant's reasoning_content to be
// passed back whenever that turn performed a tool call — otherwise the API
// returns HTTP 400.
func TestBuildRequestBase_DeepSeekForwardsReasoningContent(t *testing.T) {
	t.Parallel()
	compat := &DeepSeekCompat

	messages := []EyrieMessage{
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "4", Thinking: "Let me compute: 2+2 = 4"},
		{Role: "user", Content: "Are you sure?"},
	}

	req := buildRequestBase(messages, ChatOptions{Model: "deepseek-v4-flash"}, false, compat)

	body, _ := json.Marshal(req)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	msgs, ok := parsed["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages field missing or wrong type")
	}

	// The assistant turn's reasoning_content must be forwarded back.
	asst, ok := msgs[1].(map[string]interface{})
	if !ok {
		t.Fatalf("messages[1] wrong type")
	}
	if asst["role"] != "assistant" {
		t.Errorf("messages[1].role = %v, want assistant", asst["role"])
	}
	if asst["content"] != "4" {
		t.Errorf("messages[1].content = %v, want 4", asst["content"])
	}
	if rc, present := asst["reasoning_content"]; !present || rc != "Let me compute: 2+2 = 4" {
		t.Errorf("messages[1].reasoning_content = %v, want forwarded thinking text", rc)
	}
}

// TestBuildRequestBase_NonDeepSeekDoesNotForwardReasoningContent verifies that
// providers without RequiresReasoningPassback never transmit reasoning_content
// in the request body, preserving the prior behavior for all other providers.
func TestBuildRequestBase_NonDeepSeekDoesNotForwardReasoningContent(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompat // requires passback false

	messages := []EyrieMessage{
		{Role: "user", Content: "What is 2+2?"},
		{Role: "assistant", Content: "4", Thinking: "Let me compute: 2+2 = 4"},
		{Role: "user", Content: "Are you sure?"},
	}

	req := buildRequestBase(messages, ChatOptions{Model: "gpt-4o"}, false, compat)

	body, _ := json.Marshal(req)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	msgs, ok := parsed["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages field missing or wrong type")
	}

	for i, raw := range msgs {
		msg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, present := msg["reasoning_content"]; present {
			t.Errorf("message[%d] must not contain reasoning_content, got %v", i, msg)
		}
	}
}

func TestAnthropicRequest_ThinkingSerialization(t *testing.T) {
	t.Parallel()
	t.Run("budget set emits thinking object", func(t *testing.T) {
		req := anthropicRequest{
			Model: "claude-3", MaxTokens: 1024,
			Thinking: thinkingForBudget(4096),
		}
		b, _ := json.Marshal(req)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		th, ok := m["thinking"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected thinking object, got %T", m["thinking"])
		}
		if th["type"] != "enabled" {
			t.Errorf("thinking.type = %v, want enabled", th["type"])
		}
		if th["budget_tokens"] != float64(4096) {
			t.Errorf("thinking.budget_tokens = %v, want 4096", th["budget_tokens"])
		}
	})

	t.Run("zero budget omits thinking", func(t *testing.T) {
		req := anthropicRequest{
			Model: "claude-3", MaxTokens: 1024,
			Thinking: thinkingForBudget(0),
		}
		b, _ := json.Marshal(req)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		if _, present := m["thinking"]; present {
			t.Errorf("thinking should be omitted when budget is zero, got %v", m["thinking"])
		}
	})
}
