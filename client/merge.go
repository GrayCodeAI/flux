package client

// MergeConsecutiveRoles merges adjacent messages that share the same role
// by concatenating their content with a newline separator.
//
// Messages with ToolUse or ToolResult are never merged, since those have
// special provider semantics and must remain separate.
func MergeConsecutiveRoles(messages []GraycodeRouterMessage) []GraycodeRouterMessage {
	if len(messages) == 0 {
		return messages
	}

	var result []GraycodeRouterMessage
	for _, msg := range messages {
		if len(result) == 0 {
			result = append(result, msg)
			continue
		}

		prev := &result[len(result)-1]

		// Skip merging if either message has tool use or tool result
		if hasToolData(msg) || hasToolData(*prev) {
			result = append(result, msg)
			continue
		}

		// Merge if same role
		if prev.Role == msg.Role {
			if prev.Content != "" && msg.Content != "" {
				prev.Content = prev.Content + "\n" + msg.Content
			} else if msg.Content != "" {
				prev.Content = msg.Content
			}
			// Merge images
			if len(msg.Images) > 0 {
				prev.Images = append(prev.Images, msg.Images...)
			}
		} else {
			result = append(result, msg)
		}
	}

	return result
}

// hasToolData returns true if the message contains tool use or tool result data.
func hasToolData(msg GraycodeRouterMessage) bool {
	return len(msg.ToolUse) > 0 || len(msg.ToolResults) > 0
}
