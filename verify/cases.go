package verify

import "github.com/GrayCodeAI/flux/provider/core"

// CanonicalCases is a small, provider-neutral suite covering the behaviors rho
// depends on: basic chat, deterministic content, and tool calling with valid
// arguments. It is intentionally minimal so it is cheap to run against a live
// endpoint; extend it per provider as needed.
func CanonicalCases() []Case {
	weatherTool := core.FluxTool{
		Name:        "get_weather",
		Description: "Get the current weather for a city.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"city": map[string]interface{}{"type": "string", "description": "City name"},
			},
			"required": []string{"city"},
		},
	}

	return []Case{
		{
			ID: "basic-chat",
			Messages: []core.FluxMessage{
				{Role: "user", Content: "Reply with a short greeting."},
			},
			Expect: Expectation{NonEmptyContent: true},
		},
		{
			ID: "deterministic-answer",
			Messages: []core.FluxMessage{
				{Role: "user", Content: "What is 2 + 2? Reply with just the number."},
			},
			Expect: Expectation{NonEmptyContent: true, Contains: []string{"4"}},
		},
		{
			ID: "tool-call",
			Messages: []core.FluxMessage{
				{Role: "user", Content: "What is the weather in Paris? Use the get_weather tool."},
			},
			Tools:  []core.FluxTool{weatherTool},
			Expect: Expectation{ToolName: "get_weather", RequiredArgs: []string{"city"}},
		},
	}
}
