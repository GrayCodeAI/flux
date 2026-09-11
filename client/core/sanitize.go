package core

// SanitizeMessages inspects messages for orphaned tool_use blocks
// (assistant messages with tool calls that lack matching tool_result blocks)
// and injects synthetic error results to prevent 400 errors from providers.
// This is critical for session resume and compaction scenarios.
func SanitizeMessages(messages []EyrieMessage) []EyrieMessage {
	if len(messages) == 0 {
		return messages
	}

	// Collect all tool_result IDs
	resultIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == "user" {
			for _, tr := range msg.ToolResults {
				if tr.ToolUseID != "" {
					resultIDs[tr.ToolUseID] = true
				}
			}
		}
	}

	// Find orphaned tool_use IDs and inject synthetic results
	var result []EyrieMessage
	for _, msg := range messages {
		result = append(result, msg)

		if msg.Role == "assistant" && len(msg.ToolUse) > 0 {
			for _, tc := range msg.ToolUse {
				if tc.ID != "" && !resultIDs[tc.ID] {
					// Inject synthetic error result
					result = append(result, EyrieMessage{
						Role: "user",
						ToolResults: []ToolResult{{
							ToolUseID: tc.ID,
							Content:   "Tool execution was interrupted",
							IsError:   true,
						}},
					})
					resultIDs[tc.ID] = true
				}
			}
		}
	}

	return result
}
