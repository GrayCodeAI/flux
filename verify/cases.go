package verify

import "github.com/GrayCodeAI/eyrie/client"

// CanonicalCases is a small, provider-neutral suite covering the behaviors hawk
// depends on: basic chat, deterministic content, and tool calling with valid
// arguments. It is intentionally minimal so it is cheap to run against a live
// endpoint; extend it per provider as needed.
func CanonicalCases() []Case {
	weatherTool := client.EyrieTool{
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
			Messages: []client.EyrieMessage{
				{Role: "user", Content: "Reply with a short greeting."},
			},
			Expect: Expectation{NonEmptyContent: true},
		},
		{
			ID: "deterministic-answer",
			Messages: []client.EyrieMessage{
				{Role: "user", Content: "What is 2 + 2? Reply with just the number."},
			},
			Expect: Expectation{NonEmptyContent: true, Contains: []string{"4"}},
		},
		{
			ID: "tool-call",
			Messages: []client.EyrieMessage{
				{Role: "user", Content: "What is the weather in Paris? Use the get_weather tool."},
			},
			Tools:  []client.EyrieTool{weatherTool},
			Expect: Expectation{ToolName: "get_weather", RequiredArgs: []string{"city"}},
		},
	}
}
