package verify

import (
	"context"
	"math"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client"
)

func approxEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

func TestToolCallF1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		results       []CaseResult
		wantPrecision float64
		wantRecall    float64
		wantF1        float64
	}{
		{
			name: "all TP — perfect score",
			results: []CaseResult{
				{ExpectedTool: true, CalledAnyTool: true, CorrectTool: true},
				{ExpectedTool: true, CalledAnyTool: true, CorrectTool: true},
			},
			wantPrecision: 1.0,
			wantRecall:    1.0,
			wantF1:        1.0,
		},
		{
			name: "mix of TP, FP, FN",
			// case a: TP (expected search, got search)
			// case b: FN (expected calc, got nothing)
			// case c: FP (expected nothing, got a tool)
			// TP=1, FP=1, FN=1
			// precision = 1/2 = 0.5
			// recall    = 1/2 = 0.5
			// f1        = 0.5
			results: []CaseResult{
				{ExpectedTool: true, CalledAnyTool: true, CorrectTool: true},
				{ExpectedTool: true, CalledAnyTool: false, CorrectTool: false},
				{ExpectedTool: false, CalledAnyTool: true, CorrectTool: false},
			},
			wantPrecision: 0.5,
			wantRecall:    0.5,
			wantF1:        0.5,
		},
		{
			name: "edge case — no tool calls expected, none made (all TN)",
			results: []CaseResult{
				{ExpectedTool: false, CalledAnyTool: false},
				{ExpectedTool: false, CalledAnyTool: false},
			},
			// TP=FP=FN=0 → vacuous
			wantPrecision: 1.0,
			wantRecall:    1.0,
			wantF1:        1.0,
		},
		{
			name: "wrong tool — FP and FN",
			// TP=0, FP=1, FN=1
			// precision = 0/1 = 0, recall = 0/1 = 0, f1 = 0
			results: []CaseResult{
				{ExpectedTool: true, CalledAnyTool: true, CorrectTool: false},
			},
			wantPrecision: 0.0,
			wantRecall:    0.0,
			wantF1:        0.0,
		},
		{
			name:          "empty results — vacuous",
			results:       []CaseResult{},
			wantPrecision: 1.0,
			wantRecall:    1.0,
			wantF1:        1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, r, f := ToolCallF1(tt.results, nil)
			const eps = 1e-9
			if !approxEqual(p, tt.wantPrecision, eps) {
				t.Errorf("precision = %v, want %v", p, tt.wantPrecision)
			}
			if !approxEqual(r, tt.wantRecall, eps) {
				t.Errorf("recall = %v, want %v", r, tt.wantRecall)
			}
			if !approxEqual(f, tt.wantF1, eps) {
				t.Errorf("f1 = %v, want %v", f, tt.wantF1)
			}
		})
	}
}

// TestRun_F1ScorePopulated verifies that Run populates Report.F1Score via
// the harness end-to-end, using a scripted provider.
func TestRun_F1ScorePopulated(t *testing.T) {
	t.Parallel()
	cases := []Case{
		{
			ID:       "tool-case",
			Messages: []client.GraycodeRouterMessage{{Role: "user", Content: "call it"}},
			Expect:   Expectation{ToolName: "my_tool"},
		},
		{
			ID:       "no-tool-case",
			Messages: []client.GraycodeRouterMessage{{Role: "user", Content: "no call"}},
			Expect:   Expectation{NonEmptyContent: true},
		},
	}

	// All-correct: tool case calls the right tool; no-tool case has content.
	p := &fakeProvider{name: "fake", responses: map[string]*client.GraycodeRouterResponse{
		"call it": {ToolCalls: []client.ToolCall{{Name: "my_tool", Arguments: map[string]any{}}}},
		"no call": {Content: "done"},
	}}

	rep := Run(context.Background(), p, cases)
	if rep.F1Score <= 0 {
		t.Errorf("F1Score = %v, want > 0", rep.F1Score)
	}
	if rep.F1Score != 1.0 {
		t.Errorf("F1Score = %v, want 1.0 (all correct)", rep.F1Score)
	}
}
