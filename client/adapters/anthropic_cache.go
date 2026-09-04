package adapters

import "github.com/GrayCodeAI/graycode-router/client/core"

// buildAnthropicCachedRequest builds an Anthropic request body with cache_control.
//   - System prompt gets cache_control (cached for all turns)
//   - Second-to-last message gets cache_control (caches conversation prefix)
//   - Last tool definition gets cache_control (caches tool schema)
func buildAnthropicCachedRequest(messages []core.GraycodeRouterMessage, model string, maxTokens int, temperature *float64, stream bool, tools []anthropicTool,
	thinking *anthropicThinking, toolChoice *anthropicToolChoice, topP *float64, topK *int, stopSequences []string,
) map[string]interface{} {
	msgs, system := buildAnthropicMessages(messages)

	// Apply cache breakpoint to second-to-last non-system message
	if len(msgs) >= 2 {
		idx := len(msgs) - 2
		applyCacheBreakpointToMessage(msgs[idx])
	}

	req := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   msgs,
		"stream":     stream,
	}
	if system != "" {
		req["system"] = []map[string]interface{}{
			{
				"type":          "text",
				"text":          system,
				"cache_control": map[string]string{"type": "ephemeral"},
			},
		}
	}
	if len(tools) > 0 {
		toolMaps := make([]map[string]interface{}, len(tools))
		for i, t := range tools {
			toolMaps[i] = map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.InputSchema,
			}
		}
		// Annotate last tool with cache_control
		toolMaps[len(toolMaps)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
		req["tools"] = toolMaps
	}
	if temperature != nil {
		req["temperature"] = *temperature
	}
	if thinking != nil {
		req["thinking"] = thinking
	}
	if toolChoice != nil {
		req["tool_choice"] = toolChoice
	}
	if topP != nil {
		req["top_p"] = *topP
	}
	if topK != nil {
		req["top_k"] = *topK
	}
	if len(stopSequences) > 0 {
		req["stop_sequences"] = stopSequences
	}
	return req
}

// applyCacheBreakpointToMessage adds cache_control to a message's content.
func applyCacheBreakpointToMessage(msg map[string]interface{}) {
	content := msg["content"]
	switch c := content.(type) {
	case string:
		msg["content"] = []map[string]interface{}{
			{
				"type":          "text",
				"text":          c,
				"cache_control": map[string]string{"type": "ephemeral"},
			},
		}
	case []map[string]interface{}:
		if len(c) > 0 {
			c[len(c)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
		}
	}
}

// BuildAnthropicCachedRequest creates an Anthropic request with cache breakpoints.
func BuildAnthropicCachedRequest(messages []core.GraycodeRouterMessage, model string, maxTokens int, temperature *float64, stream bool, tools []AnthropicTool,
	thinking *AnthropicThinking, toolChoice *AnthropicToolChoice, topP *float64, topK *int, stopSequences []string,
) map[string]interface{} {
	return buildAnthropicCachedRequest(messages, model, maxTokens, temperature, stream, tools, thinking, toolChoice, topP, topK, stopSequences)
}
