// Package verify provides a data-driven conformance harness that certifies a
// provider behaves correctly before it is relied on in the catalog.
//
// It feeds a set of canonical chat/tool requests to any client.Provider, scores
// each response against declared expectations (non-empty content, expected tool
// call, valid JSON arguments, …), and produces a report. Because it takes the
// Provider interface, the same suite can be run against a live endpoint or
// against a client.RecorderProvider replaying a recorded baseline cassette —
// the latter giving a cheap, deterministic regression check without burning
// tokens.
//
// This certifies behavioral conformance (does the provider actually answer and
// call tools correctly), which is distinct from the existing parity checks that
// only confirm registry/wiring.
package verify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// Expectation declares what a correct response to a Case looks like.
type Expectation struct {
	// NonEmptyContent requires the response to contain assistant text.
	NonEmptyContent bool
	// ToolName, when set, requires the model to call exactly this tool.
	ToolName string
	// RequiredArgs lists argument keys that must be present on the tool call.
	RequiredArgs []string
	// Contains lists case-insensitive substrings the content must include
	// (e.g. a deterministic answer like "4").
	Contains []string
}

// Case is a single canonical request plus its expectation.
type Case struct {
	ID       string
	Messages []client.EyrieMessage
	Tools    []client.EyrieTool
	Expect   Expectation
}

// CaseResult is the verdict for one case.
type CaseResult struct {
	ID            string
	Passed        bool
	Failures      []string // human-readable reasons it failed
	Err           string   // transport/provider error, if any
	Latency       time.Duration
	ToolCalled    string // first tool the model actually called, empty if none
	ExpectedTool  bool   // case declared a ToolName expectation
	CalledAnyTool bool   // model emitted at least one tool call
	CorrectTool   bool   // model called the expected tool name
}

// Report aggregates the suite outcome.
type Report struct {
	Provider string
	Total    int
	Passed   int
	F1Score  float64
	Results  []CaseResult
}

// Score is the pass fraction in [0,1].
func (r Report) Score() float64 {
	if r.Total == 0 {
		return 1.0
	}
	return float64(r.Passed) / float64(r.Total)
}

// Run executes every case against p and scores the responses.
func Run(ctx context.Context, p client.Provider, cases []Case) Report {
	rep := Report{Provider: p.Name(), Total: len(cases), Results: make([]CaseResult, 0, len(cases))}
	for _, c := range cases {
		res := runCase(ctx, p, c)
		if res.Passed {
			rep.Passed++
		}
		rep.Results = append(rep.Results, res)
	}
	_, _, rep.F1Score = ToolCallF1(rep.Results, cases)
	return rep
}

func runCase(ctx context.Context, p client.Provider, c Case) CaseResult {
	res := CaseResult{ID: c.ID}
	start := time.Now()
	resp, err := p.Chat(ctx, c.Messages, client.ChatOptions{Tools: c.Tools})
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err.Error()
		res.Failures = append(res.Failures, "provider error: "+err.Error())
		return res
	}
	if resp == nil {
		res.Failures = append(res.Failures, "nil response")
		return res
	}

	if len(resp.ToolCalls) > 0 {
		res.ToolCalled = resp.ToolCalls[0].Name
		res.CalledAnyTool = true
	}
	if c.Expect.ToolName != "" {
		res.ExpectedTool = true
		for _, tc := range resp.ToolCalls {
			if tc.Name == c.Expect.ToolName {
				res.CorrectTool = true
				break
			}
		}
	}
	res.Failures = scoreResponse(resp, c.Expect)
	res.Passed = len(res.Failures) == 0
	return res
}

// scoreResponse checks a response against an expectation, returning the list of
// unmet expectations (empty == passed).
func scoreResponse(resp *client.EyrieResponse, exp Expectation) []string {
	var fail []string

	if exp.NonEmptyContent && strings.TrimSpace(resp.Content) == "" {
		fail = append(fail, "expected non-empty content")
	}

	for _, sub := range exp.Contains {
		if !strings.Contains(strings.ToLower(resp.Content), strings.ToLower(sub)) {
			fail = append(fail, fmt.Sprintf("content missing expected substring %q", sub))
		}
	}

	if exp.ToolName != "" {
		var call *client.ToolCall
		for i := range resp.ToolCalls {
			if resp.ToolCalls[i].Name == exp.ToolName {
				call = &resp.ToolCalls[i]
				break
			}
		}
		if call == nil {
			fail = append(fail, fmt.Sprintf("expected a call to tool %q, got %s", exp.ToolName, toolNames(resp.ToolCalls)))
		} else {
			for _, key := range exp.RequiredArgs {
				if _, ok := call.Arguments[key]; !ok {
					fail = append(fail, fmt.Sprintf("tool %q missing required arg %q", exp.ToolName, key))
				}
			}
		}
	}

	return fail
}

func toolNames(calls []client.ToolCall) string {
	if len(calls) == 0 {
		return "no tool calls"
	}
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// DiffBaseline compares a fresh report against a recorded baseline report and
// returns the case IDs whose pass/fail verdict regressed (passed in baseline,
// failed now). This is the "did onboarding this provider change behavior"
// check; an empty result means no regressions.
func DiffBaseline(baseline, current Report) []string {
	base := make(map[string]bool, len(baseline.Results))
	for _, r := range baseline.Results {
		base[r.ID] = r.Passed
	}
	var regressed []string
	for _, r := range current.Results {
		if was, ok := base[r.ID]; ok && was && !r.Passed {
			regressed = append(regressed, r.ID)
		}
	}
	return regressed
}

// Markdown renders the report.
func (r Report) Markdown() string {
	out := fmt.Sprintf("## Conformance: %s\n\n", r.Provider)
	out += fmt.Sprintf("- score: %.0f%% (%d/%d)\n", r.Score()*100, r.Passed, r.Total)
	out += fmt.Sprintf("- tool_call_f1: %.2f\n\n", r.F1Score)
	out += "| case | result | latency | detail |\n|---|---|---|---|\n"
	for _, c := range r.Results {
		status := "+"
		detail := ""
		if !c.Passed {
			status = "x"
			detail = strings.Join(c.Failures, "; ")
		}
		out += fmt.Sprintf("| %s | %s | %s | %s |\n", c.ID, status, c.Latency.Round(time.Millisecond), detail)
	}
	return out
}
