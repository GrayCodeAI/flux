package verify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-router/client"
)

// fakeProvider is a scripted client.Provider for testing the harness without a
// live endpoint. It returns a canned response (or error) per case, keyed by the
// first user message's content.
type fakeProvider struct {
	name      string
	responses map[string]*client.GraycodeRouterResponse
	errs      map[string]error
}

func (f *fakeProvider) Name() string                 { return f.name }
func (f *fakeProvider) Ping(_ context.Context) error { return nil }

func (f *fakeProvider) Chat(_ context.Context, msgs []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.GraycodeRouterResponse, error) {
	key := ""
	if len(msgs) > 0 {
		key = msgs[0].Content
	}
	if f.errs != nil {
		if e, ok := f.errs[key]; ok {
			return nil, e
		}
	}
	return f.responses[key], nil
}

func (f *fakeProvider) StreamChat(_ context.Context, _ []client.GraycodeRouterMessage, _ client.ChatOptions) (*client.StreamResult, error) {
	return nil, errors.New("not implemented")
}

func TestRun_AllPass(t *testing.T) {
	t.Parallel()
	cases := CanonicalCases()
	resp := map[string]*client.GraycodeRouterResponse{
		"Reply with a short greeting.":                            {Content: "Hello!"},
		"What is 2 + 2? Reply with just the number.":              {Content: "4"},
		"What is the weather in Paris? Use the get_weather tool.": {ToolCalls: []client.ToolCall{{Name: "get_weather", Arguments: map[string]any{"city": "Paris"}}}},
	}
	p := &fakeProvider{name: "fake", responses: resp}

	rep := Run(context.Background(), p, cases)
	if rep.Passed != rep.Total {
		t.Fatalf("expected all %d to pass, got %d:\n%s", rep.Total, rep.Passed, rep.Markdown())
	}
	if rep.Score() != 1.0 {
		t.Errorf("score = %v, want 1.0", rep.Score())
	}
}

func TestRun_DetectsFailures(t *testing.T) {
	t.Parallel()
	cases := CanonicalCases()
	resp := map[string]*client.GraycodeRouterResponse{
		"Reply with a short greeting.":               {Content: ""},     // empty → fail
		"What is 2 + 2? Reply with just the number.": {Content: "five"}, // missing "4" → fail
		// tool case: wrong tool + missing arg → fail
		"What is the weather in Paris? Use the get_weather tool.": {ToolCalls: []client.ToolCall{{Name: "search", Arguments: map[string]any{}}}},
	}
	p := &fakeProvider{name: "fake", responses: resp}

	rep := Run(context.Background(), p, cases)
	if rep.Passed != 0 {
		t.Errorf("expected 0 passes, got %d", rep.Passed)
	}
	for _, r := range rep.Results {
		if r.Passed || len(r.Failures) == 0 {
			t.Errorf("case %s should have failures", r.ID)
		}
	}
}

func TestRun_ToolMissingRequiredArg(t *testing.T) {
	t.Parallel()
	cases := []Case{{
		ID:       "tool",
		Messages: []client.GraycodeRouterMessage{{Role: "user", Content: "go"}},
		Expect:   Expectation{ToolName: "get_weather", RequiredArgs: []string{"city"}},
	}}
	// Right tool, but missing the "city" arg.
	p := &fakeProvider{name: "fake", responses: map[string]*client.GraycodeRouterResponse{
		"go": {ToolCalls: []client.ToolCall{{Name: "get_weather", Arguments: map[string]any{}}}},
	}}
	rep := Run(context.Background(), p, cases)
	if rep.Passed != 0 {
		t.Fatalf("expected fail for missing arg")
	}
	if !strings.Contains(strings.Join(rep.Results[0].Failures, " "), "missing required arg") {
		t.Errorf("failures = %v, want missing-arg detail", rep.Results[0].Failures)
	}
}

func TestRun_ProviderError(t *testing.T) {
	t.Parallel()
	cases := []Case{{
		ID:       "boom",
		Messages: []client.GraycodeRouterMessage{{Role: "user", Content: "x"}},
		Expect:   Expectation{NonEmptyContent: true},
	}}
	p := &fakeProvider{name: "fake", errs: map[string]error{"x": errors.New("503 unavailable")}}
	rep := Run(context.Background(), p, cases)
	if rep.Passed != 0 {
		t.Fatal("provider error should fail the case")
	}
	if rep.Results[0].Err == "" {
		t.Error("expected Err to be recorded")
	}
}

func TestDiffBaseline(t *testing.T) {
	t.Parallel()
	baseline := Report{Results: []CaseResult{
		{ID: "a", Passed: true},
		{ID: "b", Passed: true},
		{ID: "c", Passed: false},
	}}
	current := Report{Results: []CaseResult{
		{ID: "a", Passed: true},  // unchanged pass
		{ID: "b", Passed: false}, // regression!
		{ID: "c", Passed: false}, // still failing, not a regression
	}}
	regressed := DiffBaseline(baseline, current)
	if len(regressed) != 1 || regressed[0] != "b" {
		t.Errorf("regressed = %v, want [b]", regressed)
	}
}

func TestReport_Markdown(t *testing.T) {
	t.Parallel()
	rep := Report{
		Provider: "fake",
		Total:    1,
		Passed:   0,
		Results:  []CaseResult{{ID: "x", Passed: false, Failures: []string{"expected non-empty content"}}},
	}
	md := rep.Markdown()
	if !strings.Contains(md, "Conformance: fake") || !strings.Contains(md, "expected non-empty content") {
		t.Errorf("markdown missing content:\n%s", md)
	}
}
